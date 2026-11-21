package union

import (
	"bytes"
	"context"
	"fmt"
	"log"
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
				}
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

func TestAssessmentVerifiedProgress(t *testing.T) {
	for _, tc := range []struct {
		name, fail string
	}{
		{"complete", ""},
		{"failed second page", "page"},
		{"failed second post-page count", "post-count"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			previous := log.Writer()
			log.SetOutput(&output)
			defer log.SetOutput(previous)
			var rightCounts int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				where := q.Get("where")
				right := where == AssessmentWhere(math.MaxInt64/2+1, math.MaxInt64)
				if q.Has("resultRecordCount") {
					if right && tc.fail == "page" {
						http.Error(w, "failed page", http.StatusBadGateway)
						return
					}
					oid := int64(1)
					if right {
						oid = math.MaxInt64
					}
					fmt.Fprint(w, `{"features":[`)
					for i := 0; i < 1001; i++ {
						if i != 0 {
							fmt.Fprint(w, ",")
						}
						fmt.Fprintf(w, `{"attributes":{"OBJECTID":%d}}`, oid)
					}
					fmt.Fprint(w, `]}`)
					return
				}
				if right {
					rightCounts++
					if rightCounts == 2 && tc.fail == "post-count" {
						fmt.Fprint(w, `{"count":1000}`)
						return
					}
				}
				n := 2002
				if where != "1=1" && where != AssessmentWhere(0, math.MaxInt64) {
					n = 1001
				}
				fmt.Fprintf(w, `{"count":%d}`, n)
			}))
			defer server.Close()
			source, err := NewGISSource(GISSourceConfig{BaseURL: server.URL + "/server/rest/services/OperationalLayers/MapServer/215/query", RateLimit: 1000})
			if err != nil {
				t.Fatal(err)
			}
			_, err = source.CaptureVacancyLayer(context.Background(), AssessedValueLayer, func(string, []byte, int, int, bool) error { return nil })
			if (err == nil) != (tc.fail == "") {
				t.Fatalf("capture error=%v", err)
			}
			logs := output.String()
			if !strings.Contains(logs, "Admitted count plan: receipts=9/2048; count requests=4; resolved leaves=2; resolved rows=2002; checked 0/2002 rows") {
				t.Fatalf("missing resolved planning progress: %s", logs)
			}
			if strings.Contains(logs, "Count ranges: checked 1001") {
				t.Fatalf("planning claimed verified rows: %s", logs)
			}
			if !strings.Contains(logs, "Range checked: 1001/2002 rows") {
				t.Fatalf("missing nonzero verified progress: %s", logs)
			}
			if tc.fail != "" && (strings.Contains(logs, "Range checked: 2002/2002 rows") || strings.Contains(logs, "Capture complete")) {
				t.Fatalf("failed leaf advanced progress: %s", logs)
			}
			if tc.fail == "" && !strings.Contains(logs, "Capture complete: checked 2002/2002 rows") {
				t.Fatalf("missing completion total: %s", logs)
			}
		})
	}
}

func TestAssessmentOverBudgetNeverPages(t *testing.T) {
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
	counts := map[string]int64{AssessmentWhere(0, math.MaxInt64): 2002, AssessmentWhere(0, math.MaxInt64/2): 1001, AssessmentWhere(math.MaxInt64/2+1, math.MaxInt64): 1001}
	plan, err := PlanAssessment(2002, func(q url.Values) (int64, error) { return counts[q.Get("where")], nil })
	if err != nil || plan.receipts != 9 {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
}

func TestAssessmentCountProgressBeforeAdmission(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(previous)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("resultRecordCount") {
			t.Error("page requested before admission")
			return
		}
		where := r.URL.Query().Get("where")
		if where == "1=1" {
			fmt.Fprint(w, `{"count":2002}`)
			return
		}
		var lo, hi int64
		if _, err := fmt.Sscanf(where, "OBJECTID >= %d AND OBJECTID <= %d", &lo, &hi); err != nil {
			t.Error(err)
			return
		}
		n := 0
		if lo <= 1 && 1 <= hi {
			n += 2001
		}
		if lo <= math.MaxInt64 && math.MaxInt64 <= hi {
			n++
		}
		fmt.Fprintf(w, `{"count":%d}`, n)
	}))
	defer server.Close()
	source, err := NewGISSource(GISSourceConfig{BaseURL: server.URL + "/server/rest/services/OperationalLayers/MapServer/215/query", RateLimit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	_, err = source.CaptureVacancyLayer(context.Background(), AssessedValueLayer, func(string, []byte, int, int, bool) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "indivisible") {
		t.Fatalf("expected census rejection: %v", err)
	}
	if !strings.Contains(output.String(), "Count ranges: checked 0/2002 rows; requests=10/2048; resolved leaves=1; resolved rows=1") {
		t.Fatalf("count progress misstates resolved work: %s", output.String())
	}
	if strings.Contains(output.String(), "Range checked:") {
		t.Fatalf("verified without pages: %s", output.String())
	}
}

func TestAssessmentRejectedSplitDoesNotCreditRows(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(previous)
	var counts, pages int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("resultRecordCount") {
			pages++
			return
		}
		counts++
		n := 2002
		switch counts {
		case 4, 6, 8:
			n = 0
		case 9:
			n = 1001
		case 10:
			n = 1002
		}
		fmt.Fprintf(w, `{"count":%d}`, n)
	}))
	defer server.Close()
	source, err := NewGISSource(GISSourceConfig{BaseURL: server.URL + "/server/rest/services/OperationalLayers/MapServer/215/query", RateLimit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	_, err = source.CaptureVacancyLayer(context.Background(), AssessedValueLayer, func(string, []byte, int, int, bool) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "partition sum mismatch") || counts != 10 || pages != 0 {
		t.Fatalf("err=%v counts=%d pages=%d", err, counts, pages)
	}
	if !strings.Contains(output.String(), "Count ranges: checked 0/2002 rows; requests=10/2048; resolved leaves=0; resolved rows=0") {
		t.Fatalf("rejected split credited rows: %s", output.String())
	}
}
