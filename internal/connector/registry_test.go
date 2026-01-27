package connector

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/models"
)

// mockDataSource implements DataSource interface for testing
type mockDataSource struct {
	name       string
	county     string
	sourceType string
}

func (m *mockDataSource) Name() string       { return m.name }
func (m *mockDataSource) County() string     { return m.county }
func (m *mockDataSource) SourceType() string { return m.sourceType }
func (m *mockDataSource) Fetch(ctx context.Context) ([]RawRecord, error) {
	return nil, nil
}
func (m *mockDataSource) SupportsIncremental() bool { return false }
func (m *mockDataSource) FetchSince(ctx context.Context, since time.Time) ([]RawRecord, error) {
	return nil, nil
}

// mockTranslator implements Translator interface for testing
type mockTranslator struct {
	canTranslate bool
	priority     int
}

func (m *mockTranslator) Translate(raw RawRecord) (*models.Parcel, error) {
	return &models.Parcel{ParcelId: raw.ParcelID}, nil
}
func (m *mockTranslator) CanTranslate(sourceName string) bool { return m.canTranslate }
func (m *mockTranslator) Priority() int                       { return m.priority }

// mockMerger implements Merger interface for testing
type mockMerger struct{}

func (m *mockMerger) Merge(parcels []*models.Parcel) (*models.Parcel, error) {
	if len(parcels) == 0 {
		return nil, nil
	}
	return parcels[0], nil
}

// resetRegistry clears the global registry for isolated tests
func resetRegistry() {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.sources = make(map[string]DataSource)
	registry.translators = make(map[string][]Translator)
	registry.mergers = make(map[string]Merger)
}

func TestRegisterSource(t *testing.T) {
	tests := []struct {
		name        string
		sources     []*mockDataSource
		wantErr     bool
		errContains string
	}{
		{
			name: "register single source successfully",
			sources: []*mockDataSource{
				{name: "test_source", county: "test", sourceType: "gis"},
			},
			wantErr: false,
		},
		{
			name: "register multiple unique sources successfully",
			sources: []*mockDataSource{
				{name: "source_one", county: "county_a", sourceType: "gis"},
				{name: "source_two", county: "county_b", sourceType: "tax"},
			},
			wantErr: false,
		},
		{
			name: "fail on duplicate source name",
			sources: []*mockDataSource{
				{name: "duplicate_source", county: "county_a", sourceType: "gis"},
				{name: "duplicate_source", county: "county_b", sourceType: "tax"},
			},
			wantErr:     true,
			errContains: "already registered",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetRegistry()

			var lastErr error
			for _, source := range tt.sources {
				if err := RegisterSource(source); err != nil {
					lastErr = err
				}
			}

			if tt.wantErr {
				if lastErr == nil {
					t.Error("expected error but got none")
				} else if tt.errContains != "" && !containsString(lastErr.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", lastErr.Error(), tt.errContains)
				}
			} else if lastErr != nil {
				t.Errorf("unexpected error: %v", lastErr)
			}
		})
	}
}

func TestRegisterTranslator(t *testing.T) {
	tests := []struct {
		name         string
		county       string
		translators  []*mockTranslator
		wantCountLen int
	}{
		{
			name:         "register single translator",
			county:       "test_county",
			translators:  []*mockTranslator{{canTranslate: true, priority: 10}},
			wantCountLen: 1,
		},
		{
			name:   "register multiple translators for same county",
			county: "test_county",
			translators: []*mockTranslator{
				{canTranslate: true, priority: 10},
				{canTranslate: true, priority: 5},
			},
			wantCountLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetRegistry()

			for _, translator := range tt.translators {
				if err := RegisterTranslator(tt.county, translator); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			translators := GetTranslatorsForCounty(tt.county)
			if len(translators) != tt.wantCountLen {
				t.Errorf("got %d translators, want %d", len(translators), tt.wantCountLen)
			}
		})
	}
}

func TestRegisterMerger(t *testing.T) {
	tests := []struct {
		name        string
		county      string
		registerTwo bool
		wantErr     bool
		errContains string
	}{
		{
			name:    "register single merger successfully",
			county:  "test_county",
			wantErr: false,
		},
		{
			name:        "fail on duplicate merger for same county",
			county:      "test_county",
			registerTwo: true,
			wantErr:     true,
			errContains: "already registered",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetRegistry()

			merger := &mockMerger{}
			err := RegisterMerger(tt.county, merger)
			if err != nil {
				t.Fatalf("first registration failed: %v", err)
			}

			if tt.registerTwo {
				err = RegisterMerger(tt.county, merger)
				if tt.wantErr {
					if err == nil {
						t.Error("expected error on second registration but got none")
					} else if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
						t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
					}
				}
			}
		})
	}
}

func TestGetSourcesForCounty(t *testing.T) {
	resetRegistry()

	// Register sources for multiple counties
	sources := []*mockDataSource{
		{name: "county_a_gis", county: "county_a", sourceType: "gis"},
		{name: "county_a_tax", county: "county_a", sourceType: "tax"},
		{name: "county_b_gis", county: "county_b", sourceType: "gis"},
	}

	for _, source := range sources {
		if err := RegisterSource(source); err != nil {
			t.Fatalf("failed to register source: %v", err)
		}
	}

	tests := []struct {
		name       string
		county     string
		wantCount  int
		wantNames  []string
	}{
		{
			name:      "get sources for county_a",
			county:    "county_a",
			wantCount: 2,
			wantNames: []string{"county_a_gis", "county_a_tax"},
		},
		{
			name:      "get sources for county_b",
			county:    "county_b",
			wantCount: 1,
			wantNames: []string{"county_b_gis"},
		},
		{
			name:      "get sources for non-existent county",
			county:    "county_c",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sources := GetSourcesForCounty(tt.county)
			if len(sources) != tt.wantCount {
				t.Errorf("got %d sources, want %d", len(sources), tt.wantCount)
			}

			if tt.wantNames != nil {
				gotNames := make(map[string]bool)
				for _, s := range sources {
					gotNames[s.Name()] = true
				}
				for _, name := range tt.wantNames {
					if !gotNames[name] {
						t.Errorf("missing expected source: %s", name)
					}
				}
			}
		})
	}
}

func TestGetSource(t *testing.T) {
	resetRegistry()

	source := &mockDataSource{name: "test_source", county: "test", sourceType: "gis"}
	if err := RegisterSource(source); err != nil {
		t.Fatalf("failed to register source: %v", err)
	}

	tests := []struct {
		name       string
		sourceName string
		wantErr    bool
	}{
		{
			name:       "get existing source",
			sourceName: "test_source",
			wantErr:    false,
		},
		{
			name:       "get non-existent source",
			sourceName: "non_existent",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetSource(tt.sourceName)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if got.Name() != tt.sourceName {
					t.Errorf("got source name %q, want %q", got.Name(), tt.sourceName)
				}
			}
		})
	}
}

func TestGetTranslatorsForCounty(t *testing.T) {
	resetRegistry()

	translator := &mockTranslator{canTranslate: true, priority: 10}
	if err := RegisterTranslator("test_county", translator); err != nil {
		t.Fatalf("failed to register translator: %v", err)
	}

	tests := []struct {
		name      string
		county    string
		wantCount int
	}{
		{
			name:      "get translators for existing county",
			county:    "test_county",
			wantCount: 1,
		},
		{
			name:      "get translators for non-existent county",
			county:    "non_existent",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translators := GetTranslatorsForCounty(tt.county)
			if len(translators) != tt.wantCount {
				t.Errorf("got %d translators, want %d", len(translators), tt.wantCount)
			}
		})
	}
}

func TestGetMergerForCounty(t *testing.T) {
	resetRegistry()

	merger := &mockMerger{}
	if err := RegisterMerger("test_county", merger); err != nil {
		t.Fatalf("failed to register merger: %v", err)
	}

	tests := []struct {
		name    string
		county  string
		wantErr bool
	}{
		{
			name:    "get merger for existing county",
			county:  "test_county",
			wantErr: false,
		},
		{
			name:    "get merger for non-existent county",
			county:  "non_existent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GetMergerForCounty(tt.county)
			if tt.wantErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestListRegisteredCounties(t *testing.T) {
	resetRegistry()

	sources := []*mockDataSource{
		{name: "county_a_gis", county: "county_a", sourceType: "gis"},
		{name: "county_a_tax", county: "county_a", sourceType: "tax"},
		{name: "county_b_gis", county: "county_b", sourceType: "gis"},
	}

	for _, source := range sources {
		if err := RegisterSource(source); err != nil {
			t.Fatalf("failed to register source: %v", err)
		}
	}

	counties := ListRegisteredCounties()
	if len(counties) != 2 {
		t.Errorf("got %d counties, want 2", len(counties))
	}

	countyMap := make(map[string]bool)
	for _, c := range counties {
		countyMap[c] = true
	}

	if !countyMap["county_a"] || !countyMap["county_b"] {
		t.Errorf("missing expected counties, got: %v", counties)
	}
}

func TestRegistryConcurrentAccess(t *testing.T) {
	resetRegistry()

	var wg sync.WaitGroup
	numGoroutines := 10

	// Test concurrent source registration
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			source := &mockDataSource{
				name:       "source_" + string(rune('a'+id)),
				county:     "concurrent_county",
				sourceType: "gis",
			}
			_ = RegisterSource(source)
		}(i)
	}

	// Test concurrent reads while writing
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = GetSourcesForCounty("concurrent_county")
			_ = ListRegisteredCounties()
		}()
	}

	wg.Wait()

	// Verify no data corruption
	counties := ListRegisteredCounties()
	if len(counties) == 0 {
		t.Error("expected at least one county registered")
	}
}

// containsString checks if substr is in s
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
