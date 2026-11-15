package soil

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/JustinMonty20/landman/internal/county"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twpayne/go-geom/encoding/ewkb"
)

// ErrNoStoredSet distinguishes a missing prerequisite from an ingested empty set.
var ErrNoStoredSet = errors.New("soil screening pending: no stored soil set; run soil ingestion")

// ScreenStored refreshes overlaps and USDA screening without fetching county soil
// or changing soil fetch timestamps. Replaced set heads cannot be published.
func ScreenStored(ctx context.Context, dsn string, countyID county.ID, source string, interval time.Duration) (int, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return 0, err
	}
	defer pool.Close()
	return screenStored(ctx, pool, http.DefaultClient, soilDataAccessURL, countyID, source, interval)
}

func screenStored(ctx context.Context, pool *pgxpool.Pool, client *http.Client, endpoint string, countyID county.ID, source string, interval time.Duration) (int, error) {
	setID, polygons, units, err := storedSet(ctx, pool, countyID, source)
	if err != nil {
		return 0, err
	}
	err = screen(ctx, pool, client, endpoint, countyID, source, polygons, units, ScreeningOptions{SoilSetID: setID, FreshnessInterval: interval})
	return len(polygons), err
}

func storedSet(ctx context.Context, pool *pgxpool.Pool, countyID county.ID, source string) (string, []Polygon, []MapUnit, error) {
	var setID string
	if err := pool.QueryRow(ctx, `SELECT set_id::text FROM soil_polygon_set_heads WHERE county_id=$1 AND source_name=$2`, countyID.String(), source).Scan(&setID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil, nil, ErrNoStoredSet
		}
		return "", nil, nil, err
	}
	// The set is immutable. A later head switch is checked again in the publication transaction.
	rows, err := pool.Query(ctx, `SELECT p.object_id,m.map_unit_key,ST_AsEWKB(m.shape),m.spatial_version,m.survey_saved_on,u.map_unit_symbol,u.area_symbol
FROM soil_polygon_set_members m JOIN soil_polygons p ON p.id=m.polygon_id
JOIN soil_map_units u ON u.map_unit_key=m.map_unit_key
WHERE m.set_id=$1 ORDER BY m.polygon_id`, setID)
	if err != nil {
		return "", nil, nil, err
	}
	defer rows.Close()
	polygons := []Polygon{}
	units := []MapUnit{}
	keys := map[string]bool{}
	for rows.Next() {
		p := Polygon{County: countyID, SourceName: source}
		var unit MapUnit
		var raw []byte
		if err := rows.Scan(&p.ObjectID, &p.MapUnitKey, &raw, &p.SpatialVersion, &p.SurveySavedOn, &unit.MapUnitSymbol, &unit.AreaSymbol); err != nil {
			return "", nil, nil, err
		}
		shape, err := ewkb.Unmarshal(raw)
		if err != nil {
			return "", nil, nil, fmt.Errorf("decode stored soil polygon: %w", err)
		}
		p.Shape = &shape
		polygons = append(polygons, p)
		if !keys[p.MapUnitKey] {
			unit.MapUnitKey = p.MapUnitKey
			units = append(units, unit)
			keys[p.MapUnitKey] = true
		}
	}
	return setID, polygons, units, rows.Err()
}
