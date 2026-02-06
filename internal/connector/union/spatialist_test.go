package union

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewUnionCountySpatialist(t *testing.T) {
	tests := []struct {
		name    string
		config  SpatialistConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: SpatialistConfig{
				BaseURL:      "https://example.com/search",
				RateLimitRPS: 1,
			},
			wantErr: false,
		},
		{
			name: "empty baseURL",
			config: SpatialistConfig{
				BaseURL: "",
			},
			wantErr: true,
		},
		{
			name: "invalid scheme",
			config: SpatialistConfig{
				BaseURL: "ftp://example.com/search",
			},
			wantErr: true,
		},
		{
			name: "missing host",
			config: SpatialistConfig{
				BaseURL: "https:///search",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewUnionCountySpatialist(tt.config)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestUnionCountySpatialist_FetchByParcelID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/P-123") {
			http.Error(w, "missing parcelId path", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok": true}`))
	}))
	defer server.Close()

	client, err := NewUnionCountySpatialist(SpatialistConfig{
		BaseURL:      server.URL,
		RateLimitRPS: 1,
	})
	if err != nil {
		t.Fatalf("failed to create spatialist client: %v", err)
	}

	record, err := client.FetchByParcelID(context.Background(), "P-123")
	if err != nil {
		t.Fatalf("FetchByParcelID returned error: %v", err)
	}

	if record.ParcelID != "P-123" {
		t.Fatalf("ParcelID = %q, want %q", record.ParcelID, "P-123")
	}

	if record.SourceName != client.Name() {
		t.Fatalf("SourceName = %q, want %q", record.SourceName, client.Name())
	}
}

func TestUnionCountySpatialist_FetchByParcelID_EmptyParcelID(t *testing.T) {
	client, err := NewUnionCountySpatialist(SpatialistConfig{
		BaseURL:      "https://example.com/search",
		RateLimitRPS: 1,
	})
	if err != nil {
		t.Fatalf("failed to create spatialist client: %v", err)
	}

	if _, err := client.FetchByParcelID(context.Background(), ""); err == nil {
		t.Fatalf("expected error for empty parcel ID")
	}
}

func TestUnionCountySpatialist_FetchByParcelID_BadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusTeapot)
	}))
	defer server.Close()

	client, err := NewUnionCountySpatialist(SpatialistConfig{
		BaseURL:      server.URL,
		RateLimitRPS: 1,
	})
	if err != nil {
		t.Fatalf("failed to create spatialist client: %v", err)
	}

	if _, err := client.FetchByParcelID(context.Background(), "P-123"); err == nil {
		t.Fatalf("expected error for non-2xx response")
	}
}

func TestUnionCountySpatialist_FetchByParcelID_BadJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":`))
	}))
	defer server.Close()

	client, err := NewUnionCountySpatialist(SpatialistConfig{
		BaseURL:      server.URL,
		RateLimitRPS: 1,
	})
	if err != nil {
		t.Fatalf("failed to create spatialist client: %v", err)
	}

	if _, err := client.FetchByParcelID(context.Background(), "P-123"); err == nil {
		t.Fatalf("expected error for invalid json")
	}
}
