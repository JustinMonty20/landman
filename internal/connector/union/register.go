package union

import (
	"fmt"

	"github.com/JustinMonty20/landman/internal/connector"
)

// Register registers all Union County connectors with the global registry
// This should be called from main() or init()
//
// Example usage in main.go:
//   import "github.com/JustinMonty20/landman/internal/connector/union"
//
//   func main() {
//       if err := union.Register(); err != nil {
//           log.Fatal(err)
//       }
//       // Now all Union County connectors are registered and ready to use
//   }
//
// Each data source manages its own configuration via its Default*Config() function.
// To customize, create the sources manually instead of using Register().
func Register() error {
	// Create and register GIS source with default config
	gisSource, err := NewUnionCountyGISSource(DefaultGISSourceConfig())
	if err != nil {
		return fmt.Errorf("failed to create GIS source: %w", err)
	}

	if err := connector.RegisterSource(gisSource); err != nil {
		return fmt.Errorf("failed to register GIS source: %w", err)
	}

	// Register translator
	translator := NewUnionCountyTranslator()
	if err := connector.RegisterTranslator("union", translator); err != nil {
		return fmt.Errorf("failed to register translator: %w", err)
	}

	// Register merger
	merger := NewUnionCountyMerger()
	if err := connector.RegisterMerger("union", merger); err != nil {
		return fmt.Errorf("failed to register merger: %w", err)
	}

	return nil
}
