package taxstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/county"
	"github.com/JustinMonty20/landman/internal/taxsource"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DelinquentResult reports stored and skipped rows for one retained file.
type DelinquentResult struct {
	Saved     int
	Deleted   int
	SkippedPP int
	SkippedRR int
	Unmatched int
}

// SaveDelinquent replaces balances from a complete retained file and links its evidence.
// The caller rolls the transaction back on any error and commits only after success.
func SaveDelinquent(ctx context.Context, tx pgx.Tx, rows []union.DelinquentRow, meta taxsource.Meta, skippedPP, skippedRR int) (DelinquentResult, error) {
	return saveDelinquentBatch(ctx, tx, rows, meta, skippedPP, skippedRR, true)
}

// PublishDelinquentFile replaces balances from a complete Gob-encoded DelinquentRow stream in one transaction.
// The caller must parse and validate the retained source file before it supplies the stream.
func PublishDelinquentFile(ctx context.Context, pool *pgxpool.Pool, input io.Reader, meta taxsource.Meta, skippedPP, skippedRR int) (DelinquentResult, error) {
	add := func(total *DelinquentResult, batch DelinquentResult) {
		total.Saved += batch.Saved
		total.Unmatched += batch.Unmatched
		total.Deleted += batch.Deleted
		total.SkippedPP = batch.SkippedPP
		total.SkippedRR = batch.SkippedRR
	}
	return publishStagedRows[union.DelinquentRow, DelinquentResult](ctx, pool, input,
		func(tx pgx.Tx, rows []union.DelinquentRow) (DelinquentResult, error) {
			return saveDelinquentBatch(ctx, tx, rows, meta, skippedPP, skippedRR, false)
		},
		func(tx pgx.Tx) (DelinquentResult, error) {
			return saveDelinquentBatch(ctx, tx, nil, meta, skippedPP, skippedRR, true)
		}, add)
}

// PublishDelinquentInTx publishes a complete staged source in the engine transaction.
func PublishDelinquentInTx(ctx context.Context, tx pgx.Tx, input io.Reader, meta taxsource.Meta, skippedPP, skippedRR int) (DelinquentResult, error) {
	return PublishStagedInTx[union.DelinquentRow, DelinquentResult](ctx, tx, input,
		func(rows []union.DelinquentRow) (DelinquentResult, error) {
			return saveDelinquentBatch(ctx, tx, rows, meta, skippedPP, skippedRR, false)
		},
		func() (DelinquentResult, error) {
			return saveDelinquentBatch(ctx, tx, nil, meta, skippedPP, skippedRR, true)
		},
		func(total *DelinquentResult, batch DelinquentResult) {
			total.Saved += batch.Saved
			total.Deleted += batch.Deleted
			total.Unmatched += batch.Unmatched
			total.SkippedPP = batch.SkippedPP
			total.SkippedRR = batch.SkippedRR
		})
}

func saveDelinquentBatch(ctx context.Context, tx pgx.Tx, rows []union.DelinquentRow, meta taxsource.Meta, skippedPP, skippedRR int, complete bool) (DelinquentResult, error) {
	var result DelinquentResult
	if meta.Name != "union_delinquent" || meta.URL == "" || meta.FetchedAt.IsZero() || !meta.Artifact.Verified() || meta.SHA256 != meta.Artifact.SHA256() || skippedPP < 0 || skippedRR < 0 {
		return result, errors.New("invalid delinquent import evidence")
	}
	result.SkippedPP, result.SkippedRR = skippedPP, skippedRR
	id, err := delinquentImportID(ctx, tx, meta)
	if err != nil {
		return result, err
	}
	digest := meta.Artifact.SHA256()
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE IF NOT EXISTS delinquent_seen (parcel_id uuid, tax_year smallint, PRIMARY KEY (parcel_id,tax_year)) ON COMMIT DROP`); err != nil {
		return result, err
	}
	for _, row := range rows {
		if row.ParcelKey == "" || row.Year < 1900 || row.Year > 2200 || row.Balance < 0 || row.PrimarySource != "RE" {
			return result, errors.New("invalid delinquent row")
		}
		var parcelID string
		err := tx.QueryRow(ctx, `SELECT id FROM parcels WHERE county_id = $1 AND source_name = $2 AND source_parcel_id = $3`, county.Union, "union_county_gis", row.ParcelKey).Scan(&parcelID)
		if errors.Is(err, pgx.ErrNoRows) {
			if err := recordUnmatched(ctx, tx, id, row.ParcelKey, row.Year); err != nil {
				return result, err
			}
			result.Unmatched++
			continue
		}
		if err != nil {
			return result, fmt.Errorf("lookup delinquent parcel PIN %q for tax year %d: %w", row.ParcelKey, row.Year, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO delinquent_seen(parcel_id,tax_year) VALUES ($1,$2) ON CONFLICT DO NOTHING`, parcelID, row.Year); err != nil {
			return result, err
		}
		balance := fmt.Sprintf("%d.%02d", row.Balance/100, row.Balance%100)
		_, err = tx.Exec(ctx, `
INSERT INTO tax_delinquent_balances (
  county_id, source_name, parcel_id, tax_year, balance_due, primary_source,
  source_url, file_sha256, fetched_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (county_id, source_name, parcel_id, tax_year)
DO UPDATE SET
  balance_due = EXCLUDED.balance_due,
  primary_source = EXCLUDED.primary_source,
  source_url = EXCLUDED.source_url,
  file_sha256 = EXCLUDED.file_sha256,
  fetched_at = EXCLUDED.fetched_at,
  updated_at = now()
WHERE (tax_delinquent_balances.balance_due, tax_delinquent_balances.primary_source) IS DISTINCT FROM (EXCLUDED.balance_due, EXCLUDED.primary_source)`, county.Union, meta.Name, parcelID, row.Year, balance,
			row.PrimarySource, meta.URL, digest[:], meta.FetchedAt)
		if err != nil {
			return result, fmt.Errorf("save delinquent balance for PIN %q (parcel_id=%s, tax year %d): %w", row.ParcelKey, parcelID, row.Year, err)
		}
		result.Saved++
	}
	if !complete {
		return result, nil
	}
	deleted, err := tx.Exec(ctx, `DELETE FROM tax_delinquent_balances
WHERE county_id = $1 AND source_name = $2 AND NOT EXISTS (SELECT 1 FROM delinquent_seen seen WHERE seen.parcel_id=tax_delinquent_balances.parcel_id AND seen.tax_year=tax_delinquent_balances.tax_year)`, county.Union, meta.Name)
	if err != nil {
		return result, fmt.Errorf("remove balances absent from delinquent file: %w", err)
	}
	result.Deleted = int(deleted.RowsAffected())
	if err := ActivateImport(ctx, tx, id); err != nil {
		return result, err
	}
	return result, nil
}

func delinquentImportID(ctx context.Context, tx pgx.Tx, meta taxsource.Meta) (string, error) {
	digest := meta.Artifact.SHA256()
	var id, validation string
	err := tx.QueryRow(ctx, `SELECT id::text, validation_result FROM tax_file_imports
WHERE county_id = $1 AND source_name = $2 AND source_url = $3
AND file_sha256 = $4 AND fetched_at = $5`, county.Union, meta.Name,
		meta.URL, digest[:], meta.FetchedAt).Scan(&id, &validation)
	if err == nil {
		if validation != string(ImportValid) {
			return "", errors.New("delinquent file import is invalid")
		}
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("find delinquent file import: %w", err)
	}
	return SaveImportEvidence(ctx, tx, ImportEvidence{
		CountyID: county.Union, SourceName: meta.Name, SourceURL: meta.URL,
		FetchedAt: meta.FetchedAt, Artifact: meta.Artifact, Validation: ImportValid,
	})
}

// DelinquentBalances reads the most recent imported tax balance time.
type DelinquentBalances struct{ pool *pgxpool.Pool }

func NewDelinquentBalances(pool *pgxpool.Pool) DelinquentBalances {
	return DelinquentBalances{pool: pool}
}

// Latest returns false when no delinquent balance has been stored.
func (s DelinquentBalances) Latest(ctx context.Context) (time.Time, bool, error) {
	if s.pool == nil {
		return time.Time{}, false, errors.New("delinquent database required")
	}
	var latest *time.Time
	if err := s.pool.QueryRow(ctx, `SELECT max(fetched_at) FROM tax_delinquent_balances WHERE county_id = $1 AND source_name = $2`, county.Union, "union_delinquent").Scan(&latest); err != nil {
		return time.Time{}, false, fmt.Errorf("latest delinquent file: %w", err)
	}
	if latest == nil {
		return time.Time{}, false, nil
	}
	return *latest, true, nil
}
