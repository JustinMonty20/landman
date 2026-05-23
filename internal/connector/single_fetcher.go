package connector

import "context"

// SingleParcelFetcher retrieves data for a single parcel ID.
// This is intended for sources that only support per-parcel lookups.
type SingleParcelFetcher interface {
	Name() string
	FetchByParcelID(ctx context.Context, parcelID string) (RawRecord, error)
}
