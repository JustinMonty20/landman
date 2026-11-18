package soiljob

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"

	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/soil"
	"github.com/twpayne/go-geom/encoding/ewkb"
)

func digest(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func geometryDigest(polygons []soil.Polygon) (string, error) {
	h := sha256.New()
	for _, p := range polygons {
		if err := writeGeometry(h, p); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeGeometry(h hash.Hash, p soil.Polygon) error {
	shape, err := ewkb.Marshal(*p.Shape, ewkb.NDR)
	if err != nil {
		return err
	}
	metadata, err := json.Marshal([]any{p.County, p.SourceName, p.ObjectID, p.MapUnitKey, p.SpatialVersion, p.SurveySavedOn, len(shape)})
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
