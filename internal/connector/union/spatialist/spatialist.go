package spatialist

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/connector/shared/httpclient"
)

// SpatialistConfig holds configuration for the Spatialist property search.
type SpatialistConfig struct {
	BaseURL      string
	RateLimitRPS int
}

// DefaultSpatialistConfig returns a baseline config.
func DefaultSpatialistConfig() SpatialistConfig {
	return SpatialistConfig{
		BaseURL:      "https://property.spatialest.com/nc/union/api/v1/recordcard",
		RateLimitRPS: 5,
	}
}

// UnionCountySpatialist fetches property search data by parcel ID.
type UnionCountySpatialist struct {
	baseURL    string
	httpClient *httpclient.RateLimitedClient
}

// NewUnionCountySpatialist creates a Spatialist client.
func NewUnionCountySpatialist(config SpatialistConfig) (*UnionCountySpatialist, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("baseURL cannot be empty")
	}

	parsed, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid baseURL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("invalid baseURL scheme")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("invalid baseURL host")
	}

	if config.RateLimitRPS <= 0 {
		config.RateLimitRPS = 1
	}

	httpClient := httpclient.NewHTTPClient()
	rlClient := httpclient.NewRateLimitedClient(httpClient, config.RateLimitRPS)

	return &UnionCountySpatialist{
		baseURL:    config.BaseURL,
		httpClient: rlClient,
	}, nil
}

// Name returns the unique identifier for this source.
func (s *UnionCountySpatialist) Name() string {
	return "union_county_spatialist"
}

// FetchByParcelID retrieves property search data for a single parcel ID.
func (s *UnionCountySpatialist) FetchByParcelID(ctx context.Context, parcelID string) (connector.RawRecord, error) {
	if parcelID == "" {
		return connector.RawRecord{}, fmt.Errorf("parcelID cannot be empty")
	}

	parsed, err := url.Parse(s.baseURL)
	if err != nil {
		return connector.RawRecord{}, fmt.Errorf("invalid baseURL: %w", err)
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + url.PathEscape(parcelID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return connector.RawRecord{}, err
	}
	req.Header.Set("User-Agent", "ParcelDataCollection/0.0.1")

	resp, err := s.httpClient.Do(ctx, req)
	if err != nil {
		return connector.RawRecord{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return connector.RawRecord{}, fmt.Errorf("spatialist unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return connector.RawRecord{}, err
	}

	var raw map[string]interface{}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &raw); err != nil {
			return connector.RawRecord{}, fmt.Errorf("spatialist invalid json: %w", err)
		}
	}
	if raw == nil {
		raw = map[string]interface{}{}
	}

	return connector.RawRecord{
		SourceName: s.Name(),
		ParcelID:   parcelID,
		FetchedAt:  time.Now(),
		RawData:    raw,
	}, nil
}
