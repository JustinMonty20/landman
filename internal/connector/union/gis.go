package union

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

// GISSourceConfig holds configuration for GISSource.
type GISSourceConfig struct {
	BaseURL   string
	RateLimit int // requests per second
	BatchSize int // number of records to fetch per request
}

// DefaultGISSourceConfig returns the default configuration for Union County GIS.
func DefaultGISSourceConfig() GISSourceConfig {
	return GISSourceConfig{
		BaseURL:   "https://atlas.unioncountync.gov/server/rest/services/OperationalLayers/MapServer/215/query",
		RateLimit: 1,
		BatchSize: 50,
	}
}

// GISSource fetches parcel data from Union County's ArcGIS REST API.
type GISSource struct {
	baseURL    string
	httpClient *httpclient.RateLimitedClient
	batchSize  int
}

// ArcGISParams represents Union County's ArcGIS query parameters.
type ArcGISParams struct {
	Where             string
	ReturnCountOnly   bool
	ReturnGeometry    bool
	ResultOffset      int
	ResultRecordCount int
	Format            string
	OutFields         string
}

// NewGISSource creates a new Union County GIS data source.
func NewGISSource(config GISSourceConfig) (*GISSource, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("baseURL cannot be empty")
	}

	if err := validateURL(config.BaseURL); err != nil {
		return nil, err
	}

	if config.BatchSize <= 0 {
		config.BatchSize = 50
	}

	if config.RateLimit <= 0 {
		config.RateLimit = 1
	}

	httpClient := httpclient.NewHTTPClient()
	rateLimitedClient := httpclient.NewRateLimitedClient(httpClient, config.RateLimit)

	return &GISSource{
		baseURL:    config.BaseURL,
		httpClient: rateLimitedClient,
		batchSize:  config.BatchSize,
	}, nil
}

func (s *GISSource) Name() string              { return "union_county_gis" }
func (s *GISSource) County() string            { return "union" }
func (s *GISSource) SourceType() string        { return "gis" }
func (s *GISSource) SupportsIncremental() bool { return false }

func (s *GISSource) FetchSince(ctx context.Context, since time.Time) ([]connector.RawRecord, error) {
	return nil, fmt.Errorf("union county gis does not support incremental fetching")
}

func (s *GISSource) Fetch(ctx context.Context) ([]connector.RawRecord, error) {
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

func (s *GISSource) FetchBatches(ctx context.Context, onBatch connector.BatchHook) error {
	if onBatch == nil {
		return fmt.Errorf("onBatch cannot be nil")
	}

	countParams := ArcGISParams{
		Where:           "1=1",
		ReturnCountOnly: true,
		Format:          "json",
	}

	totalCount, err := s.queryTotalParcels(ctx, countParams)
	if err != nil {
		return fmt.Errorf("failed to query total parcels: %w", err)
	}

	log.Printf("[%s] Total parcels to fetch: %d", s.Name(), totalCount)

	batchCount := (int(totalCount) + s.batchSize - 1) / s.batchSize
	log.Printf("[%s] Batch size: %d, Total batches: %d", s.Name(), s.batchSize, batchCount)

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
		log.Printf("[%s] Batch %d/%d complete: fetched %d records (total so far: %d)",
			s.Name(), i+1, batchCount, len(batchRecords), totalFetched)

		if err := onBatch(batchRecords); err != nil {
			return err
		}
	}

	log.Printf("[%s] Fetch complete: %d total records in %v", s.Name(), totalFetched, time.Since(fetchedAt))
	return nil
}

func (s *GISSource) queryTotalParcels(ctx context.Context, params ArcGISParams) (int64, error) {
	rawURL, err := s.buildURL(params)
	if err != nil {
		return 0, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
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

	result := gjson.Get(string(body), "count")
	return result.Int(), nil
}

func (s *GISSource) fetchBatch(ctx context.Context, params ArcGISParams, fetchedAt time.Time) ([]connector.RawRecord, error) {
	rawURL, err := s.buildURL(params)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
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

	features := gjson.Get(string(body), "features").Array()

	records := make([]connector.RawRecord, 0, len(features))
	for _, feature := range features {
		parcelID := feature.Get("attributes.PID").String()

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

func (s *GISSource) buildURL(params ArcGISParams) (string, error) {
	queryParams, err := params.ToURLValues()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s?%s", s.baseURL, queryParams.Encode()), nil
}

// ToURLValues converts ArcGISParams to url.Values.
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
