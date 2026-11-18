// Package taxstore persists tax data and the source-file evidence for each import.
package taxstore

import (
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/JustinMonty20/landman/internal/county"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ValidationResult describes the outcome of parsing a retained source file.
type ValidationResult string

const (
	ImportValid   ValidationResult = "valid"
	ImportInvalid ValidationResult = "invalid"
)

// ImportEvidence links a source-file version to its retained exact bytes.
type ImportEvidence struct {
	CountyID           county.ID
	SourceName         string
	PublicationVersion *string
	SourceURL          string
	FetchedAt          time.Time
	Artifact           taxartifact.Artifact
	Validation         ValidationResult
}

// SaveImportEvidence records the file outcome, including files with no matched parcels.
// The artifact must come from a successful Store.Put before the database transaction.
func SaveImportEvidence(ctx context.Context, tx pgx.Tx, evidence ImportEvidence) (string, error) {
	if !evidence.CountyID.Valid() || evidence.SourceName == "" || evidence.SourceURL == "" || evidence.FetchedAt.IsZero() {
		return "", errors.New("incomplete tax file import evidence")
	}
	if err := evidence.Artifact.Valid(); err != nil {
		return "", err
	}
	if !evidence.Artifact.Verified() {
		return "", errors.New("tax artifact was not verified by the selected store")
	}
	if !strings.HasPrefix(evidence.Artifact.Key(), "tax/"+evidence.SourceName+"/sha256/") {
		return "", errors.New("tax source does not match artifact key")
	}
	mode := evidence.Artifact.Mode()
	if mode != "s3" && mode != "local" {
		return "", errors.New("invalid artifact storage mode")
	}
	if evidence.Validation != ImportValid && evidence.Validation != ImportInvalid {
		return "", errors.New("invalid tax file validation result")
	}
	var id string
	digest := evidence.Artifact.SHA256()
	err := tx.QueryRow(ctx, `
INSERT INTO tax_file_imports (
    county_id, source_name, publication_version, source_url, fetched_at,
    byte_size, file_sha256, artifact_key, storage_mode, validation_result
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
RETURNING id::text`, evidence.CountyID, evidence.SourceName, evidence.PublicationVersion,
		evidence.SourceURL, evidence.FetchedAt, evidence.Artifact.Size(),
		digest[:], evidence.Artifact.Key(), mode, evidence.Validation).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("save tax file import evidence: %w", err)
	}
	return id, nil
}

// publishStagedRows consumes verified-file rows before it deletes omitted rows and activates the import.
// A failed read or write rolls back the transaction and retains the prior active import.
func publishStagedRows[T any, R any](ctx context.Context, pool *pgxpool.Pool, input io.Reader, write func(pgx.Tx, []T) (R, error), finish func(pgx.Tx) (R, error), add func(*R, R)) (result R, err error) {
	if pool == nil || input == nil {
		return result, errors.New("tax publication requires a database and complete row stream")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer func() {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_ = tx.Rollback(rollbackCtx)
		if err != nil {
			var zero R
			result = zero
		}
	}()
	decoder := gob.NewDecoder(input)
	for {
		rows := make([]T, 0, 512)
		for len(rows) < cap(rows) {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			var row T
			decodeErr := decoder.Decode(&row)
			if decodeErr == io.EOF {
				break
			}
			if decodeErr != nil {
				return result, fmt.Errorf("read staged tax rows: %w", decodeErr)
			}
			rows = append(rows, row)
		}
		if len(rows) == 0 {
			break
		}
		batch, err := write(tx, rows)
		if err != nil {
			return result, err
		}
		add(&result, batch)
	}
	last, err := finish(tx)
	if err != nil {
		return result, err
	}
	add(&result, last)
	if err := tx.Commit(ctx); err != nil {
		return result, err
	}
	return result, nil
}

// PublishStagedInTx applies a validated row stream within the ingestion engine's
// transaction, including omission cleanup and import activation.
func PublishStagedInTx[T any, R any](ctx context.Context, tx pgx.Tx, input io.Reader, write func([]T) (R, error), finish func() (R, error), add func(*R, R)) (result R, err error) {
	if input == nil {
		return result, errors.New("tax publication requires a complete row stream")
	}
	decoder := gob.NewDecoder(input)
	for {
		rows := make([]T, 0, 512)
		for len(rows) < cap(rows) {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			var row T
			decodeErr := decoder.Decode(&row)
			if decodeErr == io.EOF {
				break
			}
			if decodeErr != nil {
				return result, fmt.Errorf("read staged tax rows: %w", decodeErr)
			}
			rows = append(rows, row)
		}
		if len(rows) == 0 {
			break
		}
		batch, err := write(rows)
		if err != nil {
			return result, err
		}
		add(&result, batch)
	}
	last, err := finish()
	if err != nil {
		return result, err
	}
	add(&result, last)
	return result, nil
}

// ActivateImport publishes a validated import in the caller's typed-row transaction.
// On any error, the caller must roll back that transaction to retain the prior active import.
func ActivateImport(ctx context.Context, tx pgx.Tx, id string) error {
	var countyID, sourceName string
	err := tx.QueryRow(ctx, `
SELECT county_id, source_name FROM tax_file_imports
WHERE id = $1 AND validation_result = 'valid' FOR UPDATE`, id).Scan(&countyID, &sourceName)
	if err != nil {
		return fmt.Errorf("find validated tax import: %w", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE tax_file_imports SET active = false
WHERE county_id = $1 AND source_name = $2 AND active`, countyID, sourceName); err != nil {
		return fmt.Errorf("retire prior tax import: %w", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE tax_file_imports SET active = true, activated_at = now()
WHERE id = $1`, id); err != nil {
		return fmt.Errorf("activate tax import: %w", err)
	}
	return nil
}
