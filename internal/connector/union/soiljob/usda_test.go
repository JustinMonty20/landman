package soiljob

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/soil"
	"github.com/JustinMonty20/landman/internal/taxartifact"
)

func TestScreeningPublicationPreparesOnlyRetainedOrderedUSDA(t *testing.T) {
	ctx := context.Background()
	store, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "evidence"))
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{"115328", "115330", "115341"}
	selectFixture := func(query string) string {
		switch {
		case strings.Contains(query, "FROM sdvattribute"):
			return "union-rules-raw.json"
		case strings.Contains(query, "JOIN muaggatt"):
			return "union-muaggatt-raw.json"
		case strings.Contains(query, "JOIN sacatalog"):
			return "union-catalog-raw.json"
		case strings.Contains(query, "FROM component c WHERE"):
			return "union-ancillary-components-raw.json"
		case strings.Contains(query, "JOIN cointerp"):
			return "union-components-raw.json"
		case strings.Contains(query, "JOIN cosoilmoist"):
			return "union-moisture-raw.json"
		case strings.Contains(query, "JOIN comonth"):
			return "union-monthly-raw.json"
		case strings.Contains(query, "JOIN chorizon"):
			return "union-horizons-raw.json"
		case strings.Contains(query, "JOIN corestrictions"):
			return "union-restrictions-raw.json"
		}
		t.Fatalf("unknown query %s", query)
		return ""
	}
	makeCapture := func(badRequest, extra bool) ingest.VerifiedCapture {
		t.Helper()
		recorder, e := ingest.NewRecorder(store, interpretationSource, time.Now().UTC())
		if e != nil {
			t.Fatal(e)
		}
		header, e := json.Marshal(struct{ Keys, NameKeys []string }{keys, []string{}})
		if e != nil {
			t.Fatal(e)
		}
		if e = recorder.Record(ctx, setRequestPrefix+"11111111-1111-1111-1111-111111111111", "", 0, strings.NewReader(string(header))); e != nil {
			t.Fatal(e)
		}
		queries := 0
		_, e = soil.ParseInterpretations(ctx, soil.USDAEndpoint, keys, func(ctx context.Context, query string) ([]byte, error) {
			path := filepath.Join("..", "..", "..", "soil", "testdata", selectFixture(query))
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil, readErr
			}
			request := usdaRequest(query)
			if badRequest && queries == 1 {
				request += "&unexpected=true"
			}
			queries++
			if err := recorder.Record(ctx, request, usdaRevision, 0, strings.NewReader(string(raw))); err != nil {
				return nil, err
			}
			return raw, nil
		})
		if e != nil {
			t.Fatal(e)
		}
		if extra {
			if e := recorder.Record(ctx, "unexpected", usdaRevision, 0, strings.NewReader(`{"Table":[]}`)); e != nil {
				t.Fatal(e)
			}
		}
		v, e := recorder.Verify(ctx)
		if e != nil {
			t.Fatal(e)
		}
		return v
	}
	var publication screeningPublication
	if err := publication.Prepare(ctx, makeCapture(false, false)); err != nil {
		t.Fatal(err)
	}
	if len(publication.values) != 9 || publication.values[0].MapUnitKey != keys[0] {
		t.Fatalf("wrong retained interpretations: %+v", publication.values)
	}
	if !publication.values[0].FetchedAt.Equal(publication.values[0].Measurements[0].FetchedAt) {
		t.Fatal("retained observation timestamp not used")
	}
	for _, variant := range []struct{ badRequest, extra bool }{{true, false}, {false, true}} {
		var candidate screeningPublication
		if err := candidate.Prepare(ctx, makeCapture(variant.badRequest, variant.extra)); err == nil {
			t.Fatalf("accepted altered USDA sequence %+v", variant)
		}
	}
}
