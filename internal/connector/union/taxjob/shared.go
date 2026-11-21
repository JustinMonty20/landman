package taxjob

import (
	"context"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/county"
	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/JustinMonty20/landman/internal/taxsource"
	"github.com/JustinMonty20/landman/internal/taxstore"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type taxPublisher struct {
	pool                             *pgxpool.Pool
	store                            taxartifact.Store
	dir                              string
	source, url                      string
	meta                             taxsource.Meta
	staged                           *os.File
	canonical                        [32]byte
	normalized                       [32]byte
	skippedPP, skippedRR             int
	cases                            []union.ForeclosureCase
	properties                       []union.PipelineProperty
	result                           jobs.Result
	saved, deleted, unmatched, joins int
}

func (p *taxPublisher) close() {
	if p.staged != nil {
		name := p.staged.Name()
		_ = p.staged.Close()
		_ = os.Remove(name)
	}
}

func (p *taxPublisher) Prepare(ctx context.Context, capture ingest.VerifiedCapture) error {
	manifest := capture.Manifest()
	if manifest.Source != p.source || len(manifest.Receipts) != 1 || manifest.Receipts[0].Request != p.url {
		return errors.New("tax capture source or declared URL differs from job")
	}
	artifact, err := capture.Artifact(0)
	if err != nil {
		return err
	}
	p.meta = taxsource.Meta{URL: p.url, Name: p.source, FetchedAt: manifest.ObservedAt, SHA256: artifact.SHA256(), Artifact: artifact}
	if p.source == "union_delinquent" || p.source == "union_billing" {
		p.staged, err = os.CreateTemp(p.dir, ".union-tax-rows-*")
		if err != nil {
			return err
		}
		encoder := gob.NewEncoder(p.staged)
		normalized := sha256.New()
		normalizer := json.NewEncoder(normalized)
		switch p.source {
		case "union_delinquent":
			parser, e := union.NewDelinquentList("retained", p.store)
			if e != nil {
				return e
			}
			p.skippedPP, p.skippedRR, err = parser.ReadRetained(ctx, artifact, func(row union.DelinquentRow) error {
				if err := encoder.Encode(row); err != nil {
					return err
				}
				return normalizer.Encode(row)
			})
		case "union_billing":
			parser, e := union.NewBillingFile("retained", p.url, p.store)
			if e != nil {
				return e
			}
			p.skippedPP, err = parser.ReadRetained(ctx, artifact, func(row union.BillRow) error {
				if err := encoder.Encode(row); err != nil {
					return err
				}
				return normalizer.Encode(row)
			})
		}
		if err != nil {
			return err
		}
		if err = p.staged.Sync(); err != nil {
			return err
		}
		if _, err = p.staged.Seek(0, io.SeekStart); err != nil {
			return err
		}
		h := sha256.New()
		if _, err = io.Copy(h, p.staged); err != nil {
			return err
		}
		copy(p.canonical[:], h.Sum(nil))
		copy(p.normalized[:], normalized.Sum(nil))
		_, err = p.staged.Seek(0, io.SeekStart)
		return err
	}
	h := sha256.New()
	encoder := gob.NewEncoder(h)
	normalized := sha256.New()
	normalizer := json.NewEncoder(normalized)
	switch p.source {
	case "union_foreclosure":
		parser, e := union.NewForeclosureList("retained", p.store)
		if e != nil {
			return e
		}
		p.skippedPP, err = parser.ReadRetained(ctx, artifact, func(row union.ForeclosureCase) error {
			if err := encoder.Encode(row); err != nil {
				return err
			}
			if err := normalizer.Encode(row); err != nil {
				return err
			}
			p.cases = append(p.cases, row)
			return nil
		})
	case "union_foreclosure_pipeline":
		parser, e := union.NewPipelineList("retained", p.url, p.store)
		if e != nil {
			return e
		}
		p.skippedPP, err = parser.ReadRetained(ctx, artifact, func(row union.PipelineProperty) error {
			if err := encoder.Encode(row); err != nil {
				return err
			}
			if err := normalizer.Encode(row); err != nil {
				return err
			}
			p.properties = append(p.properties, row)
			return nil
		})
	default:
		return fmt.Errorf("unknown tax source %q", p.source)
	}
	if err != nil {
		return err
	}
	copy(p.canonical[:], h.Sum(nil))
	copy(p.normalized[:], normalized.Sum(nil))
	return nil
}

func (p *taxPublisher) missingLinks(ctx context.Context, tx pgx.Tx) (bool, error) {
	switch p.source {
	case "union_delinquent", "union_billing":
		table := "tax_delinquent_balances"
		if p.source == "union_billing" {
			table = "tax_bills"
		}
		if _, err := p.staged.Seek(0, io.SeekStart); err != nil {
			return false, err
		}
		defer p.staged.Seek(0, io.SeekStart)
		decoder := gob.NewDecoder(p.staged)
		for {
			keys := make([]string, 0, 512)
			years := make([]int16, 0, 512)
			for len(keys) < cap(keys) {
				if err := ctx.Err(); err != nil {
					return false, err
				}
				if p.source == "union_billing" {
					var row union.BillRow
					err := decoder.Decode(&row)
					if err == io.EOF {
						break
					}
					if err != nil {
						return false, err
					}
					keys = append(keys, row.ParcelKey)
					years = append(years, row.Year)
				} else {
					var row union.DelinquentRow
					err := decoder.Decode(&row)
					if err == io.EOF {
						break
					}
					if err != nil {
						return false, err
					}
					keys = append(keys, row.ParcelKey)
					years = append(years, row.Year)
				}
			}
			if len(keys) == 0 {
				return false, nil
			}
			var missing bool
			query := fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM unnest($1::text[],$2::smallint[]) AS input(key,yr) JOIN parcels p ON p.county_id='union' AND p.source_name='union_county_gis' AND p.source_parcel_id=input.key LEFT JOIN %s t ON t.parcel_id=p.id AND t.tax_year=input.yr AND t.county_id='union' AND t.source_name=$3 WHERE t.id IS NULL)`, table)
			if err := tx.QueryRow(ctx, query, keys, years, p.source).Scan(&missing); err != nil {
				return false, err
			}
			if missing {
				return true, nil
			}
		}
	case "union_foreclosure":
		current, err := taxstore.ForeclosureLinksCurrent(ctx, tx, p.cases)
		return !current, err
	case "union_foreclosure_pipeline":
		keys := make([]string, 0, len(p.properties))
		for _, row := range p.properties {
			keys = append(keys, row.ParcelKey)
		}
		var missing bool
		err := tx.QueryRow(ctx, `WITH matched AS (SELECT DISTINCT p.id FROM unnest($1::text[]) AS input(key) JOIN parcels p ON p.county_id='union' AND p.source_name='union_county_gis' AND p.source_parcel_id=input.key), saved AS (SELECT parcel_id AS id FROM tax_foreclosure_pipeline WHERE county_id='union' AND source_name=$2) SELECT EXISTS(SELECT id FROM matched EXCEPT SELECT id FROM saved) OR EXISTS(SELECT id FROM saved EXCEPT SELECT id FROM matched)`, keys, p.source).Scan(&missing)
		return missing, err
	}
	return false, nil
}

func (p *taxPublisher) Apply(ctx context.Context, tx pgx.Tx, observation ingest.Observation) error {
	var previousID *string
	var previousURL string
	var previousByte, previousCanonical, previousNormalized []byte
	var previousCanonicalVersion, previousVersion string
	err := tx.QueryRow(ctx, `SELECT capture_id::text, source_url, byte_sha256, canonical_sha256, canonical_version, normalized_sha256, normalization_version FROM tax_ingest_publications WHERE source_name=$1 FOR UPDATE`, p.source).Scan(&previousID, &previousURL, &previousByte, &previousCanonical, &previousCanonicalVersion, &previousNormalized, &previousVersion)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	// The source lock is held by Engine. The latest accepted observation may
	// have no typed writes and therefore differ from the active publication.
	var latestID string
	var latestAt time.Time
	err = tx.QueryRow(ctx, `SELECT capture_id::text,observed_at FROM tax_latest_observations WHERE source_name=$1`, p.source).Scan(&latestID, &latestAt)
	if errors.Is(err, pgx.ErrNoRows) && previousID != nil {
		latestID = *previousID
		err = tx.QueryRow(ctx, `SELECT observed_at FROM ingest_captures WHERE id=$1`, latestID).Scan(&latestAt)
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if latestID != "" && latestID != observation.ID && !latestAt.Before(observation.Manifest.ObservedAt) {
		return ingest.ErrNoPublication
	}
	missing, err := p.missingLinks(ctx, tx)
	if err != nil {
		return err
	}
	sameProjection := previousID != nil && string(previousNormalized) == string(p.normalized[:]) && previousVersion == "tax-normalized-v1"
	sameBytes := previousID != nil && string(previousByte) == string(p.meta.SHA256[:])
	decision := ingest.Classify(
		ingest.Fingerprint{Source: hex.EncodeToString(previousByte), SourceVersion: "tax-bytes-v1", Normalized: hex.EncodeToString(previousNormalized), NormalizedVersion: previousVersion},
		ingest.Fingerprint{Source: hex.EncodeToString(p.meta.SHA256[:]), SourceVersion: "tax-bytes-v1", Normalized: hex.EncodeToString(p.normalized[:]), NormalizedVersion: "tax-normalized-v1"}, observation.Replay)
	if decision.CompareProjection {
		// Normalized digests from different versions are not comparable. The
		// separately versioned typed-row encoding remains comparable.
		sameProjection = previousID != nil && previousCanonicalVersion == "tax-gob-v1" && string(previousCanonical) == string(p.canonical[:])
	}
	if observation.Published && (previousID == nil || *previousID != observation.ID || (!missing && sameProjection)) {
		return ingest.ErrNoPublication
	}
	if _, err := tx.Exec(ctx, `INSERT INTO tax_latest_observations(source_name,capture_id,observed_at) VALUES ($1,$2,$3)
ON CONFLICT (source_name) DO UPDATE SET capture_id=EXCLUDED.capture_id,observed_at=EXCLUDED.observed_at`, p.source, observation.ID, observation.Manifest.ObservedAt); err != nil {
		return err
	}
	cause := decision.Cause
	if sameBytes && missing && sameProjection {
		cause = ingest.OfflineRetry
	}
	if sameBytes && !sameProjection {
		cause = ingest.NormalizationCorrection
	}
	if cause == "" {
		cause = ingest.SourceObservation
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('landman.tax_change_cause',$1,true)`, string(cause)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('landman.tax_capture_id',$1,true)`, observation.ID); err != nil {
		return err
	}
	if sameBytes && sameProjection && previousURL == p.url && !missing {
		return ingest.ErrObservationOnly
	}
	if !sameProjection || missing {
		switch p.source {
		case "union_delinquent":
			if _, err := p.staged.Seek(0, io.SeekStart); err != nil {
				return err
			}
			saved, err := taxstore.PublishDelinquentInTx(ctx, tx, p.staged, p.meta, p.skippedPP, p.skippedRR)
			if err != nil {
				return err
			}
			p.result.Processed = saved.Saved
			p.saved, p.deleted, p.unmatched = saved.Saved, saved.Deleted, saved.Unmatched
		case "union_billing":
			if _, err := p.staged.Seek(0, io.SeekStart); err != nil {
				return err
			}
			saved, err := taxstore.PublishBillInTx(ctx, tx, p.staged, p.meta, p.skippedPP)
			if err != nil {
				return err
			}
			p.result.Processed = saved.Saved
			p.saved, p.deleted, p.unmatched = saved.Saved, saved.Deleted, saved.Unmatched
		case "union_foreclosure":
			saved, err := taxstore.SaveForeclosure(ctx, tx, p.cases, p.meta, p.skippedPP)
			if err != nil {
				return err
			}
			p.result.Processed = saved.SavedCases
			p.saved, p.joins, p.unmatched = saved.SavedCases, saved.SavedJoins, saved.Unmatched
		case "union_foreclosure_pipeline":
			saved, err := taxstore.SavePipeline(ctx, tx, p.properties, p.meta, p.skippedPP)
			if err != nil {
				return err
			}
			p.result.Processed = saved.Saved
			p.saved, p.deleted, p.unmatched = saved.Saved, saved.Deleted, saved.Unmatched
		}
	} else {
		id, err := taxstore.SaveImportEvidence(ctx, tx, taxstore.ImportEvidence{CountyID: county.Union, SourceName: p.source, SourceURL: p.url, FetchedAt: p.meta.FetchedAt, Artifact: p.meta.Artifact, Validation: taxstore.ImportValid})
		if err != nil {
			return err
		}
		// Equivalent tax rows still need findings on the new active import.
		if _, err := tx.Exec(ctx, `INSERT INTO tax_unmatched_records(import_id,parcel_key,tax_year,capture_id)
SELECT $1,u.parcel_key,u.tax_year,$2::uuid FROM tax_unmatched_records u
JOIN tax_file_imports i ON i.id=u.import_id
WHERE i.county_id='union' AND i.source_name=$3 AND i.active
AND NOT EXISTS (SELECT 1 FROM parcels p WHERE p.county_id='union' AND p.source_name='union_county_gis' AND p.source_parcel_id=u.parcel_key)
ON CONFLICT (import_id,parcel_key,tax_year) DO NOTHING`, id, observation.ID, p.source); err != nil {
			return fmt.Errorf("retain unmatched tax PINs for source=%s capture=%s import=%s: %w", p.source, observation.ID, id, err)
		}
		if err := taxstore.ActivateImport(ctx, tx, id); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO tax_ingest_publications (source_name,source_url,capture_id,byte_sha256,canonical_sha256,canonical_version,normalized_sha256,normalization_version) VALUES ($1,$2,$3,$4,$5,'tax-gob-v1',$6,'tax-normalized-v1') ON CONFLICT (source_name) DO UPDATE SET source_url=EXCLUDED.source_url,capture_id=EXCLUDED.capture_id,byte_sha256=EXCLUDED.byte_sha256,canonical_sha256=EXCLUDED.canonical_sha256,canonical_version=EXCLUDED.canonical_version,normalized_sha256=EXCLUDED.normalized_sha256,normalization_version=EXCLUDED.normalization_version`, p.source, p.url, observation.ID, p.meta.SHA256[:], p.canonical[:], p.normalized[:])
	return err
}

func ingestFile(ctx context.Context, pool *pgxpool.Pool, store taxartifact.Store, dir, source, url, path string) (jobs.Result, error) {
	var result jobs.Result
	log.Printf("[union.tax] source=%s: Retain local input file", source)
	recorder, err := ingest.NewRecorder(store, source, time.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		return result, err
	}
	if attempt, ok := jobs.AttemptID(ctx); ok {
		if err := recorder.BindAttempt("union.tax", attempt, "full_scan", 0); err != nil {
			return result, err
		}
	}
	file, err := os.Open(path)
	if err != nil {
		return result, err
	}
	err = recorder.Record(ctx, url, "", 0, file)
	err = errors.Join(err, file.Close())
	if err != nil {
		return result, err
	}
	publisher := &taxPublisher{pool: pool, store: store, dir: dir, source: source, url: url}
	defer publisher.close()
	id, err := (ingest.Engine{Pool: pool, Store: store}).Ingest(ctx, recorder, publisher)
	if err != nil && id != "" {
		return result, fmt.Errorf("retained capture %s: %w", id, err)
	}
	if err == nil {
		log.Printf("union.tax %s saved=%d joins=%d deleted=%d skipped=%d skippedRR=%d unmatched=%d mode=%s verified=%t", source, publisher.saved, publisher.joins, publisher.deleted, publisher.skippedPP, publisher.skippedRR, publisher.unmatched, store.Mode(), publisher.meta.Artifact.Verified())
	}
	return publisher.result, err
}

// Replay republishes a retained tax capture without reading an operator file.
func Replay(ctx context.Context, pool *pgxpool.Pool, dir, id string) (jobs.Result, error) {
	var result jobs.Result
	store, err := taxartifact.NewLocal(dir)
	if err != nil {
		return result, err
	}
	var source string
	var manifest []byte
	if err := pool.QueryRow(ctx, `SELECT source_name,manifest FROM ingest_captures WHERE id=$1::uuid`, id).Scan(&source, &manifest); err != nil {
		return result, err
	}
	var m ingest.Manifest
	if err := json.Unmarshal(manifest, &m); err != nil {
		return result, err
	}
	if len(m.Receipts) != 1 {
		return result, errors.New("tax replay requires one complete file")
	}
	publisher := &taxPublisher{pool: pool, store: store, dir: dir, source: source, url: m.Receipts[0].Request}
	defer publisher.close()
	err = (ingest.Engine{Pool: pool, Store: store}).Replay(ctx, id, "union.tax", publisher)
	return publisher.result, err
}
