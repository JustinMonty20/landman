//go:build postgres

package parcelstore_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/models"
	"github.com/JustinMonty20/landman/internal/parcelstore"
)

func TestSaveSaleKeepsID(t *testing.T) {
	ctx, tx := testTx(t)
	parcel := unionParcel(t)
	parcelID, err := parcelstore.SaveParcel(ctx, tx, parcel)
	if err != nil {
		t.Fatalf("save parcel: %v", err)
	}

	zone := time.FixedZone("UTC-8", -8*60*60)
	sold := time.Date(2024, 6, 15, 20, 30, 0, 0, zone)
	localYear, localMonth, localDay := sold.Date()
	utcYear, utcMonth, utcDay := sold.UTC().Date()
	if localYear == utcYear && localMonth == utcMonth && localDay == utcDay {
		t.Fatal("fixture UTC date matches the local date")
	}
	if sold.Hour() == 0 && sold.Minute() == 0 && sold.Second() == 0 {
		t.Fatal("fixture clock time is zero")
	}

	callerSaleID := models.SaleID("caller-sale-id")
	callerParcelID := models.ParcelID("caller-sale-parcel-id")
	amount := 10.0
	sale := models.ParcelSale{
		ID:       callerSaleID,
		ParcelID: callerParcelID,
		Sequence: 1,
		SaleDate: &sold,
		Amount:   &amount,
	}

	first, err := parcelstore.SaveSales(ctx, tx, parcel.Identity, []models.ParcelSale{sale})
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("first ids %d", len(first))
	}

	amount = 99
	sale.Amount = &amount
	second, err := parcelstore.SaveSales(ctx, tx, parcel.Identity, []models.ParcelSale{sale})
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("second ids %d", len(second))
	}
	fmt.Printf("%s %s\n", first[0], second[0])

	if first[0] == "" || second[0] == "" {
		t.Fatalf("empty sale id %q %q", first[0], second[0])
	}
	if first[0] != second[0] {
		t.Fatalf("sale id changed from %s to %s", first[0], second[0])
	}
	if first[0] == callerSaleID || second[0] == callerSaleID {
		t.Fatalf("stored sale id %s is the caller id", first[0])
	}

	var count int
	err = tx.QueryRow(ctx, `
		SELECT count(*)
		FROM parcel_sales
		WHERE parcel_id = $1
	`, parcelID).Scan(&count)
	if err != nil {
		t.Fatalf("count sales: %v", err)
	}
	if count != 1 {
		t.Fatalf("sale rows %d", count)
	}

	var (
		storedParcelID models.ParcelID
		gotAmount      float64
		gotDate        time.Time
	)
	err = tx.QueryRow(ctx, `
		SELECT parcel_id, amount, sale_date
		FROM parcel_sales
		WHERE id = $1
	`, first[0]).Scan(&storedParcelID, &gotAmount, &gotDate)
	if err != nil {
		t.Fatalf("read sale: %v", err)
	}
	if storedParcelID != parcelID {
		t.Fatalf("stored parcel id %s, SaveParcel returned %s", storedParcelID, parcelID)
	}
	if storedParcelID == callerParcelID {
		t.Fatalf("stored parcel id is the caller id %s", callerParcelID)
	}
	if gotAmount != 99 {
		t.Fatalf("amount %v", gotAmount)
	}
	gotYear, gotMonth, gotDay := gotDate.UTC().Date()
	if gotYear != utcYear || gotMonth != utcMonth || gotDay != utcDay {
		t.Fatalf("sale_date %04d-%02d-%02d, UTC date %04d-%02d-%02d", gotYear, gotMonth, gotDay, utcYear, utcMonth, utcDay)
	}
}

func TestSaveSalesKeepsOmittedRow(t *testing.T) {
	ctx, tx := testTx(t)
	parcel := unionParcel(t)
	parcelID, err := parcelstore.SaveParcel(ctx, tx, parcel)
	if err != nil {
		t.Fatalf("save parcel: %v", err)
	}

	firstAmount := 10.0
	secondAmount := 20.0
	first, err := parcelstore.SaveSales(ctx, tx, parcel.Identity, []models.ParcelSale{
		{Sequence: 1, Amount: &firstAmount},
		{Sequence: 2, Amount: &secondAmount},
	})
	if err != nil {
		t.Fatalf("save two sales: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("sale ids %d", len(first))
	}

	updated := 11.0
	second, err := parcelstore.SaveSales(ctx, tx, parcel.Identity, []models.ParcelSale{
		{Sequence: 1, Amount: &updated},
	})
	if err != nil {
		t.Fatalf("save first sale: %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("second call ids %d", len(second))
	}
	if second[0] != first[0] {
		t.Fatalf("first sale id changed from %s to %s", first[0], second[0])
	}

	var count int
	err = tx.QueryRow(ctx, `
		SELECT count(*)
		FROM parcel_sales
		WHERE parcel_id = $1
	`, parcelID).Scan(&count)
	if err != nil {
		t.Fatalf("count sales: %v", err)
	}
	if count != 2 {
		t.Fatalf("sale rows %d", count)
	}

	var keptAmount float64
	err = tx.QueryRow(ctx, `
		SELECT amount
		FROM parcel_sales
		WHERE id = $1
	`, first[1]).Scan(&keptAmount)
	if err != nil {
		t.Fatalf("read omitted sale: %v", err)
	}
	if keptAmount != secondAmount {
		t.Fatalf("omitted amount %v", keptAmount)
	}
}
