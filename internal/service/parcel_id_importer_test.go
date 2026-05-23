package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector"
)

type fakeBatchSource struct {
	batches     [][]connector.RawRecord
	fetchCalled bool
}

func (f *fakeBatchSource) Name() string              { return "fake" }
func (f *fakeBatchSource) County() string            { return "fake" }
func (f *fakeBatchSource) SourceType() string        { return "gis" }
func (f *fakeBatchSource) SupportsIncremental() bool { return false }
func (f *fakeBatchSource) FetchSince(context.Context, time.Time) ([]connector.RawRecord, error) {
	return nil, errors.New("not supported")
}
func (f *fakeBatchSource) Fetch(ctx context.Context) ([]connector.RawRecord, error) {
	f.fetchCalled = true
	return nil, errors.New("fetch should not be called")
}
func (f *fakeBatchSource) FetchBatches(ctx context.Context, onBatch connector.BatchHook) error {
	for _, batch := range f.batches {
		if err := onBatch(batch); err != nil {
			return err
		}
	}
	return nil
}

type fakeSource struct {
	records []connector.RawRecord
}

func (f *fakeSource) Name() string              { return "fake" }
func (f *fakeSource) County() string            { return "fake" }
func (f *fakeSource) SourceType() string        { return "gis" }
func (f *fakeSource) SupportsIncremental() bool { return false }
func (f *fakeSource) FetchSince(context.Context, time.Time) ([]connector.RawRecord, error) {
	return nil, errors.New("not supported")
}
func (f *fakeSource) Fetch(ctx context.Context) ([]connector.RawRecord, error) {
	return f.records, nil
}

func TestParcelIDImportService_Run_UsesBatchSource(t *testing.T) {
	src := &fakeBatchSource{
		batches: [][]connector.RawRecord{
			{{ParcelID: "P1"}, {ParcelID: "P2"}},
			{{ParcelID: "P2"}, {ParcelID: "P3"}, {ParcelID: ""}},
		},
	}

	svc := NewParcelIDImportService(src, 50)

	var got [][]string
	err := svc.Run(context.Background(), func(ctx context.Context, batchIndex int, parcelIDs []string) error {
		got = append(got, parcelIDs)
		return nil
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if src.fetchCalled {
		t.Fatalf("Fetch should not be called when BatchFetchSource is available")
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(got))
	}

	if want := []string{"P1", "P2"}; !equalStrings(got[0], want) {
		t.Errorf("batch 1 = %v, want %v", got[0], want)
	}

	if want := []string{"P2", "P3"}; !equalStrings(got[1], want) {
		t.Errorf("batch 2 = %v, want %v", got[1], want)
	}
}

func TestParcelIDImportService_Run_FallbackToFetch(t *testing.T) {
	src := &fakeSource{
		records: []connector.RawRecord{
			{ParcelID: "P1"},
			{ParcelID: "P2"},
			{ParcelID: "P3"},
			{ParcelID: "P4"},
			{ParcelID: "P5"},
		},
	}

	svc := NewParcelIDImportService(src, 2)

	var got [][]string
	err := svc.Run(context.Background(), func(ctx context.Context, batchIndex int, parcelIDs []string) error {
		got = append(got, parcelIDs)
		return nil
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 batches, got %d", len(got))
	}

	want := [][]string{
		{"P1", "P2"},
		{"P3", "P4"},
		{"P5"},
	}
	for i := range want {
		if !equalStrings(got[i], want[i]) {
			t.Errorf("batch %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestParcelIDImportService_Run_NilHook(t *testing.T) {
	src := &fakeSource{}
	svc := NewParcelIDImportService(src, 2)

	if err := svc.Run(context.Background(), nil); err == nil {
		t.Fatalf("expected error for nil onBatch")
	}
}

func TestParcelIDImportService_Run_Filter_BatchSource(t *testing.T) {
	src := &fakeBatchSource{
		batches: [][]connector.RawRecord{
			{{ParcelID: "VAC-1"}, {ParcelID: "DEV-1"}},
			{{ParcelID: "VAC-2"}, {ParcelID: "DEV-2"}},
		},
	}

	svc := NewParcelIDImportService(
		src,
		50,
		WithParcelRecordFilter(func(record connector.RawRecord) bool {
			return strings.HasPrefix(record.ParcelID, "VAC-")
		}),
	)

	var got [][]string
	err := svc.Run(context.Background(), func(ctx context.Context, batchIndex int, parcelIDs []string) error {
		got = append(got, parcelIDs)
		return nil
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(got))
	}

	if want := []string{"VAC-1"}; !equalStrings(got[0], want) {
		t.Errorf("batch 1 = %v, want %v", got[0], want)
	}

	if want := []string{"VAC-2"}; !equalStrings(got[1], want) {
		t.Errorf("batch 2 = %v, want %v", got[1], want)
	}
}

func TestParcelIDImportService_Run_Filter_FallbackFetch(t *testing.T) {
	src := &fakeSource{
		records: []connector.RawRecord{
			{ParcelID: "VAC-1"},
			{ParcelID: "DEV-1"},
			{ParcelID: "VAC-2"},
			{ParcelID: "DEV-2"},
		},
	}

	svc := NewParcelIDImportService(
		src,
		2,
		WithParcelRecordFilter(func(record connector.RawRecord) bool {
			return strings.HasPrefix(record.ParcelID, "VAC-")
		}),
	)

	var got [][]string
	err := svc.Run(context.Background(), func(ctx context.Context, batchIndex int, parcelIDs []string) error {
		got = append(got, parcelIDs)
		return nil
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(got))
	}

	if want := []string{"VAC-1", "VAC-2"}; !equalStrings(got[0], want) {
		t.Errorf("batch = %v, want %v", got[0], want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
