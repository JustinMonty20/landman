package union

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
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
	_, err = source.CaptureVacancyLayer(ctx, StructuresLayer, func(string, []byte, int, int, bool) error { t.Fatal("captured after cancellation"); return nil })
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
		{"complete", `{"count":2}`, `{"features":[{"attributes":{"OBJECTID":1}},{"attributes":{"OBJECTID":1}}]}`, 5, false},
		{"unsupported range", `{"count":2001}`, `{}`, 128, true},
		{"over row gate", `{"count":500001}`, `{}`, 1, true},
		{"partial", `{"count":2}`, `{"features":[{}]}`, 3, true},
		{"transfer limit", `{"count":1}`, `{"features":[{}],"exceededTransferLimit":true}`, 3, true},
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
			_, err = source.CaptureVacancyLayer(context.Background(), AssessedValueLayer, func(_ string, _ []byte, _, _ int, _ bool) error { captured++; return nil })
			if (err != nil) != tt.fail || calls != tt.want || captured != calls {
				t.Fatalf("err=%v calls=%d captured=%d", err, calls, captured)
			}
		})
	}
}

func TestAssessmentCountAboveOldGateReachesPartitionCheck(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("where") == "1=1" {
			fmt.Fprint(w, `{"count":490180}`)
		} else {
			fmt.Fprint(w, `{"count":0}`)
		}
	}))
	defer server.Close()
	source, err := NewGISSource(GISSourceConfig{BaseURL: server.URL + "/server/rest/services/OperationalLayers/MapServer/215/query", RateLimit: 100})
	if err != nil {
		t.Fatal(err)
	}
	_, err = source.CaptureVacancyLayer(context.Background(), AssessedValueLayer, func(string, []byte, int, int, bool) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "partition count drift") || calls != 2 {
		t.Fatalf("expected partition drift after accepted initial count; err=%v calls=%d", err, calls)
	}
}

func TestAssessmentCaptureReportsPageAndRowFailures(t *testing.T) {
	for _, tc := range []struct {
		name, page, want string
		split            bool
	}{
		{"decoder", `{"features":[],"features":[]}`, "decode assessment page for OBJECTID range 0–9223372036854775807: duplicate assessment features", false},
		{"identity", `{"features":[{"attributes":{}}]}`, "read assessment identity in range 0–9223372036854775807, row 1: missing assessment OBJECTID", false},
		{"outside range", `{"features":[{"attributes":{"OBJECTID":4611686018427387904}}]}`, "assessment OBJECTID 4611686018427387904 at row 1 is outside range 0–4611686018427387903", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if q.Get("returnCountOnly") != "true" {
					fmt.Fprint(w, tc.page)
					return
				}
				if tc.split && q.Get("where") != AssessmentWhere(0, 4611686018427387903) {
					fmt.Fprint(w, `{"count":2001}`)
					return
				}
				fmt.Fprint(w, `{"count":1}`)
			}))
			defer server.Close()
			source, err := NewGISSource(GISSourceConfig{BaseURL: server.URL + "/server/rest/services/OperationalLayers/MapServer/215/query", RateLimit: 100})
			if err != nil {
				t.Fatal(err)
			}
			_, err = source.CaptureVacancyLayer(context.Background(), AssessedValueLayer, func(string, []byte, int, int, bool) error { return nil })
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want error containing %q", err, tc.want)
			}
		})
	}
}

func TestVacancyRejectedPageRetainedBeforeValidation(t *testing.T) {
	for _, tc := range []struct {
		name, page string
		status     int
	}{
		{"invalid CRS", `{"spatialReference":{"wkid":4326},"features":[]}`, 200},
		{"malformed JSON", `{broken`, 200},
		{"refusal", `{"error":"access denied"}`, 403},
		{"oversize", strings.Repeat("x", (32<<20)+1), 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("returnCountOnly") == "true" {
					fmt.Fprint(w, `{"count":0}`)
					return
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.page)
			}))
			defer server.Close()
			source, err := NewGISSource(GISSourceConfig{BaseURL: server.URL + "/server/rest/services/OperationalLayers/MapServer/215/query", RateLimit: 100})
			if err != nil {
				t.Fatal(err)
			}
			var receipts [][]byte
			_, err = source.CaptureVacancyLayer(context.Background(), StructuresLayer, func(_ string, b []byte, _, _ int, _ bool) error {
				receipts = append(receipts, append([]byte(nil), b...))
				return nil
			})
			if err == nil || len(receipts) != 2 || string(receipts[1]) != tc.page {
				t.Fatalf("rejected page not retained: %v; receipts %d", err, len(receipts))
			}
		})
	}
}

func TestVacancyAggregateCaptureLimit(t *testing.T) {
	padding := strings.Repeat(" ", 29<<20)
	var calls, retained int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		q := r.URL.Query()
		if q.Get("returnCountOnly") == "true" {
			fmt.Fprint(w, `{"count":18000}`)
			return
		}
		if q.Get("returnIdsOnly") == "true" {
			fmt.Fprint(w, `{"objectIdFieldName":"OBJECTID","objectIds":[`)
			for i := 1; i <= 18000; i++ {
				if i > 1 {
					fmt.Fprint(w, ",")
				}
				fmt.Fprint(w, i)
			}
			fmt.Fprint(w, `]}`)
			return
		}
		offset, _ := strconv.Atoi(q.Get("resultOffset"))
		fmt.Fprint(w, `{"spatialReference":{"wkid":3857},"features":[`)
		for i := offset + 1; i <= offset+2000; i++ {
			if i > offset+1 {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w, `{"attributes":{"OBJECTID":%d}}`, i)
		}
		fmt.Fprint(w, `]}`)
		fmt.Fprint(w, padding)
	}))
	defer server.Close()
	source, err := NewGISSource(GISSourceConfig{BaseURL: server.URL + "/server/rest/services/OperationalLayers/MapServer/215/query", RateLimit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	_, err = source.CaptureVacancyLayer(context.Background(), StructuresLayer, func(_ string, _ []byte, _ int, _ int, _ bool) error { retained++; return nil })
	if err == nil || !strings.Contains(err.Error(), "aggregate byte limit") || retained != 10 || calls != 11 {
		t.Fatalf("aggregate gate: %v; retained=%d requests=%d", err, retained, calls)
	}
}
