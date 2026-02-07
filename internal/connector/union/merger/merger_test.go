package merger

import (
	"strings"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/models"
)

// Ensure interface is implemented (uses connector import)
var _ connector.Merger = (*UnionCountyMerger)(nil)

func TestNewUnionCountyMerger(t *testing.T) {
	merger := NewUnionCountyMerger()

	if merger == nil {
		t.Fatal("expected non-nil merger")
	}
}

func TestUnionCountyMerger_Merge(t *testing.T) {
	merger := NewUnionCountyMerger()

	tests := []struct {
		name        string
		parcels     []*models.Parcel
		wantErr     bool
		errContains string
		checkResult func(t *testing.T, result *models.Parcel)
	}{
		{
			name:        "empty slice returns error",
			parcels:     []*models.Parcel{},
			wantErr:     true,
			errContains: "no parcels to merge",
		},
		{
			name:    "nil slice returns error",
			parcels: nil,
			wantErr: true,
		},
		{
			name: "single parcel returns same parcel",
			parcels: []*models.Parcel{
				{ParcelId: "12-345-678", CountyId: "union"},
			},
			wantErr: false,
			checkResult: func(t *testing.T, result *models.Parcel) {
				if result.ParcelId != "12-345-678" {
					t.Errorf("ParcelId = %q, want %q", result.ParcelId, "12-345-678")
				}
				if result.CountyId != "union" {
					t.Errorf("CountyId = %q, want %q", result.CountyId, "union")
				}
			},
		},
		{
			name: "multiple parcels returns first (current implementation)",
			parcels: []*models.Parcel{
				{ParcelId: "first-parcel", CountyId: "union"},
				{ParcelId: "second-parcel", CountyId: "union"},
				{ParcelId: "third-parcel", CountyId: "union"},
			},
			wantErr: false,
			checkResult: func(t *testing.T, result *models.Parcel) {
				// Current implementation returns first parcel
				if result.ParcelId != "first-parcel" {
					t.Errorf("ParcelId = %q, want %q", result.ParcelId, "first-parcel")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := merger.Merge(tt.parcels)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
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

func TestUnionCountyMerger_Interface(t *testing.T) {
	// Verify UnionCountyMerger implements connector.Merger
	// (compile-time check at package level)
	merger := NewUnionCountyMerger()
	if merger == nil {
		t.Fatal("merger should not be nil")
	}
}

func TestUnionCountyMerger_SingleParcelIdentity(t *testing.T) {
	merger := NewUnionCountyMerger()

	original := &models.Parcel{
		ParcelId: "test-parcel",
		CountyId: "union",
	}

	result, err := merger.Merge([]*models.Parcel{original})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return the exact same pointer
	if result != original {
		t.Error("expected Merge to return the same parcel instance for single input")
	}
}

func containsString(s, substr string) bool {
	return strings.Contains(s, substr)
}
