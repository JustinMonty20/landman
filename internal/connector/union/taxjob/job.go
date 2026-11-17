// Package taxjob imports the four operator-supplied Union tax files in source order.
package taxjob

import (
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
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
		file, err := os.Open(path)
		if err != nil {
			return result, fmt.Errorf("tax input %q: %w", filepath.Base(path), err)
		}
		var probe [1]byte
		_, readErr := file.Read(probe[:])
		closeErr := file.Close()
		if readErr != nil && readErr != io.EOF || closeErr != nil {
			return result, fmt.Errorf("tax input %q unreadable: %w", filepath.Base(path), errors.Join(readErr, closeErr))
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
	balanceFile, dm, err := stageRows(ctx, config.ArtifactDir, delinquent.Read)
	if err != nil {
		return result, fmt.Errorf("delinquent: %w", err)
	}
	defer releaseRows(balanceFile)
	if current, err := currentFile(ctx, pool, dm); err != nil {
		return result, err
	} else if current {
		log.Printf("union.tax delinquent unchanged: verified local artifact")
	} else {
		pp, rr, err := delinquent.Skipped(ctx, dm)
		if err != nil {
			return result, err
		}
		var unmatched, deleted int
		if err := saveFile(ctx, pool, &result, func(tx pgx.Tx) (int, error) {
			count, err := saveRows(ctx, tx, balanceFile, func(rows []union.DelinquentRow) (int, error) {
				saved, err := taxstore.SaveDelinquentBatch(ctx, tx, rows, dm, pp, rr, false)
				unmatched += saved.Unmatched
				return saved.Saved, err
			})
			if err != nil {
				return count, err
			}
			final, err := taxstore.SaveDelinquentBatch(ctx, tx, nil, dm, pp, rr, true)
			deleted = final.Deleted
			return count, err
		}); err != nil {
			return result, err
		}
		log.Printf("union.tax delinquent saved=%d deleted=%d PP=%d RR=%d unmatched=%d store=%s verified=%t", result.Processed, deleted, pp, rr, unmatched, dm.Artifact.Mode(), dm.Artifact.Verified())
	}
	billFile, bm, err := stageRows(ctx, config.ArtifactDir, billing.Read)
	if err != nil {
		return result, fmt.Errorf("billing: %w", err)
	}
	defer releaseRows(billFile)
	if current, err := currentFile(ctx, pool, bm); err != nil {
		return result, err
	} else if current {
		log.Printf("union.tax billing unchanged: verified local artifact")
	} else {
		skippedPP, err := billing.Skipped(ctx, bm)
		if err != nil {
			return result, err
		}
		var savedCount, unmatched, deleted int
		if err := saveFile(ctx, pool, &result, func(tx pgx.Tx) (int, error) {
			count, err := saveRows(ctx, tx, billFile, func(rows []union.BillRow) (int, error) {
				saved, err := taxstore.SaveBillBatch(ctx, tx, rows, bm, skippedPP, false)
				savedCount += saved.Saved
				unmatched += saved.Unmatched
				return saved.Saved, err
			})
			if err != nil {
				return count, err
			}
			final, err := taxstore.SaveBillBatch(ctx, tx, nil, bm, skippedPP, true)
			deleted = final.Deleted
			return count, err
		}); err != nil {
			return result, err
		}
		log.Printf("union.tax billing saved=%d deleted=%d PP=%d unmatched=%d store=%s verified=%t", savedCount, deleted, skippedPP, unmatched, bm.Artifact.Mode(), bm.Artifact.Verified())
	}
	var cases []union.ForeclosureCase
	fm, err := foreclosure.Read(ctx, func(row union.ForeclosureCase) error { cases = append(cases, row); return nil })
	if err != nil {
		return result, fmt.Errorf("foreclosure: %w", err)
	}
	if current, err := currentFile(ctx, pool, fm); err != nil {
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
	if current, err := currentFile(ctx, pool, pm); err != nil {
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
func currentFile(ctx context.Context, pool *pgxpool.Pool, meta taxsource.Meta) (bool, error) {
	var activated bool
	digest := meta.Artifact.SHA256()
	err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tax_file_imports i WHERE i.county_id=$1 AND i.source_name=$2 AND i.source_url=$4 AND i.active AND i.file_sha256=$3 AND NOT EXISTS(SELECT 1 FROM parcels p WHERE p.county_id=$1 AND p.source_name='union_county_gis' AND p.updated_at>=i.activated_at))`, county.Union, meta.Name, digest[:], meta.URL).Scan(&activated)
	if err != nil || !activated {
		return false, err
	}
	return activated, nil
}

// stageRows holds parsed rows on private temporary disk until Read returns verified metadata.
func stageRows[T any](ctx context.Context, dir string, read func(context.Context, func(T) error) (taxsource.Meta, error)) (*os.File, taxsource.Meta, error) {
	file, err := os.CreateTemp(dir, ".union-tax-rows-*")
	if err != nil {
		return nil, taxsource.Meta{}, err
	}
	encoder := gob.NewEncoder(file)
	meta, err := read(ctx, func(row T) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return encoder.Encode(row)
	})
	if err == nil {
		_, err = file.Seek(0, io.SeekStart)
	}
	if err != nil {
		file.Close()
		os.Remove(file.Name())
		return nil, meta, err
	}
	return file, meta, nil
}

func releaseRows(file *os.File) { file.Close(); os.Remove(file.Name()) }

// saveRows batches typed writes inside one transaction; a failed batch rolls back the file.
func saveRows[T any](ctx context.Context, tx pgx.Tx, file *os.File, save func([]T) (int, error)) (int, error) {
	decoder := gob.NewDecoder(file)
	total := 0
	seen := false
	for {
		rows := make([]T, 0, 512)
		for len(rows) < cap(rows) {
			if err := ctx.Err(); err != nil {
				return total, err
			}
			var row T
			err := decoder.Decode(&row)
			if err == io.EOF {
				break
			}
			if err != nil {
				return total, err
			}
			rows = append(rows, row)
		}
		if len(rows) == 0 {
			if !seen {
				return save(nil)
			}
			return total, nil
		}
		seen = true
		count, err := save(rows)
		if err != nil {
			return total, err
		}
		total += count
	}
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
