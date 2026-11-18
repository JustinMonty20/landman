package soiljob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"

	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/soil"
	"github.com/jackc/pgx/v5"
	"github.com/twpayne/go-geom/encoding/ewkb"
)

func digest(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func normalizedDigest(polygons []soil.Polygon, units []soil.MapUnit) string {
	type metadata struct {
		ObjectID       string
		MapUnitKey     string
		SpatialVersion *float64
		SurveySavedOn  string
	}
	values := make([]metadata, 0, len(polygons))
	for _, polygon := range polygons {
		date := ""
		if polygon.SurveySavedOn != nil {
			date = polygon.SurveySavedOn.Format("2006-01-02")
		}
		values = append(values, metadata{polygon.ObjectID, polygon.MapUnitKey, polygon.SpatialVersion, date})
	}
	return digest([]any{values, units})
}

func geometryDigest(ctx context.Context, tx pgx.Tx, polygons []soil.Polygon) (string, error) {
	h := sha256.New()
	for _, p := range polygons {
		shape, err := ewkb.Marshal(*p.Shape, ewkb.NDR)
		if err != nil {
			return "", err
		}
		var canonical []byte
		if err := tx.QueryRow(ctx, `SELECT ST_AsEWKB(ST_Normalize(ST_GeomFromEWKB($1)))`, shape).Scan(&canonical); err != nil {
			return "", err
		}
		if err := writeGeometry(h, p, canonical); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeGeometry(h hash.Hash, p soil.Polygon, shape []byte) error {
	metadata, err := json.Marshal([]any{p.County, p.SourceName, p.ObjectID, len(shape)})
	if err != nil {
		return err
	}
	_, _ = h.Write(metadata)
	_, _ = h.Write(shape)
	return nil
}

func responseDigest(m ingest.Manifest) string {
	type response struct {
		Request, Revision, SHA256 string
		CRS                       int
	}
	receipts := make([]response, 0, len(m.Receipts))
	for _, r := range m.Receipts {
		receipts = append(receipts, response{r.Request, r.Revision, r.SHA256, r.CRS})
	}
	return digest(receipts)
}
