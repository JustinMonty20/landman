package taxstore

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// recordUnmatched retains a missing parcel link in the publication transaction.
// Repeated references to the same PIN and year produce one review record.
func recordUnmatched(ctx context.Context, tx pgx.Tx, importID, key string, year int16) error {
	_, err := tx.Exec(ctx, `INSERT INTO tax_unmatched_records(import_id,parcel_key,tax_year,capture_id)
 VALUES ($1,$2,$3,NULLIF(current_setting('landman.tax_capture_id',true),'')::uuid) ON CONFLICT (import_id,parcel_key,tax_year) DO NOTHING`, importID, key, year)
	if err != nil {
		return fmt.Errorf("record unmatched tax PIN %q (import_id=%s, tax year=%d): %w", key, importID, year, err)
	}
	return nil
}
