package storage

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/JustinMonty20/landman/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/twpayne/go-geom/encoding/wkb"
)

// ParcelInsertError identifies a parcel that failed during batch persistence.
type ParcelInsertError struct {
	ParcelID string
	Err      error
}

// BatchInsertResult summarizes a batch persistence attempt.
type BatchInsertResult struct {
	Inserted int
	Errors   []ParcelInsertError
}

type beginner interface {
	Begin(ctx context.Context) (transaction, error)
}

type transaction interface {
	Begin(ctx context.Context) (transaction, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) row
}

type row interface {
	Scan(dest ...any) error
}

type pgxBeginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type pgxBeginnerAdapter struct {
	db pgxBeginner
}

func (p pgxBeginnerAdapter) Begin(ctx context.Context) (transaction, error) {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return pgxTxAdapter{tx: tx}, nil
}

type pgxTxAdapter struct {
	tx pgx.Tx
}

func (p pgxTxAdapter) Begin(ctx context.Context) (transaction, error) {
	tx, err := p.tx.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return pgxTxAdapter{tx: tx}, nil
}

func (p pgxTxAdapter) Commit(ctx context.Context) error   { return p.tx.Commit(ctx) }
func (p pgxTxAdapter) Rollback(ctx context.Context) error { return p.tx.Rollback(ctx) }
func (p pgxTxAdapter) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	return p.tx.Exec(ctx, sql, arguments...)
}
func (p pgxTxAdapter) QueryRow(ctx context.Context, sql string, args ...any) row {
	return p.tx.QueryRow(ctx, sql, args...)
}

// PostgresParcelStore persists normalized parcels into the existing parcels schema.
type PostgresParcelStore struct {
	beginner beginner
}

// NewPostgresParcelStore creates a parcel store backed by a pgx pool or connection.
func NewPostgresParcelStore(db pgxBeginner) *PostgresParcelStore {
	return &PostgresParcelStore{beginner: pgxBeginnerAdapter{db: db}}
}

func newPostgresParcelStore(beginner beginner) *PostgresParcelStore {
	return &PostgresParcelStore{beginner: beginner}
}

// InsertBatch inserts a fetched batch in one transaction. Each parcel is isolated by a nested transaction savepoint.
func (s *PostgresParcelStore) InsertBatch(ctx context.Context, parcels []*models.GISParcel) (BatchInsertResult, error) {
	var result BatchInsertResult
	if len(parcels) == 0 {
		return result, nil
	}
	if s == nil || s.beginner == nil {
		return result, fmt.Errorf("parcel store cannot be nil")
	}

	tx, err := s.beginner.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)

	for _, parcel := range parcels {
		parcelID := sourceParcelID(parcel)
		sp, err := tx.Begin(ctx)
		if err != nil {
			return result, err
		}

		if err := insertParcelWithSales(ctx, sp, parcel); err != nil {
			_ = sp.Rollback(ctx)
			result.Errors = append(result.Errors, ParcelInsertError{ParcelID: parcelID, Err: err})
			continue
		}
		if err := sp.Commit(ctx); err != nil {
			_ = sp.Rollback(ctx)
			result.Errors = append(result.Errors, ParcelInsertError{ParcelID: parcelID, Err: err})
			continue
		}
		result.Inserted++
	}

	if err := tx.Commit(ctx); err != nil {
		return result, err
	}
	return result, nil
}

func insertParcelWithSales(ctx context.Context, tx transaction, parcel *models.GISParcel) error {
	if err := validateParcel(parcel); err != nil {
		return err
	}

	args, err := parcelArgs(parcel)
	if err != nil {
		return err
	}

	var parcelUUID string
	if err := tx.QueryRow(ctx, insertParcelSQL, args...).Scan(&parcelUUID); err != nil {
		return err
	}

	for _, sale := range parcel.Sales {
		if _, err := tx.Exec(ctx, insertParcelSaleSQL,
			parcelUUID,
			sale.Sequence,
			sale.SaleDate,
			sale.OGSaleDate,
			sale.Amount,
			sale.DeedBook,
			sale.DeedPage,
			sale.DeedType,
			sale.DeedQuality,
			sale.Grantor,
			sale.Grantor2,
		); err != nil {
			return err
		}
	}
	return nil
}

func validateParcel(parcel *models.GISParcel) error {
	if parcel == nil {
		return fmt.Errorf("parcel cannot be nil")
	}
	if parcel.Identity.CountyID == "" {
		return fmt.Errorf("county_id cannot be empty")
	}
	if parcel.Identity.CountyCode == "" {
		return fmt.Errorf("county_code cannot be empty")
	}
	if parcel.Identity.SourceName == "" {
		return fmt.Errorf("source_name cannot be empty")
	}
	if parcel.Identity.SourceParcelID == "" {
		return fmt.Errorf("source_parcel_id cannot be empty")
	}
	if _, err := smallInt("parcel_year", parcel.Identity.ParcelYear, 0, 2200); err != nil {
		return err
	}
	if _, err := smallInt("year_built", parcel.Building.YearBuilt, 0, 2200); err != nil {
		return err
	}
	if parcel.Geometry.Shape != nil && parcel.Geometry.SRID <= 0 {
		return fmt.Errorf("srid must be positive when shape is present")
	}
	return nil
}

func parcelArgs(parcel *models.GISParcel) ([]any, error) {
	parcelYear, err := smallInt("parcel_year", parcel.Identity.ParcelYear, 0, 2200)
	if err != nil {
		return nil, err
	}
	yearBuilt, err := smallInt("year_built", parcel.Building.YearBuilt, 0, 2200)
	if err != nil {
		return nil, err
	}
	shape, err := shapeWKB(parcel)
	if err != nil {
		return nil, err
	}
	sourceExtras, err := json.Marshal(parcel.SourceExtras)
	if err != nil {
		return nil, err
	}

	return []any{
		parcel.Identity.CountyID,
		parcel.Identity.CountyCode,
		parcel.Identity.SourceName,
		parcel.Identity.SourceParcelID,
		parcel.Identity.ParcelNumber,
		parcel.Identity.AccountNumber,
		parcelYear,
		parcel.Owner.CurrentName1,
		parcel.Owner.CurrentName2,
		parcel.Owner.TaxName1,
		parcel.Owner.TaxName2,
		intText(parcel.Owner.TaxOwnerID1),
		intText(parcel.Owner.TaxOwnerID2),
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
		yearBuilt,
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
		parcel.Geometry.SRID,
		parcel.Geometry.SourceSRID,
		parcel.Geometry.SourceArea,
		parcel.Geometry.SourceLength,
		sourceExtras,
	}, nil
}

func smallInt(name string, value *int64, min, max int64) (*int16, error) {
	if value == nil {
		return nil, nil
	}
	if *value < min || *value > max {
		return nil, fmt.Errorf("%s must be between %d and %d", name, min, max)
	}
	v := int16(*value)
	return &v, nil
}

func intText(value *int64) *string {
	if value == nil {
		return nil
	}
	text := strconv.FormatInt(*value, 10)
	return &text
}

func shapeWKB(parcel *models.GISParcel) ([]byte, error) {
	if parcel.Geometry.Shape == nil {
		return nil, nil
	}
	return wkb.Marshal(*parcel.Geometry.Shape, binary.LittleEndian)
}

func sourceParcelID(parcel *models.GISParcel) string {
	if parcel == nil {
		return ""
	}
	return parcel.Identity.SourceParcelID
}

const insertParcelSQL = `
INSERT INTO parcels (
    county_id, county_code, source_name, source_parcel_id, parcel_number, account_number, parcel_year,
    current_name_1, current_name_2, tax_name_1, tax_name_2, tax_owner_id_1, tax_owner_id_2,
    mailing_line_1, mailing_line_2, mailing_city, mailing_state, mailing_postal_code,
    situs_line_1, situs_line_2, situs_city, situs_state, situs_postal_code,
    legal_description, plat_book, plat_page, deed_book, deed_page,
    land_code, land_type, property_use, subdivision, mapped_acres, gross_acres, valued_acres, valued_sqft,
    year_built, sqft, basement, quality_code, structure, structure_style,
    valuation_land, valuation_improvement, valuation_total, valuation_assessed,
    shape, srid, source_srid, source_area, source_length, source_extras
) VALUES (
    $1, $2, $3, $4, $5, $6, $7,
    $8, $9, $10, $11, $12, $13,
    $14, $15, $16, $17, $18,
    $19, $20, $21, $22, $23,
    $24, $25, $26, $27, $28,
    $29, $30, $31, $32, $33, $34, $35, $36,
    $37, $38, $39, $40, $41, $42,
    $43, $44, $45, $46,
    CASE WHEN $47::bytea IS NULL THEN NULL ELSE ST_SetSRID(ST_GeomFromWKB($47), $48) END,
    $48, $49, $50, $51, $52
)
RETURNING id`

const insertParcelSaleSQL = `
INSERT INTO parcel_sales (
    parcel_id, sequence, sale_date, og_sale_date, amount, deed_book, deed_page, deed_type, deed_quality, grantor, grantor_2
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)`
