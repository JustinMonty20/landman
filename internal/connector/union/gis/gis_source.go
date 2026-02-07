package gis

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/connector/shared/httpclient"
	"github.com/tidwall/gjson"
)

// GISSourceConfig holds configuration for UnionCountyGISSource
type GISSourceConfig struct {
	BaseURL   string
	RateLimit int // requests per second
	BatchSize int // number of records to fetch per request
}

// DefaultGISSourceConfig returns the default configuration for Union County GIS
func DefaultGISSourceConfig() GISSourceConfig {
	return GISSourceConfig{
		BaseURL:   "https://atlas.unioncountync.gov/server/rest/services/OperationalLayers/MapServer/215/query",
		RateLimit: 1,  // Conservative: 1 request per second
		BatchSize: 50, // Fetch 50 parcels per request
	}
}

// UnionCountyGISSource fetches parcel data from Union County's ArcGIS REST API
// This implements the connector.DataSource interface.
//
// Other counties would have their own GIS sources:
// - MecklenburgCountyGISSource might use a different ArcGIS endpoint with different parameters
// - WakeCountyGISSource might use a completely different API (not ArcGIS)
// - Some counties might use WFS (Web Feature Service) instead of ArcGIS REST
type UnionCountyGISSource struct {
	baseURL    string
	httpClient *httpclient.RateLimitedClient
	batchSize  int // Number of records to fetch per request
}

// ArcGISParams represents Union County's ArcGIS query parameters
// Other counties might have different parameter structures:
// - Some might not support pagination the same way
// - Some might have different format options
// - Some might require authentication tokens
type ArcGISParams struct {
	Where             string
	ReturnCountOnly   bool
	ReturnGeometry    bool
	ResultOffset      int    // pagination mechanism
	ResultRecordCount int    // batch size
	Format            string // json or geojson
	OutFields         string
}

// NewUnionCountyGISSource creates a new Union County GIS data source
func NewUnionCountyGISSource(config GISSourceConfig) (*UnionCountyGISSource, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("baseURL cannot be empty")
	}

	if err := validateURL(config.BaseURL); err != nil {
		return nil, err
	}

	if config.BatchSize <= 0 {
		config.BatchSize = 50 // default batch size
	}

	if config.RateLimit <= 0 {
		config.RateLimit = 1 // default rate limit
	}

	// Create HTTP client infrastructure internally
	httpClient := httpclient.NewHTTPClient()
	rateLimitedClient := httpclient.NewRateLimitedClient(httpClient, config.RateLimit)

	return &UnionCountyGISSource{
		baseURL:    config.BaseURL,
		httpClient: rateLimitedClient,
		batchSize:  config.BatchSize,
	}, nil
}

// Name returns the unique identifier for this data source
func (s *UnionCountyGISSource) Name() string {
	return "union_county_gis"
}

// County returns which county this source serves
func (s *UnionCountyGISSource) County() string {
	return "union"
}

// SourceType returns the type of data
func (s *UnionCountyGISSource) SourceType() string {
	return "gis"
}

// SupportsIncremental indicates if this source supports fetching only changed records
// Union County's ArcGIS doesn't support this (most don't), so we return false
func (s *UnionCountyGISSource) SupportsIncremental() bool {
	return false
}

// FetchSince is not supported by Union County GIS
func (s *UnionCountyGISSource) FetchSince(ctx context.Context, since time.Time) ([]connector.RawRecord, error) {
	return nil, fmt.Errorf("union county gis does not support incremental fetching")
}

// Fetch retrieves all parcel data from Union County GIS
// This method handles pagination internally and returns all records
func (s *UnionCountyGISSource) Fetch(ctx context.Context) ([]connector.RawRecord, error) {
	var allRecords []connector.RawRecord
	err := s.FetchBatches(ctx, func(batch []connector.RawRecord) error {
		allRecords = append(allRecords, batch...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return allRecords, nil
}

// FetchBatches retrieves all parcel data from Union County GIS in batches.
// The provided hook is called once per batch.
func (s *UnionCountyGISSource) FetchBatches(ctx context.Context, onBatch connector.BatchHook) error {
	if onBatch == nil {
		return fmt.Errorf("onBatch cannot be nil")
	}

	// Step 1: Query total count
	countParams := ArcGISParams{
		Where:           "1=1", // Get all records
		ReturnCountOnly: true,
		Format:          "json",
	}

	totalCount, err := s.queryTotalParcels(ctx, countParams)
	if err != nil {
		return fmt.Errorf("failed to query total parcels: %w", err)
	}

	log.Printf("[%s] Total parcels to fetch: %d", s.Name(), totalCount)

	// Step 2: Calculate number of batches needed
	batchCount := (int(totalCount) + s.batchSize - 1) / s.batchSize

	log.Printf("[%s] Batch size: %d, Total batches: %d", s.Name(), s.batchSize, batchCount)

	// Step 3: Fetch all batches
	fetchedAt := time.Now()

	dataParams := ArcGISParams{
		Where:             "1=1",
		ReturnCountOnly:   false,
		ReturnGeometry:    true,
		ResultRecordCount: s.batchSize,
		Format:            "json",
		OutFields:         "*",
	}

	var totalFetched int
	for i := 0; i < batchCount; i++ {
		select {
		case <-ctx.Done():
			log.Printf("[%s] Fetch cancelled after %d batches", s.Name(), i)
			return ctx.Err()
		default:
		}

		dataParams.ResultOffset = i * s.batchSize

		batchRecords, err := s.fetchBatch(ctx, dataParams, fetchedAt)
		if err != nil {
			return fmt.Errorf("failed to fetch batch %d: %w", i, err)
		}

		totalFetched += len(batchRecords)

		// Log batch progress with a sample parcel from this batch
		log.Printf("[%s] Batch %d/%d complete: fetched %d records (total so far: %d)",
			s.Name(), i+1, batchCount, len(batchRecords), totalFetched)

		if err := onBatch(batchRecords); err != nil {
			return err
		}
	}

	log.Printf("[%s] Fetch complete: %d total records in %v",
		s.Name(), totalFetched, time.Since(fetchedAt))

	return nil
}

// queryTotalParcels queries the total number of parcels
func (s *UnionCountyGISSource) queryTotalParcels(ctx context.Context, params ArcGISParams) (int64, error) {
	url, err := s.buildURL(params)
	if err != nil {
		return 0, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, err
	}

	req.Header.Set("User-Agent", "ParcelDataCollection/0.0.1")
	resp, err := s.httpClient.Do(ctx, req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	json := string(body)
	result := gjson.Get(json, "count")
	return result.Int(), nil
}

// fetchBatch fetches a single batch of parcel data and converts to RawRecords
func (s *UnionCountyGISSource) fetchBatch(ctx context.Context, params ArcGISParams, fetchedAt time.Time) ([]connector.RawRecord, error) {
	url, err := s.buildURL(params)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "ParcelDataCollection/0.0.1")
	resp, err := s.httpClient.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	json := string(body)
	features := gjson.Get(json, "features").Array()

	// Convert gjson results to RawRecords
	records := make([]connector.RawRecord, 0, len(features))
	for _, feature := range features {
		// Extract parcel ID (Union County uses "PID" field)
		// Other counties might use "PARCEL_ID", "REID", etc.
		parcelID := feature.Get("attributes.PID").String()

		// Convert feature to map for RawData
		rawData := make(map[string]interface{})
		feature.ForEach(func(key, value gjson.Result) bool {
			rawData[key.String()] = value.Value()
			return true
		})

		records = append(records, connector.RawRecord{
			SourceName: s.Name(),
			ParcelID:   parcelID,
			FetchedAt:  fetchedAt,
			RawData:    rawData,
		})
	}

	return records, nil
}

// buildURL builds the full URL with query parameters
func (s *UnionCountyGISSource) buildURL(params ArcGISParams) (string, error) {
	queryParams, err := params.ToURLValues()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s?%s", s.baseURL, queryParams.Encode()), nil
}

// ToURLValues converts ArcGISParams to url.Values
func (p ArcGISParams) ToURLValues() (url.Values, error) {
	qps := url.Values{}

	if p.Where != "" {
		qps.Add("where", p.Where)
	}

	if p.ReturnCountOnly {
		qps.Add("returnCountOnly", "true")
	}

	if p.ReturnGeometry {
		qps.Add("returnGeometry", "true")
	}

	if p.ResultOffset != 0 {
		qps.Add("resultOffset", fmt.Sprintf("%d", p.ResultOffset))
	}

	if p.ResultRecordCount != 0 {
		qps.Add("resultRecordCount", fmt.Sprintf("%d", p.ResultRecordCount))
	}

	if p.Format != "json" && p.Format != "geojson" {
		return nil, fmt.Errorf("invalid format. Must be json or geojson")
	}
	qps.Add("f", p.Format)

	if p.OutFields == "" {
		qps.Add("outFields", "*")
	} else {
		qps.Add("outFields", p.OutFields)
	}

	return qps, nil
}

// validateURL validates the baseURL
func validateURL(baseURL string) error {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("invalid baseURL! invalid format: %v", err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("invalid baseURL! must include scheme (http:// or https://)")
	}

	if parsed.Host == "" {
		return fmt.Errorf("invalid baseURL! must include host")
	}

	return nil
}
