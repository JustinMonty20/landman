package parcelevidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/JustinMonty20/landman/internal/models"
)

const NormalizedHashVersion = "parcel-storage-v1"
const GeometryHashVersion = "postgis-normalized-ewkb-v1"

func checksum(prefix string, data []byte) string {
	h := sha256.Sum256(append([]byte(prefix+"\n"), data...))
	return hex.EncodeToString(h[:])
}

// GeometryHash uses the complete dissolved PostGIS geometry in normalized EWKB.
func GeometryHash(ewkb []byte) string { return checksum(GeometryHashVersion, ewkb) }

// NormalizeStorageNumbers matches the parcel and sale NUMERIC scales. Source
// JSON is never changed; this projection represents only persisted values.
func NormalizeStorageNumbers(p *models.GISParcel) error {
	fields := []struct {
		value **float64
		scale int
	}{
		{&p.Land.MappedAcres, 6}, {&p.Land.GrossAcres, 6}, {&p.Land.ValuedAcres, 6},
		{&p.Land.ValuedSqFt, 2}, {&p.Building.SqFt, 2},
		{&p.Valuation.Land, 2}, {&p.Valuation.Improvement, 2}, {&p.Valuation.Total, 2}, {&p.Valuation.Assessed, 2},
		{&p.Geometry.SourceArea, 6}, {&p.Geometry.SourceLength, 6},
	}
	for i := range p.Sales {
		fields = append(fields, struct {
			value **float64
			scale int
		}{&p.Sales[i].Amount, 2})
	}
	for _, field := range fields {
		if *field.value == nil {
			continue
		}
		rounded, err := roundDecimal(**field.value, field.scale)
		if err != nil {
			return err
		}
		*field.value = &rounded
	}
	return nil
}

func roundDecimal(value float64, scale int) (float64, error) {
	text := strconv.FormatFloat(value, 'g', -1, 64)
	r, ok := new(big.Rat).SetString(text)
	if !ok {
		return 0, fmt.Errorf("invalid numeric %s", text)
	}
	multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
	r.Mul(r, new(big.Rat).SetInt(multiplier))
	quotient, rem := new(big.Int).QuoRem(r.Num(), r.Denom(), new(big.Int))
	if new(big.Int).Mul(new(big.Int).Abs(rem), big.NewInt(2)).Cmp(r.Denom()) >= 0 {
		quotient.Add(quotient, big.NewInt(int64(r.Sign())))
	}
	text = new(big.Rat).SetFrac(quotient, multiplier).FloatString(scale)
	return strconv.ParseFloat(text, 64)
}

// NormalizedHash excludes generated IDs and observation metadata. The caller
// supplies the complete persisted geometry and the final sale slots.
func NormalizedHash(p models.GISParcel, vacant bool, geometryHash string) (string, error) {
	p.ID = ""
	p.Geometry.Shape = nil
	for i := range p.Sales {
		p.Sales[i].ID = ""
		p.Sales[i].ParcelID = ""
	}
	if err := NormalizeStorageNumbers(&p); err != nil {
		return "", err
	}
	payload, err := json.Marshal(struct {
		Parcel   models.GISParcel `json:"parcel"`
		Vacant   bool             `json:"likely_vacant"`
		Geometry string           `json:"geometry_hash"`
	}{p, vacant, geometryHash})
	if err != nil {
		return "", err
	}
	return checksum(NormalizedHashVersion, payload), nil
}

func SourcePartHash(features []json.RawMessage) (string, error) {
	var hashes []string
	for _, feature := range features {
		h, err := SourceHash(feature)
		if err != nil {
			return "", err
		}
		hashes = append(hashes, h)
	}
	return checksum(SourceHashVersion, []byte(strings.Join(hashes, "\n"))), nil
}
