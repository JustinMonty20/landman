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

const NormalizedHashVersion = "parcel-storage-v4"
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
	return roundDecimalText(strconv.FormatFloat(value, 'g', -1, 64), scale)
}

func roundDecimalText(text string, scale int) (float64, error) {
	exact, err := exactDecimalText(text, scale)
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(exact, 64)
}

func exactDecimalText(text string, scale int) (string, error) {
	mantissa, exponentText, scientific := strings.Cut(strings.ToLower(text), "e")
	if !scientific {
		mantissa = text
	}
	r, ok := new(big.Rat).SetString(mantissa)
	if !ok {
		return "", fmt.Errorf("invalid numeric %s", text)
	}
	if scientific {
		exponent, err := strconv.Atoi(exponentText)
		if err != nil {
			return "", fmt.Errorf("invalid numeric %s", text)
		}
		if exponent < -(len(mantissa) + scale + 1) {
			return new(big.Rat).FloatString(scale), nil
		}
		if exponent > 10000 || exponent < -10000 {
			return "", fmt.Errorf("numeric exponent exceeds supported precision")
		}
		magnitude := exponent
		if magnitude < 0 {
			magnitude = -magnitude
		}
		power := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(magnitude)), nil))
		if exponent >= 0 {
			r.Mul(r, power)
		} else {
			r.Quo(r, power)
		}
	}
	multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
	r.Mul(r, new(big.Rat).SetInt(multiplier))
	quotient, rem := new(big.Int).QuoRem(r.Num(), r.Denom(), new(big.Int))
	if new(big.Int).Mul(new(big.Int).Abs(rem), big.NewInt(2)).Cmp(r.Denom()) >= 0 {
		quotient.Add(quotient, big.NewInt(int64(r.Sign())))
	}
	return new(big.Rat).SetFrac(quotient, multiplier).FloatString(scale), nil
}

// QuantizeSourceNumbers rounds source decimals before binary float conversion.
// Original source values remain unchanged in retained evidence.
func QuantizeSourceNumbers(attributes map[string]interface{}, p *models.GISParcel) error {
	fields := []struct {
		key   string
		value **float64
		scale int
	}{
		{"mapped_acres", &p.Land.MappedAcres, 6}, {"gross_acres", &p.Land.GrossAcres, 6},
		{"VAL_AC", &p.Land.ValuedAcres, 6}, {"VAL_SQFT", &p.Land.ValuedSqFt, 2},
		{"SQFT", &p.Building.SqFt, 2}, {"FMV_LAND", &p.Valuation.Land, 2},
		{"FMV_IMPRV", &p.Valuation.Improvement, 2}, {"FMV_TOTAL", &p.Valuation.Total, 2},
		{"TOTVAL", &p.Valuation.Assessed, 2}, {"SHAPE.STArea()", &p.Geometry.SourceArea, 6},
		{"SHAPE.STLength()", &p.Geometry.SourceLength, 6},
	}
	for i, prefix := range []string{"s1", "s2", "s3"} {
		for j := range p.Sales {
			if p.Sales[j].Sequence == i+1 {
				fields = append(fields, struct {
					key   string
					value **float64
					scale int
				}{prefix + "_SALESAMT", &p.Sales[j].Amount, 2})
			}
		}
	}
	for _, field := range fields {
		if *field.value == nil {
			continue
		}
		var text string
		switch v := attributes[field.key].(type) {
		case json.Number:
			text = string(v)
		case float64:
			text = strconv.FormatFloat(v, 'g', -1, 64)
		case string:
			text = strings.TrimSpace(v)
		default:
			continue
		}
		exact, err := exactDecimalText(text, field.scale)
		if err != nil {
			return err
		}
		value, err := strconv.ParseFloat(exact, 64)
		if err != nil {
			return err
		}
		*field.value = &value
		if p.StorageNumbers == nil {
			p.StorageNumbers = make(map[string]string)
		}
		p.StorageNumbers[field.key] = exact
	}
	return nil
}

// NormalizedHash excludes generated IDs and observation metadata. The caller
// supplies the complete persisted geometry and the final sale slots.
func NormalizedHash(p models.GISParcel, geometryHash string) (string, error) {
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
		Geometry string           `json:"geometry_hash"`
	}{p, geometryHash})
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
