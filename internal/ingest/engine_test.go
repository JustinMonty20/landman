package ingest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/taxartifact"
)

func TestRecorderRetainsOrderedVerifiedResponses(t *testing.T) {
	ctx := context.Background()
	store, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewRecorder(store, "union_gis_soil", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"count":1}`, `{"features":[{"attributes":{"OBJECTID_1":9}}]}`} {
		if err := r.Record(ctx, "https://example.invalid/query", "rev-1", 3857, strings.NewReader(body)); err != nil {
			t.Fatal(err)
		}
	}
	capture, err := (Engine{Store: store}).verify(ctx, r.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	if len(capture.Manifest().Receipts) != 2 || capture.Manifest().Receipts[0].ArtifactKey == capture.Manifest().Receipts[1].ArtifactKey {
		t.Fatal("lost response order or bytes")
	}
	for i := range capture.Manifest().Receipts {
		body, err := capture.Open(ctx, i)
		if err != nil {
			t.Fatal(err)
		}
		if err := body.Close(); err != nil {
			t.Fatal(err)
		}
	}
	altered := r.Manifest()
	altered.Receipts[1].SHA256 = altered.Receipts[0].SHA256
	if _, err := (Engine{Store: store}).verify(ctx, altered); err == nil {
		t.Fatal("accepted altered receipt")
	}
	path := filepath.Join(t.TempDir(), "other")
	other, err := taxartifact.NewLocal(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (Engine{Store: other}).verify(ctx, r.Manifest()); !os.IsNotExist(err) {
		t.Fatalf("unretained capture accepted: %v", err)
	}
}

func TestChangeClassificationKeepsHashVersionsIndependent(t *testing.T) {
	old := Fingerprint{Source: "source", SourceVersion: "v1", Normalized: "mapped", NormalizedVersion: "v1", Geometry: "shape", GeometryVersion: "v1"}
	if decision := Classify(old, old, false); decision.WriteBusiness || decision.CompareProjection {
		t.Fatal("unchanged observation wrote business history")
	}
	fixed := old
	fixed.Normalized = "corrected"
	if decision := Classify(old, fixed, false); !decision.WriteBusiness || decision.Cause != NormalizationCorrection {
		t.Fatal("normalizer correction misclassified")
	}
	observed := old
	observed.Source = "other"
	if decision := Classify(old, observed, false); decision.WriteBusiness || decision.CompareProjection {
		t.Fatal("new source bytes with unchanged projection wrote business history")
	}
	observed.Normalized = "changed"
	if decision := Classify(old, observed, false); !decision.WriteBusiness || decision.Cause != SourceObservation {
		t.Fatal("changed source and business projection misclassified")
	}
	migrated := old
	migrated.NormalizedVersion = "v2"
	if decision := Classify(old, migrated, true); decision.WriteBusiness || !decision.CompareProjection {
		t.Fatal("hash migration wrote without read-only projection comparison")
	}
}

func TestVerifiedCaptureManifestCannotMutateReceiptBinding(t *testing.T) {
	ctx := context.Background()
	store, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	recorder, err := NewRecorder(store, "union_county_gis", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := recorder.Record(ctx, "https://example.invalid/query", "", 3857, strings.NewReader(`{"features":[]}`)); err != nil {
		t.Fatal(err)
	}
	verified, err := recorder.Verify(ctx)
	if err != nil {
		t.Fatal(err)
	}
	original := verified.Manifest().Receipts[0].SHA256
	external := verified.Manifest()
	external.Receipts[0].SHA256 = strings.Repeat("0", 64)
	if verified.Manifest().Receipts[0].SHA256 != original {
		t.Fatal("verified receipt binding mutated through caller manifest")
	}
	retained := recorder.Manifest()
	retained.Receipts[0].SHA256 = strings.Repeat("f", 64)
	if recorder.Manifest().Receipts[0].SHA256 != original {
		t.Fatal("recorder receipt binding mutated through caller manifest")
	}
}
