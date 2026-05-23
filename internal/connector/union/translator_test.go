package union

import (
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector"
)

// compile-time interface check
var _ connector.Translator = (*GISTranslator)(nil)

func TestNewGISTranslator(t *testing.T) {
	tr := NewGISTranslator()
	if tr == nil {
		t.Fatal("expected non-nil translator")
	}
	if tr.priority != 10 {
		t.Errorf("priority = %d, want 10", tr.priority)
	}
}

func TestGISTranslator_CanTranslate(t *testing.T) {
	tr := NewGISTranslator()

	tests := []struct {
		name       string
		sourceName string
		want       bool
	}{
		{name: "union county gis source", sourceName: "union_county_gis", want: true},
		{name: "other county gis source", sourceName: "mecklenburg_county_gis", want: false},
		{name: "union county tax source", sourceName: "union_county_tax", want: false},
		{name: "empty source name", sourceName: "", want: false},
		{name: "similar but not exact match", sourceName: "union_county_gis_v2", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tr.CanTranslate(tt.sourceName); got != tt.want {
				t.Errorf("CanTranslate(%q) = %v, want %v", tt.sourceName, got, tt.want)
			}
		})
	}
}

func TestGISTranslator_Priority(t *testing.T) {
	if got := NewGISTranslator().Priority(); got != 10 {
		t.Errorf("Priority() = %d, want 10", got)
	}
}

func TestGISTranslator_Translate(t *testing.T) {
	tr := NewGISTranslator()

	tests := []struct {
		name        string
		rawRecord   connector.RawRecord
		wantErr     bool
		errContains string
		checkParcel func(t *testing.T, parcelID string, countyID string)
	}{
		{
			name: "valid union county gis record",
			rawRecord: connector.RawRecord{
				SourceName: "union_county_gis",
				ParcelID:   "12-345-678",
				RawData:    map[string]interface{}{"attributes": map[string]interface{}{"PID": "12-345-678"}},
			},
			checkParcel: func(t *testing.T, parcelID string, countyID string) {
				if parcelID != "12-345-678" {
					t.Errorf("ParcelNumber = %q, want %q", parcelID, "12-345-678")
				}
				if countyID != "union" {
					t.Errorf("CountyID = %q, want %q", countyID, "union")
				}
			},
		},
		{
			name: "parcel id can come from attributes",
			rawRecord: connector.RawRecord{
				SourceName: "union_county_gis",
				RawData:    map[string]interface{}{"attributes": map[string]interface{}{"PID": "98-765-432"}},
			},
			checkParcel: func(t *testing.T, parcelID string, countyID string) {
				if parcelID != "98-765-432" {
					t.Errorf("ParcelNumber = %q, want %q", parcelID, "98-765-432")
				}
			},
		},
		{
			name:        "wrong source name",
			rawRecord:   connector.RawRecord{SourceName: "mecklenburg_county_gis", ParcelID: "12-345-678"},
			wantErr:     true,
			errContains: "cannot translate source",
		},
		{
			name:        "missing parcel ID",
			rawRecord:   connector.RawRecord{SourceName: "union_county_gis", ParcelID: "", RawData: map[string]interface{}{}},
			wantErr:     true,
			errContains: "missing parcel ID",
		},
		{
			name:      "empty raw data still works if parcel ID present",
			rawRecord: connector.RawRecord{SourceName: "union_county_gis", ParcelID: "99-999-999", RawData: nil},
			checkParcel: func(t *testing.T, parcelID string, countyID string) {
				if parcelID != "99-999-999" {
					t.Errorf("ParcelNumber = %q, want %q", parcelID, "99-999-999")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parcel, err := tr.Translate(tt.rawRecord)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if parcel == nil {
				t.Fatal("expected non-nil parcel")
			}
			if tt.checkParcel != nil {
				if parcel.Identity.ParcelNumber == nil {
					t.Fatal("ParcelNumber is nil")
				}
				tt.checkParcel(t, *parcel.Identity.ParcelNumber, parcel.Identity.CountyID)
			}
		})
	}
}

func TestGISTranslator_TranslatePopulatesAllGISParcelSections(t *testing.T) {
	parcel, err := NewGISTranslator().Translate(connector.RawRecord{
		SourceName: "union_county_gis",
		RawData: map[string]interface{}{
			"attributes": populatedAttributes(),
		},
	})
	if err != nil {
		t.Fatalf("Translate returned error: %v", err)
	}

	assertString(t, parcel.Identity.ParcelNumber, "12-345-678", "Identity.ParcelNumber")
	assertString(t, parcel.Identity.AccountNumber, "ACCT-1", "Identity.AccountNumber")
	assertInt(t, parcel.Identity.ParcelYear, 2024, "Identity.ParcelYear")
	if parcel.Identity.CountyID != "union" || parcel.Identity.CountyCode != "union" || parcel.Identity.SourceName != "union_county_gis" || parcel.Identity.SourceParcelID != "12-345-678" {
		t.Fatalf("identity not populated as expected: %+v", parcel.Identity)
	}

	assertString(t, parcel.Owner.CurrentName1, "Current One", "Owner.CurrentName1")
	assertString(t, parcel.Owner.CurrentName2, "Current Two", "Owner.CurrentName2")
	assertString(t, parcel.Owner.TaxName1, "Tax One", "Owner.TaxName1")
	assertString(t, parcel.Owner.TaxName2, "Tax Two", "Owner.TaxName2")
	assertInt(t, parcel.Owner.TaxOwnerID1, 101, "Owner.TaxOwnerID1")
	assertInt(t, parcel.Owner.TaxOwnerID2, 202, "Owner.TaxOwnerID2")

	assertString(t, parcel.Mailing.Line1, "123 Mail St", "Mailing.Line1")
	assertString(t, parcel.Mailing.Line2, "Suite 4", "Mailing.Line2")
	assertString(t, parcel.Mailing.City, "Monroe", "Mailing.City")
	assertString(t, parcel.Mailing.State, "NC", "Mailing.State")
	assertString(t, parcel.Mailing.PostalCode, "28112", "Mailing.PostalCode")
	assertString(t, parcel.Situs.Line1, "456 Site Rd", "Situs.Line1")

	assertString(t, parcel.Legal.Description, "LOT 1", "Legal.Description")
	assertString(t, parcel.Legal.PlatBook, "PB", "Legal.PlatBook")
	assertString(t, parcel.Legal.PlatPage, "12", "Legal.PlatPage")
	assertString(t, parcel.Legal.DeedBook, "DB1", "Legal.DeedBook")
	assertString(t, parcel.Legal.DeedPage, "DP1", "Legal.DeedPage")

	assertString(t, parcel.Land.LandCode, "LC", "Land.LandCode")
	assertString(t, parcel.Land.LandType, "Residential", "Land.LandType")
	assertString(t, parcel.Land.PropertyUse, "Single Family", "Land.PropertyUse")
	assertString(t, parcel.Land.Subdivision, "Subdiv", "Land.Subdivision")
	assertFloat(t, parcel.Land.MappedAcres, 1.25, "Land.MappedAcres")
	assertFloat(t, parcel.Land.GrossAcres, 1.5, "Land.GrossAcres")
	assertFloat(t, parcel.Land.ValuedAcres, 1.1, "Land.ValuedAcres")
	assertFloat(t, parcel.Land.ValuedSqFt, 47916, "Land.ValuedSqFt")

	assertInt(t, parcel.Building.YearBuilt, 1999, "Building.YearBuilt")
	assertFloat(t, parcel.Building.SqFt, 2400, "Building.SqFt")
	assertString(t, parcel.Building.Basement, "YES", "Building.Basement")
	assertString(t, parcel.Building.QualityCode, "Q1", "Building.QualityCode")
	assertString(t, parcel.Building.Structure, "DWELLING", "Building.Structure")
	assertString(t, parcel.Building.StructureStyle, "RANCH", "Building.StructureStyle")

	assertFloat(t, parcel.Valuation.Land, 100000, "Valuation.Land")
	assertFloat(t, parcel.Valuation.Improvement, 250000, "Valuation.Improvement")
	assertFloat(t, parcel.Valuation.Total, 350000, "Valuation.Total")
	assertFloat(t, parcel.Valuation.Assessed, 300000, "Valuation.Assessed")

	if parcel.SourceExtras.Union == nil {
		t.Fatal("SourceExtras.Union is nil")
	}
	assertInt(t, parcel.SourceExtras.Union.OBJECTID, 1, "Extras.OBJECTID")
	assertInt(t, parcel.SourceExtras.Union.OBJECTID_1, 2, "Extras.OBJECTID_1")
	assertString(t, parcel.SourceExtras.Union.SUBCODE, "SUB", "Extras.SUBCODE")
	assertString(t, parcel.SourceExtras.Union.DIST_TWN, "DT", "Extras.DIST_TWN")
	assertString(t, parcel.SourceExtras.Union.DIST_CODE, "DC", "Extras.DIST_CODE")
	assertString(t, parcel.SourceExtras.Union.TAX_DISTRI, "TD", "Extras.TAX_DISTRI")
	assertString(t, parcel.SourceExtras.Union.TWSHP_CD, "TC", "Extras.TWSHP_CD")
	assertString(t, parcel.SourceExtras.Union.TWSHP_DESC, "Township", "Extras.TWSHP_DESC")
	assertString(t, parcel.SourceExtras.Union.DESC1_DESC, "Desc", "Extras.DESC1_DESC")
	assertString(t, parcel.SourceExtras.Union.DESC1_CODE, "D1", "Extras.DESC1_CODE")
	assertString(t, parcel.SourceExtras.Union.LUV_YES_NO, "N", "Extras.LUV_YES_NO")
	assertString(t, parcel.SourceExtras.Union.MULSTR, "M", "Extras.MULSTR")
	assertString(t, parcel.SourceExtras.Union.MULT_LANDSEQ, "SEQ", "Extras.MULT_LANDSEQ")
	assertString(t, parcel.SourceExtras.Union.NBHDNAME, "Neighborhood", "Extras.NBHDNAME")
	assertString(t, parcel.SourceExtras.Union.NBHDNUM, "100", "Extras.NBHDNUM")
	assertString(t, parcel.SourceExtras.Union.NBHNEW, "NEW", "Extras.NBHNEW")
	assertInt(t, parcel.SourceExtras.Union.IDCOL, 999, "Extras.IDCOL")
}

func TestGISTranslator_TranslateSalesAndDates(t *testing.T) {
	attrs := populatedAttributes()
	parcel, err := NewGISTranslator().Translate(connector.RawRecord{SourceName: "union_county_gis", RawData: map[string]interface{}{"attributes": attrs}})
	if err != nil {
		t.Fatalf("Translate returned error: %v", err)
	}

	if len(parcel.Sales) != 3 {
		t.Fatalf("len(Sales) = %d, want 3", len(parcel.Sales))
	}
	assertInt(t, parcel.Sales[0].SaleYear, 2020, "Sales[0].SaleYear")
	assertInt(t, parcel.Sales[0].SaleMonth, 5, "Sales[0].SaleMonth")
	assertInt(t, parcel.Sales[0].SaleDay, 15, "Sales[0].SaleDay")
	assertFloat(t, parcel.Sales[0].Amount, 123456, "Sales[0].Amount")
	assertString(t, parcel.Sales[0].DeedBook, "DB1", "Sales[0].DeedBook")
	assertString(t, parcel.Sales[0].DeedPage, "DP1", "Sales[0].DeedPage")
	assertString(t, parcel.Sales[0].DeedType, "WD", "Sales[0].DeedType")
	assertString(t, parcel.Sales[0].DeedQuality, "Qualified", "Sales[0].DeedQuality")
	assertString(t, parcel.Sales[0].Grantor, "Grantor 1", "Sales[0].Grantor")
	assertString(t, parcel.Sales[0].Grantor2, "Grantor 1B", "Sales[0].Grantor2")
	if parcel.Sales[0].OGSaleDate == nil || *parcel.Sales[0].OGSaleDate != int64(1589500800000) {
		t.Fatalf("Sales[0].OGSaleDate = %v, want 1589500800000", parcel.Sales[0].OGSaleDate)
	}
	wantSaleDate := time.Date(2020, 5, 15, 0, 0, 0, 0, time.UTC)
	if parcel.Sales[0].SaleDate == nil || !parcel.Sales[0].SaleDate.Equal(wantSaleDate) {
		t.Fatalf("Sales[0].SaleDate = %v, want %v", parcel.Sales[0].SaleDate, wantSaleDate)
	}

	if parcel.SourceExtras.Union.DATE_CRT == nil || !parcel.SourceExtras.Union.DATE_CRT.Equal(time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("DATE_CRT = %v, want 2021-01-01 UTC", parcel.SourceExtras.Union.DATE_CRT)
	}
	assertInt(t, parcel.SourceExtras.Union.OG_DATE_CRT, 1609459200000, "Extras.OG_DATE_CRT")
}

func TestGISTranslator_TranslateGeometry(t *testing.T) {
	attrs := populatedAttributes()
	parcel, err := NewGISTranslator().Translate(connector.RawRecord{
		SourceName: "union_county_gis",
		RawData: map[string]interface{}{
			"attributes": attrs,
			"geometry": map[string]interface{}{
				"rings": []interface{}{
					[]interface{}{
						[]interface{}{0.0, 0.0},
						[]interface{}{1.0, 0.0},
						[]interface{}{1.0, 1.0},
						[]interface{}{0.0, 0.0},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Translate returned error: %v", err)
	}

	if parcel.Geometry.SRID != unionGISSourceSRID {
		t.Errorf("Geometry.SRID = %d, want %d", parcel.Geometry.SRID, unionGISSourceSRID)
	}
	if parcel.Geometry.SourceSRID == nil || *parcel.Geometry.SourceSRID != unionGISSourceSRID {
		t.Fatalf("Geometry.SourceSRID = %v, want %d", parcel.Geometry.SourceSRID, unionGISSourceSRID)
	}
	assertFloat(t, parcel.Geometry.SourceArea, 123.45, "Geometry.SourceArea")
	assertFloat(t, parcel.Geometry.SourceLength, 67.89, "Geometry.SourceLength")
	if parcel.Geometry.Shape == nil {
		t.Fatal("Geometry.Shape is nil")
	}
	shape := *parcel.Geometry.Shape
	if shape.SRID() != unionGISSourceSRID {
		t.Errorf("shape.SRID = %d, want %d", shape.SRID(), unionGISSourceSRID)
	}
	if got := shape.FlatCoords(); len(got) != 8 || got[0] != 0 || got[2] != 1 || got[5] != 1 {
		t.Fatalf("shape.FlatCoords() = %v, want parsed ring coordinates", got)
	}
}

func populatedAttributes() map[string]interface{} {
	return map[string]interface{}{
		"PID":                  "12-345-678",
		"ACCTNO":               "ACCT-1",
		"parcel_year":          2024,
		"CURR_NAME1":           "Current One",
		"CURR_NAME2":           "Current Two",
		"JAN1_NAME1":           "Tax One",
		"JAN1_NAME2":           "Tax Two",
		"JAN1_OWNERID":         101,
		"JAN1_OWNERID2":        202,
		"CURR_ADDR1":           "123 Mail St",
		"CURR_ADDR2":           "Suite 4",
		"CURR_CITY":            "Monroe",
		"CURR_STATE":           "NC",
		"CURR_ZIPCODE":         "28112",
		"PHYSSTRADD":           "456 Site Rd",
		"LEGDESC_1":            "LOT 1",
		"PLAT_BOOK":            "PB",
		"PLAT_PAGE":            "12",
		"LAND_CODE":            "LC",
		"LAND_TYPE":            "Residential",
		"property_use":         "Single Family",
		"subdivision":          "Subdiv",
		"mapped_acres":         1.25,
		"gross_acres":          1.5,
		"VAL_AC":               1.1,
		"VAL_SQFT":             47916,
		"YEARBLT":              1999,
		"SQFT":                 2400.0,
		"BASEMENT":             "YES",
		"BLDQUAL_CODE":         "Q1",
		"STRUCTTYPE":           "DWELLING",
		"STRUCTSTYLE":          "RANCH",
		"FMV_LAND":             100000,
		"FMV_IMPRV":            250000,
		"FMV_TOTAL":            350000,
		"TOTVAL":               300000,
		"s1_DEED_BOOK":         "DB1",
		"s1_DEED_PAGE":         "DP1",
		"s1_DEEDQUAL_CODEDESC": "Qualified",
		"s1_DEEDTYPE":          "WD",
		"s1_SALE_YEAR":         2020,
		"s1_SALE_MTH":          5,
		"s1_SALE_DAY":          15,
		"s1_SALESAMT":          123456,
		"s1_SALEDATE":          1589500800000,
		"s1_grantor":           "Grantor 1",
		"s1_grantor2":          "Grantor 1B",
		"s2_DEED_BOOK":         "DB2",
		"s2_DEED_PAGE":         "DP2",
		"s2_DEEDQUAL_CODEDESC": "Unqualified",
		"s2_DEEDTYPE":          "QC",
		"s2_SALE_YEAR":         2018,
		"s2_SALE_MTH":          6,
		"s2_SALE_DAY":          20,
		"s2_SALESAMT":          100000,
		"s2_SALEDATE":          1529452800000,
		"s2_grantor":           "Grantor 2",
		"s2_grantor2":          "Grantor 2B",
		"s3_DEED_BOOK":         "DB3",
		"s3_DEED_PAGE":         "DP3",
		"s3_DEEDQUAL_CODEDESC": "Qualified",
		"s3_DEEDTYPE":          "WD",
		"s3_SALE_YEAR":         2016,
		"s3_SALE_MTH":          7,
		"s3_SALE_DAY":          25,
		"s3_SALESAMT":          90000,
		"s3_SALEDATE":          1469404800000,
		"s3_grantor":           "Grantor 3",
		"s3_grantor2":          "Grantor 3B",
		"SHAPE.STArea()":       123.45,
		"SHAPE.STLength()":     67.89,
		"OBJECTID":             1,
		"OBJECTID_1":           2,
		"SUBCODE":              "SUB",
		"DIST_TWN":             "DT",
		"DIST_CODE":            "DC",
		"TAX_DISTRI":           "TD",
		"TWSHP_CD":             "TC",
		"TWSHP_DESC":           "Township",
		"DESC1_DESC":           "Desc",
		"DESC1_CODE":           "D1",
		"LUV_YES_NO":           "N",
		"MULSTR":               "M",
		"MULT_LANDSEQ":         "SEQ",
		"NBHDNAME":             "Neighborhood",
		"NBHDNUM":              "100",
		"NBHNEW":               "NEW",
		"idcol":                999,
		"DATE_CRT":             1609459200000,
	}
}

func assertString(t *testing.T, got *string, want string, field string) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("%s = %v, want %q", field, got, want)
	}
}

func assertInt(t *testing.T, got *int64, want int64, field string) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("%s = %v, want %d", field, got, want)
	}
}

func assertFloat(t *testing.T, got *float64, want float64, field string) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("%s = %v, want %f", field, got, want)
	}
}
