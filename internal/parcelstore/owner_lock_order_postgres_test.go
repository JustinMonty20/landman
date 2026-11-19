//go:build postgres

package parcelstore_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/parcelstore"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOwnerLabelsParcelLockOrderings(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var db string
	if err := pool.QueryRow(ctx, `SELECT current_database()`).Scan(&db); err != nil || !(strings.HasPrefix(db, "landman_owner_test_") || db == "landman_parcelstore_test_ci") {
		t.Fatal("refusing non-task database")
	}
	wait := func(fragment string) {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			var blocked bool
			if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%'||$1||'%' AND pid<>pg_backend_pid())`, fragment).Scan(&blocked); err != nil {
				t.Fatal(err)
			}
			if blocked {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("no observed row lock wait for %s", fragment)
	}
	for _, first := range []string{"backfill", "publication"} {
		t.Run(first+"_first", func(t *testing.T) {
			pid := fmt.Sprintf("owner-lock-%s-%d", first, time.Now().UnixNano())
			var id string
			if err := pool.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,current_name_1,mailing_state) VALUES('union','union','union_county_gis',$1,'ACME LLC','SC') RETURNING id::text`, pid).Scan(&id); err != nil {
				t.Fatal(err)
			}
			defer pool.Exec(context.Background(), `DELETE FROM parcels WHERE id=$1`, id)
			parcel := unionParcel(t)
			parcel.Identity.SourceParcelID = pid
			nc, person := "NC", "PERSON"
			parcel.Owner.CurrentName1, parcel.Mailing.State = &person, &nc
			firstTx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer firstTx.Rollback(context.Background())
			if first == "backfill" {
				var locked string
				if err := firstTx.QueryRow(ctx, `SELECT p.id::text FROM parcels p WHERE p.id=$1 FOR UPDATE OF p`, id).Scan(&locked); err != nil {
					t.Fatal(err)
				}
				if _, err := firstTx.Exec(ctx, `INSERT INTO parcel_owner_labels(parcel_id,out_of_state_mailing,entity_name_indicator,corporate_name_indicator,mailing_state,matched_suffix,matched_owner_line,classified_at) VALUES($1,'yes','matched','not_detected','SC','LLC',1,now())`, id); err != nil {
					t.Fatal(err)
				}
			} else if _, err := parcelstore.SaveParcel(ctx, firstTx, parcel); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			if first == "backfill" {
				go func() {
					tx, err := pool.Begin(ctx)
					if err == nil {
						_, err = parcelstore.SaveParcel(ctx, tx, parcel)
						if err == nil {
							err = tx.Commit(ctx)
						} else {
							_ = tx.Rollback(context.Background())
						}
					}
					done <- err
				}()
				wait("INSERT INTO parcels (")
			} else {
				go func() { _, _, err := parcelstore.BackfillOwnerLabels(ctx, pool); done <- err }()
				wait("FOR UPDATE OF p")
			}
			if err := firstTx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("row-lock ordering timed out", ctx.Err())
			}
			var name, mailing string
			if err := pool.QueryRow(ctx, `SELECT p.current_name_1,l.out_of_state_mailing FROM parcels p JOIN parcel_owner_labels l ON l.parcel_id=p.id WHERE p.id=$1`, id).Scan(&name, &mailing); err != nil || name != "PERSON" || mailing != "no" {
				t.Fatalf("parcel and label diverged: %s %s: %v", name, mailing, err)
			}
		})
	}
}
