package models

import (
	"time"

	"github.com/twpayne/go-geom"
)

// UnionGISParcel is the source-specific domain shape for a Union County GIS
// parcel record. It keeps every field currently advertised by the Union County
// ArcGIS parcel layer without retaining the raw source payload.
//
// Geometry is expected to be requested as GeoJSON with outSR=4326, then parsed
// into SHAPE for PostGIS storage/querying.
type UnionGISParcel struct {
	CountyID       string `json:"county_id"`
	SourceName     string `json:"source_name"`
	SourceParcelID string `json:"source_parcel_id"`

	OBJECTID   *int64  `json:"OBJECTID,omitempty"`
	OBJECTID_1 *int64  `json:"OBJECTID_1,omitempty"`
	ACCTNO     *string `json:"ACCTNO,omitempty"`
	SUBCODE    *string `json:"SUBCODE,omitempty"`
	PID        *string `json:"PID,omitempty"`

	BASEMENT     *string  `json:"BASEMENT,omitempty"`
	BLDQUAL_CODE *string  `json:"BLDQUAL_CODE,omitempty"`
	STRUCTTYPE   *string  `json:"STRUCTTYPE,omitempty"`
	STRUCTSTYLE  *string  `json:"STRUCTSTYLE,omitempty"`
	YEARBLT      *int64   `json:"YEARBLT,omitempty"`
	SQFT         *float64 `json:"SQFT,omitempty"`

	CURR_NAME1   *string `json:"CURR_NAME1,omitempty"`
	CURR_NAME2   *string `json:"CURR_NAME2,omitempty"`
	CURR_ADDR1   *string `json:"CURR_ADDR1,omitempty"`
	CURR_ADDR2   *string `json:"CURR_ADDR2,omitempty"`
	CURR_CITY    *string `json:"CURR_CITY,omitempty"`
	CURR_STATE   *string `json:"CURR_STATE,omitempty"`
	CURR_ZIPCODE *string `json:"CURR_ZIPCODE,omitempty"`

	JAN1_OWNERID  *int64  `json:"JAN1_OWNERID,omitempty"`
	JAN1_OWNERID2 *int64  `json:"JAN1_OWNERID2,omitempty"`
	JAN1_NAME1    *string `json:"JAN1_NAME1,omitempty"`
	JAN1_NAME2    *string `json:"JAN1_NAME2,omitempty"`
	JAN1_ADDR1    *string `json:"JAN1_ADDR1,omitempty"`
	JAN1_ADDR2    *string `json:"JAN1_ADDR2,omitempty"`
	JAN1_CITY     *string `json:"JAN1_CITY,omitempty"`
	JAN1_STATE    *string `json:"JAN1_STATE,omitempty"`
	JAN1_ZIPCO    *string `json:"JAN1_ZIPCO,omitempty"`

	DATE_CRT    *time.Time `json:"DATE_CRT,omitempty"`
	OG_DATE_CRT *int64     `json:"og_DATE_CRT,omitempty"`

	DIST_TWN   *string `json:"DIST_TWN,omitempty"`
	DIST_CODE  *string `json:"DIST_CODE,omitempty"`
	TAX_DISTRI *string `json:"TAX_DISTRI,omitempty"`
	TWSHP_CD   *string `json:"TWSHP_CD,omitempty"`
	TWSHP_DESC *string `json:"TWSHP_DESC,omitempty"`

	DESC1_DESC   *string `json:"DESC1_DESC,omitempty"`
	DESC1_CODE   *string `json:"DESC1_CODE,omitempty"`
	LAND_CODE    *string `json:"LAND_CODE,omitempty"`
	LAND_TYPE    *string `json:"LAND_TYPE,omitempty"`
	PROPERTY_USE *string `json:"property_use,omitempty"`
	LEGDESC_1    *string `json:"LEGDESC_1,omitempty"`
	LUV_YES_NO   *string `json:"LUV_YES_NO,omitempty"`

	MULSTR       *string `json:"MULSTR,omitempty"`
	MULT_LANDSEQ *string `json:"MULT_LANDSEQ,omitempty"`
	NBHDNAME     *string `json:"NBHDNAME,omitempty"`
	NBHDNUM      *string `json:"NBHDNUM,omitempty"`
	NBHNEW       *string `json:"NBHNEW,omitempty"`
	PHYSSTRADD   *string `json:"PHYSSTRADD,omitempty"`
	SUBDIVISION  *string `json:"subdivision,omitempty"`

	PLAT_BOOK *string `json:"PLAT_BOOK,omitempty"`
	PLAT_PAGE *string `json:"PLAT_PAGE,omitempty"`

	FMV_IMPRV    *float64 `json:"FMV_IMPRV,omitempty"`
	FMV_LAND     *float64 `json:"FMV_LAND,omitempty"`
	FMV_TOTAL    *float64 `json:"FMV_TOTAL,omitempty"`
	TOTVAL       *float64 `json:"TOTVAL,omitempty"`
	VAL_AC       *float64 `json:"VAL_AC,omitempty"`
	VAL_SQFT     *float64 `json:"VAL_SQFT,omitempty"`
	MAPPED_ACRES *float64 `json:"mapped_acres,omitempty"`
	GROSS_ACRES  *float64 `json:"gross_acres,omitempty"`

	S1_DEED_BOOK         *string    `json:"s1_DEED_BOOK,omitempty"`
	S1_DEED_PAGE         *string    `json:"s1_DEED_PAGE,omitempty"`
	S1_DEEDQUAL_CODEDESC *string    `json:"s1_DEEDQUAL_CODEDESC,omitempty"`
	S1_DEEDTYPE          *string    `json:"s1_DEEDTYPE,omitempty"`
	S1_SALE_YEAR         *int64     `json:"s1_SALE_YEAR,omitempty"`
	S1_SALE_MTH          *int64     `json:"s1_SALE_MTH,omitempty"`
	S1_SALE_DAY          *int64     `json:"s1_SALE_DAY,omitempty"`
	S1_SALESAMT          *float64   `json:"s1_SALESAMT,omitempty"`
	S1_SALEDATE          *time.Time `json:"s1_SALEDATE,omitempty"`
	OG_S1_SALEDATE       *int64     `json:"og_s1_SALEDATE,omitempty"`
	S1_GRANTOR           *string    `json:"s1_grantor,omitempty"`
	S1_GRANTOR2          *string    `json:"s1_grantor2,omitempty"`
	S2_DEED_BOOK         *string    `json:"s2_DEED_BOOK,omitempty"`
	S2_DEED_PAGE         *string    `json:"s2_DEED_PAGE,omitempty"`
	S2_DEEDQUAL_CODEDESC *string    `json:"s2_DEEDQUAL_CODEDESC,omitempty"`
	S2_DEEDTYPE          *string    `json:"s2_DEEDTYPE,omitempty"`
	S2_SALE_YEAR         *int64     `json:"s2_SALE_YEAR,omitempty"`
	S2_SALE_MTH          *int64     `json:"s2_SALE_MTH,omitempty"`
	S2_SALE_DAY          *int64     `json:"s2_SALE_DAY,omitempty"`
	S2_SALESAMT          *float64   `json:"s2_SALESAMT,omitempty"`
	S2_SALEDATE          *time.Time `json:"s2_SALEDATE,omitempty"`
	OG_S2_SALEDATE       *int64     `json:"og_s2_SALEDATE,omitempty"`
	S2_GRANTOR           *string    `json:"s2_grantor,omitempty"`
	S2_GRANTOR2          *string    `json:"s2_grantor2,omitempty"`
	S3_DEED_BOOK         *string    `json:"s3_DEED_BOOK,omitempty"`
	S3_DEED_PAGE         *string    `json:"s3_DEED_PAGE,omitempty"`
	S3_DEEDQUAL_CODEDESC *string    `json:"s3_DEEDQUAL_CODEDESC,omitempty"`
	S3_DEEDTYPE          *string    `json:"s3_DEEDTYPE,omitempty"`
	S3_SALE_YEAR         *int64     `json:"s3_SALE_YEAR,omitempty"`
	S3_SALE_MTH          *int64     `json:"s3_SALE_MTH,omitempty"`
	S3_SALE_DAY          *int64     `json:"s3_SALE_DAY,omitempty"`
	S3_SALESAMT          *float64   `json:"s3_SALESAMT,omitempty"`
	S3_SALEDATE          *time.Time `json:"s3_SALEDATE,omitempty"`
	OG_S3_SALEDATE       *int64     `json:"og_s3_SALEDATE,omitempty"`
	S3_GRANTOR           *string    `json:"s3_grantor,omitempty"`
	S3_GRANTOR2          *string    `json:"s3_grantor2,omitempty"`

	IDCOL       *int64 `json:"idcol,omitempty"`
	PARCEL_YEAR *int64 `json:"parcel_year,omitempty"`

	SHAPE          *geom.T  `json:"geometry,omitempty"`
	SHAPE_STArea   *float64 `json:"SHAPE.STArea(),omitempty"`
	SHAPE_STLength *float64 `json:"SHAPE.STLength(),omitempty"`
	SRID           int      `json:"srid"`

	changelog
}
