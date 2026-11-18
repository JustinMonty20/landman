package parcelevidence_test

import (
	"testing"

	"github.com/JustinMonty20/landman/internal/models"
	"github.com/JustinMonty20/landman/internal/parcelevidence"
)

func TestStorageNumericFingerprintRoundsHalfAwayFromZero(t *testing.T) {
	for _, tc := range []struct{ input, stored float64 }{
		{1.234, 1.23}, {1.235, 1.24}, {-1.234, -1.23}, {-1.235, -1.24},
		{0.005, 0.01}, {-0.005, -0.01}, {0, 0},
	} {
		amount := tc.input
		parcel := models.GISParcel{Sales: []models.ParcelSale{{Sequence: 1, Amount: &amount}}}
		if err := parcelevidence.NormalizeStorageNumbers(&parcel); err != nil {
			t.Fatal(err)
		}
		if got := *parcel.Sales[0].Amount; got != tc.stored {
			t.Errorf("storage amount for %v = %v, want %v", tc.input, got, tc.stored)
		}
	}

	fingerprint := func(amount float64) string {
		t.Helper()
		parcel := models.GISParcel{Sales: []models.ParcelSale{{Sequence: 1, Amount: &amount}}}
		hash, err := parcelevidence.NormalizedHash(parcel, false, "same-geometry")
		if err != nil {
			t.Fatal(err)
		}
		return hash
	}
	if fingerprint(-1.234) == fingerprint(-1.235) {
		t.Fatal("different stored amounts share a fingerprint")
	}
	if fingerprint(-1.235) != fingerprint(-1.24) {
		t.Fatal("equivalent stored amounts have different fingerprints")
	}
}
