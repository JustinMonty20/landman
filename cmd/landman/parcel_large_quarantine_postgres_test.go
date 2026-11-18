//go:build postgres

package main

import (
	"encoding/json"
	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/jobs"
	"os"
	"path/filepath"
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
	var saved []byte
	if err := pool.QueryRow(ctx, `SELECT manifest FROM ingest_captures WHERE id=$1::uuid`, snapshotID).Scan(&saved); err != nil {
		t.Fatal(err)
	}
	var manifest ingest.Manifest
	if err := json.Unmarshal(saved, &manifest); err != nil {
		t.Fatal(err)
	}
	receipt := manifest.Receipts[1]
	wantReference := receipt.ArtifactKey + "@" + receipt.SHA256 + "#0"
	if reference != wantReference {
		t.Fatalf("invalid evidence reference %s; want %s", reference, wantReference)
	}
	body, err := os.ReadFile(filepath.Join(os.Getenv("PARCEL_EVIDENCE_DIR"), filepath.FromSlash(receipt.ArtifactKey)))
	if err != nil || !strings.Contains(string(body), strings.Repeat("x", 70*1024)) {
		t.Fatal("original source feature missing", err)
	}
}
