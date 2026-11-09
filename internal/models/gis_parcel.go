package models

import (
	"encoding/json"
	"time"

	"github.com/twpayne/go-geom"
)

// GISParcel is the county-agnostic, normalized parcel shape persisted by the
// storage layer. Fields that do not resolve into these concepts belong in
// SourceExtras as JSON. The writing package chooses the key.
type GISParcel struct {
	Identity  ParcelIdentity  `json:"identity"`
	Owner     ParcelOwner     `json:"owner"`
	Mailing   ParcelAddress   `json:"mailing"`
	Situs     ParcelAddress   `json:"situs"`
	Legal     ParcelLegal     `json:"legal"`
	Land      ParcelLand      `json:"land"`
	Building  ParcelBuilding  `json:"building"`
	Valuation ParcelValuation `json:"valuation"`
	Sales     []ParcelSale    `json:"sales,omitempty"`
	Geometry  GISGeometry     `json:"geometry"`

	SourceExtras json.RawMessage `db:"source_extras" json:"source_extras,omitempty"`

	changelog
}

type ParcelIdentity struct {
	CountyID       string  `db:"county_id" json:"county_id"`
	CountyCode     string  `db:"county_code" json:"county_code"`
	SourceName     string  `db:"source_name" json:"source_name"`
	SourceParcelID string  `db:"source_parcel_id" json:"source_parcel_id"`
	ParcelNumber   *string `db:"parcel_number" json:"parcel_number,omitempty"`
	AccountNumber  *string `db:"account_number" json:"account_number,omitempty"`
	ParcelYear     *int64  `db:"parcel_year" json:"parcel_year,omitempty"`
}

type ParcelOwner struct {
	CurrentName1 *string `db:"current_name_1" json:"current_name_1,omitempty"`
	CurrentName2 *string `db:"current_name_2" json:"current_name_2,omitempty"`
	TaxName1     *string `db:"tax_name_1" json:"tax_name_1,omitempty"`
	TaxName2     *string `db:"tax_name_2" json:"tax_name_2,omitempty"`
	TaxOwnerID1  *string `db:"tax_owner_id_1" json:"tax_owner_id_1,omitempty"`
	TaxOwnerID2  *string `db:"tax_owner_id_2" json:"tax_owner_id_2,omitempty"`
}

type ParcelAddress struct {
	Line1      *string `db:"line_1" json:"line_1,omitempty"`
	Line2      *string `db:"line_2" json:"line_2,omitempty"`
	City       *string `db:"city" json:"city,omitempty"`
	State      *string `db:"state" json:"state,omitempty"`
	PostalCode *string `db:"postal_code" json:"postal_code,omitempty"`
}

type ParcelLegal struct {
	Description *string `db:"description" json:"description,omitempty"`
	PlatBook    *string `db:"plat_book" json:"plat_book,omitempty"`
	PlatPage    *string `db:"plat_page" json:"plat_page,omitempty"`
	DeedBook    *string `db:"deed_book" json:"deed_book,omitempty"`
	DeedPage    *string `db:"deed_page" json:"deed_page,omitempty"`
}

type ParcelLand struct {
	LandCode    *string  `db:"land_code" json:"land_code,omitempty"`
	LandType    *string  `db:"land_type" json:"land_type,omitempty"`
	PropertyUse *string  `db:"property_use" json:"property_use,omitempty"`
	Subdivision *string  `db:"subdivision" json:"subdivision,omitempty"`
	MappedAcres *float64 `db:"mapped_acres" json:"mapped_acres,omitempty"`
	GrossAcres  *float64 `db:"gross_acres" json:"gross_acres,omitempty"`
	ValuedAcres *float64 `db:"valued_acres" json:"valued_acres,omitempty"`
	ValuedSqFt  *float64 `db:"valued_sqft" json:"valued_sqft,omitempty"`
}

type ParcelBuilding struct {
	YearBuilt      *int64   `db:"year_built" json:"year_built,omitempty"`
	SqFt           *float64 `db:"sqft" json:"sqft,omitempty"`
	Basement       *string  `db:"basement" json:"basement,omitempty"`
	QualityCode    *string  `db:"quality_code" json:"quality_code,omitempty"`
	Structure      *string  `db:"structure" json:"structure,omitempty"`
	StructureStyle *string  `db:"structure_style" json:"structure_style,omitempty"`
}

type ParcelValuation struct {
	Land        *float64 `db:"land" json:"land,omitempty"`
	Improvement *float64 `db:"improvement" json:"improvement,omitempty"`
	Total       *float64 `db:"total" json:"total,omitempty"`
	Assessed    *float64 `db:"assessed" json:"assessed,omitempty"`
}

type ParcelSale struct {
	Sequence    int        `db:"sequence" json:"sequence"`
	SaleDate    *time.Time `db:"sale_date" json:"sale_date,omitempty"`
	OGSaleDate  *int64     `db:"og_sale_date" json:"og_sale_date,omitempty"`
	Amount      *float64   `db:"amount" json:"amount,omitempty"`
	DeedBook    *string    `db:"deed_book" json:"deed_book,omitempty"`
	DeedPage    *string    `db:"deed_page" json:"deed_page,omitempty"`
	DeedType    *string    `db:"deed_type" json:"deed_type,omitempty"`
	DeedQuality *string    `db:"deed_quality" json:"deed_quality,omitempty"`
	Grantor     *string    `db:"grantor" json:"grantor,omitempty"`
	Grantor2    *string    `db:"grantor_2" json:"grantor_2,omitempty"`
}

type GISGeometry struct {
	Shape        *geom.T  `db:"shape" json:"shape,omitempty"`
	SRID         int      `db:"srid" json:"srid"`
	SourceSRID   *int     `db:"source_srid" json:"source_srid,omitempty"`
	SourceArea   *float64 `db:"source_area" json:"source_area,omitempty"`
	SourceLength *float64 `db:"source_length" json:"source_length,omitempty"`
}

// ParcelSourceExtras is the JSON object stored in source_extras.
// A writing package marshals its own key into this payload.
// The shared model stores it as json.RawMessage.
type ParcelSourceExtras = json.RawMessage
