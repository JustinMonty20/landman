package qualification

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	ZoningMethod = "stored-overlay-measured/4; EPSG:3857 grid=0.000001m; EPSG:2264 acres=ftUS^2/43560"
	RoadsMethod  = "stored-nearest-measured/3; EPSG:2264; radius=5280ft; ties<=0.01ft"
	maxSourceAge = 90 * 24 * time.Hour
)

// Store owns retained qualification generations and typed parcel evidence.
type Store struct{ Pool *pgxpool.Pool }

// failed_at orders attempts by their original start, not the time the error was written.
// Publication owns only the head; freshness compares this observation with its started_at.
func (s Store) recordRefreshFailure(group string, started time.Time, cause error) error {
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.Pool.Exec(c, `INSERT INTO qualification_refresh_failures(source_group,failed_at,error)
VALUES($1,$2,$3)
ON CONFLICT(source_group) DO UPDATE SET failed_at=excluded.failed_at,error=excluded.error
WHERE qualification_refresh_failures.failed_at<=excluded.failed_at`, group, started, cause.Error())
	if err != nil {
		return fmt.Errorf("record qualification %s refresh failure: %w", group, err)
	}
	return nil
}

// Publication remains serialized per group; failure observations do not take this lock.
func lockQualificationPublication(ctx context.Context, tx pgx.Tx, group string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('qualification-publication'),hashtext($1))`, group)
	return err
}

// RefreshWithGate includes lock acquisition in the source attempt. A failed
// waiter records failure without replacing the previously published generation.
func (s Store) RefreshWithGate(ctx context.Context, source *union.GISSource, group string, gate func(context.Context, func(context.Context) error) error) (processed int, err error) {
	if group != union.QualificationZoning && group != union.QualificationRoads {
		return 0, fmt.Errorf("invalid qualification group %s", group)
	}
	started := time.Now().UTC()
	var refreshErr error
	err = gate(ctx, func(work context.Context) error {
		processed, refreshErr = s.refresh(work, source, group, started)
		return refreshErr
	})
	if err != nil && (refreshErr == nil || !errors.Is(err, refreshErr)) {
		// The host gate can fail after the callback; its attempt still began at started.
		err = errors.Join(err, s.recordRefreshFailure(group, started, err))
	}
	return processed, err
}

func (s Store) Refresh(ctx context.Context, source *union.GISSource, group string) (int, error) {
	if group != union.QualificationZoning && group != union.QualificationRoads {
		return 0, fmt.Errorf("invalid qualification group %s", group)
	}
	return s.refresh(ctx, source, group, time.Now().UTC())
}

func (s Store) refresh(ctx context.Context, source *union.GISSource, group string, started time.Time) (processed int, err error) {
	defer func() {
		if err != nil {
			err = errors.Join(err, s.recordRefreshFailure(group, started, err))
		}
	}()
	var gen string
	if err = s.Pool.QueryRow(ctx, `INSERT INTO qualification_generations(source_group,started_at) VALUES($1,$2) RETURNING id::text`, group, started).Scan(&gen); err != nil {
		return 0, err
	}
	for _, id := range union.QualificationLayers(group) {
		var receipt union.QualificationReceipt
		receipt, err = source.FetchQualificationLayer(ctx, id, func(f union.QualificationFeature) error { return s.insertFeature(ctx, gen, f) })
		if err != nil {
			return processed, fmt.Errorf("qualification layer %d: %w", id, err)
		}
		if id == 221 && receipt.Count == 0 {
			return processed, errors.New("county boundary is empty")
		}
		if _, err = s.Pool.Exec(ctx, `INSERT INTO qualification_layer_receipts(generation_id,layer_id,feature_count,source_srid,service_url,source_metadata) VALUES($1,$2,$3,$4,$5,$6)`, gen, id, receipt.Count, receipt.SourceSRID, receipt.ServiceURL, receipt.Metadata); err != nil {
			return processed, err
		}
		processed += receipt.Count
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return processed, e
	}
	defer tx.Rollback(context.Background())
	if e = lockQualificationPublication(ctx, tx, group); e != nil {
		return processed, e
	}
	var actual int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM qualification_layer_receipts WHERE generation_id=$1`, gen).Scan(&actual); e != nil {
		return processed, e
	}
	if actual != len(union.QualificationLayers(group)) {
		return processed, errors.New("incomplete qualification layer group")
	}
	if _, e = tx.Exec(ctx, `UPDATE qualification_generations SET completed_at=now() WHERE id=$1`, gen); e != nil {
		return processed, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO qualification_heads(source_group,generation_id) VALUES($1,$2) ON CONFLICT(source_group) DO UPDATE SET generation_id=excluded.generation_id`, group, gen); e != nil {
		return processed, e
	}
	if e = tx.Commit(ctx); e != nil {
		return processed, e
	}
	return processed, nil
}

func (s Store) insertFeature(ctx context.Context, gen string, feature union.QualificationFeature) error {
	var err error
	var identity union.SourceIdentity
	switch f := feature.(type) {
	case union.ZoneFeature:
		identity = f.SourceIdentity
		_, err = s.Pool.Exec(ctx, `WITH input AS (SELECT ST_Multi(ST_GeomFromEWKB($5)) g),
repair AS (SELECT g, ST_Multi(ST_CollectionExtract(ST_MakeValid(g),3)) fixed FROM input)
INSERT INTO qualification_zones(generation_id,layer_id,source_id,source_srid,shape,raw_shape,repair_status,repair_delta_m2,jurisdiction,district_code,district_description,case_reference,official_url,source_date)
SELECT $1,$2,$3,$4,
CASE WHEN ST_IsValid(g) THEN g WHEN ST_IsValid(fixed) AND NOT ST_IsEmpty(fixed)
AND abs(ST_Area(fixed)-ST_Area(g)) <= 0.01
AND abs(ST_Area(fixed)-ST_Area(g)) <= 1e-8 * greatest(ST_Area(g),ST_Area(fixed))
THEN fixed END,g,CASE WHEN ST_IsValid(g) THEN 'original' ELSE 'repaired' END,
CASE WHEN ST_IsValid(g) THEN 0 ELSE abs(ST_Area(fixed)-ST_Area(g)) END,$6,NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),NULLIF($11,'') FROM repair`, gen, f.LayerID, f.SourceID, f.SourceSRID, f.Shape, f.Jurisdiction, f.DistrictCode, f.Description, f.CaseReference, f.OfficialURL, f.SourceDate)
	case union.JurisdictionFeature:
		identity = f.SourceIdentity
		_, err = s.Pool.Exec(ctx, `WITH input AS (SELECT ST_Multi(ST_GeomFromEWKB($5)) g),
repair AS (SELECT g, ST_Multi(ST_CollectionExtract(ST_MakeValid(g),3)) fixed FROM input)
INSERT INTO qualification_jurisdictions(generation_id,layer_id,source_id,source_srid,shape,raw_shape,repair_status,repair_delta_m2,name,source_date)
SELECT $1,$2,$3,$4,
CASE WHEN ST_IsValid(g) THEN g WHEN ST_IsValid(fixed) AND NOT ST_IsEmpty(fixed)
AND abs(ST_Area(fixed)-ST_Area(g)) <= 0.01
AND abs(ST_Area(fixed)-ST_Area(g)) <= 1e-8 * greatest(ST_Area(g),ST_Area(fixed))
THEN fixed END,g,CASE WHEN ST_IsValid(g) THEN 'original' ELSE 'repaired' END,
CASE WHEN ST_IsValid(g) THEN 0 ELSE abs(ST_Area(fixed)-ST_Area(g)) END,$6,$7 FROM repair`, gen, f.LayerID, f.SourceID, f.SourceSRID, f.Shape, f.Name, f.SourceDate)
	case union.BoundaryFeature:
		identity = f.SourceIdentity
		_, err = s.Pool.Exec(ctx, `WITH input AS (SELECT ST_Multi(ST_GeomFromEWKB($5)) g),
repair AS (SELECT g, ST_Multi(ST_CollectionExtract(ST_MakeValid(g),3)) fixed FROM input)
INSERT INTO qualification_boundaries(generation_id,layer_id,source_id,source_srid,shape,raw_shape,repair_status,repair_delta_m2)
SELECT $1,$2,$3,$4,
CASE WHEN ST_IsValid(g) THEN g WHEN ST_IsValid(fixed) AND NOT ST_IsEmpty(fixed)
AND abs(ST_Area(fixed)-ST_Area(g)) <= 0.01
AND abs(ST_Area(fixed)-ST_Area(g)) <= 1e-8 * greatest(ST_Area(g),ST_Area(fixed))
THEN fixed END,g,CASE WHEN ST_IsValid(g) THEN 'original' ELSE 'repaired' END,
CASE WHEN ST_IsValid(g) THEN 0 ELSE abs(ST_Area(fixed)-ST_Area(g)) END FROM repair`, gen, f.LayerID, f.SourceID, f.SourceSRID, f.Shape)
	case union.RoadFeature:
		identity = f.SourceIdentity
		_, err = s.Pool.Exec(ctx, `INSERT INTO qualification_roads(generation_id,layer_id,source_id,source_srid,shape,name,road_class,status,source_date) VALUES($1,$2,$3,$4,ST_Multi(ST_GeomFromEWKB($5)),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),$9)`, gen, f.LayerID, f.SourceID, f.SourceSRID, f.Shape, f.Name, f.Class, f.Status, f.SourceDate)
	default:
		return errors.New("unsupported qualification feature")
	}
	if err != nil {
		return fmt.Errorf("qualification feature layer %d OID %d: %w", identity.LayerID, identity.SourceID, err)
	}
	return nil
}

// Reconcile derives only stale parcel groups. A source or parcel change after a
// successful import remains detectable until this job commits the new versions.
func (s Store) Reconcile(ctx context.Context) (int, error) {
	groups := []struct{ group, method string }{{union.QualificationZoning, ZoningMethod}, {union.QualificationRoads, RoadsMethod}}
	total := 0
	var failures []error
	for _, g := range groups {
		if err := ctx.Err(); err != nil {
			return total, errors.Join(append(failures, err)...)
		}
		count, err := s.reconcileOne(ctx, g.group, g.method)
		total += count
		if err != nil {
			failures = append(failures, fmt.Errorf("reconcile %s: %w", g.group, err))
		}
	}
	return total, errors.Join(failures...)
}

func (s Store) reconcileOne(ctx context.Context, group, method string) (int, error) {
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(context.Background())
	var gen string
	err = tx.QueryRow(ctx, `SELECT generation_id::text FROM qualification_heads WHERE source_group=$1 FOR SHARE`, group).Scan(&gen)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	count, err := s.reconcileGroup(ctx, tx, group, method, gen)
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return count, nil
}

func (s Store) reconcileGroup(ctx context.Context, tx pgx.Tx, group, method, gen string) (int, error) {
	rows, err := tx.Query(ctx, `SELECT p.id FROM parcels p LEFT JOIN qualification_assessments a ON a.parcel_id=p.id AND a.source_group=$1 WHERE p.county_id='union' AND (a.parcel_id IS NULL OR a.generation_id<>$2 OR a.method_version<>$3 OR a.geometry_fingerprint IS DISTINCT FROM COALESCE(md5(ST_AsEWKB(p.shape)),'missing') OR ($1='roads' AND a.boundary_generation_id IS DISTINCT FROM (SELECT generation_id FROM qualification_heads WHERE source_group='zoning'))) FOR SHARE OF p`, group, gen, method)
	if err != nil {
		return 0, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	if _, err = tx.Exec(ctx, `CREATE TEMP TABLE qualification_targets ON COMMIT DROP AS SELECT id,shape,COALESCE(md5(ST_AsEWKB(shape)),'missing') fingerprint,CASE WHEN shape IS NOT NULL AND ST_IsValid(shape) THEN ST_CollectionExtract(ST_Transform(ST_Segmentize(ST_Simplify(shape,0),10),2264),3) END geom,CASE WHEN $2='zoning' AND shape IS NOT NULL AND ST_IsValid(shape) THEN ST_ReducePrecision(shape,0.000001) END analysis_geom FROM parcels WHERE id=ANY($1::uuid[])`, ids, group); err != nil {
		return 0, err
	}
	for _, sql := range []string{`CREATE UNIQUE INDEX ON qualification_targets(id)`, `CREATE INDEX ON qualification_targets USING GIST(shape)`, `ANALYZE qualification_targets`} {
		if _, err = tx.Exec(ctx, sql); err != nil {
			return 0, err
		}
	}
	if group == union.QualificationZoning {
		if _, err = tx.Exec(ctx, `DO $$ BEGIN IF EXISTS (SELECT 1 FROM qualification_targets WHERE shape IS NOT NULL AND ST_Area(shape)>0 AND ST_Area(analysis_geom)=0) THEN RAISE EXCEPTION 'analysis precision collapsed a required parcel'; END IF; END $$`); err != nil {
			return 0, err
		}
		err = s.reconcileZoning(ctx, tx, gen, method)
	} else {
		err = s.reconcileRoads(ctx, tx, gen, method)
	}
	if err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `DROP TABLE qualification_targets`); err != nil {
		return 0, err
	}
	return len(ids), nil
}
