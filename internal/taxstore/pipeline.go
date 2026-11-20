package taxstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/county"
	"github.com/JustinMonty20/landman/internal/taxsource"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PipelineResult counts one complete retained file's reconciliation.
type PipelineResult struct{ Saved, Deleted, SkippedPages, Unmatched int }

// SavePipeline publishes current matched parcels and evidence in the caller's transaction.
func SavePipeline(ctx context.Context, tx pgx.Tx, rows []union.PipelineProperty, meta taxsource.Meta, skippedPages int) (PipelineResult, error) {
	var result PipelineResult
	if meta.Name != "union_foreclosure_pipeline" || meta.URL == "" || meta.FetchedAt.IsZero() || !meta.Artifact.Verified() || meta.SHA256 != meta.Artifact.SHA256() || skippedPages < 0 {
		return result, errors.New("invalid pipeline import evidence")
	}
	result.SkippedPages = skippedPages
	digest := meta.Artifact.SHA256()
	var importID, validation string
	err := tx.QueryRow(ctx, `SELECT id::text, validation_result FROM tax_file_imports WHERE county_id=$1 AND source_name=$2 AND source_url=$3 AND file_sha256=$4 AND fetched_at=$5`, county.Union, meta.Name, meta.URL, digest[:], meta.FetchedAt).Scan(&importID, &validation)
	if errors.Is(err, pgx.ErrNoRows) {
		importID, err = SaveImportEvidence(ctx, tx, ImportEvidence{CountyID: county.Union, SourceName: meta.Name, SourceURL: meta.URL, FetchedAt: meta.FetchedAt, Artifact: meta.Artifact, Validation: ImportValid})
	} else if err == nil && validation != string(ImportValid) {
		return result, errors.New("pipeline import was invalid")
	}
	if err != nil {
		return result, fmt.Errorf("pipeline import evidence: %w", err)
	}
	// The set of matched UUIDs, not the source PINs, defines the active join.
	seen := map[string]bool{}
	for _, row := range rows {
		if row.ParcelKey == "" || row.TaxValue != nil && (*row.TaxValue < 0 || *row.TaxValue > 99999999999999) {
			return result, errors.New("invalid pipeline row")
		}
		var parcelID string
		err := tx.QueryRow(ctx, `SELECT id::text FROM parcels WHERE county_id=$1 AND source_name=$2 AND source_parcel_id=$3`, county.Union, "union_county_gis", row.ParcelKey).Scan(&parcelID)
		if errors.Is(err, pgx.ErrNoRows) {
			result.Unmatched++
			continue
		}
		if err != nil {
			return result, fmt.Errorf("lookup pipeline parcel PIN %q: %w", row.ParcelKey, err)
		}
		if seen[parcelID] {
			continue
		}
		seen[parcelID] = true
		var value any
		if row.TaxValue != nil {
			value = fmt.Sprintf("%d.%02d", *row.TaxValue/100, *row.TaxValue%100)
		}
		_, err = tx.Exec(ctx, `INSERT INTO tax_foreclosure_pipeline
 (county_id,source_name,parcel_id,owner_name,situs_line,township,acreage,tax_value,case_number,sale_on,schedule_text,status,redeemed,source_url,file_sha256,fetched_at)
 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
 ON CONFLICT (county_id,source_name,parcel_id) DO UPDATE SET
 owner_name=EXCLUDED.owner_name,situs_line=EXCLUDED.situs_line,township=EXCLUDED.township,acreage=EXCLUDED.acreage,tax_value=EXCLUDED.tax_value,
 case_number=EXCLUDED.case_number,sale_on=EXCLUDED.sale_on,schedule_text=EXCLUDED.schedule_text,status=EXCLUDED.status,redeemed=EXCLUDED.redeemed,
 source_url=EXCLUDED.source_url,file_sha256=EXCLUDED.file_sha256,fetched_at=EXCLUDED.fetched_at,updated_at=now() WHERE (tax_foreclosure_pipeline.owner_name,tax_foreclosure_pipeline.situs_line,tax_foreclosure_pipeline.township,tax_foreclosure_pipeline.acreage,tax_foreclosure_pipeline.tax_value,tax_foreclosure_pipeline.case_number,tax_foreclosure_pipeline.sale_on,tax_foreclosure_pipeline.schedule_text,tax_foreclosure_pipeline.status,tax_foreclosure_pipeline.redeemed) IS DISTINCT FROM (EXCLUDED.owner_name,EXCLUDED.situs_line,EXCLUDED.township,EXCLUDED.acreage,EXCLUDED.tax_value,EXCLUDED.case_number,EXCLUDED.sale_on,EXCLUDED.schedule_text,EXCLUDED.status,EXCLUDED.redeemed)`,
			county.Union, meta.Name, parcelID, billingNullable(row.OwnerName), billingNullable(row.SitusLine), billingNullable(row.Township), billingNullable(row.Acreage), value, billingNullable(row.CaseNumber), row.SaleOn, billingNullable(row.ScheduleText), billingNullable(row.Status), row.Redeemed, meta.URL, digest[:], meta.FetchedAt)
		if err != nil {
			return result, fmt.Errorf("save pipeline parcel PIN %q (parcel_id=%s, case=%q): %w", row.ParcelKey, parcelID, row.CaseNumber, err)
		}
		result.Saved++
	}
	// A repeat of identical bytes must also remove rows omitted from that file.
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	deleted, err := tx.Exec(ctx, `DELETE FROM tax_foreclosure_pipeline WHERE county_id=$1 AND source_name=$2 AND NOT (parcel_id::text = ANY($3::text[]))`, county.Union, meta.Name, ids)
	if err != nil {
		return result, fmt.Errorf("remove absent pipeline parcels: %w", err)
	}
	result.Deleted = int(deleted.RowsAffected())
	if err := ActivateImport(ctx, tx, importID); err != nil {
		return result, err
	}
	return result, nil
}

type Pipeline struct{ pool *pgxpool.Pool }

func NewPipeline(pool *pgxpool.Pool) Pipeline { return Pipeline{pool: pool} }

// Latest reports the most recent matched parcel observation, not a freshness gate.
func (s Pipeline) Latest(ctx context.Context) (time.Time, bool, error) {
	if s.pool == nil {
		return time.Time{}, false, errors.New("pipeline database required")
	}
	var latest *time.Time
	if err := s.pool.QueryRow(ctx, `SELECT max(fetched_at) FROM tax_foreclosure_pipeline WHERE county_id=$1 AND source_name=$2`, county.Union, "union_foreclosure_pipeline").Scan(&latest); err != nil {
		return time.Time{}, false, err
	}
	if latest == nil {
		return time.Time{}, false, nil
	}
	return *latest, true, nil
}
