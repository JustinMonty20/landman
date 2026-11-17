// Package taxjob imports the four operator-supplied Union tax files in source order.
package taxjob

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/county"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/JustinMonty20/landman/internal/taxsource"
	"github.com/JustinMonty20/landman/internal/taxstore"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config identifies four local files and their declared source URLs.
type Config struct {
	ArtifactDir, DelinquentPath, BillingPath, ForeclosurePath, PipelinePath string
	BillingSourceURL, PipelineSourceURL                                     string
}

// Run validates input paths before the first typed write and publishes each complete file atomically.
func Run(ctx context.Context, pool *pgxpool.Pool, config Config) (jobs.Result, error) {
	var result jobs.Result
	if config.ArtifactDir == "" || config.BillingSourceURL == "" || config.PipelineSourceURL == "" {
		return result, errors.New("tax job requires private artifact directory and declared source URLs")
	}
	for _, path := range []string{config.DelinquentPath, config.BillingPath, config.ForeclosurePath, config.PipelinePath} {
		if path == "" {
			return result, errors.New("tax job requires all four file paths")
		}
		info, err := os.Stat(path)
		if err != nil {
			return result, fmt.Errorf("tax input %q: %w", filepath.Base(path), err)
		}
		if !info.Mode().IsRegular() {
			return result, errors.New("tax input must be a regular file")
		}
	}
	store, err := taxartifact.NewLocal(config.ArtifactDir)
	if err != nil {
		return result, fmt.Errorf("select private tax artifact store: %w", err)
	}
	delinquent, err := union.NewDelinquentList(config.DelinquentPath, store)
	if err != nil {
		return result, err
	}
	billing, err := union.NewBillingFile(config.BillingPath, config.BillingSourceURL, store)
	if err != nil {
		return result, err
	}
	foreclosure, err := union.NewForeclosureList(config.ForeclosurePath, store)
	if err != nil {
		return result, err
	}
	pipeline, err := union.NewPipelineList(config.PipelinePath, config.PipelineSourceURL, store)
	if err != nil {
		return result, err
	}
	var balances []union.DelinquentRow
	dm, err := delinquent.Read(ctx, func(row union.DelinquentRow) error { balances = append(balances, row); return nil })
	if err != nil {
		return result, fmt.Errorf("delinquent: %w", err)
	}
	if current, err := currentFile(ctx, pool, dm, "tax_delinquent_balances", len(balances)); err != nil {
		return result, err
	} else if current {
		balances = nil
		log.Printf("union.tax delinquent unchanged: verified local artifact")
	} else {
		pp, rr, err := delinquent.Skipped(ctx, dm)
		if err != nil {
			return result, err
		}
		if err := saveFile(ctx, pool, &result, func(tx pgx.Tx) (int, error) {
			saved, err := taxstore.SaveDelinquent(ctx, tx, balances, dm, pp, rr)
			if err == nil {
				log.Printf("union.tax delinquent saved=%d deleted=%d PP=%d RR=%d unmatched=%d store=%s verified=%t", saved.Saved, saved.Deleted, saved.SkippedPP, saved.SkippedRR, saved.Unmatched, dm.Artifact.Mode(), dm.Artifact.Verified())
			}
			return saved.Saved, err
		}); err != nil {
			return result, err
		}
		balances = nil
	}
	var bills []union.BillRow
	bm, err := billing.Read(ctx, func(row union.BillRow) error { bills = append(bills, row); return nil })
	if err != nil {
		return result, fmt.Errorf("billing: %w", err)
	}
	if current, err := currentFile(ctx, pool, bm, "tax_bills", len(bills)); err != nil {
		return result, err
	} else if current {
		bills = nil
		log.Printf("union.tax billing unchanged: verified local artifact")
	} else {
		skippedPP, err := billing.Skipped(ctx, bm)
		if err != nil {
			return result, err
		}
		if err := saveFile(ctx, pool, &result, func(tx pgx.Tx) (int, error) {
			saved, err := taxstore.SaveBill(ctx, tx, bills, bm, skippedPP)
			if err == nil {
				log.Printf("union.tax billing saved=%d deleted=%d PP=%d unmatched=%d store=%s verified=%t", saved.Saved, saved.Deleted, saved.SkippedPP, saved.Unmatched, bm.Artifact.Mode(), bm.Artifact.Verified())
			}
			return saved.Saved, err
		}); err != nil {
			return result, err
		}
		bills = nil
	}
	var cases []union.ForeclosureCase
	fm, err := foreclosure.Read(ctx, func(row union.ForeclosureCase) error { cases = append(cases, row); return nil })
	if err != nil {
		return result, fmt.Errorf("foreclosure: %w", err)
	}
	pinCount := 0
	for _, c := range cases {
		pinCount += len(c.PINs)
	}
	if current, err := currentForeclosure(ctx, pool, fm, len(cases), pinCount); err != nil {
		return result, err
	} else if current {
		cases = nil
		log.Printf("union.tax foreclosure unchanged: verified local artifact")
	} else {
		skippedBlocks, err := foreclosure.Skipped(ctx, fm)
		if err != nil {
			return result, err
		}
		if err := saveFile(ctx, pool, &result, func(tx pgx.Tx) (int, error) {
			saved, err := taxstore.SaveForeclosure(ctx, tx, cases, fm, skippedBlocks)
			if err == nil {
				log.Printf("union.tax foreclosure cases=%d joins=%d skipped=%d unmatched=%d store=%s verified=%t", saved.SavedCases, saved.SavedJoins, saved.SkippedBlocks, saved.Unmatched, fm.Artifact.Mode(), fm.Artifact.Verified())
			}
			return saved.SavedCases, err
		}); err != nil {
			return result, err
		}
		cases = nil
	}
	var properties []union.PipelineProperty
	pm, err := pipeline.Read(ctx, func(row union.PipelineProperty) error { properties = append(properties, row); return nil })
	if err != nil {
		return result, fmt.Errorf("pipeline: %w", err)
	}
	if current, err := currentFile(ctx, pool, pm, "tax_foreclosure_pipeline", len(properties)); err != nil {
		return result, err
	} else if current {
		log.Printf("union.tax pipeline unchanged: verified local artifact")
		return result, nil
	}
	skippedPages, err := pipeline.Skipped(ctx, pm)
	if err != nil {
		return result, err
	}
	if err := saveFile(ctx, pool, &result, func(tx pgx.Tx) (int, error) {
		saved, err := taxstore.SavePipeline(ctx, tx, properties, pm, skippedPages)
		if err == nil {
			log.Printf("union.tax pipeline saved=%d deleted=%d skipped=%d unmatched=%d store=%s verified=%t", saved.Saved, saved.Deleted, saved.SkippedPages, saved.Unmatched, pm.Artifact.Mode(), pm.Artifact.Verified())
		}
		return saved.Saved, err
	}); err != nil {
		return result, err
	}
	return result, nil
}

// currentFile skips only a complete matched generation with no newer parcel records.
func currentFile(ctx context.Context, pool *pgxpool.Pool, meta taxsource.Meta, table string, expected int) (bool, error) {
	var activated bool
	digest := meta.Artifact.SHA256()
	err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tax_file_imports i WHERE i.county_id=$1 AND i.source_name=$2 AND i.active AND i.file_sha256=$3 AND NOT EXISTS(SELECT 1 FROM parcels p WHERE p.county_id=$1 AND p.source_name='union_county_gis' AND p.updated_at>=i.activated_at))`, county.Union, meta.Name, digest[:]).Scan(&activated)
	if err != nil || !activated {
		return false, err
	}
	// Table is selected from fixed code constants, never from operator input.
	var count int
	err = pool.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE county_id=$1 AND source_name=$2 AND file_sha256=$3", county.Union, meta.Name, digest[:]).Scan(&count)
	return count == expected, err
}

func currentForeclosure(ctx context.Context, pool *pgxpool.Pool, meta taxsource.Meta, cases, pins int) (bool, error) {
	current, err := currentFile(ctx, pool, meta, "tax_foreclosure_cases", cases)
	if err != nil || !current {
		return false, err
	}
	var count int
	digest := meta.Artifact.SHA256()
	err = pool.QueryRow(ctx, `SELECT count(*) FROM tax_foreclosure_parcels j JOIN tax_foreclosure_cases c ON c.id=j.case_id WHERE c.county_id=$1 AND c.source_name=$2 AND c.file_sha256=$3`, county.Union, meta.Name, digest[:]).Scan(&count)
	return count == pins, err
}

func saveFile(ctx context.Context, pool *pgxpool.Pool, result *jobs.Result, save func(pgx.Tx) (int, error)) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	count, err := save(tx)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	result.Processed += count
	return nil
}
