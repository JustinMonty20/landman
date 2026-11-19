package union

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVacancyCaptureCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	source, err := NewGISSource(GISSourceConfig{BaseURL: "http://127.0.0.1:1/server/rest/services/OperationalLayers/MapServer/215/query"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = source.CaptureVacancyLayer(ctx, StructuresLayer, func(string, []byte, int) error { t.Fatal("captured after cancellation"); return nil })
	if err == nil {
		t.Fatal("cancelled request accepted")
	}
}
func TestVacancyCaptureFailClosed(t *testing.T) {
	for _, tt := range []struct {
		name, count, page string
		want              int
		fail              bool
	}{
		{"complete", `{"count":2}`, `{"features":[{"attributes":{"OBJECTID":1}},{"attributes":{"OBJECTID":1}}]}`, 2, false},
		{"unsafe pagination", `{"count":2001}`, ``, 1, true},
		{"partial", `{"count":2}`, `{"features":[{}]}`, 2, true},
		{"transfer limit", `{"count":1}`, `{"features":[{}],"exceededTransferLimit":true}`, 2, true},
		{"server error", `{"error":{"message":"denied"}}`, ``, 1, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if !strings.HasSuffix(r.URL.Path, "/0/query") {
					t.Errorf("wrong path %s", r.URL.Path)
				}
				if r.URL.Query().Get("returnCountOnly") == "true" {
					fmt.Fprint(w, tt.count)
				} else {
					fmt.Fprint(w, tt.page)
				}
			}))
			defer server.Close()
			source, err := NewGISSource(GISSourceConfig{BaseURL: server.URL + "/server/rest/services/OperationalLayers/MapServer/215/query", RateLimit: 100})
			if err != nil {
				t.Fatal(err)
			}
			captured := 0
			_, err = source.CaptureVacancyLayer(context.Background(), AssessedValueLayer, func(_ string, _ []byte, _ int) error { captured++; return nil })
			if (err != nil) != tt.fail || calls != tt.want || captured != calls {
				t.Fatalf("err=%v calls=%d captured=%d", err, calls, captured)
			}
		})
	}
}
