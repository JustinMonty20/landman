package union

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/connector/shared/httpclient"
	"github.com/tidwall/gjson"
)

// GISSourceConfig holds configuration for GISSource.
type GISSourceConfig struct {
	BaseURL      string
	SoilQueryURL string
	RateLimit    int // requests per second
	BatchSize    int // number of records to fetch per request
}

const defaultGISBatchSize = 500

// DefaultGISSourceConfig returns the default configuration for Union County GIS.
func DefaultGISSourceConfig() GISSourceConfig {
	return GISSourceConfig{
		BaseURL:      "https://atlas.unioncountync.gov/server/rest/services/OperationalLayers/MapServer/215/query",
		SoilQueryURL: "https://atlas.unioncountync.gov/server/rest/services/OperationalLayers/MapServer/197/query",
		RateLimit:    1,
		BatchSize:    defaultGISBatchSize,
	}
}

// GISSource fetches parcel data from Union County's ArcGIS REST API.
type GISSource struct {
	baseURL      string
	soilQueryURL string
	httpClient   *httpclient.RateLimitedClient
	batchSize    int
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
	OrderByFields     string
}

// NewGISSource creates a new Union County GIS data source.
func NewGISSource(config GISSourceConfig) (*GISSource, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("baseURL cannot be empty")
	}

	if err := validateURL(config.BaseURL); err != nil {
		return nil, err
	}

	if config.SoilQueryURL == "" {
		config.SoilQueryURL = DefaultGISSourceConfig().SoilQueryURL
	}
	if err := validateURL(config.SoilQueryURL); err != nil {
		return nil, fmt.Errorf("invalid soil query URL: %w", err)
	}
	soilURL, _ := url.Parse(config.SoilQueryURL)
	if soilURL.RawQuery != "" || soilURL.ForceQuery || strings.Contains(config.SoilQueryURL, "#") {
		return nil, fmt.Errorf("invalid soil query URL: query and fragment are not allowed")
	}

	if config.BatchSize <= 0 {
		config.BatchSize = defaultGISBatchSize
	}

	if config.RateLimit <= 0 {
		config.RateLimit = 1
	}

	httpClient := httpclient.NewHTTPClient()
	// A redirect is a failed source response, not an unbudgeted second request.
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	rateLimitedClient := httpclient.NewRateLimitedClient(httpClient, config.RateLimit)

	return &GISSource{
		baseURL:      config.BaseURL,
		soilQueryURL: config.SoilQueryURL,
		httpClient:   rateLimitedClient,
		batchSize:    config.BatchSize,
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
		OrderByFields:     "OBJECTID",
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

		got := len(batchRecords)
		want := s.batchSize
		if remain := int(totalCount) - dataParams.ResultOffset; remain < want {
			want = remain
		}
		if got != want {
			return fmt.Errorf("count mismatch: page %d got %d want %d", i, got, want)
		}

		totalFetched += got
		log.Printf("[%s] Batch %d/%d complete: fetched %d records (total so far: %d)",
			s.Name(), i+1, batchCount, got, totalFetched)

		if err := onBatch(batchRecords); err != nil {
			return err
		}
	}

	if totalFetched != int(totalCount) {
		return fmt.Errorf("count mismatch: fetched %d want %d", totalFetched, totalCount)
	}
	log.Printf("[%s] Fetch complete: %d total records in %v", s.Name(), totalFetched, time.Since(fetchedAt))
	return nil
}

// CaptureParcelPages saves each exact response before decoding or normalizing it.
// The callback must durably store bytes before returning.
func (s *GISSource) CaptureParcelPages(ctx context.Context, save func(string, []byte, int) error, onBatch connector.BatchHook) (int, error) {
	countParams := ArcGISParams{Where: "1=1", ReturnCountOnly: true, Format: "json"}
	countURL, err := s.buildURL(countParams)
	if err != nil {
		return 0, err
	}
	countBody, err := s.get(ctx, countURL)
	if err != nil {
		return 0, err
	}
	if err := save(countURL, countBody, 0); err != nil {
		return 0, err
	}
	count := gjson.GetBytes(countBody, "count")
	if !count.Exists() || count.Type != gjson.Number || count.Int() < 0 || count.Num != float64(count.Int()) {
		return 0, fmt.Errorf("invalid count response")
	}
	total := int(count.Int())
	params := ArcGISParams{Where: "1=1", ReturnGeometry: true, ResultRecordCount: s.batchSize, Format: "json", OutFields: "*", OrderByFields: "OBJECTID"}
	fetchedAt := time.Now().UTC()
	for offset := 0; offset < total; offset += s.batchSize {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		params.ResultOffset = offset
		rawURL, err := s.buildURL(params)
		if err != nil {
			return 0, err
		}
		body, err := s.get(ctx, rawURL)
		if err != nil {
			return 0, err
		}
		crs, err := ParcelResponseCRS(body)
		if err != nil {
			return 0, err
		}
		if err := save(rawURL, body, crs); err != nil {
			return 0, err
		}
		records, err := DecodeParcelPage(body, fetchedAt, s.Name())
		if err != nil {
			return 0, err
		}
		if len(records) != min(s.batchSize, total-offset) {
			return 0, fmt.Errorf("count mismatch at offset %d", offset)
		}
		if err := onBatch(records); err != nil {
			return 0, err
		}
	}
	return total, nil
}

// ParcelResponseCRS requires an original ArcGIS declaration, not a requested output CRS.
func ParcelResponseCRS(body []byte) (int, error) {
	ref := gjson.GetBytes(body, "spatialReference.wkid")
	latest := gjson.GetBytes(body, "spatialReference.latestWkid")
	valid := func(value gjson.Result) bool {
		return value.Type == gjson.Number && (value.Raw == "3857" || value.Raw == "102100")
	}
	if (!ref.Exists() && !latest.Exists()) || (ref.Exists() && !valid(ref)) || (latest.Exists() && !valid(latest)) {
		return 0, fmt.Errorf("missing or unsupported parcel source CRS")
	}
	crs := int(latest.Int())
	if ref.Exists() {
		crs = int(ref.Int())
	}
	features := gjson.GetBytes(body, "features")
	if !features.IsArray() {
		return 0, fmt.Errorf("parcel response missing features")
	}
	for _, feature := range features.Array() {
		for _, field := range []string{"geometry.spatialReference.wkid", "geometry.spatialReference.latestWkid"} {
			local := feature.Get(field)
			if local.Exists() && !valid(local) {
				return 0, fmt.Errorf("feature CRS conflicts with response")
			}
		}
	}
	return crs, nil
}

// DecodeParcelPage retains exact decoded JSON numbers for source evidence.
func DecodeParcelPage(body []byte, fetchedAt time.Time, sourceName string) ([]connector.RawRecord, error) {
	crs, err := ParcelResponseCRS(body)
	if err != nil {
		return nil, err
	}
	var page struct {
		Features []json.RawMessage `json:"features"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, err
	}
	out := make([]connector.RawRecord, 0, len(page.Features))
	for _, feature := range page.Features {
		decoder := json.NewDecoder(bytes.NewReader(feature))
		decoder.UseNumber()
		var data map[string]interface{}
		if err := decoder.Decode(&data); err != nil {
			return nil, err
		}
		attrs, _ := data["attributes"].(map[string]interface{})
		pid, _ := attrs["PID"].(string)
		if pid == "" {
			identity, err := ParcelRecordIdentity(connector.RawRecord{RawData: data})
			if err != nil {
				return nil, err
			}
			pid = identity.PID
		}
		declared := crs
		if ref := gjson.GetBytes(feature, "geometry.spatialReference.wkid"); ref.Exists() {
			declared = int(ref.Int())
		}
		data["source_declared_crs"] = declared
		out = append(out, connector.RawRecord{SourceName: sourceName, ParcelID: pid, FetchedAt: fetchedAt, RawData: data})
	}
	return out, nil
}

func (s *GISSource) queryTotalParcels(ctx context.Context, params ArcGISParams) (int64, error) {
	rawURL, err := s.buildURL(params)
	if err != nil {
		return 0, err
	}

	body, err := s.get(ctx, rawURL)
	if err != nil {
		return 0, err
	}

	count := gjson.Get(string(body), "count")
	if !count.Exists() || count.Type != gjson.Number || count.Int() < 0 || count.Num != float64(count.Int()) {
		return 0, fmt.Errorf("union county gis response missing count")
	}
	return count.Int(), nil
}

func (s *GISSource) fetchBatch(ctx context.Context, params ArcGISParams, fetchedAt time.Time) ([]connector.RawRecord, error) {
	rawURL, err := s.buildURL(params)
	if err != nil {
		return nil, err
	}

	body, err := s.get(ctx, rawURL)
	if err != nil {
		return nil, err
	}

	if arcErr := gjson.Get(string(body), "error"); arcErr.Exists() {
		return nil, fmt.Errorf("union county gis error: %s", arcErr.Raw)
	}
	features := gjson.Get(string(body), "features")
	if !features.IsArray() {
		return nil, fmt.Errorf("union county gis response missing features")
	}
	featureList := features.Array()

	records := make([]connector.RawRecord, 0, len(featureList))
	for _, feature := range featureList {
		parcelID := feature.Get("attributes.PID").String()
		if parcelID == "" {
			objectID := feature.Get("attributes.OBJECTID")
			if objectID.Type != gjson.Number {
				return nil, fmt.Errorf("union county gis feature missing PID and OBJECTID")
			}
			parcelID = objectID.Raw
		}

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

func (s *GISSource) get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "ParcelDataCollection/0.0.1")
	resp, err := s.httpClient.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := unexpectedHTTPStatus(resp.StatusCode); err != nil {
		return nil, err
	}

	const maxResponseBytes = 32 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxResponseBytes {
		return nil, fmt.Errorf("union county gis response exceeds size limit")
	}
	if !gjson.ValidBytes(body) {
		return nil, fmt.Errorf("union county gis invalid JSON response")
	}
	return body, nil
}

type httpStatusError struct{ code int }

func (e httpStatusError) Error() string {
	return fmt.Sprintf("union county gis unexpected status: %d", e.code)
}

func unexpectedHTTPStatus(statusCode int) error {
	if statusCode < http.StatusOK || statusCode > 299 {
		return httpStatusError{code: statusCode}
	}
	return nil
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
	if p.OrderByFields != "" {
		qps.Add("orderByFields", p.OrderByFields)
	}

	return qps, nil
}

func validateURL(baseURL string) error {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("invalid baseURL! invalid format: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("invalid baseURL! must include scheme (http:// or https://)")
	}
	if parsed.Host == "" {
		return fmt.Errorf("invalid baseURL! must include host")
	}
	return nil
}
