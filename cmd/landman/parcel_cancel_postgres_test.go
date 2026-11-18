//go:build postgres

package main

import (
	"context"
	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/connector/union/parceljob"
	"github.com/JustinMonty20/landman/internal/parcelevidence"
	"os"
	"testing"
)

func TestParcelCanceledPublicationKeepsCurrent(t *testing.T) {
	ctx, dsn, pool := parcelFixtureDB(t)
	fixture := &parcelHTTPFixture{features: []map[string]interface{}{feature(1, "A", "old", true, 1)}}
	source := fixture.source(t, 1)
	if _, _, err := runParcelFixture(ctx, pool, dsn, source, false, 0); err != nil {
		t.Fatal(err)
	}
	before := parcelSnapshot(t, ctx, pool)
	fixture.features[0] = feature(1, "A", "new", true, 0)
	snapshot, err := parcelevidence.New(os.Getenv("PARCEL_EVIDENCE_DIR"), "full_scan")
	if err != nil {
		t.Fatal(err)
	}
	n, err := source.CaptureParcelPages(ctx, snapshot.Save, func([]connector.RawRecord) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err = snapshot.Complete(n); err != nil {
		t.Fatal(err)
	}
	stopped, cancel := context.WithCancel(ctx)
	cancel()
	job := parceljob.Job{Root: os.Getenv("PARCEL_EVIDENCE_DIR"), DSN: dsn}
	if _, err = job.Replay(stopped, snapshot.Manifest.ID); err == nil {
		t.Fatal("canceled publication succeeded")
	}
	after := parcelSnapshot(t, ctx, pool)
	if len(after) != len(before) || after[0] != before[0] || countRows(t, ctx, pool, "parcel_evidence_observations") != 1 {
		t.Fatal("cancellation advanced current state")
	}
}
