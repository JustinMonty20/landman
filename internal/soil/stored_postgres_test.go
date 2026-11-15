//go:build postgres

package soil

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/county"
)

func TestStoredRefreshPendingAndEmptySet(t *testing.T) {
	f := newAssessmentFixture(t)
	if _, err := screenStored(f.ctx, f.db, f.server.Client(), f.server.URL, county.Union, f.source, DefaultFreshnessInterval); !errors.Is(err, ErrNoStoredSet) {
		t.Fatalf("missing set error=%v", err)
	}
	if f.calls.Load() != 0 {
		t.Fatal("missing set requested USDA")
	}
	f.store(nil)
	if count, err := screenStored(f.ctx, f.db, f.server.Client(), f.server.URL, county.Union, f.source, DefaultFreshnessInterval); err != nil || count != 0 {
		t.Fatalf("ingested empty set count=%d error=%v", count, err)
	}
}

func TestStoredRefreshCancellationRollsBackOverlapsAndHead(t *testing.T) {
	f := newAssessmentFixture(t)
	parcelID := f.parcel("parcel", f.box(0, 0, 100, 100), false)
	f.store([]Polygon{f.polygon("active", "115328", 0, 100)})
	refresh := func(ctx context.Context) error {
		_, err := screenStored(ctx, f.db, f.server.Client(), f.server.URL, county.Union, f.source, DefaultFreshnessInterval)
		return err
	}
	if err := refresh(f.ctx); err != nil {
		t.Fatal(err)
	}
	previousHead := f.read(parcelID).InterpretationSetRevision
	overlap := func() float64 {
		t.Helper()
		var acres float64
		if err := f.db.QueryRow(f.ctx, `SELECT overlap_acres FROM soil_polygon_parcels WHERE parcel_id=$1`, parcelID).Scan(&acres); err != nil {
			t.Fatal(err)
		}
		return acres
	}
	previousOverlap := overlap()
	f.parcel("parcel", f.box(0, 0, 50, 100), false)
	blocker, err := f.db.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(f.ctx)
	if _, err := blocker.Exec(f.ctx, `LOCK TABLE soil_parcel_assessments IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 20*time.Second)
	done := make(chan error, 1)
	finished := false
	defer func() {
		cancel()
		if !finished {
			<-done
		}
	}()
	go func() { done <- refresh(ctx) }()
	blocked := false
	for ctx.Err() == nil {
		if err = f.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE relation='soil_parcel_assessments'::regclass AND NOT granted)`).Scan(&blocked); err != nil || blocked {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	refreshErr := <-done
	finished = true
	if err != nil || !blocked || refreshErr == nil {
		t.Fatalf("refresh did not cancel after overlap persistence: blocked=%t refresh=%v wait=%v", blocked, refreshErr, err)
	}
	if err := blocker.Rollback(f.ctx); err != nil {
		t.Fatal(err)
	}
	near(t, overlap(), previousOverlap)
	if assessment := f.read(parcelID); assessment.InterpretationSetRevision != previousHead || assessment.Freshness.RefreshFailedAt == nil {
		t.Fatal("cancellation changed the published head or hid the failure")
	}
	if err := refresh(f.ctx); err != nil {
		t.Fatal(err)
	}
	near(t, overlap(), 5000.0/43560)
	if assessment := f.read(parcelID); assessment.InterpretationSetRevision == previousHead || assessment.Freshness.Stale {
		t.Fatal("successful retry did not publish the changed parcel")
	}
}

func TestStoredRefreshUsesCurrentMembershipWithoutNewFetch(t *testing.T) {
	f := newAssessmentFixture(t)
	parcelID := f.parcel("parcel", f.box(0, 0, 100, 100), false)
	active := f.polygon("active", "115328", 0, 70)
	retired := f.polygon("retired", "115330", 70, 100)
	f.store([]Polygon{active, retired})
	f.store([]Polygon{active})
	var before string
	fingerprint := `SELECT jsonb_build_object('sets',(SELECT count(*) FROM soil_polygon_sets WHERE source_name=$1),'fetches',(SELECT jsonb_agg(jsonb_build_array(id,fetched_at) ORDER BY id) FROM soil_polygons WHERE source_name=$1),'head',(SELECT set_id FROM soil_polygon_set_heads WHERE source_name=$1))::text`
	if err := f.db.QueryRow(f.ctx, fingerprint, f.source).Scan(&before); err != nil {
		t.Fatal(err)
	}
	refresh := func() {
		t.Helper()
		count, err := screenStored(f.ctx, f.db, f.server.Client(), f.server.URL, county.Union, f.source, DefaultFreshnessInterval)
		if err != nil || count != 1 {
			t.Fatalf("stored refresh count=%d error=%v", count, err)
		}
	}
	refresh()
	assessment := f.read(parcelID)
	if assessment.SoilSetID != f.setID || assessment.Freshness.Stale {
		t.Fatalf("wrong set or freshness: %+v", assessment.Freshness)
	}
	use := useResult(t, assessment, ConventionalSeptic)
	if len(use.MapUnits) != 1 || use.MapUnits[0].MapUnitKey != "115328" {
		t.Fatalf("retired membership entered assessment: %+v", use.MapUnits)
	}
	near(t, use.Uncovered.Acres, 3000.0/43560)
	var retiredBefore float64
	if err := f.db.QueryRow(f.ctx, `SELECT j.overlap_acres FROM soil_polygon_parcels j JOIN soil_polygons p ON p.id=j.soil_polygon_id WHERE p.source_name=$1 AND p.object_id='retired'`, f.source).Scan(&retiredBefore); err != nil {
		t.Fatal(err)
	}
	f.parcel("parcel", f.box(0, 0, 50, 100), false)
	if !f.read(parcelID).Freshness.Stale {
		t.Fatal("changed geometry not stale")
	}
	refresh()
	assessment = f.read(parcelID)
	if assessment.Freshness.Stale {
		t.Fatalf("parcel-only refresh stale: %+v", assessment.Freshness)
	}
	near(t, *assessment.TotalAcres, 5000.0/43560)
	near(t, useResult(t, assessment, ConventionalSeptic).Uncovered.Acres, 0)
	var activeAcres, retiredAfter float64
	for objectID, target := range map[string]*float64{"active": &activeAcres, "retired": &retiredAfter} {
		if err := f.db.QueryRow(f.ctx, `SELECT j.overlap_acres FROM soil_polygon_parcels j JOIN soil_polygons p ON p.id=j.soil_polygon_id WHERE p.source_name=$1 AND p.object_id=$2`, f.source, objectID).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	near(t, activeAcres, 5000.0/43560)
	near(t, retiredAfter, retiredBefore)
	var after string
	if err := f.db.QueryRow(f.ctx, fingerprint, f.source).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("derived refresh falsely recorded new soil fetch: %s -> %s", before, after)
	}

	previous := assessment.InterpretationSetRevision
	f.mode.Store(2)
	if _, err := screenStored(f.ctx, f.db, f.server.Client(), f.server.URL, county.Union, f.source, DefaultFreshnessInterval); err == nil {
		t.Fatal("required USDA failure succeeded")
	}
	failed := f.read(parcelID)
	if failed.InterpretationSetRevision != previous || failed.Freshness.RefreshFailedAt == nil {
		t.Fatal("failure lost prior head or refresh status")
	}
}
