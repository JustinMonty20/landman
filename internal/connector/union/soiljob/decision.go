package soiljob

import (
	"context"
	"errors"

	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/jackc/pgx/v5"
)

func previousSoilFingerprint(ctx context.Context, tx pgx.Tx, source string) (ingest.Fingerprint, string, error) {
	var previous ingest.Fingerprint
	var setID string
	err := tx.QueryRow(ctx, `SELECT canonical_digest,canonical_version,normalized_digest,normalized_version,geometry_digest,geometry_version,COALESCE(published_set_id::text,'') FROM soil_ingest_publications WHERE source_name=$1 ORDER BY observed_at DESC, capture_id DESC LIMIT 1`, source).Scan(&previous.Source, &previous.SourceVersion, &previous.Normalized, &previous.NormalizedVersion, &previous.Geometry, &previous.GeometryVersion, &setID)
	if errors.Is(err, pgx.ErrNoRows) {
		return previous, "", nil
	}
	return previous, setID, err
}

func soilChangeCause(previous, next ingest.Fingerprint, replay bool) ingest.ChangeCause {
	decision := ingest.Classify(previous, next, replay)
	if decision.CompareProjection {
		if (previous.Normalized != next.Normalized || previous.Geometry != next.Geometry) && previous.Source != "" && previous.SourceVersion == next.SourceVersion && previous.Source == next.Source {
			return ingest.NormalizationCorrection
		}
	}
	if decision.Cause != "" {
		return decision.Cause
	}
	if replay {
		return ingest.OfflineRetry
	}
	return ingest.SourceObservation
}
