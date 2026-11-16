package taxsource

import (
	"context"
	"time"
)

// Meta identifies the immutable source file read by a Source.
type Meta struct {
	URL       string
	Name      string
	SHA256    [32]byte
	FetchedAt time.Time
}

// Source streams records from one tax source file.
type Source[T any] interface {
	Read(ctx context.Context, yield func(T) error) (Meta, error)
}
