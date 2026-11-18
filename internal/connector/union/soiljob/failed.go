package soiljob

import (
	"context"
	"errors"
	"time"

	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5/pgxpool"
)

// sealFailedCapture retains a partial manifest for diagnostics without publishing it.
func sealFailedCapture(ctx context.Context, pool *pgxpool.Pool, store taxartifact.Store, recorder *ingest.Recorder, sourceErr error) error {
	if recorder == nil || len(recorder.Manifest().Receipts) == 0 {
		return sourceErr
	}
	registrationCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	_, sealErr := (ingest.Engine{Pool: pool, Store: store}).Seal(registrationCtx, recorder)
	return errors.Join(sourceErr, sealErr)
}
