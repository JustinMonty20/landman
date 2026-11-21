package union

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAssessmentAdmissionAndDrift(t *testing.T) {
	for _, tc := range []struct {
		name        string
		total       int
		post, final int
		wantError   string
		pages       int
	}{
		{"empty", 0, 0, 0, "", 0},
		{"duplicate rows", 2, 2, 2, "", 1},
		{"post-page drift", 2, 1, 2, "partition", 1},
		{"final drift", 2, 2, 1, "total count drift", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var pages, receipts int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if q.Has("resultRecordCount") {
					pages++
					fmt.Fprint(w, `{"features":[{"attributes":{"OBJECTID":1,"parcel_number":"001"}},{"attributes":{"OBJECTID":1,"parcel_number":"001"}}]}`)
					return
				}
				receipts++
				n := tc.total
				if receipts == 3 {
					n = tc.post
				} // global, root, then leaf post-count
				if receipts == 4 {
					n = tc.final
				}
				fmt.Fprintf(w, `{"count":%d}`, n)
			}))
			defer server.Close()
			source, err := NewGISSource(GISSourceConfig{BaseURL: server.URL + "/server/rest/services/OperationalLayers/MapServer/215/query", RateLimit: 1000})
			if err != nil {
				t.Fatal(err)
			}
			var urls []string
			_, err = source.CaptureVacancyLayer(context.Background(), AssessedValueLayer, func(u string, _ []byte, _, _ int, _ bool) error { urls = append(urls, u); return nil })
			if (err == nil) != (tc.wantError == "") || err != nil && !strings.Contains(err.Error(), tc.wantError) || pages != tc.pages {
				t.Fatalf("err=%v pages=%d receipts=%d", err, pages, len(urls))
			}
			if tc.total == 0 && len(urls) != 3 {
				t.Fatalf("empty range requested page: %v", urls)
			}
		})
	}
}

func TestAssessmentOverBudgetNeverPages(t *testing.T) {
	// Each populated group needs a deep split to separate 2000 duplicates from one adjacent row.
	// A complete census of these groups cannot fit the receipt budget.
	var pages, counts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Has("resultRecordCount") {
			pages++
			fmt.Fprint(w, `{"features":[]}`)
			return
		}
		counts++
		lo, hi := int64(0), int64(math.MaxInt64)
		if q.Get("where") != "1=1" {
			if _, err := fmt.Sscanf(q.Get("where"), "OBJECTID >= %d AND OBJECTID <= %d", &lo, &hi); err != nil {
				t.Error(err)
			}
		}
		n := 0
		for i := int64(0); i < 100; i++ {
			oid := i*1000000000 + 1
			if lo <= oid && oid <= hi {
				n += 2000
			}
			if lo <= oid+1 && oid+1 <= hi {
				n++
			}
		}
		fmt.Fprintf(w, `{"count":%d}`, n)
	}))
	defer server.Close()
	source, err := NewGISSource(GISSourceConfig{BaseURL: server.URL + "/server/rest/services/OperationalLayers/MapServer/215/query", RateLimit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	_, err = source.CaptureVacancyLayer(context.Background(), AssessedValueLayer, func(string, []byte, int, int, bool) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "receipt limit") || pages != 0 || counts > AssessmentRangeLimit {
		t.Fatalf("err=%v counts=%d pages=%d", err, counts, pages)
	}
}

func TestAssessmentProgramAdmissionBoundary(t *testing.T) {
	for _, tc := range []struct {
		name     string
		groups   int64
		spacing  int64
		want     int
		admitted bool
	}{
		{"largest exact fit", 53, 100000, 2047, true},
		{"first slot over", 215, 10, 2049, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls int
			plan, err := PlanAssessment(tc.groups*2001, func(q url.Values) (int64, error) {
				calls++
				var lo, hi int64
				if _, e := fmt.Sscanf(q.Get("where"), "OBJECTID >= %d AND OBJECTID <= %d", &lo, &hi); e != nil {
					return 0, e
				}
				var n int64
				for i := int64(0); i < tc.groups; i++ {
					oid := i*tc.spacing + 1
					if lo <= oid && oid <= hi {
						n += 2000
					}
					if lo <= oid+1 && oid+1 <= hi {
						n++
					}
				}
				return n, nil
			})
			if tc.admitted {
				if err != nil || plan.receipts != tc.want {
					t.Fatalf("admission=%+v err=%v", plan, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("need %d", tc.want)) {
				t.Fatalf("expected exact rejection: %v", err)
			}
			if calls >= AssessmentRangeLimit {
				t.Fatalf("census exceeded receipt limit: %d", calls)
			}
		})
	}
}

func TestAssessmentProgramReceiptAccounting(t *testing.T) {
	// 2002 rows split into two populated leaves: global, root, two children,
	// two pages, two post-page counts, and final global count.
	counts := map[string]int64{AssessmentWhere(0, math.MaxInt64): 2002, AssessmentWhere(0, math.MaxInt64/2): 1001, AssessmentWhere(math.MaxInt64/2+1, math.MaxInt64): 1001}
	plan, err := PlanAssessment(2002, func(q url.Values) (int64, error) { return counts[q.Get("where")], nil })
	if err != nil || plan.receipts != 9 {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
}
