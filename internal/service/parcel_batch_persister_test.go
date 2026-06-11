package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/models"
	"github.com/JustinMonty20/landman/internal/storage"
)

type fakeTranslator struct {
	errFor map[string]error
}

func (f fakeTranslator) Translate(raw connector.RawRecord) (*models.GISParcel, error) {
	if err := f.errFor[raw.ParcelID]; err != nil {
		return nil, err
	}
	return &models.GISParcel{Identity: models.ParcelIdentity{SourceParcelID: raw.ParcelID}}, nil
}

func (f fakeTranslator) CanTranslate(sourceName string) bool { return true }
func (f fakeTranslator) Priority() int                       { return 1 }

type fakeParcelStore struct {
	received []*models.GISParcel
	result   storage.BatchInsertResult
	err      error
}

func (f *fakeParcelStore) InsertBatch(ctx context.Context, parcels []*models.GISParcel) (storage.BatchInsertResult, error) {
	f.received = parcels
	return f.result, f.err
}

type bufferLogger struct {
	lines []string
}

func (b *bufferLogger) Printf(format string, args ...any) {
	b.lines = append(b.lines, fmt.Sprintf(format, args...))
}

func TestParcelBatchPersister_PersistRawBatch_LogsTranslationAndPersistenceErrors(t *testing.T) {
	translationErr := errors.New("bad raw record")
	persistenceErr := errors.New("duplicate parcel")
	store := &fakeParcelStore{
		result: storage.BatchInsertResult{
			Inserted: 1,
			Errors:   []storage.ParcelInsertError{{ParcelID: "P2", Err: persistenceErr}},
		},
	}
	logger := &bufferLogger{}
	persister, err := NewParcelBatchPersister(fakeTranslator{errFor: map[string]error{"BAD": translationErr}}, store, logger)
	if err != nil {
		t.Fatalf("NewParcelBatchPersister error: %v", err)
	}

	result, err := persister.PersistRawBatch(context.Background(), 3, []connector.RawRecord{{ParcelID: "P1"}, {ParcelID: "BAD"}, {ParcelID: "P2"}})
	if err != nil {
		t.Fatalf("PersistRawBatch error: %v", err)
	}

	if result.Translated != 2 || result.Inserted != 1 || result.Errors != 2 {
		t.Fatalf("result = %+v, want translated=2 inserted=1 errors=2", result)
	}
	if len(store.received) != 2 {
		t.Fatalf("stored parcels = %d, want 2", len(store.received))
	}
	if len(logger.lines) != 2 {
		t.Fatalf("log lines = %d, want 2", len(logger.lines))
	}
	if !strings.Contains(logger.lines[0], "translation error") || !strings.Contains(logger.lines[1], "persistence error") {
		t.Fatalf("unexpected logs: %v", logger.lines)
	}
}

func TestParcelBatchPersister_PersistRawBatch_ReturnsFatalStoreError(t *testing.T) {
	fatalErr := errors.New("commit failed")
	store := &fakeParcelStore{err: fatalErr}
	persister, err := NewParcelBatchPersister(fakeTranslator{errFor: map[string]error{}}, store, nil)
	if err != nil {
		t.Fatalf("NewParcelBatchPersister error: %v", err)
	}

	_, err = persister.PersistRawBatch(context.Background(), 1, []connector.RawRecord{{ParcelID: "P1"}})
	if !errors.Is(err, fatalErr) {
		t.Fatalf("error = %v, want fatal store error", err)
	}
}
