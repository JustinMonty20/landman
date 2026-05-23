package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/JustinMonty20/landman/internal/connector"
)

// Logger is a minimal logger used by the batch enricher.
type Logger interface {
	Printf(format string, args ...any)
}

// SourceConfig defines per-source runtime behavior.
type SourceConfig struct {
	Concurrency int
}

// BatchEnricher fetches per-parcel data from multiple sources.
type BatchEnricher struct {
	fetchers map[string]connector.SingleParcelFetcher
	configs  map[string]SourceConfig
	logger   Logger
}

// NewBatchEnricher creates a new batch enricher.
func NewBatchEnricher(fetchers map[string]connector.SingleParcelFetcher, configs map[string]SourceConfig, logger Logger) (*BatchEnricher, error) {
	if len(fetchers) == 0 {
		return nil, fmt.Errorf("fetchers cannot be empty")
	}
	if logger == nil {
		logger = log.Default()
	}
	if configs == nil {
		configs = map[string]SourceConfig{}
	}

	return &BatchEnricher{
		fetchers: fetchers,
		configs:  configs,
		logger:   logger,
	}, nil
}

// EnrichBatch fetches data for each parcel ID from all configured sources.
// It returns a map of parcelID -> sourceName -> RawRecord.
func (b *BatchEnricher) EnrichBatch(ctx context.Context, parcelIDs []string) (map[string]map[string]connector.RawRecord, error) {
	if len(parcelIDs) == 0 {
		return map[string]map[string]connector.RawRecord{}, nil
	}

	type result struct {
		parcelID string
		source   string
		record   connector.RawRecord
		err      error
	}

	results := make(chan result)
	var wg sync.WaitGroup

	for sourceName, fetcher := range b.fetchers {
		conf := b.configs[sourceName]
		if conf.Concurrency <= 0 {
			conf.Concurrency = 5
		}

		idCh := make(chan string)
		wg.Add(conf.Concurrency)

		for i := 0; i < conf.Concurrency; i++ {
			go func(srcName string, f connector.SingleParcelFetcher, ids <-chan string) {
				defer wg.Done()
				for id := range ids {
					if ctx.Err() != nil {
						return
					}
					rec, err := f.FetchByParcelID(ctx, id)
					results <- result{
						parcelID: id,
						source:   srcName,
						record:   rec,
						err:      err,
					}
				}
			}(sourceName, fetcher, idCh)
		}

		go func(ids []string, out chan<- string) {
			defer close(out)
			for _, id := range ids {
				select {
				case <-ctx.Done():
					return
				case out <- id:
				}
			}
		}(parcelIDs, idCh)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	records := make(map[string]map[string]connector.RawRecord, len(parcelIDs))
	var errs []error

	for res := range results {
		if res.err != nil {
			b.logger.Printf("parcel %s source %s error: %v", res.parcelID, res.source, res.err)
			errs = append(errs, fmt.Errorf("%s/%s: %w", res.source, res.parcelID, res.err))
			continue
		}
		if _, exists := records[res.parcelID]; !exists {
			records[res.parcelID] = make(map[string]connector.RawRecord)
		}
		records[res.parcelID][res.source] = res.record
	}

	if len(errs) > 0 {
		return records, fmt.Errorf("batch enrichment completed with %d errors: %w", len(errs), errors.Join(errs...))
	}

	return records, nil
}
