//go:build postgres

package main

import (
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/parcelevidence"
	"os"
	"strings"
	"testing"
)

func TestParcelLargeInvalidFeatureUsesCompleteFileEvidence(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	bad := feature(1, "A", "owner", false, 0)
	bad["attributes"].(map[string]interface{})["LONG_SOURCE_FIELD"] = strings.Repeat("x", 70*1024)
	fixture := &parcelHTTPFixture{features: []map[string]interface{}{bad}}
	outcome, _, err := runParcelFixture(ctx, pool, dsn, fixture.source(t, 1), false, 0)
	if err != nil || outcome != jobs.OutcomeSuccessWithQuarantine {
		t.Fatal(outcome, err)
	}
	var reference, snapshotID string
	var payloadSize int
	if err := pool.QueryRow(ctx, `SELECT payload_reference,length(payload::text) FROM quarantined_records`).Scan(&reference, &payloadSize); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT artifact_id FROM parcel_evidence_observations`).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	if payloadSize >= 64*1024 {
		t.Fatalf("unbounded quarantine payload %d", payloadSize)
	}
	dir, err := os.Getenv("PARCEL_EVIDENCE_DIR"), nil
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := parcelevidence.Open(dir, snapshotID)
	if err != nil {
		t.Fatal(err)
	}
	wantReference := snapshotID + "/" + snapshot.Manifest.Receipts[1].Path + "@" + snapshot.Manifest.Receipts[1].SHA256 + "#0"
	if reference != wantReference {
		t.Fatalf("invalid evidence reference %s; want %s", reference, wantReference)
	}
	body, err := snapshot.Read(1)
	if err != nil || !strings.Contains(string(body), strings.Repeat("x", 70*1024)) {
		t.Fatal("original source feature missing", err)
	}
}
