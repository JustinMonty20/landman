package union

import (
	"testing"

	"github.com/JustinMonty20/landman/internal/connector"
)

func TestNewUnionCountyTranslator(t *testing.T) {
	translator := NewUnionCountyTranslator()

	if translator == nil {
		t.Fatal("expected non-nil translator")
	}

	if translator.priority != 10 {
		t.Errorf("priority = %d, want 10", translator.priority)
	}
}

func TestUnionCountyTranslator_CanTranslate(t *testing.T) {
	translator := NewUnionCountyTranslator()

	tests := []struct {
		name       string
		sourceName string
		want       bool
	}{
		{
			name:       "union county gis source",
			sourceName: "union_county_gis",
			want:       true,
		},
		{
			name:       "other county gis source",
			sourceName: "mecklenburg_county_gis",
			want:       false,
		},
		{
			name:       "union county tax source",
			sourceName: "union_county_tax",
			want:       false,
		},
		{
			name:       "empty source name",
			sourceName: "",
			want:       false,
		},
		{
			name:       "similar but not exact match",
			sourceName: "union_county_gis_v2",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := translator.CanTranslate(tt.sourceName); got != tt.want {
				t.Errorf("CanTranslate(%q) = %v, want %v", tt.sourceName, got, tt.want)
			}
		})
	}
}

func TestUnionCountyTranslator_Priority(t *testing.T) {
	translator := NewUnionCountyTranslator()

	if got := translator.Priority(); got != 10 {
		t.Errorf("Priority() = %d, want 10", got)
	}
}

func TestUnionCountyTranslator_Translate(t *testing.T) {
	translator := NewUnionCountyTranslator()

	tests := []struct {
		name        string
		rawRecord   connector.RawRecord
		wantErr     bool
		errContains string
		checkParcel func(t *testing.T, parcelID, countyID string)
	}{
		{
			name: "valid union county gis record",
			rawRecord: connector.RawRecord{
				SourceName: "union_county_gis",
				ParcelID:   "12-345-678",
				RawData: map[string]interface{}{
					"attributes": map[string]interface{}{
						"PID": "12-345-678",
					},
				},
			},
			wantErr: false,
			checkParcel: func(t *testing.T, parcelID, countyID string) {
				if parcelID != "12-345-678" {
					t.Errorf("ParcelId = %q, want %q", parcelID, "12-345-678")
				}
				if countyID != "union" {
					t.Errorf("CountyId = %q, want %q", countyID, "union")
				}
			},
		},
		{
			name: "wrong source name",
			rawRecord: connector.RawRecord{
				SourceName: "mecklenburg_county_gis",
				ParcelID:   "12-345-678",
			},
			wantErr:     true,
			errContains: "cannot translate source",
		},
		{
			name: "missing parcel ID",
			rawRecord: connector.RawRecord{
				SourceName: "union_county_gis",
				ParcelID:   "",
				RawData:    map[string]interface{}{},
			},
			wantErr:     true,
			errContains: "missing parcel ID",
		},
		{
			name: "empty raw data still works if parcel ID present",
			rawRecord: connector.RawRecord{
				SourceName: "union_county_gis",
				ParcelID:   "99-999-999",
				RawData:    nil,
			},
			wantErr: false,
			checkParcel: func(t *testing.T, parcelID, countyID string) {
				if parcelID != "99-999-999" {
					t.Errorf("ParcelId = %q, want %q", parcelID, "99-999-999")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parcel, err := translator.Translate(tt.rawRecord)

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

			if parcel == nil {
				t.Fatal("expected non-nil parcel")
			}

			if tt.checkParcel != nil {
				tt.checkParcel(t, parcel.ParcelId, parcel.CountyId)
			}
		})
	}
}

func TestUnionCountyTranslator_Interface(t *testing.T) {
	// Verify UnionCountyTranslator implements connector.Translator
	var _ connector.Translator = (*UnionCountyTranslator)(nil)
}
