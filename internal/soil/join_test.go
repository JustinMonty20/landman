package soil

import (
	"context"
	"strings"
	"testing"

	"github.com/JustinMonty20/landman/internal/county"
)

func TestSoilJoinStatement(t *testing.T) {
	for _, part := range []string{"ST_Transform", "2264", "soil_polygon_set_members", "m.set_id=$3", "soil_polygon_parcels", "ST_Intersection", "overlap_acres > 0", "parcel_boundary_usable(p.shape)", "p.source_name = s.source_name", "CROSS JOIN LATERAL", "p.shape && s.shape", "OFFSET 0", "ON CONFLICT (soil_polygon_id, parcel_id)"} {
		if !strings.Contains(overlapInsert, part) {
			t.Errorf("missing %s", part)
		}
	}
	for _, part := range []string{"likely_vacant", "source_parcel_id", "INSERT INTO parcels", "INSERT INTO soil_polygon_parcels (id"} {
		if strings.Contains(overlapInsert, part) {
			t.Errorf("unexpected %s", part)
		}
	}
	if strings.Count(overlapInsert, "INSERT INTO") != 1 {
		t.Fatal("join must be one insert")
	}
}

func TestSoilJoinCancelledBeforeConnection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Store(ctx, "postgres://localhost:1/no-db", county.Union, "union_county_gis", nil, nil); err == nil {
		t.Fatal("canceled import succeeded")
	}
}
