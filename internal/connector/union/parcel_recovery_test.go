package union_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
)

func TestFetchParcelPartsSafetyAndBudgets(t *testing.T) {
	for _, kind := range []string{"multipart", "fallback", "escaped PID", "missing", "changed", "incomplete", "duplicate", "wrong PID", "missing selected", "requests", "features", "deadline"} {
		t.Run(kind, func(t *testing.T) {
			pid := "A"
			if kind == "fallback" {
				pid = "1"
			}
			if kind == "escaped PID" {
				pid = "A'B"
			}
			wirePID := pid
			if kind == "fallback" {
				wirePID = ""
			}
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				q := r.URL.Query()
				where := q.Get("where")
				if strings.HasPrefix(where, "OBJECTID") {
					if kind == "missing" {
						fmt.Fprint(w, `{"features":[]}`)
						return
					}
					value := wirePID
					if kind == "changed" {
						value = "changed"
					}
					fmt.Fprintf(w, `{"features":[{"attributes":{"OBJECTID":1,"PID":%q}}]}`, value)
					return
				}
				if !strings.Contains(where, "PID = '"+strings.ReplaceAll(pid, "'", "''")+"'") {
					t.Errorf("unsafe predicate %q", where)
				}
				if kind == "fallback" && !strings.Contains(where, "PID IS NULL") {
					t.Error("missing fallback predicate")
				}
				if q.Get("returnCountOnly") == "true" {
					fmt.Fprint(w, `{"count":2}`)
					return
				}
				offset, _ := strconv.Atoi(q.Get("resultOffset"))
				oid := offset + 1
				value := wirePID
				if kind == "incomplete" && offset == 1 {
					fmt.Fprint(w, `{"features":[]}`)
					return
				}
				if kind == "duplicate" {
					oid = 1
				}
				if kind == "wrong PID" {
					value = "wrong"
				}
				if kind == "missing selected" {
					oid += 10
				}
				if kind == "fallback" && oid == 2 {
					value = "1"
				}
				fmt.Fprintf(w, `{"features":[{"attributes":{"OBJECTID":%d,"PID":%q}}]}`, oid, value)
			}))
			defer server.Close()
			source, err := union.NewGISSource(union.GISSourceConfig{BaseURL: server.URL, BatchSize: 1, RateLimit: 1000000})
			if err != nil {
				t.Fatal(err)
			}
			budget := union.DefaultParcelFetchBudget()
			if kind == "requests" {
				budget.Requests = 2
			}
			if kind == "features" {
				budget.Features = 2
			}
			ctx := context.Background()
			if kind == "deadline" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
				defer cancel()
			}
			records, err := source.FetchParcelParts(ctx, 1, pid, budget)
			success := kind == "multipart" || kind == "fallback" || kind == "escaped PID"
			if success {
				if err != nil || len(records) != 2 {
					t.Fatalf("parts=%d error=%v", len(records), err)
				}
			} else if err == nil {
				t.Fatal("unsafe source accepted")
			}
			if kind == "requests" && requests != 2 {
				t.Fatalf("requests=%d", requests)
			}
			if kind == "features" && requests != 2 {
				t.Fatalf("feature exhaustion requested PID pages, requests=%d", requests)
			}
			if kind == "deadline" && requests != 0 {
				t.Fatalf("expired deadline sent %d requests", requests)
			}
		})
	}
}

func TestParcelSourceDoesNotFollowUnbudgetedRedirects(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Location", "/another")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()
	source, err := union.NewGISSource(union.GISSourceConfig{BaseURL: server.URL, RateLimit: 1000000})
	if err != nil {
		t.Fatal(err)
	}
	_, err = source.FetchParcelParts(context.Background(), 1, "A", union.DefaultParcelFetchBudget())
	if err == nil || requests != 1 {
		t.Fatalf("error=%v requests=%d", err, requests)
	}
}
