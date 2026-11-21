package union

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/models"
)

// ParcelIdentity is the source identity, including the existing OBJECTID fallback.
type ParcelIdentity struct {
	PID      string
	ObjectID int64
}

// ParcelRecordIdentity rejects ambiguous identity rather than guessing a parcel to exclude.
func ParcelRecordIdentity(raw connector.RawRecord) (ParcelIdentity, error) {
	attrs, ok := asMap(raw.RawData["attributes"])
	if !ok {
		return ParcelIdentity{}, fmt.Errorf("union parcel: missing attributes")
	}
	oid, ok := floatValue(attrs["OBJECTID"])
	if !ok || math.IsNaN(oid) || math.IsInf(oid, 0) || oid <= 0 || oid > 9007199254740991 || math.Trunc(oid) != oid {
		return ParcelIdentity{}, fmt.Errorf("union parcel: invalid OBJECTID")
	}
	id := ParcelIdentity{ObjectID: int64(oid)}
	if value := attrs["PID"]; value != nil {
		pid, ok := value.(string)
		if !ok || (pid != "" && strings.TrimSpace(pid) == "") {
			return ParcelIdentity{}, fmt.Errorf("union parcel: invalid PID identity")
		}
		id.PID = pid
	}
	if id.PID == "" {
		id.PID = strconv.FormatInt(id.ObjectID, 10)
	}
	if raw.ParcelID != "" && raw.ParcelID != id.PID {
		return ParcelIdentity{}, fmt.Errorf("union parcel: conflicting parcel identity")
	}
	return id, nil
}

// InvalidParcelRecord describes only invalid individual source data, never infrastructure errors.
const InvalidParcelGeometry = "invalid geometry"

type InvalidParcelRecord struct {
	Identity ParcelIdentity
	Reason   string
}

func (e *InvalidParcelRecord) Error() string { return "union parcel: " + e.Reason }

func NormalizeParcel(raw connector.RawRecord) (models.GISParcel, bool, error) {
	id, err := ParcelRecordIdentity(raw)
	if err != nil {
		return models.GISParcel{}, false, err
	}
	raw.ParcelID = id.PID
	invalid := func(reason string) (models.GISParcel, bool, error) {
		return models.GISParcel{}, false, &InvalidParcelRecord{Identity: id, Reason: reason}
	}
	attrs := attributes(raw.RawData)
	for _, field := range []struct {
		name                string
		min, upperExclusive float64
		integer             bool
	}{
		{"parcel_year", 1900, 2201, true}, {"YEARBLT", 1600, 2201, true},
		{"mapped_acres", 0, 1e8 - 0.0000005, false}, {"gross_acres", 0, 1e8 - 0.0000005, false}, {"VAL_AC", 0, 1e8 - 0.0000005, false},
		{"VAL_SQFT", 0, 1e14, false}, {"SQFT", 0, 1e12 - 0.005, false},
		{"FMV_LAND", 0, 1e12 - 0.005, false}, {"FMV_IMPRV", 0, 1e12 - 0.005, false}, {"FMV_TOTAL", 0, 1e12 - 0.005, false}, {"TOTVAL", 0, 1e12 - 0.005, false},
		{"SHAPE.STArea()", 0, 1e14, false}, {"SHAPE.STLength()", 0, 1e14, false},
		{"s1_SALESAMT", 0, 1e12 - 0.005, false}, {"s2_SALESAMT", 0, 1e12 - 0.005, false}, {"s3_SALESAMT", 0, 1e12 - 0.005, false},
	} {
		value := attrs[field.name]
		if value == nil {
			continue
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			continue
		}
		number, ok := floatValue(value)
		if field.name == "YEARBLT" && number == 0 && ok {
			continue
		}
		if !ok || math.IsNaN(number) || math.IsInf(number, 0) || number < field.min || number >= field.upperExclusive || (field.integer && math.Trunc(number) != number) {
			return invalid("invalid " + field.name)
		}
	}
	parcel, err := NewGISTranslator().Translate(raw)
	if err != nil {
		return models.GISParcel{}, false, err
	}
	if parcel.Geometry.Shape == nil || *parcel.Geometry.Shape == nil || (*parcel.Geometry.Shape).Empty() {
		return invalid(InvalidParcelGeometry)
	}
	for _, n := range (*parcel.Geometry.Shape).FlatCoords() {
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return invalid(InvalidParcelGeometry)
		}
	}
	vacant := IsLikelyVacantGISRecord(raw) && str(attrs, "PID") != ""
	return *parcel, vacant, nil
}
