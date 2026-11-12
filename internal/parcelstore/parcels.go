// Package parcelstore writes parcels and parcel sales on the caller's transaction.
package parcelstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/JustinMonty20/landman/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twpayne/go-geom/encoding/ewkb"
)

// ShapeWriter stores all pages in one transaction so failed imports leave no partial rows.
type ShapeWriter struct {
	pool *pgxpool.Pool
	tx   pgx.Tx
}

// NewShapeWriter opens one import transaction from the supplied connection string.
func NewShapeWriter(ctx context.Context, connectionString string) (*ShapeWriter, error) {
	pool, err := pgxpool.New(ctx, connectionString)
	if err != nil {
		return nil, err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		pool.Close()
		return nil, err
	}
	_, err = tx.Exec(ctx, `CREATE TEMP TABLE parcel_shape_parts (
		source_parcel_id TEXT NOT NULL, shape GEOMETRY(Geometry, 3857) NOT NULL,
		likely_vacant BOOLEAN NOT NULL
	) ON COMMIT DROP`)
	if err != nil {
		_ = tx.Rollback(ctx)
		pool.Close()
		return nil, err
	}
	return &ShapeWriter{pool: pool, tx: tx}, nil
}

// SavePart stores a mapped parcel and its original shape on the same transaction.
func (w *ShapeWriter) SavePart(ctx context.Context, parcel models.GISParcel, likelyVacant bool) error {
	shape, _, err := encodeShape(parcel.Geometry)
	if err != nil {
		return err
	}
	if len(shape) == 0 {
		return fmt.Errorf("parcelstore: %s has no rings", parcel.Identity.SourceParcelID)
	}
	if _, err := SaveParcel(ctx, w.tx, parcel, likelyVacant); err != nil {
		return err
	}
	_, err = w.tx.Exec(ctx, `INSERT INTO parcel_shape_parts (source_parcel_id, shape, likely_vacant)
		VALUES ($1, ST_GeomFromEWKB($2), $3)`, parcel.Identity.SourceParcelID, shape, likelyVacant)
	return err
}

// Commit combines parts across pages and replaces the shape and vacancy flag.
func (w *ShapeWriter) Commit(ctx context.Context) error {
	_, err := w.tx.Exec(ctx, `UPDATE parcels AS p SET shape = parts.shape,
		likely_vacant = parts.likely_vacant, updated_at = now()
		FROM (SELECT source_parcel_id, ST_Multi(ST_Union(shape)) AS shape,
			bool_or(likely_vacant) AS likely_vacant
			FROM parcel_shape_parts GROUP BY source_parcel_id) AS parts
		WHERE p.county_id = 'union' AND p.source_name = 'union_county_gis'
		AND p.source_parcel_id = parts.source_parcel_id`)
	if err != nil {
		return err
	}
	return w.tx.Commit(ctx)
}

// Close releases the connection and rolls back an unfinished import.
func (w *ShapeWriter) Close() {
	_ = w.tx.Rollback(context.Background())
	w.pool.Close()
}

// ErrMissingNaturalKey is returned when TrimSpace of CountyID, SourceName,
// or SourceParcelID is empty. CountyCode is not part of this check.
// SaveParcel and SaveSales return it before they query.
var ErrMissingNaturalKey = errors.New("parcelstore: missing parcel natural key")

// defaultStorageSRID is the parcels.srid column default.
const defaultStorageSRID = 3857

// SaveParcel inserts one parcels row, or updates the mapped columns when
// (county_id, source_name, source_parcel_id) already exists.
//
// It ignores parcel.ID and parcel.Sales. It omits id, created_at, and
// updated_at on insert so the column defaults fire. On conflict it does
// not assign id or created_at, and it sets updated_at to now(). It does
// not delete. It does not write parcel_sales.
//
// A natural key is missing when TrimSpace of CountyID, SourceName, or
// SourceParcelID is empty. The stored text is the caller's original string.
//
// A nil Geometry.Shape writes NULL. A shape whose SRID is 0 is an error.
// A non-zero Geometry.SRID that disagrees with the shape SRID is an error.
// The stored srid is 3857 when the shape is nil and Geometry.SRID is 0,
// and the shape SRID when the shape is present. A nil or empty
// source_extras value is stored as {}.
//
// The caller owns tx. SaveParcel does not begin or commit a transaction.
// An error leaves tx aborted. The caller rolls it back.
func SaveParcel(ctx context.Context, tx pgx.Tx, parcel models.GISParcel, likelyVacant ...bool) (models.ParcelID, error) {
	if err := identityKey(parcel.Identity); err != nil {
		return "", err
	}
	shape, srid, err := encodeShape(parcel.Geometry)
	if err != nil {
		return "", err
	}
	extras := parcel.SourceExtras
	if len(extras) == 0 {
		extras = json.RawMessage(`{}`)
	}

	vacant := false
	if len(likelyVacant) > 0 {
		vacant = likelyVacant[0]
	}
	var id models.ParcelID
	err = tx.QueryRow(ctx, saveParcelSQL,
		parcel.Identity.CountyID,
		parcel.Identity.CountyCode,
		parcel.Identity.SourceName,
		parcel.Identity.SourceParcelID,
		parcel.Identity.ParcelNumber,
		parcel.Identity.AccountNumber,
		parcel.Identity.ParcelYear,
		parcel.Owner.CurrentName1,
		parcel.Owner.CurrentName2,
		parcel.Owner.TaxName1,
		parcel.Owner.TaxName2,
		parcel.Owner.TaxOwnerID1,
		parcel.Owner.TaxOwnerID2,
		parcel.Mailing.Line1,
		parcel.Mailing.Line2,
		parcel.Mailing.City,
		parcel.Mailing.State,
		parcel.Mailing.PostalCode,
		parcel.Situs.Line1,
		parcel.Situs.Line2,
		parcel.Situs.City,
		parcel.Situs.State,
		parcel.Situs.PostalCode,
		parcel.Legal.Description,
		parcel.Legal.PlatBook,
		parcel.Legal.PlatPage,
		parcel.Legal.DeedBook,
		parcel.Legal.DeedPage,
		parcel.Land.LandCode,
		parcel.Land.LandType,
		parcel.Land.PropertyUse,
		parcel.Land.Subdivision,
		parcel.Land.MappedAcres,
		parcel.Land.GrossAcres,
		parcel.Land.ValuedAcres,
		parcel.Land.ValuedSqFt,
		parcel.Building.YearBuilt,
		parcel.Building.SqFt,
		parcel.Building.Basement,
		parcel.Building.QualityCode,
		parcel.Building.Structure,
		parcel.Building.StructureStyle,
		parcel.Valuation.Land,
		parcel.Valuation.Improvement,
		parcel.Valuation.Total,
		parcel.Valuation.Assessed,
		shape,
		srid,
		parcel.Geometry.SourceSRID,
		parcel.Geometry.SourceArea,
		parcel.Geometry.SourceLength,
		extras,
		vacant,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("parcelstore: save parcel: %w", err)
	}
	return id, nil
}

func identityKey(id models.ParcelIdentity) error {
	var missing []string
	if strings.TrimSpace(id.CountyID) == "" {
		missing = append(missing, "county_id")
	}
	if strings.TrimSpace(id.SourceName) == "" {
		missing = append(missing, "source_name")
	}
	if strings.TrimSpace(id.SourceParcelID) == "" {
		missing = append(missing, "source_parcel_id")
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrMissingNaturalKey, strings.Join(missing, ", "))
}

// A nil shape returns a nil byte slice, which the INSERT binds as NULL.
func encodeShape(g models.GISGeometry) ([]byte, int, error) {
	if g.Shape == nil || *g.Shape == nil {
		srid := g.SRID
		if srid == 0 {
			srid = defaultStorageSRID
		}
		return nil, srid, nil
	}

	shape := *g.Shape
	shapeSRID := shape.SRID()
	if shapeSRID == 0 {
		return nil, 0, errors.New("parcelstore: geometry shape SRID is 0")
	}
	if g.SRID != 0 && g.SRID != shapeSRID {
		return nil, 0, fmt.Errorf("parcelstore: geometry SRID %d disagrees with shape SRID %d", g.SRID, shapeSRID)
	}
	encoded, err := ewkb.Marshal(shape, ewkb.NDR)
	if err != nil {
		return nil, 0, fmt.Errorf("parcelstore: encode geometry: %w", err)
	}
	return encoded, shapeSRID, nil
}

// saveParcelSQL names parcels columns. Address, legal, and valuation
// db tags do not match those columns, so the statement does not follow the tags.
const saveParcelSQL = `
INSERT INTO parcels (
	county_id,
	county_code,
	source_name,
	source_parcel_id,
	parcel_number,
	account_number,
	parcel_year,
	current_name_1,
	current_name_2,
	tax_name_1,
	tax_name_2,
	tax_owner_id_1,
	tax_owner_id_2,
	mailing_line_1,
	mailing_line_2,
	mailing_city,
	mailing_state,
	mailing_postal_code,
	situs_line_1,
	situs_line_2,
	situs_city,
	situs_state,
	situs_postal_code,
	legal_description,
	plat_book,
	plat_page,
	deed_book,
	deed_page,
	land_code,
	land_type,
	property_use,
	subdivision,
	mapped_acres,
	gross_acres,
	valued_acres,
	valued_sqft,
	year_built,
	sqft,
	basement,
	quality_code,
	structure,
	structure_style,
	valuation_land,
	valuation_improvement,
	valuation_total,
	valuation_assessed,
	shape,
	srid,
	source_srid,
	source_area,
	source_length,
	source_extras,
	likely_vacant
) VALUES (
	$1, $2, $3, $4, $5, $6, $7,
	$8, $9, $10, $11, $12, $13,
	$14, $15, $16, $17, $18,
	$19, $20, $21, $22, $23,
	$24, $25, $26, $27, $28,
	$29, $30, $31, $32, $33, $34, $35, $36,
	$37, $38, $39, $40, $41, $42,
	$43, $44, $45, $46,
	ST_GeomFromEWKB($47),
	$48,
	$49, $50, $51,
	COALESCE($52::jsonb, '{}'::jsonb),
	$53
)
ON CONFLICT (county_id, source_name, source_parcel_id)
DO UPDATE SET
	county_code = EXCLUDED.county_code,
	parcel_number = EXCLUDED.parcel_number,
	account_number = EXCLUDED.account_number,
	parcel_year = EXCLUDED.parcel_year,
	current_name_1 = EXCLUDED.current_name_1,
	current_name_2 = EXCLUDED.current_name_2,
	tax_name_1 = EXCLUDED.tax_name_1,
	tax_name_2 = EXCLUDED.tax_name_2,
	tax_owner_id_1 = EXCLUDED.tax_owner_id_1,
	tax_owner_id_2 = EXCLUDED.tax_owner_id_2,
	mailing_line_1 = EXCLUDED.mailing_line_1,
	mailing_line_2 = EXCLUDED.mailing_line_2,
	mailing_city = EXCLUDED.mailing_city,
	mailing_state = EXCLUDED.mailing_state,
	mailing_postal_code = EXCLUDED.mailing_postal_code,
	situs_line_1 = EXCLUDED.situs_line_1,
	situs_line_2 = EXCLUDED.situs_line_2,
	situs_city = EXCLUDED.situs_city,
	situs_state = EXCLUDED.situs_state,
	situs_postal_code = EXCLUDED.situs_postal_code,
	legal_description = EXCLUDED.legal_description,
	plat_book = EXCLUDED.plat_book,
	plat_page = EXCLUDED.plat_page,
	deed_book = EXCLUDED.deed_book,
	deed_page = EXCLUDED.deed_page,
	land_code = EXCLUDED.land_code,
	land_type = EXCLUDED.land_type,
	property_use = EXCLUDED.property_use,
	subdivision = EXCLUDED.subdivision,
	mapped_acres = EXCLUDED.mapped_acres,
	gross_acres = EXCLUDED.gross_acres,
	valued_acres = EXCLUDED.valued_acres,
	valued_sqft = EXCLUDED.valued_sqft,
	year_built = EXCLUDED.year_built,
	sqft = EXCLUDED.sqft,
	basement = EXCLUDED.basement,
	quality_code = EXCLUDED.quality_code,
	structure = EXCLUDED.structure,
	structure_style = EXCLUDED.structure_style,
	valuation_land = EXCLUDED.valuation_land,
	valuation_improvement = EXCLUDED.valuation_improvement,
	valuation_total = EXCLUDED.valuation_total,
	valuation_assessed = EXCLUDED.valuation_assessed,
	shape = EXCLUDED.shape,
	srid = EXCLUDED.srid,
	source_srid = EXCLUDED.source_srid,
	source_area = EXCLUDED.source_area,
	source_length = EXCLUDED.source_length,
	source_extras = EXCLUDED.source_extras,
	likely_vacant = EXCLUDED.likely_vacant,
	updated_at = now()
RETURNING id
`
