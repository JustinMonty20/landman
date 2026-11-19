package parcelstore

import (
	"context"
	"fmt"

	"github.com/JustinMonty20/landman/internal/models"
	"github.com/JustinMonty20/landman/internal/parcelowner"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func saveOwnerLabels(ctx context.Context, tx pgx.Tx, id models.ParcelID, owner models.ParcelOwner, mailing models.ParcelAddress) error {
	l := parcelowner.Classify(owner, mailing)
	_, err := tx.Exec(ctx, `INSERT INTO parcel_owner_labels(parcel_id,out_of_state_mailing,entity_name_indicator,corporate_name_indicator,mailing_state,matched_suffix,matched_owner_line,classified_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,now()) ON CONFLICT(parcel_id) DO UPDATE SET
 out_of_state_mailing=EXCLUDED.out_of_state_mailing,entity_name_indicator=EXCLUDED.entity_name_indicator,
 corporate_name_indicator=EXCLUDED.corporate_name_indicator,mailing_state=EXCLUDED.mailing_state,
 matched_suffix=EXCLUDED.matched_suffix,matched_owner_line=EXCLUDED.matched_owner_line,
 classified_at=EXCLUDED.classified_at`, id, l.OutOfStateMailing, l.EntityNameIndicator, l.CorporateNameIndicator, l.MailingState, l.MatchedSuffix, l.MatchedOwnerLine)
	return err
}

// BackfillOwnerLabels classifies stored Union GIS parcels in bounded transactions.
// An interrupted run can be repeated without fetching source data.
func BackfillOwnerLabels(ctx context.Context, pool *pgxpool.Pool) (int64, int64, error) {
	var processed int64
	for {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return 0, 0, err
		}
		rows, err := tx.Query(ctx, `SELECT p.id::text,p.current_name_1,p.current_name_2,p.mailing_state FROM parcels p
 WHERE p.county_id='union' AND p.source_name='union_county_gis'
 AND NOT EXISTS(SELECT 1 FROM parcel_owner_labels l WHERE l.parcel_id=p.id)
 ORDER BY p.id LIMIT 500 FOR UPDATE OF p`)
		if err != nil {
			_ = tx.Rollback(ctx)
			return 0, 0, err
		}
		type input struct {
			id                  models.ParcelID
			name1, name2, state *string
		}
		batch := []input{}
		for rows.Next() {
			var item input
			err = rows.Scan(&item.id, &item.name1, &item.name2, &item.state)
			if err != nil {
				break
			}
			batch = append(batch, item)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err == nil {
			for _, item := range batch {
				err = saveOwnerLabels(ctx, tx, item.id, models.ParcelOwner{CurrentName1: item.name1, CurrentName2: item.name2}, models.ParcelAddress{State: item.state})
				if err != nil {
					break
				}
			}
		}
		if err == nil {
			err = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
		if err != nil {
			return 0, 0, fmt.Errorf("backfill owner labels: %w", err)
		}
		processed += int64(len(batch))
		if len(batch) == 0 {
			break
		}
	}
	var total, labeled int64
	err := pool.QueryRow(ctx, `SELECT count(*),count(l.parcel_id) FROM parcels p LEFT JOIN parcel_owner_labels l ON l.parcel_id=p.id WHERE p.county_id='union' AND p.source_name='union_county_gis'`).Scan(&total, &labeled)
	if err == nil && total != labeled {
		err = fmt.Errorf("owner labels incomplete: %d of %d", labeled, total)
	}
	return processed, total, err
}
