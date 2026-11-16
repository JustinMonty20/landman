// Package taxstore persists tax data and the source-file evidence for each import.
package taxstore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JustinMonty20/landman/internal/county"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5"
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
