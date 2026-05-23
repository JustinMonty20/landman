package union

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/models"
	"github.com/twpayne/go-geom"
)

const unionGISSourceSRID = 3857

// GISTranslator converts Union County's raw GIS data into normalized GISParcel objects.
// It knows Union County's specific field names: "PID" for parcel ID, "OWNER_NAME", etc.
type GISTranslator struct {
	priority int
}

// NewGISTranslator creates a new Union County GIS translator.
func NewGISTranslator() *GISTranslator {
	return &GISTranslator{priority: 10}
}

// CanTranslate checks if this translator can handle the given source.
func (t *GISTranslator) CanTranslate(sourceName string) bool {
	return sourceName == "union_county_gis"
}

// Priority returns the priority of this translator (higher = preferred).
func (t *GISTranslator) Priority() int {
	return t.priority
}

// Translate converts a raw record from Union County GIS into a normalized GISParcel.
func (t *GISTranslator) Translate(raw connector.RawRecord) (*models.GISParcel, error) {
	if !t.CanTranslate(raw.SourceName) {
		return nil, fmt.Errorf("cannot translate source: %s", raw.SourceName)
	}

	attrs := attributes(raw.RawData)
	parcelID := raw.ParcelID
	if parcelID == "" {
		parcelID = str(attrs, "PID")
	}
	if parcelID == "" {
		return nil, fmt.Errorf("missing parcel ID in raw record")
	}

	dateCRT, ogDateCRT := arcGISDate(attrs, "DATE_CRT")
	shape := geometryShape(raw.RawData)
	sourceSRID := unionGISSourceSRID

	parcel := &models.GISParcel{
		Identity: models.ParcelIdentity{
			CountyID:       "union",
			CountyCode:     "union",
			SourceName:     raw.SourceName,
			SourceParcelID: parcelID,
			ParcelNumber:   stringPtr(parcelID),
			AccountNumber:  strPtr(attrs, "ACCTNO"),
			ParcelYear:     intPtr(attrs, "parcel_year"),
		},
		Owner: models.ParcelOwner{
			CurrentName1: strPtr(attrs, "CURR_NAME1"),
			CurrentName2: strPtr(attrs, "CURR_NAME2"),
			TaxName1:     strPtr(attrs, "JAN1_NAME1"),
			TaxName2:     strPtr(attrs, "JAN1_NAME2"),
			TaxOwnerID1:  intPtr(attrs, "JAN1_OWNERID"),
			TaxOwnerID2:  intPtr(attrs, "JAN1_OWNERID2"),
		},
		Mailing: models.ParcelAddress{
			Line1:      strPtr(attrs, "CURR_ADDR1"),
			Line2:      strPtr(attrs, "CURR_ADDR2"),
			City:       strPtr(attrs, "CURR_CITY"),
			State:      strPtr(attrs, "CURR_STATE"),
			PostalCode: strPtr(attrs, "CURR_ZIPCODE"),
		},
		Situs: models.ParcelAddress{
			Line1: strPtr(attrs, "PHYSSTRADD"),
		},
		Legal: models.ParcelLegal{
			Description: strPtr(attrs, "LEGDESC_1"),
			PlatBook:    strPtr(attrs, "PLAT_BOOK"),
			PlatPage:    strPtr(attrs, "PLAT_PAGE"),
			DeedBook:    strPtr(attrs, "s1_DEED_BOOK"),
			DeedPage:    strPtr(attrs, "s1_DEED_PAGE"),
		},
		Land: models.ParcelLand{
			LandCode:    strPtr(attrs, "LAND_CODE"),
			LandType:    strPtr(attrs, "LAND_TYPE"),
			PropertyUse: strPtr(attrs, "property_use"),
			Subdivision: strPtr(attrs, "subdivision"),
			MappedAcres: floatPtr(attrs, "mapped_acres"),
			GrossAcres:  floatPtr(attrs, "gross_acres"),
			ValuedAcres: floatPtr(attrs, "VAL_AC"),
			ValuedSqFt:  floatPtr(attrs, "VAL_SQFT"),
		},
		Building: models.ParcelBuilding{
			YearBuilt:      intPtr(attrs, "YEARBLT"),
			SqFt:           floatPtr(attrs, "SQFT"),
			Basement:       strPtr(attrs, "BASEMENT"),
			QualityCode:    strPtr(attrs, "BLDQUAL_CODE"),
			Structure:      strPtr(attrs, "STRUCTTYPE"),
			StructureStyle: strPtr(attrs, "STRUCTSTYLE"),
		},
		Valuation: models.ParcelValuation{
			Land:        floatPtr(attrs, "FMV_LAND"),
			Improvement: floatPtr(attrs, "FMV_IMPRV"),
			Total:       floatPtr(attrs, "FMV_TOTAL"),
			Assessed:    floatPtr(attrs, "TOTVAL"),
		},
		Sales: buildSales(attrs),
		Geometry: models.GISGeometry{
			Shape:        shape,
			SRID:         unionGISSourceSRID,
			SourceSRID:   &sourceSRID,
			SourceArea:   floatPtr(attrs, "SHAPE.STArea()"),
			SourceLength: floatPtr(attrs, "SHAPE.STLength()"),
		},
		SourceExtras: models.ParcelSourceExtras{
			Union: &models.UnionParcelExtras{
				OBJECTID:     intPtr(attrs, "OBJECTID"),
				OBJECTID_1:   intPtr(attrs, "OBJECTID_1"),
				SUBCODE:      strPtr(attrs, "SUBCODE"),
				DIST_TWN:     strPtr(attrs, "DIST_TWN"),
				DIST_CODE:    strPtr(attrs, "DIST_CODE"),
				TAX_DISTRI:   strPtr(attrs, "TAX_DISTRI"),
				TWSHP_CD:     strPtr(attrs, "TWSHP_CD"),
				TWSHP_DESC:   strPtr(attrs, "TWSHP_DESC"),
				DESC1_DESC:   strPtr(attrs, "DESC1_DESC"),
				DESC1_CODE:   strPtr(attrs, "DESC1_CODE"),
				LUV_YES_NO:   strPtr(attrs, "LUV_YES_NO"),
				MULSTR:       strPtr(attrs, "MULSTR"),
				MULT_LANDSEQ: strPtr(attrs, "MULT_LANDSEQ"),
				NBHDNAME:     strPtr(attrs, "NBHDNAME"),
				NBHDNUM:      strPtr(attrs, "NBHDNUM"),
				NBHNEW:       strPtr(attrs, "NBHNEW"),
				IDCOL:        intPtr(attrs, "idcol"),
				DATE_CRT:     dateCRT,
				OG_DATE_CRT:  ogDateCRT,
			},
		},
	}

	return parcel, nil
}

func attributes(raw map[string]interface{}) map[string]interface{} {
	if raw == nil {
		return map[string]interface{}{}
	}
	if attrs, ok := asMap(raw["attributes"]); ok {
		return attrs
	}
	return raw
}

func str(attrs map[string]interface{}, key string) string {
	value, ok := attrs[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return strings.TrimSpace(v.String())
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func stringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func strPtr(attrs map[string]interface{}, key string) *string {
	return stringPtr(str(attrs, key))
}

func intPtr(attrs map[string]interface{}, key string) *int64 {
	value, ok := intValue(attrs[key])
	if !ok {
		return nil
	}
	return &value
}

func intValue(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case nil:
		return 0, false
	case int:
		return int64(v), true
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		return int64(v), true
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		if v > uint64(^uint64(0)>>1) {
			return 0, false
		}
		return int64(v), true
	case float32:
		return int64(v), true
	case float64:
		return int64(v), true
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i, true
		}
		f, err := v.Float64()
		return int64(f), err == nil
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return 0, false
		}
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i, true
		}
		f, err := strconv.ParseFloat(v, 64)
		return int64(f), err == nil
	default:
		return 0, false
	}
}

func floatPtr(attrs map[string]interface{}, key string) *float64 {
	value, ok := floatValue(attrs[key])
	if !ok {
		return nil
	}
	return &value
}

func floatValue(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case nil:
		return 0, false
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(v, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func arcGISDate(attrs map[string]interface{}, key string) (*time.Time, *int64) {
	millis, ok := intValue(attrs[key])
	if !ok || millis == 0 {
		return nil, nil
	}
	t := time.UnixMilli(millis).UTC()
	return &t, &millis
}

func geometryShape(raw map[string]interface{}) *geom.T {
	if raw == nil {
		return nil
	}
	geometry, ok := asMap(raw["geometry"])
	if !ok {
		return nil
	}
	rings, ok := rings(geometry["rings"])
	if !ok || len(rings) == 0 {
		return nil
	}

	flatCoords := make([]float64, 0)
	ends := make([]int, 0, len(rings))
	for _, ring := range rings {
		for _, point := range ring {
			flatCoords = append(flatCoords, point[0], point[1])
		}
		ends = append(ends, len(flatCoords))
	}

	poly := geom.NewPolygonFlat(geom.XY, flatCoords, ends).SetSRID(unionGISSourceSRID)
	var g geom.T = poly
	return &g
}

func rings(value interface{}) ([][][2]float64, bool) {
	rawRings, ok := value.([]interface{})
	if !ok {
		return nil, false
	}
	out := make([][][2]float64, 0, len(rawRings))
	for _, rawRing := range rawRings {
		rawPoints, ok := rawRing.([]interface{})
		if !ok || len(rawPoints) == 0 {
			continue
		}
		ring := make([][2]float64, 0, len(rawPoints))
		for _, rawPoint := range rawPoints {
			rawCoords, ok := rawPoint.([]interface{})
			if !ok || len(rawCoords) < 2 {
				continue
			}
			x, okX := floatValue(rawCoords[0])
			y, okY := floatValue(rawCoords[1])
			if !okX || !okY {
				continue
			}
			ring = append(ring, [2]float64{x, y})
		}
		if len(ring) > 0 {
			out = append(out, ring)
		}
	}
	return out, len(out) > 0
}

func asMap(value interface{}) (map[string]interface{}, bool) {
	m, ok := value.(map[string]interface{})
	return m, ok
}

func buildSales(attrs map[string]interface{}) []models.ParcelSale {
	prefixes := []string{"s1", "s2", "s3"}
	sales := make([]models.ParcelSale, 0, len(prefixes))
	for i, prefix := range prefixes {
		saleDate, ogSaleDate := arcGISDate(attrs, prefix+"_SALEDATE")
		sale := models.ParcelSale{
			Sequence:    i + 1,
			SaleDate:    saleDate,
			OGSaleDate:  ogSaleDate,
			SaleYear:    intPtr(attrs, prefix+"_SALE_YEAR"),
			SaleMonth:   intPtr(attrs, prefix+"_SALE_MTH"),
			SaleDay:     intPtr(attrs, prefix+"_SALE_DAY"),
			Amount:      floatPtr(attrs, prefix+"_SALESAMT"),
			DeedBook:    strPtr(attrs, prefix+"_DEED_BOOK"),
			DeedPage:    strPtr(attrs, prefix+"_DEED_PAGE"),
			DeedType:    strPtr(attrs, prefix+"_DEEDTYPE"),
			DeedQuality: strPtr(attrs, prefix+"_DEEDQUAL_CODEDESC"),
			Grantor:     strPtr(attrs, prefix+"_grantor"),
			Grantor2:    strPtr(attrs, prefix+"_grantor2"),
		}
		if saleHasData(sale) {
			sales = append(sales, sale)
		}
	}
	return sales
}

func saleHasData(sale models.ParcelSale) bool {
	return sale.SaleDate != nil || sale.OGSaleDate != nil || sale.SaleYear != nil || sale.SaleMonth != nil || sale.SaleDay != nil || sale.Amount != nil || sale.DeedBook != nil || sale.DeedPage != nil || sale.DeedType != nil || sale.DeedQuality != nil || sale.Grantor != nil || sale.Grantor2 != nil
}
