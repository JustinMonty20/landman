package main

import (
	"reflect"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector"
)

func TestEligibleParcelRecords(t *testing.T) {
	records := []connector.RawRecord{
		{ParcelID: "P1"},
		{ParcelID: "P2"},
		{ParcelID: "P1"},
		{ParcelID: ""},
	}

	eligible, ids := eligibleParcelRecords(records, func(record connector.RawRecord) bool {
		return record.ParcelID != "P2"
	})

	if len(eligible) != 3 {
		t.Fatalf("eligible count = %d, want 3", len(eligible))
	}
	wantIDs := []string{"P1"}
	if !reflect.DeepEqual(ids, wantIDs) {
		t.Fatalf("ids = %v, want %v", ids, wantIDs)
	}
}
