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

// BillResult reports stored, deleted, and skipped rows from one complete billing file.
type BillResult struct {
	Saved     int
	Deleted   int
	SkippedPP int
	Unmatched int
}

// SaveBill replaces billed taxes from a complete retained file in the caller's transaction.
// The caller rolls back on any error and commits only after success.
func SaveBill(ctx context.Context, tx pgx.Tx, rows []union.BillRow, meta taxsource.Meta, skippedPP int) (BillResult, error) {
	return saveBillBatch(ctx, tx, rows, meta, skippedPP, true)
}

// PublishBillFile replaces bills from a complete Gob-encoded BillRow stream in one transaction.
// The caller must parse and validate the retained source file before it supplies the stream.
func PublishBillFile(ctx context.Context, pool *pgxpool.Pool, input io.Reader, meta taxsource.Meta, skippedPP int) (BillResult, error) {
	add := func(total *BillResult, batch BillResult) {
		total.Saved += batch.Saved
		total.Unmatched += batch.Unmatched
		total.Deleted += batch.Deleted
		total.SkippedPP = batch.SkippedPP
	}
	return publishStagedRows[union.BillRow, BillResult](ctx, pool, input,
		func(tx pgx.Tx, rows []union.BillRow) (BillResult, error) {
			return saveBillBatch(ctx, tx, rows, meta, skippedPP, false)
		},
		func(tx pgx.Tx) (BillResult, error) {
			return saveBillBatch(ctx, tx, nil, meta, skippedPP, true)
		}, add)
}

// PublishBillInTx publishes a complete staged source in the engine transaction.
func PublishBillInTx(ctx context.Context, tx pgx.Tx, input io.Reader, meta taxsource.Meta, skippedPP int) (BillResult, error) {
	return PublishStagedInTx[union.BillRow, BillResult](ctx, tx, input,
		func(rows []union.BillRow) (BillResult, error) {
			return saveBillBatch(ctx, tx, rows, meta, skippedPP, false)
		},
		func() (BillResult, error) { return saveBillBatch(ctx, tx, nil, meta, skippedPP, true) },
		func(total *BillResult, batch BillResult) {
			total.Saved += batch.Saved
			total.Deleted += batch.Deleted
			total.Unmatched += batch.Unmatched
			total.SkippedPP = batch.SkippedPP
		})
}

func saveBillBatch(ctx context.Context, tx pgx.Tx, rows []union.BillRow, meta taxsource.Meta, skippedPP int, complete bool) (BillResult, error) {
	var result BillResult
	if meta.Name != "union_billing" || meta.URL == "" || meta.FetchedAt.IsZero() || !meta.Artifact.Verified() || meta.SHA256 != meta.Artifact.SHA256() || skippedPP < 0 {
		return result, errors.New("invalid billing import evidence")
	}
	result.SkippedPP = skippedPP
	id, err := billImportID(ctx, tx, meta)
	if err != nil {
		return result, err
	}
	digest := meta.Artifact.SHA256()
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE IF NOT EXISTS billing_seen (parcel_id uuid, tax_year smallint, PRIMARY KEY (parcel_id,tax_year)) ON COMMIT DROP`); err != nil {
		return result, err
	}
	for _, row := range rows {
		if row.ParcelKey == "" || row.Year < 1900 || row.Year > 2200 || row.TotalTaxes < 0 || row.TotalTaxes > 99999999999999 {
			return result, errors.New("invalid billing row")
		}
		var parcelID string
		err := tx.QueryRow(ctx, `SELECT id FROM parcels WHERE county_id = $1 AND source_name = $2 AND source_parcel_id = $3`, county.Union, "union_county_gis", row.ParcelKey).Scan(&parcelID)
		if errors.Is(err, pgx.ErrNoRows) {
			result.Unmatched++
			continue
		}
		if err != nil {
			return result, fmt.Errorf("lookup billed parcel: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO billing_seen(parcel_id,tax_year) VALUES ($1,$2) ON CONFLICT DO NOTHING`, parcelID, row.Year); err != nil {
			return result, err
		}
		amount := fmt.Sprintf("%d.%02d", row.TotalTaxes/100, row.TotalTaxes%100)
		_, err = tx.Exec(ctx, `
INSERT INTO tax_bills (
  county_id, source_name, parcel_id, tax_year, total_taxes, property_address,
  taxpayer_name, mailing_line, mailing_city, mailing_state, mailing_postal_code,
  source_url, file_sha256, fetched_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
ON CONFLICT (county_id, source_name, parcel_id, tax_year)
DO UPDATE SET
  total_taxes = EXCLUDED.total_taxes,
  property_address = EXCLUDED.property_address,
  taxpayer_name = EXCLUDED.taxpayer_name,
  mailing_line = EXCLUDED.mailing_line,
  mailing_city = EXCLUDED.mailing_city,
  mailing_state = EXCLUDED.mailing_state,
  mailing_postal_code = EXCLUDED.mailing_postal_code,
  source_url = EXCLUDED.source_url,
  file_sha256 = EXCLUDED.file_sha256,
  fetched_at = EXCLUDED.fetched_at,
  updated_at = now()
WHERE (tax_bills.total_taxes, tax_bills.property_address, tax_bills.taxpayer_name, tax_bills.mailing_line, tax_bills.mailing_city, tax_bills.mailing_state, tax_bills.mailing_postal_code) IS DISTINCT FROM (EXCLUDED.total_taxes, EXCLUDED.property_address, EXCLUDED.taxpayer_name, EXCLUDED.mailing_line, EXCLUDED.mailing_city, EXCLUDED.mailing_state, EXCLUDED.mailing_postal_code)`, county.Union, meta.Name, parcelID, row.Year, amount,
			billingNullable(row.PropertyAddress), billingNullable(row.TaxpayerName), billingNullable(row.MailingLine),
			billingNullable(row.MailingCity), billingNullable(row.MailingState), billingNullable(row.MailingPostalCode),
			meta.URL, digest[:], meta.FetchedAt)
		if err != nil {
			return result, fmt.Errorf("save billed tax: %w", err)
		}
		result.Saved++
	}
	if !complete {
		return result, nil
	}
	deleted, err := tx.Exec(ctx, `DELETE FROM tax_bills
WHERE county_id = $1 AND source_name = $2 AND NOT EXISTS (SELECT 1 FROM billing_seen seen WHERE seen.parcel_id=tax_bills.parcel_id AND seen.tax_year=tax_bills.tax_year)`, county.Union, meta.Name)
	if err != nil {
		return result, fmt.Errorf("remove bills absent from billing file: %w", err)
	}
	result.Deleted = int(deleted.RowsAffected())
	if err := ActivateImport(ctx, tx, id); err != nil {
		return result, err
	}
	return result, nil
}

func billingNullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func billImportID(ctx context.Context, tx pgx.Tx, meta taxsource.Meta) (string, error) {
	digest := meta.Artifact.SHA256()
	var id, validation string
	err := tx.QueryRow(ctx, `SELECT id::text, validation_result FROM tax_file_imports
WHERE county_id = $1 AND source_name = $2 AND source_url = $3
AND file_sha256 = $4 AND fetched_at = $5`, county.Union, meta.Name,
		meta.URL, digest[:], meta.FetchedAt).Scan(&id, &validation)
	if err == nil {
		if validation != string(ImportValid) {
			return "", errors.New("billing file import is invalid")
		}
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("find billing file import: %w", err)
	}
	return SaveImportEvidence(ctx, tx, ImportEvidence{
		CountyID: county.Union, SourceName: meta.Name, SourceURL: meta.URL,
		FetchedAt: meta.FetchedAt, Artifact: meta.Artifact, Validation: ImportValid,
	})
}

// Bills reads the most recent stored billing time.
type Bills struct{ pool *pgxpool.Pool }

func NewBills(pool *pgxpool.Pool) Bills {
	return Bills{pool: pool}
}

// Latest returns false when no bill has matched a stored parcel.
func (s Bills) Latest(ctx context.Context) (time.Time, bool, error) {
	if s.pool == nil {
		return time.Time{}, false, errors.New("billing database required")
	}
	var latest *time.Time
	if err := s.pool.QueryRow(ctx, `SELECT max(fetched_at) FROM tax_bills WHERE county_id = $1 AND source_name = $2`, county.Union, "union_billing").Scan(&latest); err != nil {
		return time.Time{}, false, fmt.Errorf("latest billing file: %w", err)
	}
	if latest == nil {
		return time.Time{}, false, nil
	}
	return *latest, true, nil
}
