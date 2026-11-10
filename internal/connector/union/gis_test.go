package union

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/connector/shared/httpclient"
)

func TestNewGISSource(t *testing.T) {
	tests := []struct {
		name    string
		config  GISSourceConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: GISSourceConfig{
				BaseURL:   "https://example.com/api",
				RateLimit: 1,
				BatchSize: 100,
			},
			wantErr: false,
		},
		{
			name: "empty baseURL",
			config: GISSourceConfig{
				BaseURL:   "",
				RateLimit: 1,
				BatchSize: 100,
			},
			wantErr: true,
			errMsg:  "baseURL cannot be empty",
		},
		{
			name: "invalid URL - no scheme",
			config: GISSourceConfig{
				BaseURL:   "example.com/api",
				RateLimit: 1,
				BatchSize: 100,
			},
			wantErr: true,
			errMsg:  "must include scheme",
		},
		{
			name: "invalid URL - no host",
			config: GISSourceConfig{
				BaseURL:   "https:///api",
				RateLimit: 1,
				BatchSize: 100,
			},
			wantErr: true,
			errMsg:  "must include host",
		},
		{
			name: "zero batch size uses default",
			config: GISSourceConfig{
				BaseURL:   "https://example.com/api",
				RateLimit: 1,
				BatchSize: 0,
			},
			wantErr: false,
		},
		{
			name: "negative batch size uses default",
			config: GISSourceConfig{
				BaseURL:   "https://example.com/api",
				RateLimit: 1,
				BatchSize: -10,
			},
			wantErr: false,
		},
		{
			name: "zero rate limit uses default",
			config: GISSourceConfig{
				BaseURL:   "https://example.com/api",
				RateLimit: 0,
				BatchSize: 50,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, err := NewGISSource(tt.config)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if source == nil {
				t.Fatal("expected non-nil source")
			}

			if tt.config.BatchSize <= 0 && source.batchSize != 500 {
				t.Errorf("expected default batch size 500, got %d", source.batchSize)
			}
		})
	}
}

func TestDefaultGISSourceConfig(t *testing.T) {
	config := DefaultGISSourceConfig()

	if config.BaseURL == "" {
		t.Error("BaseURL should not be empty")
	}

	expectedURL := "https://atlas.unioncountync.gov/server/rest/services/OperationalLayers/MapServer/215/query"
	if config.BaseURL != expectedURL {
		t.Errorf("BaseURL = %q, want %q", config.BaseURL, expectedURL)
	}

	if config.RateLimit != 1 {
		t.Errorf("RateLimit = %d, want 1", config.RateLimit)
	}

	if config.BatchSize != 500 {
		t.Errorf("BatchSize = %d, want 500", config.BatchSize)
	}
}

func TestGISSource_Name(t *testing.T) {
	source, err := NewGISSource(GISSourceConfig{BaseURL: "https://example.com/api", RateLimit: 1, BatchSize: 50})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}
	if got := source.Name(); got != "union_county_gis" {
		t.Errorf("Name() = %q, want %q", got, "union_county_gis")
	}
}

func TestGISSource_County(t *testing.T) {
	source, err := NewGISSource(GISSourceConfig{BaseURL: "https://example.com/api", RateLimit: 1, BatchSize: 50})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}
	if got := source.County(); got != "union" {
		t.Errorf("County() = %q, want %q", got, "union")
	}
}

func TestGISSource_SourceType(t *testing.T) {
	source, err := NewGISSource(GISSourceConfig{BaseURL: "https://example.com/api", RateLimit: 1, BatchSize: 50})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}
	if got := source.SourceType(); got != "gis" {
		t.Errorf("SourceType() = %q, want %q", got, "gis")
	}
}

func TestGISSource_SupportsIncremental(t *testing.T) {
	source, err := NewGISSource(GISSourceConfig{BaseURL: "https://example.com/api", RateLimit: 1, BatchSize: 50})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}
	if source.SupportsIncremental() {
		t.Error("SupportsIncremental() should return false")
	}
}

func TestGISSource_FetchSince(t *testing.T) {
	source, err := NewGISSource(GISSourceConfig{BaseURL: "https://example.com/api", RateLimit: 1, BatchSize: 50})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}
	if _, err := source.FetchSince(context.Background(), time.Now()); err == nil {
		t.Error("FetchSince() should return error for unsupported operation")
	}
}

func TestGISSource_Fetch_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"count": 100}`))
	}))
	defer server.Close()

	source, err := NewGISSource(GISSourceConfig{BaseURL: server.URL, RateLimit: 10, BatchSize: 10})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}
	source.httpClient = httpclient.NewRateLimitedClient(server.Client(), 10)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := source.Fetch(ctx); err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestGISSource_FetchBatches(t *testing.T) {
	const total = 5

	type feature struct {
		Attributes map[string]string `json:"attributes"`
	}
	type response struct {
		Features []feature `json:"features"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		q := r.URL.Query()
		if q.Get("returnCountOnly") == "true" {
			fmt.Fprintf(w, `{"count": %d}`, total)
			return
		}

		offset, _ := strconv.Atoi(q.Get("resultOffset"))
		limit, _ := strconv.Atoi(q.Get("resultRecordCount"))
		if limit <= 0 {
			limit = total
		}

		end := offset + limit
		if end > total {
			end = total
		}

		features := make([]feature, 0, end-offset)
		for i := offset; i < end; i++ {
			features = append(features, feature{
				Attributes: map[string]string{"PID": fmt.Sprintf("PID-%d", i+1)},
			})
		}

		enc := json.NewEncoder(w)
		if err := enc.Encode(response{Features: features}); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	source, err := NewGISSource(GISSourceConfig{BaseURL: server.URL, RateLimit: 100, BatchSize: 2})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}
	source.httpClient = httpclient.NewRateLimitedClient(server.Client(), 100)

	var batches [][]string
	err = source.FetchBatches(context.Background(), func(batch []connector.RawRecord) error {
		ids := make([]string, 0, len(batch))
		for _, r := range batch {
			ids = append(ids, r.ParcelID)
		}
		batches = append(batches, ids)
		return nil
	})
	if err != nil {
		t.Fatalf("FetchBatches returned error: %v", err)
	}

	if got := len(batches); got != 3 {
		t.Fatalf("expected 3 batches, got %d", got)
	}

	wantSizes := []int{2, 2, 1}
	for i, want := range wantSizes {
		if got := len(batches[i]); got != want {
			t.Errorf("batch %d size = %d, want %d", i, got, want)
		}
	}

	var all []string
	for _, batch := range batches {
		all = append(all, batch...)
	}
	if got := len(all); got != total {
		t.Fatalf("expected %d total IDs, got %d", total, got)
	}

	for i := 0; i < total; i++ {
		want := fmt.Sprintf("PID-%d", i+1)
		if all[i] != want {
			t.Errorf("id %d = %q, want %q", i, all[i], want)
		}
	}
}

func TestGISSource_FetchBatches_NonSuccessStatus(t *testing.T) {
	t.Run("count request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"count":0}`)
		}))
		defer server.Close()

		source := newTestGISSource(t, server)
		err := source.FetchBatches(context.Background(), func(batch []connector.RawRecord) error {
			t.Fatal("hook should not run")
			return nil
		})
		if err == nil || !strings.Contains(err.Error(), "500") {
			t.Fatalf("FetchBatches error = %v, want status 500", err)
		}
	})

	t.Run("feature request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("returnCountOnly") == "true" {
				fmt.Fprint(w, `{"count":1}`)
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, `{"features":[{"attributes":{"PID":"P1"}}]}`)
		}))
		defer server.Close()

		source := newTestGISSource(t, server)
		err := source.FetchBatches(context.Background(), func(batch []connector.RawRecord) error {
			t.Fatal("hook should not run")
			return nil
		})
		if err == nil || !strings.Contains(err.Error(), "500") {
			t.Fatalf("FetchBatches error = %v, want status 500", err)
		}
	})
}

func TestGISSource_FetchBatches_MissingCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"error":{"code":400,"message":"Unable to complete operation.","details":[]}}`)
	}))
	defer server.Close()

	source := newTestGISSource(t, server)
	err := source.FetchBatches(context.Background(), func(batch []connector.RawRecord) error {
		t.Fatal("hook should not run")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "count") {
		t.Fatalf("FetchBatches error = %v, want missing count", err)
	}
}

func TestGISSource_FetchBatches_ZeroCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"count":0}`)
	}))
	defer server.Close()

	source := newTestGISSource(t, server)
	err := source.FetchBatches(context.Background(), func(batch []connector.RawRecord) error {
		t.Fatal("hook should not run for an empty layer")
		return nil
	})
	if err != nil {
		t.Fatalf("FetchBatches error = %v, want nil for count 0", err)
	}
}

func TestGISSource_FetchBatches_DefaultFeaturePage(t *testing.T) {
	const (
		total    = 1000
		pageSize = 500
	)

	type feature struct {
		Attributes map[string]string `json:"attributes"`
	}
	type response struct {
		Features []feature `json:"features"`
	}

	page := response{Features: make([]feature, pageSize)}
	for i := range page.Features {
		page.Features[i] = feature{Attributes: map[string]string{"PID": fmt.Sprintf("P%d", i)}}
	}

	var requests []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		requests = append(requests, r.URL.Query())

		if r.URL.Query().Get("returnCountOnly") == "true" {
			fmt.Fprintf(w, `{"count": %d}`, total)
			return
		}

		if err := json.NewEncoder(w).Encode(page); err != nil {
			t.Errorf("encode features: %v", err)
		}
	}))
	defer server.Close()

	cfg := DefaultGISSourceConfig()
	cfg.BaseURL = server.URL
	source, err := NewGISSource(cfg)
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}
	source.httpClient = httpclient.NewRateLimitedClient(server.Client(), 100)

	var hooks int
	err = source.FetchBatches(context.Background(), func(batch []connector.RawRecord) error {
		hooks++
		return nil
	})
	if err != nil {
		t.Fatalf("FetchBatches returned error: %v", err)
	}

	if len(requests) < 3 {
		t.Fatalf("requests = %d, want at least 3", len(requests))
	}

	count := requests[0]
	if count.Get("returnCountOnly") != "true" {
		t.Errorf("count returnCountOnly = %q, want true", count.Get("returnCountOnly"))
	}
	if _, ok := count["orderByFields"]; ok {
		t.Errorf("count request sent orderByFields=%q", count.Get("orderByFields"))
	}

	first := requests[1]
	if first.Get("resultRecordCount") != "500" {
		t.Errorf("first resultRecordCount = %q, want 500", first.Get("resultRecordCount"))
	}
	if first.Get("orderByFields") != "OBJECTID" {
		t.Errorf("first orderByFields = %q, want OBJECTID", first.Get("orderByFields"))
	}
	if first.Get("returnGeometry") != "true" {
		t.Errorf("first returnGeometry = %q, want true", first.Get("returnGeometry"))
	}
	if _, ok := first["resultOffset"]; ok {
		t.Errorf("first request sent resultOffset=%q", first.Get("resultOffset"))
	}

	second := requests[2]
	if second.Get("resultRecordCount") != "500" {
		t.Errorf("second resultRecordCount = %q, want 500", second.Get("resultRecordCount"))
	}
	if second.Get("orderByFields") != "OBJECTID" {
		t.Errorf("second orderByFields = %q, want OBJECTID", second.Get("orderByFields"))
	}
	if second.Get("resultOffset") != "500" {
		t.Errorf("second resultOffset = %q, want 500", second.Get("resultOffset"))
	}

	if hooks != 2 {
		t.Errorf("hook runs = %d, want 2", hooks)
	}
}

func TestGISSource_FetchBatches_CountMismatch(t *testing.T) {
	const total = 4

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("returnCountOnly") == "true" {
			fmt.Fprintf(w, `{"count": %d}`, total)
			return
		}

		n := 2
		if offset, _ := strconv.Atoi(r.URL.Query().Get("resultOffset")); offset > 0 {
			n = 1
		}
		fmt.Fprint(w, `{"features":[`)
		for i := 0; i < n; i++ {
			if i > 0 {
				fmt.Fprint(w, ",")
			}
			fmt.Fprintf(w, `{"attributes":{"PID":"P%d"}}`, i)
		}
		fmt.Fprint(w, `]}`)
	}))
	defer server.Close()

	source, err := NewGISSource(GISSourceConfig{BaseURL: server.URL, RateLimit: 100, BatchSize: 2})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}
	source.httpClient = httpclient.NewRateLimitedClient(server.Client(), 100)

	var hooks int
	err = source.FetchBatches(context.Background(), func(batch []connector.RawRecord) error {
		if hooks == 0 && len(batch) != 2 {
			t.Errorf("page 0 length = %d, want 2", len(batch))
		}
		if hooks > 0 {
			t.Errorf("hook ran for page %d with %d records", hooks, len(batch))
		}
		hooks++
		return nil
	})

	msg := ""
	if err != nil {
		msg = err.Error()
	}
	if !strings.Contains(msg, "count mismatch") || !strings.Contains(msg, "page 1") || !strings.Contains(msg, "got 1") || !strings.Contains(msg, "want 2") {
		t.Errorf("FetchBatches error = %q, want count mismatch for page 1 got 1 want 2", msg)
	}
	if hooks != 1 {
		t.Errorf("hook runs = %d, want 1", hooks)
	}
}

func newTestGISSource(t *testing.T, server *httptest.Server) *GISSource {
	t.Helper()
	source, err := NewGISSource(GISSourceConfig{BaseURL: server.URL, RateLimit: 100, BatchSize: 50})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}
	source.httpClient = httpclient.NewRateLimitedClient(server.Client(), 100)
	return source
}

func TestArcGISParams_ToURLValues(t *testing.T) {
	tests := []struct {
		name    string
		params  ArcGISParams
		wantErr bool
		errMsg  string
		checks  map[string]string
	}{
		{
			name: "valid json format",
			params: ArcGISParams{
				Where:           "1=1",
				ReturnCountOnly: true,
				Format:          "json",
			},
			checks: map[string]string{
				"where":           "1=1",
				"returnCountOnly": "true",
				"f":               "json",
			},
		},
		{
			name: "valid geojson format",
			params: ArcGISParams{
				Where:  "1=1",
				Format: "geojson",
			},
			checks: map[string]string{"f": "geojson"},
		},
		{
			name:    "invalid format",
			params:  ArcGISParams{Where: "1=1", Format: "xml"},
			wantErr: true,
			errMsg:  "invalid format",
		},
		{
			name: "with pagination params",
			params: ArcGISParams{
				Where:             "1=1",
				Format:            "json",
				ResultOffset:      100,
				ResultRecordCount: 50,
			},
			checks: map[string]string{
				"resultOffset":      "100",
				"resultRecordCount": "50",
			},
		},
		{
			name:   "with geometry",
			params: ArcGISParams{Where: "1=1", Format: "json", ReturnGeometry: true},
			checks: map[string]string{"returnGeometry": "true"},
		},
		{
			name:   "with outFields",
			params: ArcGISParams{Where: "1=1", Format: "json", OutFields: "PID,OWNER"},
			checks: map[string]string{"outFields": "PID,OWNER"},
		},
		{
			name:   "empty outFields defaults to *",
			params: ArcGISParams{Where: "1=1", Format: "json", OutFields: ""},
			checks: map[string]string{"outFields": "*"},
		},
		{
			name:   "zero offset not included",
			params: ArcGISParams{Where: "1=1", Format: "json", ResultOffset: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values, err := tt.params.ToURLValues()

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			for key, want := range tt.checks {
				if got := values.Get(key); got != want {
					t.Errorf("param %q = %q, want %q", key, got, want)
				}
			}
		})
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
		errMsg  string
	}{
		{name: "valid https URL", url: "https://example.com/api/query"},
		{name: "valid http URL", url: "http://example.com/api/query"},
		{name: "missing scheme", url: "example.com/api", wantErr: true, errMsg: "must include scheme"},
		{name: "missing host", url: "https:///api/query", wantErr: true, errMsg: "must include host"},
		{name: "invalid scheme", url: "ftp://example.com/api", wantErr: true, errMsg: "must include scheme"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateURL(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestGISSource_buildURL(t *testing.T) {
	source, err := NewGISSource(GISSourceConfig{BaseURL: "https://example.com/api", RateLimit: 1, BatchSize: 50})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	rawURL, err := source.buildURL(ArcGISParams{Where: "1=1", Format: "json"})
	if err != nil {
		t.Fatalf("buildURL failed: %v", err)
	}

	if !strings.Contains(rawURL, "https://example.com/api?") {
		t.Errorf("URL should start with base URL, got: %s", rawURL)
	}
	if !strings.Contains(rawURL, "where=1%3D1") {
		t.Errorf("URL should contain encoded where param, got: %s", rawURL)
	}
}
