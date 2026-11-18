package soil

import (
	"context"
	"github.com/JustinMonty20/landman/internal/county"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NeedsScreenStored checks persisted dependencies, not the last parcel job's
// transient changed count. Failed, expired, missing and stale work stays due.
func NeedsScreenStored(ctx context.Context, pool *pgxpool.Pool, countyID county.ID, source string) (bool, error) {
	var due bool
	err := pool.QueryRow(ctx, `SELECT NOT EXISTS (
 SELECT 1 FROM soil_screening_heads h JOIN soil_screening_runs r ON r.id=h.run_id
 JOIN soil_polygon_set_heads sh ON sh.county_id=h.county_id AND sh.source_name=h.source_name

 WHERE h.county_id=$1 AND h.source_name=$2
 AND h.refresh_failed_at IS NULL
 AND NOT EXISTS(SELECT 1 FROM soil_screening_refresh_status f WHERE f.county_id=$1 AND f.source_name=$2)
 AND sh.set_id=r.soil_set_id
 AND r.method=$3 AND r.assessment_method=$4 AND r.fresh_until>now()
 AND NOT EXISTS (SELECT 1 FROM parcels p LEFT JOIN soil_parcel_assessments a ON a.parcel_id=p.id AND a.run_id=r.id
 WHERE p.county_id=$1 AND p.source_name=$2
 AND (a.parcel_id IS NULL OR a.geometry_fingerprint IS DISTINCT FROM COALESCE(md5(ST_AsEWKB(p.shape)),'missing')))
 )`, countyID.String(), source, screeningMethod, assessmentMethod).Scan(&due)
	return due, err
}
