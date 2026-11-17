package union

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestQualificationHonorsSourcePageLimit(t *testing.T) {
	pages := 0
	source, close := qualificationServer(t, 170, fixtureHandler(t, 170, func(r *http.Request, n int, b string) (int, string) {
		if strings.HasSuffix(r.URL.Path, "/170") {
			return 200, strings.Replace(b, `"maxRecordCount":2000`, `"maxRecordCount":1`, 1)
		}
		if r.URL.Query().Get("returnCountOnly") != "true" {
			pages++
			if r.URL.Query().Get("resultRecordCount") != "1" {
				t.Fatalf("unbounded page size %q", r.URL.Query().Get("resultRecordCount"))
			}
			if strings.Contains(r.URL.Query().Get("outFields"), "Owner") || r.URL.Query().Get("outFields") == "*" {
				t.Fatal("qualification request included private parcel attributes")
			}
		}
		return 200, b
	}))
	defer close()
	receipt, e := source.FetchQualificationLayer(context.Background(), 170, func(QualificationFeature) error { return nil })
	if e != nil || receipt.Count != 2 || pages != 2 {
		t.Fatalf("count=%d pages=%d err=%v", receipt.Count, pages, e)
	}
}
