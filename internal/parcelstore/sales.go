package parcelstore

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/JustinMonty20/landman/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// ErrParcelNotFound is returned when SaveSales looks up the natural key
// and parcels has no row. SaveSales inserts nothing in that case.
var ErrParcelNotFound = errors.New("parcelstore: parcel not found")

// SaveSales looks up parcels.id by the parcel natural key, then upserts
// each sale on (parcel_id, sequence).
//
// The lookup is the only source of parcel_id. ParcelSale.ID and
// ParcelSale.ParcelID are ignored. Sales the caller omits stay in place.
// Sequence is not renumbered. This function does not delete.
//
// It rejects a missing natural key before it queries. No parcels row
// returns an error wrapping ErrParcelNotFound and inserts nothing.
// An empty sales slice still looks up the parcel and returns a non-nil
// empty slice when the parcel exists.
//
// A nil SaleDate writes NULL. Any other SaleDate is stored as the UTC
// calendar date. OGSaleDate is stored as given.
//
// Returned ids follow the input order. The caller owns tx. SaveSales
// does not begin or commit a transaction. An error leaves tx aborted.
// The caller rolls it back.
func SaveSales(ctx context.Context, tx pgx.Tx, identity models.ParcelIdentity, sales []models.ParcelSale) ([]models.SaleID, error) {
	if err := identityKey(identity); err != nil {
		return nil, err
	}

	var parcelID models.ParcelID
	err := tx.QueryRow(ctx, lookupParcelSQL,
		identity.CountyID,
		identity.SourceName,
		identity.SourceParcelID,
	).Scan(&parcelID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: county_id=%q source_name=%q source_parcel_id=%q",
			ErrParcelNotFound, identity.CountyID, identity.SourceName, identity.SourceParcelID)
	}
	if err != nil {
		return nil, fmt.Errorf("parcelstore: lookup parcel: %w", err)
	}

	ids := make([]models.SaleID, 0, len(sales))
	for _, sale := range sales {
		var id models.SaleID
		err := tx.QueryRow(ctx, saveSaleSQL,
			parcelID,
			sale.Sequence,
			saleCalendarDate(sale.SaleDate),
			sale.OGSaleDate,
			sale.Amount,
			sale.DeedBook,
			sale.DeedPage,
			sale.DeedType,
			sale.DeedQuality,
			sale.Grantor,
			sale.Grantor2,
		).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("parcelstore: save sale sequence %d: %w", sale.Sequence, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// saleCalendarDate returns the UTC calendar date for t.
// A nil t returns an invalid Date, which binds as NULL.
func saleCalendarDate(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	year, month, day := t.UTC().Date()
	return pgtype.Date{
		Time:  time.Date(year, month, day, 0, 0, 0, 0, time.UTC),
		Valid: true,
	}
}

const lookupParcelSQL = `
SELECT id
FROM parcels
WHERE county_id = $1
  AND source_name = $2
  AND source_parcel_id = $3
`

const saveSaleSQL = `
INSERT INTO parcel_sales (
	parcel_id,
	sequence,
	sale_date,
	og_sale_date,
	amount,
	deed_book,
	deed_page,
	deed_type,
	deed_quality,
	grantor,
	grantor_2
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)
ON CONFLICT (parcel_id, sequence)
DO UPDATE SET
	sale_date = EXCLUDED.sale_date,
	og_sale_date = EXCLUDED.og_sale_date,
	amount = EXCLUDED.amount,
	deed_book = EXCLUDED.deed_book,
	deed_page = EXCLUDED.deed_page,
	deed_type = EXCLUDED.deed_type,
	deed_quality = EXCLUDED.deed_quality,
	grantor = EXCLUDED.grantor,
	grantor_2 = EXCLUDED.grantor_2,
	updated_at = now()
RETURNING id
`
