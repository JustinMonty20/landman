package counties

import (
	"net/http"
	"strings"
	"testing"
)

func TestNewCountyConnector(t *testing.T) {
	validClient := &http.Client{}

	tests := []struct {
		name        string
		baseUrl     string
		county      string
		dataDesc    string
		client      *http.Client
		expectError bool
		errorMsg    string
	}{
		{
			"invalid url test",
			"invalid-url-duh",
			"Union",
			"Union County Parcel Data",
			validClient,
			true,
			"invalid baseUrl! needs to be a valid URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NewCountyConnector(
				tt.baseUrl,
				tt.county,
				tt.dataDesc,
				tt.client,
			)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
				return
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if tt.expectError && err != nil {
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
			}

			if result == nil {
				t.Error("Expected non-nil result")
			}

			if result.baseUrl != tt.baseUrl {
				t.Errorf("Expected baseUrl '%s' but got '%s'", tt.baseUrl, result.baseUrl)
			}

			if result.county != tt.county {
				t.Errorf("Expected county '%s' but got '%s'", tt.county, result.county)
			}
			if result.dataDesc != tt.dataDesc {
				t.Errorf("Expected dataDesc '%s' but got '%s'", tt.dataDesc, result.dataDesc)
			}

      if result.client != tt.client {
        t.Errorf("Expected client to match provided client")
      }

		})
	}

}
