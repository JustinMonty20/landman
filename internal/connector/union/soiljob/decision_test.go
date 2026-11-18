package soiljob

import (
	"testing"

	"github.com/JustinMonty20/landman/internal/ingest"
)

func TestSoilChangeCauseUsesPersistedFingerprintVersions(t *testing.T) {
	next := ingest.Fingerprint{Source: "same", SourceVersion: "response-v2", Normalized: "new", NormalizedVersion: "normal-v2", Geometry: "shape", GeometryVersion: "shape-v2"}
	previous := ingest.Fingerprint{Source: "same", SourceVersion: "response-v2", Normalized: "old", NormalizedVersion: "normal-v1", Geometry: "shape", GeometryVersion: "shape-v2"}
	if cause := soilChangeCause(previous, next, false); cause != ingest.NormalizationCorrection {
		t.Fatalf("changed projection under the same retained source: %s", cause)
	}
	previous.SourceVersion = "response-v1"
	if cause := soilChangeCause(previous, next, false); cause != ingest.SourceObservation {
		t.Fatalf("incomparable source versions: %s", cause)
	}
	previous.Normalized = next.Normalized
	previous.SourceVersion = next.SourceVersion
	if cause := soilChangeCause(previous, next, true); cause != ingest.NormalizationCorrection {
		t.Fatalf("version-only normalization replay must preserve correction provenance: %s", cause)
	}
	previous.NormalizedVersion = next.NormalizedVersion
	if cause := soilChangeCause(previous, next, true); cause != ingest.OfflineRetry {
		t.Fatalf("unchanged replay must retain a valid publication cause: %s", cause)
	}
}
