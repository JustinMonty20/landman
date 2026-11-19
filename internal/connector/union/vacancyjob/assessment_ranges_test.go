package vacancyjob

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/taxartifact"
)

func TestAssessmentRangeCaptureReplay(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := taxartifact.NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	mode := "valid"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		q := r.URL.Query()
		where := q.Get("where")
		left := where == "1=1" || where == union.AssessmentWhere(0, math.MaxInt64) || where == union.AssessmentWhere(0, math.MaxInt64/2)
		right := where == "1=1" || where == union.AssessmentWhere(0, math.MaxInt64) || where == union.AssessmentWhere(math.MaxInt64/2+1, math.MaxInt64)
		if !left && !right {
			http.Error(w, "unsupported predicate", 400)
			return
		}
		n := 0
		if left {
			n += 1001
		}
		if right {
			n += 1001
		}
		if q.Get("returnCountOnly") == "true" {
			fmt.Fprintf(w, `{"count":%d}`, n)
			return
		}
		if q.Get("returnGeometry") != "false" || q.Get("outFields") != "*" || q.Get("resultRecordCount") != "2000" {
			http.Error(w, "unsupported query", 400)
			return
		}
		fmt.Fprint(w, `{"features":[`)
		for i := 0; i < n; i++ {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			oid := int64(1)
			if right && !left {
				oid = math.MaxInt64
			}
			if mode == "wrong OID" && right && !left && i == 0 {
				oid = 1
			}
			fmt.Fprintf(w, `{"attributes":{"OBJECTID":%d,"parcel_number":"001","assess_category":"AV","market_building":0}}`, oid)
		}
		fmt.Fprint(w, `]}`)
	}))
	defer server.Close()
	source, err := union.NewGISSource(union.GISSourceConfig{BaseURL: server.URL + "/server/rest/services/OperationalLayers/MapServer/215/query", RateLimit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	capture := func() (ingest.VerifiedCapture, error) {
		r, err := ingest.NewRecorder(store, "union_assessed_value_0", time.Now())
		if err != nil {
			return ingest.VerifiedCapture{}, err
		}
		_, err = source.CaptureVacancyLayer(ctx, union.AssessedValueLayer, func(u string, b []byte, crs, status int, complete bool) error {
			return r.RecordHTTP(ctx, u, "", crs, status, complete, strings.NewReader(string(b)))
		})
		if err != nil {
			return ingest.VerifiedCapture{}, err
		}
		if err = r.BindAttempt(AssessedValueID, "00000000-0000-0000-0000-000000000001", "full_scan", 0); err != nil {
			return ingest.VerifiedCapture{}, err
		}
		return r.Verify(ctx)
	}
	v, err := capture()
	if err != nil {
		t.Fatal(err)
	}
	pub := &publisher{layer: union.AssessedValueLayer}
	if err := pub.Prepare(ctx, v); err != nil || len(pub.assessments) != 2002 || len(v.Manifest().Receipts) != 10 {
		t.Fatalf("partition replay: %v rows=%d receipts=%d", err, len(pub.assessments), len(v.Manifest().Receipts))
	}
	for _, row := range pub.assessments {
		if _, err := strconv.ParseInt(row.ObjectID, 10, 64); err != nil {
			t.Fatal(err)
		}
	}
	requests := calls
	if err := (&publisher{layer: union.AssessedValueLayer}).Prepare(ctx, v); err != nil || calls != requests {
		t.Fatalf("offline replay: %v requests=%d", err, calls-requests)
	}
	for _, tc := range []struct {
		name   string
		change func(int, string, []byte) (string, []byte, bool)
	}{
		{"missing later page", func(i int, u string, b []byte) (string, []byte, bool) { return u, b, i != 7 }},
		{"changed partition count", func(i int, u string, b []byte) (string, []byte, bool) {
			if i == 6 {
				b = []byte(`{"count":1000}`)
			}
			return u, b, true
		}},
		{"unsupported query", func(i int, u string, b []byte) (string, []byte, bool) {
			if i == 7 {
				u += "&resultOffset=1"
			}
			return u, b, true
		}},
		{"partition predicate drift", func(i int, u string, b []byte) (string, []byte, bool) {
			if i == 6 {
				parsed, _ := url.Parse(u)
				q := parsed.Query()
				q.Set("where", "OBJECTID >= 0")
				parsed.RawQuery = q.Encode()
				u = parsed.String()
			}
			return u, b, true
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := ingest.NewRecorder(store, "union_assessed_value_0", time.Now())
			if err != nil {
				t.Fatal(err)
			}
			for i, receipt := range v.Manifest().Receipts {
				body, err := v.Open(ctx, i)
				if err != nil {
					t.Fatal(err)
				}
				raw, err := io.ReadAll(body)
				if err != nil {
					t.Fatal(err)
				}
				if err = body.Close(); err != nil {
					t.Fatal(err)
				}
				u, b, keep := tc.change(i, receipt.Request, raw)
				if keep {
					if err = r.RecordHTTP(ctx, u, "", 0, 200, true, strings.NewReader(string(b))); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err = r.BindAttempt(AssessedValueID, "00000000-0000-0000-0000-000000000001", "full_scan", 0); err != nil {
				t.Fatal(err)
			}
			modified, err := r.Verify(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err = (&publisher{layer: union.AssessedValueLayer}).Prepare(ctx, modified); err == nil {
				t.Fatal("invalid partition replay accepted")
			}
		})
	}
	mode = "wrong OID"
	if _, err := capture(); err == nil {
		t.Fatal("out of range row accepted")
	}
}
