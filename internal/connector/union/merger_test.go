package union

import (
	"strings"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/models"
)

// compile-time interface check
var _ connector.Merger = (*ParcelMerger)(nil)

func TestNewParcelMerger(t *testing.T) {
	if NewParcelMerger() == nil {
		t.Fatal("expected non-nil merger")
	}
}

func TestParcelMerger_Merge(t *testing.T) {
	m := NewParcelMerger()

	tests := []struct {
		name        string
		parcels     []*models.GISParcel
		wantErr     bool
		errContains string
		checkResult func(t *testing.T, result *models.GISParcel)
	}{
		{
			name:        "empty slice returns error",
			parcels:     []*models.GISParcel{},
			wantErr:     true,
			errContains: "no parcels to merge",
		},
		{
			name:    "nil slice returns error",
			parcels: nil,
			wantErr: true,
		},
		{
			name:    "single parcel returns same parcel",
			parcels: []*models.GISParcel{testGISParcel("12-345-678", "union")},
			checkResult: func(t *testing.T, result *models.GISParcel) {
				if result.Identity.ParcelNumber == nil || *result.Identity.ParcelNumber != "12-345-678" {
					t.Errorf("ParcelNumber = %v, want %q", result.Identity.ParcelNumber, "12-345-678")
				}
				if result.Identity.CountyID != "union" {
					t.Errorf("CountyID = %q, want %q", result.Identity.CountyID, "union")
				}
			},
		},
		{
			name: "multiple parcels returns first (current implementation)",
			parcels: []*models.GISParcel{
				testGISParcel("first-parcel", "union"),
				testGISParcel("second-parcel", "union"),
				testGISParcel("third-parcel", "union"),
			},
			checkResult: func(t *testing.T, result *models.GISParcel) {
				if result.Identity.ParcelNumber == nil || *result.Identity.ParcelNumber != "first-parcel" {
					t.Errorf("ParcelNumber = %v, want %q", result.Identity.ParcelNumber, "first-parcel")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := m.Merge(tt.parcels)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkResult != nil {
				tt.checkResult(t, result)
			}
		})
	}
}

func TestParcelMerger_SingleParcelIdentity(t *testing.T) {
	m := NewParcelMerger()
	original := testGISParcel("test-parcel", "union")

	result, err := m.Merge([]*models.GISParcel{original})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != original {
		t.Error("expected Merge to return the same parcel instance for single input")
	}
}

func testGISParcel(parcelNumber, countyID string) *models.GISParcel {
	return &models.GISParcel{
		Identity: models.ParcelIdentity{
			CountyID:     countyID,
			CountyCode:   countyID,
			ParcelNumber: &parcelNumber,
		},
	}
}
