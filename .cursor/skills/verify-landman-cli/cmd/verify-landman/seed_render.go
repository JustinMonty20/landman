package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/JustinMonty20/landman/internal/models"
	"github.com/JustinMonty20/landman/internal/soil"
	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/wkt"
)

const overlapAcresExpr = `ST_Area(ST_Transform(ST_Intersection(p.shape, s.shape), 2264)) / 43560`

type renderedParcel struct {
	ID     string
	Parcel *models.GISParcel
}

type renderedSoil struct {
	ID      string
	Polygon soil.Polygon
	Unit    soil.MapUnit
}

type renderedOverlap struct {
	ID       string
	SoilID   string
	ParcelID string
	Acres    string
}

// renderSeed writes the next seed.sql text. Names for a map unit key come from
// the previous seed. A key the previous seed does not have gets NULL names.
func renderSeed(previous seedSnapshot, parcels []renderedParcel, soils []renderedSoil, overlaps []renderedOverlap, when time.Time) (string, error) {
	when = when.UTC()
	var b strings.Builder
	b.WriteString(seedHeader(when))
	b.WriteString("BEGIN;\n")
	if err := writeMapUnits(&b, previous, soils, when); err != nil {
		return "", err
	}
	if err := writeParcels(&b, previous, parcels, when); err != nil {
		return "", err
	}
	if err := writeSoilPolygons(&b, previous, soils, when); err != nil {
		return "", err
	}
	if err := writeOverlaps(&b, previous, overlaps, when); err != nil {
		return "", err
	}
	if err := writeSales(&b, previous, parcels); err != nil {
		return "", err
	}
	b.WriteString("COMMIT;\n")
	return b.String(), nil
}

func seedHeader(when time.Time) string {
	return strings.Join([]string{
		"-- Captured " + when.UTC().Format(time.RFC3339) + ".",
		"-- Parcel query " + parcelLayer + "/query orderByFields=OBJECTID",
		"-- Soil query " + soilLayer + "/query orderByFields=OBJECTID_1",
		"-- SavePart calls SaveSales and then deletes omitted sequences.",
		"-- SaveSales alone does not delete.",
		"",
	}, "\n")
}

func writeMapUnits(b *strings.Builder, previous seedSnapshot, soils []renderedSoil, when time.Time) error {
	cols := previous.columns["soil_map_units"]
	if len(cols) == 0 {
		return fmt.Errorf("seed.sql has no soil_map_units columns")
	}
	seen := map[string]soil.MapUnit{}
	var order []string
	for _, soilRow := range soils {
		key := soilRow.Unit.MapUnitKey
		if earlier, ok := seen[key]; ok {
			if earlier.MapUnitSymbol != soilRow.Unit.MapUnitSymbol || earlier.AreaSymbol != soilRow.Unit.AreaSymbol {
				return fmt.Errorf("map unit %s has conflicting symbols", key)
			}
			continue
		}
		seen[key] = soilRow.Unit
		order = append(order, key)
	}
	for _, key := range order {
		unit := seen[key]
		line, err := insertValues("soil_map_units", cols, func(col string) (string, error) {
			return mapUnitValue(col, unit, previous.mapUnits[key], previous.mapUnits[key].Key == key, when)
		})
		if err != nil {
			return err
		}
		b.WriteString(line)
	}
	return nil
}

func mapUnitValue(col string, unit soil.MapUnit, previous seedMapUnit, known bool, when time.Time) (string, error) {
	switch col {
	case "map_unit_key":
		return sqlQuote(unit.MapUnitKey)
	case "map_unit_symbol":
		return sqlQuote(unit.MapUnitSymbol)
	case "area_symbol":
		return sqlQuote(unit.AreaSymbol)
	case "map_unit_name":
		if !known || !previous.HasName {
			return "NULL", nil
		}
		return sqlQuote(previous.Name)
	case "area_name":
		if !known || !previous.HasArea {
			return "NULL", nil
		}
		return sqlQuote(previous.AreaName)
	case "fetched_at":
		return sqlTimestamp(when), nil
	default:
		return "", fmt.Errorf("no value for soil_map_units.%s", col)
	}
}

func writeParcels(b *strings.Builder, previous seedSnapshot, parcels []renderedParcel, when time.Time) error {
	cols := previous.columns["parcels"]
	if len(cols) == 0 {
		return fmt.Errorf("seed.sql has no parcels columns")
	}
	for _, row := range parcels {
		if !safeUUID(row.ID) {
			return fmt.Errorf("parcel uuid %q", row.ID)
		}
		line, err := insertValues("parcels", cols, func(col string) (string, error) {
			return parcelValue(col, row, when)
		})
		if err != nil {
			return err
		}
		b.WriteString(line)
	}
	return nil
}

func parcelValue(col string, row renderedParcel, when time.Time) (string, error) {
	parcel := row.Parcel
	switch col {
	case "id":
		return sqlUUID(row.ID), nil
	case "county_id":
		return sqlQuote(parcel.Identity.CountyID)
	case "county_code":
		return sqlQuote(parcel.Identity.CountyCode)
	case "source_name":
		return sqlQuote(parcel.Identity.SourceName)
	case "source_parcel_id":
		return sqlQuote(parcel.Identity.SourceParcelID)
	case "parcel_number":
		return sqlText(parcel.Identity.ParcelNumber), nil
	case "account_number":
		return sqlText(parcel.Identity.AccountNumber), nil
	case "parcel_year":
		return sqlInt(parcel.Identity.ParcelYear), nil
	case "current_name_1":
		return sqlText(parcel.Owner.CurrentName1), nil
	case "current_name_2":
		return sqlText(parcel.Owner.CurrentName2), nil
	case "tax_name_1":
		return sqlText(parcel.Owner.TaxName1), nil
	case "tax_name_2":
		return sqlText(parcel.Owner.TaxName2), nil
	case "tax_owner_id_1":
		return sqlText(parcel.Owner.TaxOwnerID1), nil
	case "tax_owner_id_2":
		return sqlText(parcel.Owner.TaxOwnerID2), nil
	case "mailing_line_1":
		return sqlText(parcel.Mailing.Line1), nil
	case "mailing_line_2":
		return sqlText(parcel.Mailing.Line2), nil
	case "mailing_city":
		return sqlText(parcel.Mailing.City), nil
	case "mailing_state":
		return sqlText(parcel.Mailing.State), nil
	case "mailing_postal_code":
		return sqlText(parcel.Mailing.PostalCode), nil
	case "situs_line_1":
		return sqlText(parcel.Situs.Line1), nil
	case "situs_line_2":
		return sqlText(parcel.Situs.Line2), nil
	case "situs_city":
		return sqlText(parcel.Situs.City), nil
	case "situs_state":
		return sqlText(parcel.Situs.State), nil
	case "situs_postal_code":
		return sqlText(parcel.Situs.PostalCode), nil
	case "legal_description":
		return sqlText(parcel.Legal.Description), nil
	case "plat_book":
		return sqlText(parcel.Legal.PlatBook), nil
	case "plat_page":
		return sqlText(parcel.Legal.PlatPage), nil
	case "deed_book":
		return sqlText(parcel.Legal.DeedBook), nil
	case "deed_page":
		return sqlText(parcel.Legal.DeedPage), nil
	case "land_code":
		return sqlText(parcel.Land.LandCode), nil
	case "land_type":
		return sqlText(parcel.Land.LandType), nil
	case "property_use":
		return sqlText(parcel.Land.PropertyUse), nil
	case "subdivision":
		return sqlText(parcel.Land.Subdivision), nil
	case "mapped_acres":
		return sqlFloat(parcel.Land.MappedAcres), nil
	case "gross_acres":
		return sqlFloat(parcel.Land.GrossAcres), nil
	case "valued_acres":
		return sqlFloat(parcel.Land.ValuedAcres), nil
	case "valued_sqft":
		return sqlFloat(parcel.Land.ValuedSqFt), nil
	case "year_built":
		return sqlInt(parcel.Building.YearBuilt), nil
	case "sqft":
		return sqlFloat(parcel.Building.SqFt), nil
	case "basement":
		return sqlText(parcel.Building.Basement), nil
	case "quality_code":
		return sqlText(parcel.Building.QualityCode), nil
	case "structure":
		return sqlText(parcel.Building.Structure), nil
	case "structure_style":
		return sqlText(parcel.Building.StructureStyle), nil
	case "valuation_land":
		return sqlFloat(parcel.Valuation.Land), nil
	case "valuation_improvement":
		return sqlFloat(parcel.Valuation.Improvement), nil
	case "valuation_total":
		return sqlFloat(parcel.Valuation.Total), nil
	case "valuation_assessed":
		return sqlFloat(parcel.Valuation.Assessed), nil
	case "shape":
		return sqlShape(parcel.Geometry.Shape)
	case "srid":
		return strconv.Itoa(parcel.Geometry.SRID), nil
	case "source_srid":
		return sqlInt(int64Ptr(parcel.Geometry.SourceSRID)), nil
	case "source_area":
		return sqlFloat(parcel.Geometry.SourceArea), nil
	case "source_length":
		return sqlFloat(parcel.Geometry.SourceLength), nil
	case "source_extras":
		extras := string(parcel.SourceExtras)
		if strings.TrimSpace(extras) == "" {
			extras = "{}"
		}
		text, err := sqlQuote(extras)
		if err != nil {
			return "", err
		}
		return text + "::jsonb", nil
	case "created_at", "updated_at":
		return sqlTimestamp(when), nil
	default:
		return "", fmt.Errorf("no value for parcels.%s", col)
	}
}

func writeSoilPolygons(b *strings.Builder, previous seedSnapshot, soils []renderedSoil, when time.Time) error {
	cols := previous.columns["soil_polygons"]
	if len(cols) == 0 {
		return fmt.Errorf("seed.sql has no soil_polygons columns")
	}
	for _, row := range soils {
		if !safeUUID(row.ID) {
			return fmt.Errorf("soil polygon uuid %q", row.ID)
		}
		line, err := insertValues("soil_polygons", cols, func(col string) (string, error) {
			return soilPolygonValue(col, row, when)
		})
		if err != nil {
			return err
		}
		b.WriteString(line)
	}
	return nil
}

func soilPolygonValue(col string, row renderedSoil, when time.Time) (string, error) {
	switch col {
	case "id":
		return sqlUUID(row.ID), nil
	case "county_id":
		return sqlQuote(row.Polygon.County.String())
	case "source_name":
		return sqlQuote(row.Polygon.SourceName)
	case "object_id":
		return sqlQuote(row.Polygon.ObjectID)
	case "map_unit_key":
		return sqlQuote(row.Polygon.MapUnitKey)
	case "spatial_version":
		return sqlFloat(row.Polygon.SpatialVersion), nil
	case "survey_saved_on":
		if row.Polygon.SurveySavedOn == nil {
			return "NULL", nil
		}
		return "DATE '" + row.Polygon.SurveySavedOn.UTC().Format("2006-01-02") + "'", nil
	case "shape":
		return sqlShape(row.Polygon.Shape)
	case "fetched_at":
		return sqlTimestamp(when), nil
	default:
		return "", fmt.Errorf("no value for soil_polygons.%s", col)
	}
}

func writeOverlaps(b *strings.Builder, previous seedSnapshot, overlaps []renderedOverlap, when time.Time) error {
	if len(overlaps) == 0 {
		return nil
	}
	cols := previous.columns["soil_polygon_parcels"]
	if len(cols) == 0 {
		return fmt.Errorf("seed.sql has no soil_polygon_parcels columns")
	}
	for _, row := range overlaps {
		if !safeUUID(row.ID) || !safeUUID(row.SoilID) || !safeUUID(row.ParcelID) {
			return fmt.Errorf("overlap uuid")
		}
		if _, ok := ratFromDecimal(row.Acres); !ok {
			return fmt.Errorf("overlap acres %q", row.Acres)
		}
		line, err := insertValues("soil_polygon_parcels", cols, func(col string) (string, error) {
			switch col {
			case "id":
				return sqlUUID(row.ID), nil
			case "soil_polygon_id":
				return sqlUUID(row.SoilID), nil
			case "parcel_id":
				return sqlUUID(row.ParcelID), nil
			case "overlap_acres":
				return sqlQuote(row.Acres)
			case "created_at", "updated_at":
				return sqlTimestamp(when), nil
			default:
				return "", fmt.Errorf("no value for soil_polygon_parcels.%s", col)
			}
		})
		if err != nil {
			return err
		}
		b.WriteString(line)
	}
	return nil
}

func writeSales(b *strings.Builder, previous seedSnapshot, parcels []renderedParcel) error {
	cols := previous.columns["parcel_sales"]
	for _, row := range parcels {
		if len(row.Parcel.Sales) == 0 {
			continue
		}
		if len(cols) == 0 {
			return fmt.Errorf("seed.sql has no parcel_sales columns")
		}
		for _, sale := range row.Parcel.Sales {
			line, err := insertSale(cols, row.Parcel.Identity.SourceParcelID, sale)
			if err != nil {
				return err
			}
			b.WriteString(line)
		}
	}
	return nil
}

func insertSale(cols []string, sourceParcelID string, sale models.ParcelSale) (string, error) {
	vals := make([]string, len(cols))
	for i, col := range cols {
		switch col {
		case "parcel_id":
			vals[i] = "id"
		case "sequence":
			vals[i] = strconv.Itoa(sale.Sequence)
		case "sale_date":
			vals[i] = sqlDate(sale.SaleDate)
		case "og_sale_date":
			vals[i] = sqlInt(sale.OGSaleDate)
		case "amount":
			vals[i] = sqlFloat(sale.Amount)
		case "deed_book":
			vals[i] = sqlText(sale.DeedBook)
		case "deed_page":
			vals[i] = sqlText(sale.DeedPage)
		case "deed_type":
			vals[i] = sqlText(sale.DeedType)
		case "deed_quality":
			vals[i] = sqlText(sale.DeedQuality)
		case "grantor":
			vals[i] = sqlText(sale.Grantor)
		case "grantor_2":
			vals[i] = sqlText(sale.Grantor2)
		default:
			return "", fmt.Errorf("no value for parcel_sales.%s", col)
		}
	}
	quoted, err := sqlQuote(sourceParcelID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("INSERT INTO parcel_sales (%s) SELECT %s FROM parcels WHERE county_id = 'union' AND source_name = 'union_county_gis' AND source_parcel_id = %s;\n",
		strings.Join(cols, ", "), strings.Join(vals, ", "), quoted), nil
}

func insertValues(table string, cols []string, value func(string) (string, error)) (string, error) {
	vals := make([]string, len(cols))
	for i, col := range cols {
		text, err := value(col)
		if err != nil {
			return "", err
		}
		vals[i] = text
	}
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);\n", table, strings.Join(cols, ", "), strings.Join(vals, ", ")), nil
}

func overlapSelectSQL() string {
	return "SELECT s.id::text, p.id::text, (" + overlapAcresExpr + ")::text\n" +
		"FROM parcels AS p\n" +
		"CROSS JOIN soil_polygons AS s\n" +
		"WHERE " + overlapAcresExpr + " > 0"
}

func sqlShape(g *geom.T) (string, error) {
	if g == nil || *g == nil {
		return "", fmt.Errorf("missing shape")
	}
	body, err := wkt.Marshal(*g)
	if err != nil {
		return "", err
	}
	srid := (*g).SRID()
	if srid == 0 {
		srid = 3857
	}
	quoted, err := sqlQuote("SRID=" + strconv.Itoa(srid) + ";" + body)
	if err != nil {
		return "", err
	}
	return "ST_GeomFromEWKT(" + quoted + ")", nil
}

func sqlQuote(text string) (string, error) {
	if strings.Contains(text, "\x00") {
		return "", fmt.Errorf("text contains NUL")
	}
	return "'" + strings.ReplaceAll(text, "'", "''") + "'", nil
}

func sqlText(value *string) string {
	if value == nil {
		return "NULL"
	}
	quoted, err := sqlQuote(*value)
	if err != nil {
		return "NULL"
	}
	return quoted
}

func sqlFloat(value *float64) string {
	if value == nil {
		return "NULL"
	}
	return strconv.FormatFloat(*value, 'f', -1, 64)
}

func sqlInt(value *int64) string {
	if value == nil {
		return "NULL"
	}
	return strconv.FormatInt(*value, 10)
}

func sqlDate(value *time.Time) string {
	if value == nil {
		return "NULL"
	}
	return "DATE '" + value.UTC().Format("2006-01-02") + "'"
}

func sqlTimestamp(when time.Time) string {
	return "'" + when.UTC().Format("2006-01-02 15:04:05.000000+00") + "'::timestamptz"
}

func sqlUUID(id string) string {
	return "'" + id + "'::uuid"
}

func int64Ptr(value *int) *int64 {
	if value == nil {
		return nil
	}
	n := int64(*value)
	return &n
}

func safeUUID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, r := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if r != '-' {
				return false
			}
			continue
		}
		if r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F' {
			continue
		}
		return false
	}
	return true
}
