package connector

import (
	"fmt"
	"sync"
)

// Global registry for all connectors
// Counties register themselves via init() functions when their packages are imported
var (
	registry = &Registry{
		sources:     make(map[string]DataSource),
		translators: make(map[string][]Translator),
		mergers:     make(map[string]Merger),
	}
)

// Registry holds all registered data sources, translators, and mergers
// This enables the plugin architecture where counties can register themselves
// without modifying core code.
//
// Example: When you import the union county package:
//   import _ "github.com/JustinMonty20/landman/internal/connector/union"
//
// The union package's init() function runs automatically and registers:
// - UnionCountyGISSource
// - UnionCountyTranslator
// - UnionCountyMerger
//
// Later, when Mecklenburg County is added, you just add another import:
//   import _ "github.com/JustinMonty20/landman/internal/connector/mecklenburg"
//
// No changes to core code needed!
type Registry struct {
	mu          sync.RWMutex
	sources     map[string]DataSource   // key: source name (e.g., "union_county_gis")
	translators map[string][]Translator // key: county name (e.g., "union")
	mergers     map[string]Merger       // key: county name (e.g., "union")
}

// RegisterSource registers a DataSource
// Called by county packages in their init() functions
//
// Example in Union County package (internal/connector/union/register.go):
//   func init() {
//       connector.RegisterSource(NewUnionCountyGISSource(...))
//   }
func RegisterSource(source DataSource) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	name := source.Name()
	if _, exists := registry.sources[name]; exists {
		return fmt.Errorf("data source %s already registered", name)
	}

	registry.sources[name] = source
	return nil
}

// RegisterTranslator registers a Translator for a county
// Multiple translators can be registered for the same county
//
// Example: Union County might have separate translators for GIS vs Tax data,
// or a single translator that handles both. Either pattern works.
func RegisterTranslator(county string, translator Translator) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	registry.translators[county] = append(registry.translators[county], translator)
	return nil
}

// RegisterMerger registers a Merger for a county
// Each county should have exactly one merger that knows how to combine
// data from their various sources.
func RegisterMerger(county string, merger Merger) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	if _, exists := registry.mergers[county]; exists {
		return fmt.Errorf("merger for county %s already registered", county)
	}

	registry.mergers[county] = merger
	return nil
}

// GetSourcesForCounty returns all registered DataSources for a county
func GetSourcesForCounty(county string) []DataSource {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	var sources []DataSource
	for _, source := range registry.sources {
		if source.County() == county {
			sources = append(sources, source)
		}
	}
	return sources
}

// GetSource returns a specific DataSource by name
func GetSource(name string) (DataSource, error) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	source, exists := registry.sources[name]
	if !exists {
		return nil, fmt.Errorf("data source %s not found", name)
	}
	return source, nil
}

// GetTranslatorsForCounty returns all translators registered for a county
func GetTranslatorsForCounty(county string) []Translator {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	return registry.translators[county]
}

// GetMergerForCounty returns the merger for a county
func GetMergerForCounty(county string) (Merger, error) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	merger, exists := registry.mergers[county]
	if !exists {
		return nil, fmt.Errorf("no merger registered for county %s", county)
	}
	return merger, nil
}

// ListRegisteredCounties returns all counties that have at least one registered source
func ListRegisteredCounties() []string {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	counties := make(map[string]bool)
	for _, source := range registry.sources {
		counties[source.County()] = true
	}

	result := make([]string, 0, len(counties))
	for county := range counties {
		result = append(result, county)
	}
	return result
}
