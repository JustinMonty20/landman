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
	ZoningMethod = "full-overlay-exclusive/1; EPSG:2264; acres=ftUS^2/43560"
	RoadsMethod  = "full-shape-nearest/1; EPSG:2264; radius=5280ft; ties<=0.01ft"
	maxSourceAge = 90 * 24 * time.Hour
)

// Store owns retained qualification generations and typed parcel evidence.
type Store struct{ Pool *pgxpool.Pool }

func (s Store) Refresh(ctx context.Context, source *union.GISSource, group string) (processed int, err error) {
	if group != union.QualificationZoning && group != union.QualificationRoads {
		return 0, fmt.Errorf("invalid qualification group %s", group)
	}
	var gen string
	if err = s.Pool.QueryRow(ctx, `INSERT INTO qualification_generations(source_group) VALUES($1) RETURNING id::text`, group).Scan(&gen); err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = s.Pool.Exec(c, `INSERT INTO qualification_refresh_failures(source_group,error) VALUES($1,$2) ON CONFLICT(source_group) DO UPDATE SET failed_at=now(),error=excluded.error`, group, err.Error())
		}
	}()
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
	if _, e = tx.Exec(ctx, `DELETE FROM qualification_refresh_failures WHERE source_group=$1`, group); e != nil {
		return processed, e
	}
	if e = tx.Commit(ctx); e != nil {
		return processed, e
	}
	return processed, nil
}

func (s Store) insertFeature(ctx context.Context, gen string, feature union.QualificationFeature) error {
	var err error
	switch f := feature.(type) {
	case union.ZoneFeature:
		_, err = s.Pool.Exec(ctx, `INSERT INTO qualification_zones(generation_id,layer_id,source_id,source_srid,shape,jurisdiction,district_code,district_description,case_reference,official_url,source_date) VALUES($1,$2,$3,$4,ST_Multi(ST_GeomFromEWKB($5)), $6,NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),NULLIF($10,''),NULLIF($11,''))`, gen, f.LayerID, f.SourceID, f.SourceSRID, f.Shape, f.Jurisdiction, f.DistrictCode, f.Description, f.CaseReference, f.OfficialURL, f.SourceDate)
	case union.JurisdictionFeature:
		_, err = s.Pool.Exec(ctx, `INSERT INTO qualification_jurisdictions(generation_id,layer_id,source_id,source_srid,shape,name,source_date) VALUES($1,$2,$3,$4,ST_Multi(ST_GeomFromEWKB($5)),$6,NULLIF($7,''))`, gen, f.LayerID, f.SourceID, f.SourceSRID, f.Shape, f.Name, f.SourceDate)
	case union.BoundaryFeature:
		_, err = s.Pool.Exec(ctx, `INSERT INTO qualification_boundaries(generation_id,layer_id,source_id,source_srid,shape) VALUES($1,$2,$3,$4,ST_Multi(ST_GeomFromEWKB($5)))`, gen, f.LayerID, f.SourceID, f.SourceSRID, f.Shape)
	case union.RoadFeature:
		_, err = s.Pool.Exec(ctx, `INSERT INTO qualification_roads(generation_id,layer_id,source_id,source_srid,shape,name,road_class,status,source_date) VALUES($1,$2,$3,$4,ST_Multi(ST_GeomFromEWKB($5)),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),$9)`, gen, f.LayerID, f.SourceID, f.SourceSRID, f.Shape, f.Name, f.Class, f.Status, f.SourceDate)
	default:
		return errors.New("unsupported qualification feature")
	}
	return err
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
	if _, err = tx.Exec(ctx, `CREATE TEMP TABLE qualification_targets ON COMMIT DROP AS SELECT id,shape,COALESCE(md5(ST_AsEWKB(shape)),'missing') fingerprint,CASE WHEN shape IS NOT NULL AND ST_IsValid(shape) THEN ST_CollectionExtract(ST_Transform(shape,2264),3) END geom FROM parcels WHERE id=ANY($1::uuid[])`, ids); err != nil {
		return 0, err
	}
	for _, sql := range []string{`CREATE UNIQUE INDEX ON qualification_targets(id)`, `CREATE INDEX ON qualification_targets USING GIST(shape)`, `ANALYZE qualification_targets`} {
		if _, err = tx.Exec(ctx, sql); err != nil {
			return 0, err
		}
	}
	if group == union.QualificationZoning {
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
