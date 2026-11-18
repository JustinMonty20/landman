package soiljob

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/taxartifact"
)

func TestCapturedSoilRequiresCompleteOrderedPages(t *testing.T) {
	ctx := context.Background()
	raw, err := os.ReadFile("../testdata/soil/layer197-feature.json")
	if err != nil {
		t.Fatal(err)
	}
	store, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "private"))
	if err != nil {
		t.Fatal(err)
	}
	capture := func(count string, page bool) ingest.VerifiedCapture {
		t.Helper()
		recorder, err := ingest.NewRecorder(store, captureSource, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		if err := recorder.Record(ctx, "https://example.invalid/query?where=1%3D1&returnCountOnly=true&outFields=*&f=json", "revision", 0, strings.NewReader(count)); err != nil {
			t.Fatal(err)
		}
		if page {
			if err := recorder.Record(ctx, "https://example.invalid/query?where=1%3D1&orderByFields=OBJECTID_1&resultRecordCount=2000&outFields=OBJECTID_1,AREASYMBOL,SPATIALVER,MUSYM,MUKEY&returnGeometry=true&f=json", "revision", 102100, bytes.NewReader(raw)); err != nil {
				t.Fatal(err)
			}
		}
		v, err := recorder.Verify(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	var valid publication
	if err := valid.Prepare(ctx, capture(`{"count":1}`, true)); err != nil || len(valid.polygons) != 1 {
		t.Fatalf("complete page rejected: %v", err)
	}
	for _, pair := range []struct {
		count string
		page  bool
	}{{`{"count":2}`, true}, {`{"count":1}`, false}, {`{"count":-1}`, true}} {
		var candidate publication
		if err := candidate.Prepare(ctx, capture(pair.count, pair.page)); err == nil {
			t.Fatalf("accepted incomplete count %s, page %t", pair.count, pair.page)
		}
	}
}
