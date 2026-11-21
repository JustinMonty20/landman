package parcelstore

import (
	"context"
	"fmt"

	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/parcelevidence"
	"github.com/jackc/pgx/v5"
)

const geometryPolicyVersion = "directed-segments-v1"

// RefusedGeometryParts counts staged parts withheld by whole-parcel geometry validation.
func (w *ShapeWriter) RefusedGeometryParts() int { return w.refusedParts }

func (w *ShapeWriter) refuseGeometry(ctx context.Context, pid, artifactID, reason string) error {
	_, err := w.tx.Exec(ctx, `INSERT INTO parcel_geometry_decisions(artifact_id,source_parcel_id,policy_version,outcome,object_ids,prior_valid_shape,prior_shape_at)
 SELECT $1,$2,$3,$4,ARRAY(SELECT raw_key::bigint FROM parcel_import_identities WHERE source_parcel_id=$2 ORDER BY raw_key::bigint),
 CASE WHEN parcel_boundary_usable(p.shape) THEN p.shape END,
 CASE WHEN parcel_boundary_usable(p.shape) THEN coalesce(o.observed_at,p.updated_at) END
 FROM (SELECT 1) seed LEFT JOIN parcels p ON p.county_id='union' AND p.source_name='union_county_gis' AND p.source_parcel_id=$2
 LEFT JOIN parcel_evidence_current c USING(county_id,source_name,source_parcel_id) LEFT JOIN parcel_evidence_observations o ON o.id=c.observation_id
 ON CONFLICT DO NOTHING`, artifactID, pid, geometryPolicyVersion, reason)
	if err != nil {
		return err
	}
	tag, err := w.tx.Exec(ctx, `UPDATE parcels SET shape=NULL,updated_at=now() WHERE county_id='union' AND source_name='union_county_gis' AND source_parcel_id=$1 AND shape IS NOT NULL`, pid)
	if err == nil && tag.RowsAffected() > 0 {
		w.geometryChanged = true
	}
	return err
}

// applyGeometry records exceptional decisions before their serving changes commit.
// A nil boundary with no error means the entire group was refused.
func (w *ShapeWriter) applyGeometry(ctx context.Context, pid string, invalidGeometry bool, count int, publication evidencePublication) ([]byte, error) {
	if invalidGeometry {
		return nil, w.refuseGeometry(ctx, pid, publication.artifactID, "invalid_geometry")
	}
	var shape []byte
	var repaired []int64
	if err := w.tx.QueryRow(ctx, repairParcelGeometrySQL, pid).Scan(&shape, &repaired); err != nil {
		return nil, fmt.Errorf("check parcel geometry: %w", err)
	}
	if len(shape) == 0 {
		if !publication.replay {
			var keys []string
			if err := w.tx.QueryRow(ctx, `SELECT array_agg(raw_key ORDER BY ordinal) FROM parcel_import_parts WHERE source_parcel_id=$1`, pid).Scan(&keys); err != nil {
				return nil, err
			}
			for _, key := range keys {
				if err := publication.quarantine.Quarantine(ctx, jobs.QuarantineRecord{JobID: publication.jobID, CountyID: "union", SourceName: "union_county_gis", RawRecordKey: key, SourceParcelID: pid, Reason: "parcel geometry does not meet zero-movement rule", PayloadReference: "capture:" + publication.artifactID + "#OBJECTID=" + key}); err != nil {
					return nil, err
				}
			}
		}
		w.refusedParts += count
		return nil, w.refuseGeometry(ctx, pid, publication.artifactID, "zero_movement_refused")
	}
	if len(repaired) > 0 {
		var prior []byte
		err := w.tx.QueryRow(ctx, `SELECT CASE WHEN parcel_boundary_usable(shape) THEN CASE WHEN ST_Equals(shape,ST_GeomFromEWKB($2)) THEN ST_AsEWKB(shape) END END FROM parcels WHERE county_id='union' AND source_name='union_county_gis' AND source_parcel_id=$1`, pid, shape).Scan(&prior)
		if err != nil && err != pgx.ErrNoRows {
			return nil, err
		}
		if prior != nil {
			shape = prior
		}
		if _, err := w.tx.Exec(ctx, `INSERT INTO parcel_geometry_decisions(artifact_id,source_parcel_id,policy_version,outcome,object_ids,geometry_hash)
 VALUES($1,$2,$3,'repaired',ARRAY(SELECT raw_key::bigint FROM parcel_import_parts WHERE ordinal=ANY($4) ORDER BY ordinal),$5) ON CONFLICT DO NOTHING`, publication.artifactID, pid, geometryPolicyVersion, repaired, parcelevidence.GeometryHash(shape)); err != nil {
			return nil, err
		}
	}
	return shape, nil
}
