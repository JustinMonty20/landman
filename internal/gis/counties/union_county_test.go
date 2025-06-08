package counties

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func mockCountSuccess(count int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf(`{"count": %d}`, count)))
	}))
}

func TestUCArcGisParams_ToUrlValues(t *testing.T) {
    tests := []struct {
        name        string
        params      UCArcGisParams
        expectError bool
        expected    map[string]string
    }{
        {
            name: "Valid count query",
            params: UCArcGisParams{
                Where:           "1=1",
                ReturnCountOnly: true,
                Format:          "json",
            },
            expectError: false,
            expected: map[string]string{
                "where":           "1=1",
                "returnCountOnly": "true",
                "f":               "json",
            },
        },
        {
            name: "Valid geojson query",
            params: UCArcGisParams{
                ReturnCountOnly: true,
                Format:          "geojson",
            },
            expectError: false,
            expected: map[string]string{
                "returnCountOnly": "true",
                "f":               "geojson",
            },
        },
        {
            name: "Invalid format",
            params: UCArcGisParams{
                Format: "xml",
            },
            expectError: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            urlValues, err := tt.params.ToUrlValues()
            
            if tt.expectError {
                if err == nil {
                    t.Errorf("Expected error but got none")
                }
                return
            }
            
            if err != nil {
                t.Errorf("Unexpected error: %v", err)
                return
            }
            
            for key, expectedValue := range tt.expected {
                if urlValues.Get(key) != expectedValue {
                    t.Errorf("Expected %s to be %s, got %s", key, expectedValue, urlValues.Get(key))
                }
            }
        })
    }
}
