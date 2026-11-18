package parcelstore

import (
	"context"
	"fmt"

	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/parcelevidence"
	"github.com/jackc/pgx/v5"
)

// RegisterCapture binds immutable verified bytes to their original source attempt.
// This transaction precedes publication so a failed publication retains the binding.
func RegisterCapture(ctx context.Context, dsn string, manifest parcelevidence.Manifest) error {
	attemptID, ok := jobs.AttemptID(ctx)
	if !ok || !digest.MatchString(manifest.ArtifactHash) {
		return fmt.Errorf("capture requires an active source attempt and verified digest")
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	var registered string
	err = conn.QueryRow(ctx, `INSERT INTO parcel_evidence_captures(artifact_id,artifact_hash,manifest,attempt_id,attempt_kind)
SELECT $1,$2,$3,id,attempt_kind FROM job_attempts
WHERE id=$4::uuid AND job_id='union.gis.parcels' AND outcome='running' AND attempt_kind=$5
ON CONFLICT (artifact_id) DO NOTHING
RETURNING artifact_id`, manifest.ID, manifest.ArtifactHash, manifest, attemptID, manifest.Kind).Scan(&registered)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("capture registration requires original running source attempt")
	}
	return err
}
