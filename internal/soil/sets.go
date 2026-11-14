package soil

import (
	"context"
	"fmt"

	"github.com/JustinMonty20/landman/internal/county"
	"github.com/jackc/pgx/v5"
)

func validateMembership(countyID county.ID, source string, polygons []Polygon, units []MapUnit) error {
	keys := make(map[string]bool, len(units))
	for _, unit := range units {
		if unit.MapUnitKey == "" {
			return fmt.Errorf("soil: empty map unit key")
		}
		keys[unit.MapUnitKey] = true
	}
	ids := make(map[string]bool, len(polygons))
	for _, p := range polygons {
		if p.County != countyID || p.SourceName != source {
			return fmt.Errorf("soil: polygon source differs from import")
		}
		if p.ObjectID == "" || ids[p.ObjectID] {
			return fmt.Errorf("soil: empty or duplicate polygon ID %q", p.ObjectID)
		}
		if !keys[p.MapUnitKey] {
			return fmt.Errorf("soil: polygon %s has no map unit metadata", p.ObjectID)
		}
		if p.Shape == nil {
			return fmt.Errorf("soil: polygon %s has no shape", p.ObjectID)
		}
		ids[p.ObjectID] = true
	}
	return nil
}

// Snapshot membership is independent of the mutable geometry and legacy overlap tables.
func recordPolygonSet(ctx context.Context, tx pgx.Tx, countyID county.ID, source string, polygons []Polygon) (string, error) {
	var setID string
	if err := tx.QueryRow(ctx, `INSERT INTO soil_polygon_sets(county_id,source_name,fingerprint) VALUES ($1,$2,'') RETURNING id`, countyID.String(), source).Scan(&setID); err != nil {
		return "", err
	}
	for _, p := range polygons {
		tag, err := tx.Exec(ctx, `INSERT INTO soil_polygon_set_members(set_id,polygon_id,map_unit_key,shape,spatial_version,survey_saved_on)
SELECT $1,id,map_unit_key,shape,spatial_version,survey_saved_on FROM soil_polygons WHERE county_id=$2 AND source_name=$3 AND object_id=$4`, setID, countyID.String(), source, p.ObjectID)
		if err != nil {
			return "", err
		}
		if tag.RowsAffected() != 1 {
			return "", fmt.Errorf("soil: missing polygon %s", p.ObjectID)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE soil_polygon_sets SET fingerprint=(SELECT md5(COALESCE(string_agg(polygon_id::text || ':' || map_unit_key || ':' || md5(ST_AsEWKB(shape)) || ':' || COALESCE(spatial_version::text,'unknown') || ':' || COALESCE(survey_saved_on::text,'unknown'), ',' ORDER BY polygon_id),'')) FROM soil_polygon_set_members WHERE set_id=$1) WHERE id=$1`, setID); err != nil {
		return "", err
	}
	_, err := tx.Exec(ctx, `INSERT INTO soil_polygon_set_heads(county_id,source_name,set_id) VALUES ($1,$2,$3) ON CONFLICT(county_id,source_name) DO UPDATE SET set_id=excluded.set_id`, countyID.String(), source, setID)
	return setID, err
}
