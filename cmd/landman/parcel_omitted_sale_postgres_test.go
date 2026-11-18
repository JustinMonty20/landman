//go:build postgres

package main

import "testing"

func TestReviewOmittedSaleDeletionAndForeignKeys(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	f := &parcelHTTPFixture{features: []map[string]interface{}{
		feature(1, "A", "one", true, 1), feature(2, "B", "two", true, 1),
	}}
	source := f.source(t, 2)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	var retainedSale string
	if err := pool.QueryRow(ctx, `SELECT s.id::text FROM parcel_sales s JOIN parcels p ON p.id=s.parcel_id WHERE p.source_parcel_id='B'`).Scan(&retainedSale); err != nil {
		t.Fatal(err)
	}
	f.features[0] = feature(1, "A", "one", true, 0)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	var removed, orphaned int
	var retainedAfter string
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM parcel_sales s JOIN parcels p ON p.id=s.parcel_id WHERE p.source_parcel_id='A'`).Scan(&removed); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM parcel_sales s LEFT JOIN parcels p ON p.id=s.parcel_id WHERE p.id IS NULL`).Scan(&orphaned); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT s.id::text FROM parcel_sales s JOIN parcels p ON p.id=s.parcel_id WHERE p.source_parcel_id='B'`).Scan(&retainedAfter); err != nil {
		t.Fatal(err)
	}
	if removed != 0 || orphaned != 0 || retainedAfter != retainedSale {
		t.Fatalf("omitted sale rows=%d orphaned sale rows=%d unrelated UUID stable=%v", removed, orphaned, retainedAfter == retainedSale)
	}
	t.Logf("omitted sale rows=%d; orphaned sale FK rows=%d; unrelated sale UUID stable=%v", removed, orphaned, retainedAfter == retainedSale)
}
