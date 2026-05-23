package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector"
)

type fakeFetcher struct {
	name   string
	errFor map[string]error
	mu     sync.Mutex
	calls  []string
}

func (f *fakeFetcher) Name() string { return f.name }

func (f *fakeFetcher) FetchByParcelID(ctx context.Context, parcelID string) (connector.RawRecord, error) {
	f.mu.Lock()
	f.calls = append(f.calls, parcelID)
	f.mu.Unlock()

	if err, ok := f.errFor[parcelID]; ok {
		return connector.RawRecord{}, err
	}
	return connector.RawRecord{
		SourceName: f.name,
		ParcelID:   parcelID,
		RawData:    map[string]interface{}{"ok": true},
	}, nil
}

type logBuffer struct {
	mu   sync.Mutex
	logs []string
}

func (l *logBuffer) Printf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logs = append(l.logs, format)
}

func TestBatchEnricher_ErrorFailsBatch(t *testing.T) {
	fetcher := &fakeFetcher{
		name:   "source",
		errFor: map[string]error{"P1": errors.New("boom")},
	}

	enricher, err := NewBatchEnricher(
		map[string]connector.SingleParcelFetcher{
			"source": fetcher,
		},
		map[string]SourceConfig{
			"source": {Concurrency: 1},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("NewBatchEnricher error: %v", err)
	}

	if _, err := enricher.EnrichBatch(context.Background(), []string{"P1"}); err == nil {
		t.Fatalf("expected error for fetcher failure")
	}
}
