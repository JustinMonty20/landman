package union

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestSoilCaptureStopsBeforeNextPageAfterRetentionError(t *testing.T) {
	fixture, err := os.ReadFile("testdata/soil/layer197-feature.json")
	if err != nil {
		t.Fatal(err)
	}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("returnCountOnly") == "true" {
			_, _ = w.Write([]byte(`{"count":1}`))
			return
		}
		_, _ = w.Write(fixture)
	}))
	defer server.Close()
	source, err := NewGISSource(GISSourceConfig{BaseURL: server.URL + "/parcels", SoilQueryURL: server.URL + "/soil", RateLimit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	retentionFailure := errors.New("store unavailable")
	_, _, err = source.CaptureSoilPolygons(context.Background(), func(request string, body []byte, crs int) error {
		if !strings.Contains(request, "returnCountOnly=true") || string(body) != `{"count":1}` || crs != 0 {
			t.Fatalf("count was not retained first: %s", request)
		}
		return retentionFailure
	})
	if !errors.Is(err, retentionFailure) || requests != 1 {
		t.Fatalf("retention failure did not stop before page: requests=%d error=%v", requests, err)
	}
}
