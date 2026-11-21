//go:build parcelrepairverify

package main

import (
	"context"
	"testing"
)

func TestParcelRepairVerificationMode(t *testing.T) {
	for _, tt := range []struct {
		name, mode, dsn string
		accept          bool
	}{
		{"loopback", "replay", "postgres://audit@127.0.0.1:15432/landman_parcel_repair_test?sslmode=disable", true},
		{"wrong database", "replay", "postgres://audit@127.0.0.1:15432/landman?sslmode=disable", false},
		{"other mode", parcelJobID, "postgres://audit@127.0.0.1:15432/landman_parcel_repair_test?sslmode=disable", false},
		{"remote fallback", "replay", "postgres://audit@127.0.0.1:15432,example.invalid:5432/landman_parcel_repair_test?sslmode=disable", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := checkParcelRepairVerificationMode(context.Background(), tt.mode, tt.dsn) == nil; got != tt.accept {
				t.Errorf("accepted = %t, want %t", got, tt.accept)
			}
		})
	}
}
