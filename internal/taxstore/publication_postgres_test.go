//go:build postgres

package taxstore_test

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/taxstore"
)

func TestPublishBillFileCompleteAndRollback(t *testing.T) {
	pool := billingDatabase(t)
	ctx := context.Background()
	_, firstMeta, _ := billingRows(t, billingHeader+"PUBLICATION-PARCEL,RE,2026,1.00,ROAD,OWNER,BOX,CITY,NC,28000\n")
	_, secondMeta, _ := billingRows(t, billingHeader+"PUBLICATION-PARCEL,RE,2026,2.00,ROAD,OWNER,BOX,CITY,NC,28000\n")
	_, emptyMeta, _ := billingRows(t, billingHeader)
	var parcelID string
	if err := pool.QueryRow(ctx, `INSERT INTO parcels (county_id,county_code,source_name,source_parcel_id,likely_vacant)
VALUES ('union','179','union_county_gis','PUBLICATION-PARCEL',true) RETURNING id::text`).Scan(&parcelID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM parcels WHERE id::text=$1`, parcelID)
		_, _ = pool.Exec(ctx, `DELETE FROM tax_file_imports WHERE source_name='union_billing' AND source_url=$1`, firstMeta.URL)
	})
	encode := func(rows []union.BillRow) []byte {
		t.Helper()
		var stream bytes.Buffer
		encoder := gob.NewEncoder(&stream)
		for _, row := range rows {
			if err := encoder.Encode(row); err != nil {
				t.Fatal(err)
			}
		}
		return stream.Bytes()
	}
	rows := make([]union.BillRow, 513)
	for i := range rows {
		rows[i] = union.BillRow{ParcelKey: fmt.Sprintf("UNMATCHED-%d", i), Year: 2026, TotalTaxes: 100}
	}
	rows[0].ParcelKey = "PUBLICATION-PARCEL"
	first, err := taxstore.PublishBillFile(ctx, pool, bytes.NewReader(encode(rows)), firstMeta, 0)
	if err != nil || first.Saved != 1 || first.Unmatched != 512 || first.Deleted != 0 {
		t.Fatalf("first publication: %+v %v", first, err)
	}
	changed := []union.BillRow{{ParcelKey: "PUBLICATION-PARCEL", Year: 2026, TotalTaxes: 200}}
	broken := encode(append(rows, changed...))
	if _, err := taxstore.PublishBillFile(ctx, pool, bytes.NewReader(broken[:len(broken)-1]), secondMeta, 0); err == nil {
		t.Fatal("accepted incomplete staged file")
	}
	var amount string
	var active int
	if err := pool.QueryRow(ctx, `SELECT total_taxes::text FROM tax_bills WHERE parcel_id::text=$1`, parcelID).Scan(&amount); err != nil || amount != "1.00" {
		t.Fatalf("rollback changed bill: %s %v", amount, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM tax_file_imports WHERE source_name='union_billing' AND active AND file_sha256=$1`, firstMeta.SHA256[:]).Scan(&active); err != nil || active != 1 {
		t.Fatalf("rollback changed active import: %d %v", active, err)
	}
	last, err := taxstore.PublishBillFile(ctx, pool, bytes.NewReader(nil), emptyMeta, 0)
	if err != nil || last.Saved != 0 || last.Deleted != 1 {
		t.Fatalf("empty publication: %+v %v", last, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM tax_bills WHERE parcel_id::text=$1`, parcelID).Scan(&active); err != nil || active != 0 {
		t.Fatalf("omitted row: %d %v", active, err)
	}
}
