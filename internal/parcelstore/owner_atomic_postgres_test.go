//go:build postgres

package parcelstore_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/parcelstore"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOwnerLabelsParcelTransactionRollback(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL required")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var db string
	if err = pool.QueryRow(context.Background(), `SELECT current_database()`).Scan(&db); err != nil || !strings.HasPrefix(db, "landman_owner_test") {
		t.Fatal("refusing non-task database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	parcel := unionParcel(t)
	nc, person := "NC", "PERSON"
	parcel.Owner.CurrentName1 = &person
	parcel.Mailing.State = &nc
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id, err := parcelstore.SaveParcel(ctx, tx, parcel)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), `DELETE FROM parcels WHERE id=$1`, id)
	sc, corp := "SC", "ACME INC"
	parcel.Owner.CurrentName1 = &corp
	parcel.Mailing.State = &sc
	tx, err = pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = parcelstore.SaveParcel(ctx, tx, parcel); err != nil {
		t.Fatal(err)
	}
	if err = tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var state, name, mail, entity string
	err = pool.QueryRow(ctx, `SELECT p.mailing_state,p.current_name_1,l.out_of_state_mailing,l.entity_name_indicator FROM parcels p JOIN parcel_owner_labels l ON l.parcel_id=p.id WHERE p.id=$1`, id).Scan(&state, &name, &mail, &entity)
	if err != nil || state != "NC" || name != "PERSON" || mail != "no" || entity != "not_detected" {
		t.Fatalf("rollback: %s %s %s %s %v", state, name, mail, entity, err)
	}
}
