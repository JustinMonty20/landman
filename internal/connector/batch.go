package connector

import "context"

// BatchHook is called with each batch of raw records as they are fetched.
// Returning an error stops the fetch.
type BatchHook func([]RawRecord) error

// BatchFetchSource is an optional interface for DataSources that can stream
// batches instead of returning all records at once.
type BatchFetchSource interface {
	DataSource
	FetchBatches(ctx context.Context, onBatch BatchHook) error
}
