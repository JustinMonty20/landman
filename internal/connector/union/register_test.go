package union

import (
	"testing"

	"github.com/JustinMonty20/landman/internal/connector"
)

func TestRegister(t *testing.T) {
	// Reset registry before test
	connector.ResetRegistry()

	err := Register()
	if err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	// Verify source was registered
	source, err := connector.GetSource("union_county_gis")
	if err != nil {
		t.Errorf("source not registered: %v", err)
	}
	if source != nil && source.Name() != "union_county_gis" {
		t.Errorf("source name = %q, want %q", source.Name(), "union_county_gis")
	}

	// Verify translator was registered
	translators := connector.GetTranslatorsForCounty("union")
	if len(translators) == 0 {
		t.Error("no translators registered for union county")
	}

	// Verify merger was registered
	merger, err := connector.GetMergerForCounty("union")
	if err != nil {
		t.Errorf("merger not registered: %v", err)
	}
	if merger == nil {
		t.Error("merger should not be nil")
	}
}

func TestRegister_DuplicateRegistration(t *testing.T) {
	connector.ResetRegistry()

	// First registration should succeed
	err := Register()
	if err != nil {
		t.Fatalf("first Register() failed: %v", err)
	}

	// Second registration should fail (duplicate source)
	err = Register()
	if err == nil {
		t.Error("expected error for duplicate registration")
	}
}
