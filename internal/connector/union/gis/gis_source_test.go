package union

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/connector"
)

func TestNewUnionCountyGISSource(t *testing.T) {
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
			source, err := NewUnionCountyGISSource(tt.config)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errMsg != "" && !containsString(err.Error(), tt.errMsg) {
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

			// Verify default batch size is applied
			if tt.config.BatchSize <= 0 && source.batchSize != 50 {
				t.Errorf("expected default batch size 50, got %d", source.batchSize)
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

	if config.BatchSize != 50 {
		t.Errorf("BatchSize = %d, want 50", config.BatchSize)
	}
}

func TestUnionCountyGISSource_Name(t *testing.T) {
	source, err := NewUnionCountyGISSource(GISSourceConfig{
		BaseURL:   "https://example.com/api",
		RateLimit: 1,
		BatchSize: 50,
	})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	if got := source.Name(); got != "union_county_gis" {
		t.Errorf("Name() = %q, want %q", got, "union_county_gis")
	}
}

func TestUnionCountyGISSource_County(t *testing.T) {
	source, err := NewUnionCountyGISSource(GISSourceConfig{
		BaseURL:   "https://example.com/api",
		RateLimit: 1,
		BatchSize: 50,
	})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	if got := source.County(); got != "union" {
		t.Errorf("County() = %q, want %q", got, "union")
	}
}

func TestUnionCountyGISSource_SourceType(t *testing.T) {
	source, err := NewUnionCountyGISSource(GISSourceConfig{
		BaseURL:   "https://example.com/api",
		RateLimit: 1,
		BatchSize: 50,
	})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	if got := source.SourceType(); got != "gis" {
		t.Errorf("SourceType() = %q, want %q", got, "gis")
	}
}

func TestUnionCountyGISSource_SupportsIncremental(t *testing.T) {
	source, err := NewUnionCountyGISSource(GISSourceConfig{
		BaseURL:   "https://example.com/api",
		RateLimit: 1,
		BatchSize: 50,
	})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	if source.SupportsIncremental() {
		t.Error("SupportsIncremental() should return false")
	}
}

func TestUnionCountyGISSource_FetchSince(t *testing.T) {
	source, err := NewUnionCountyGISSource(GISSourceConfig{
		BaseURL:   "https://example.com/api",
		RateLimit: 1,
		BatchSize: 50,
	})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	ctx := context.Background()
	_, err = source.FetchSince(ctx, time.Now())
	if err == nil {
		t.Error("FetchSince() should return error for unsupported operation")
	}
}

func TestUnionCountyGISSource_Fetch_ContextCancellation(t *testing.T) {
	// Create a mock server that returns a count
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"count": 100}`))
	}))
	defer server.Close()

	// Create source with test server URL
	source, err := NewUnionCountyGISSource(GISSourceConfig{
		BaseURL:   server.URL,
		RateLimit: 10,
		BatchSize: 10,
	})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	// Replace the http client with the test server's client
	source.httpClient = NewRateLimitedClient(server.Client(), 10)

	// Create already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = source.Fetch(ctx)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestUnionCountyGISSource_FetchBatches(t *testing.T) {
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
			pid := fmt.Sprintf("PID-%d", i+1)
			features = append(features, feature{
				Attributes: map[string]string{"PID": pid},
			})
		}

		enc := json.NewEncoder(w)
		if err := enc.Encode(response{Features: features}); err != nil {
			t.Fatalf("failed to encode response: %v", err)
		}
	}))
	defer server.Close()

	source, err := NewUnionCountyGISSource(GISSourceConfig{
		BaseURL:   server.URL,
		RateLimit: 100,
		BatchSize: 2,
	})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}
	source.httpClient = NewRateLimitedClient(server.Client(), 100)

	var batches [][]string
	ctx := context.Background()
	err = source.FetchBatches(ctx, func(batch []connector.RawRecord) error {
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
			wantErr: false,
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
			wantErr: false,
			checks: map[string]string{
				"f": "geojson",
			},
		},
		{
			name: "invalid format",
			params: ArcGISParams{
				Where:  "1=1",
				Format: "xml",
			},
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
			wantErr: false,
			checks: map[string]string{
				"resultOffset":      "100",
				"resultRecordCount": "50",
			},
		},
		{
			name: "with geometry",
			params: ArcGISParams{
				Where:          "1=1",
				Format:         "json",
				ReturnGeometry: true,
			},
			wantErr: false,
			checks: map[string]string{
				"returnGeometry": "true",
			},
		},
		{
			name: "with outFields",
			params: ArcGISParams{
				Where:     "1=1",
				Format:    "json",
				OutFields: "PID,OWNER",
			},
			wantErr: false,
			checks: map[string]string{
				"outFields": "PID,OWNER",
			},
		},
		{
			name: "empty outFields defaults to *",
			params: ArcGISParams{
				Where:     "1=1",
				Format:    "json",
				OutFields: "",
			},
			wantErr: false,
			checks: map[string]string{
				"outFields": "*",
			},
		},
		{
			name: "zero offset not included",
			params: ArcGISParams{
				Where:        "1=1",
				Format:       "json",
				ResultOffset: 0,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values, err := tt.params.ToURLValues()

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errMsg != "" && !containsString(err.Error(), tt.errMsg) {
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
		{
			name:    "valid https URL",
			url:     "https://example.com/api/query",
			wantErr: false,
		},
		{
			name:    "valid http URL",
			url:     "http://example.com/api/query",
			wantErr: false,
		},
		{
			name:    "missing scheme",
			url:     "example.com/api",
			wantErr: true,
			errMsg:  "must include scheme",
		},
		{
			name:    "missing host",
			url:     "https:///api/query",
			wantErr: true,
			errMsg:  "must include host",
		},
		{
			name:    "invalid scheme",
			url:     "ftp://example.com/api",
			wantErr: true,
			errMsg:  "must include scheme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateURL(tt.url)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errMsg != "" && !containsString(err.Error(), tt.errMsg) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errMsg)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestUnionCountyGISSource_buildURL(t *testing.T) {
	source, err := NewUnionCountyGISSource(GISSourceConfig{
		BaseURL:   "https://example.com/api",
		RateLimit: 1,
		BatchSize: 50,
	})
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}

	params := ArcGISParams{
		Where:  "1=1",
		Format: "json",
	}

	url, err := source.buildURL(params)
	if err != nil {
		t.Fatalf("buildURL failed: %v", err)
	}

	if !containsString(url, "https://example.com/api?") {
		t.Errorf("URL should start with base URL, got: %s", url)
	}

	if !containsString(url, "where=1%3D1") {
		t.Errorf("URL should contain encoded where param, got: %s", url)
	}
}

// containsString checks if substr is in s
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
