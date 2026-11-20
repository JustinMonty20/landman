package parcelstore

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/models"
	"github.com/twpayne/go-geom/encoding/ewkb"
)

func (w *ShapeWriter) ObserveIdentity(ctx context.Context, rawKey, pid string) error {
	if rawKey == "" || pid == "" {
		return ErrMissingNaturalKey
	}
	tag, err := w.tx.Exec(ctx, `INSERT INTO parcel_import_identities(raw_key,source_parcel_id) VALUES($1,$2)
ON CONFLICT (raw_key) DO UPDATE SET source_parcel_id=EXCLUDED.source_parcel_id
WHERE parcel_import_identities.source_parcel_id=EXCLUDED.source_parcel_id`, rawKey, pid)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("parcelstore: conflicting PID for OBJECTID %s", rawKey)
	}
	return nil
}

// StagePart defers every parcel and sale mutation until whole-PID eligibility is known.
// Payload is normalized data. Geometry is encoded separately, never as a geom.T interface.
func (w *ShapeWriter) StagePart(ctx context.Context, parcel models.GISParcel, vacant bool, rawKey string) error {
	return w.stagePart(ctx, parcel, vacant, rawKey, nil, nil)
}

func (w *ShapeWriter) stagePart(ctx context.Context, parcel models.GISParcel, vacant bool, rawKey string, feature json.RawMessage, hash *string) error {
	if err := identityKey(parcel.Identity); err != nil {
		return err
	}
	shape, _, err := encodeShape(parcel.Geometry)
	if err != nil {
		return err
	}
	if len(shape) == 0 || rawKey == "" {
		return fmt.Errorf("parcelstore: stage requires shape and raw key")
	}
	parcel.Geometry.Shape = nil
	payload, err := json.Marshal(parcel)
	if err != nil {
		return err
	}
	_, err = w.tx.Exec(ctx, `INSERT INTO parcel_import_parts(source_parcel_id,payload,shape,likely_vacant,raw_key,source_feature,source_hash) VALUES($1,$2,$3,$4,$5,$6,$7)`, parcel.Identity.SourceParcelID, payload, shape, vacant, rawKey, feature, hash)
	return err
}

// RejectPID excludes all parts of this key, regardless of their arrival order.
func (w *ShapeWriter) RejectPID(ctx context.Context, pid string) error {
	if pid == "" {
		return ErrMissingNaturalKey
	}
	_, err := w.tx.Exec(ctx, `INSERT INTO parcel_import_rejected VALUES($1) ON CONFLICT DO NOTHING`, pid)
	return err
}

type stagedPart struct {
	ordinal        int64
	payload, shape []byte
	vacant         bool
	rawKey         string
}

// Publish applies only accepted PIDs in source order through SavePart, then dissolves and commits.
// Evidence resolution shares that commit, so crashes cannot resolve an unpublished parcel.
func (w *ShapeWriter) Publish(ctx context.Context, quarantine *jobs.QuarantineStore, jobID string, selected []jobs.PendingRecord) (int, error) {
	if _, err := w.tx.Exec(ctx, `ANALYZE parcel_import_parts, parcel_import_rejected`); err != nil {
		return 0, err
	}
	var ordinal int64
	var processed int
	for {
		rows, err := w.tx.Query(ctx, `SELECT ordinal,payload,shape,likely_vacant,raw_key FROM parcel_import_parts p
   WHERE ordinal>$1 AND NOT EXISTS(SELECT 1 FROM parcel_import_rejected r WHERE r.source_parcel_id=p.source_parcel_id)
   ORDER BY ordinal LIMIT 2000`, ordinal)
		if err != nil {
			return 0, err
		}
		batch := make([]stagedPart, 0, 2000)
		for rows.Next() {
			var part stagedPart
			if err := rows.Scan(&part.ordinal, &part.payload, &part.shape, &part.vacant, &part.rawKey); err != nil {
				rows.Close()
				return 0, err
			}
			batch = append(batch, part)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return 0, err
		}
		if len(batch) == 0 {
			break
		}
		keys := make([]string, 0, len(batch))
		// pgx cannot write on this transaction while its SELECT rows are still open.
		for _, part := range batch {
			var parcel models.GISParcel
			if err := json.Unmarshal(part.payload, &parcel); err != nil {
				return 0, err
			}
			shape, err := ewkb.Unmarshal(part.shape)
			if err != nil {
				return 0, err
			}
			parcel.Geometry.Shape = &shape
			if err := w.SavePart(ctx, parcel, part.vacant); err != nil {
				return 0, err
			}
			ordinal = part.ordinal
			processed++
			keys = append(keys, part.rawKey)
		}
		if err := quarantine.ResolveObserved(ctx, w.tx, jobID, keys, selected); err != nil {
			return 0, err
		}
	}
	if err := w.Commit(ctx); err != nil {
		return 0, err
	}
	return processed, nil
}
