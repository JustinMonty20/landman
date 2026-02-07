package union

import (
	"net/http"

	"github.com/JustinMonty20/landman/internal/connector/shared/httpclient"
	"github.com/JustinMonty20/landman/internal/connector/union/gis"
	"github.com/JustinMonty20/landman/internal/connector/union/merger"
	"github.com/JustinMonty20/landman/internal/connector/union/spatialist"
	"github.com/JustinMonty20/landman/internal/connector/union/translator"
)

// Re-export GIS source types and constructors.
type GISSourceConfig = gis.GISSourceConfig
type UnionCountyGISSource = gis.UnionCountyGISSource

func DefaultGISSourceConfig() GISSourceConfig {
	return gis.DefaultGISSourceConfig()
}

func NewUnionCountyGISSource(config GISSourceConfig) (*UnionCountyGISSource, error) {
	return gis.NewUnionCountyGISSource(config)
}

// Re-export Spatialist types and constructors.
type SpatialistConfig = spatialist.SpatialistConfig
type UnionCountySpatialist = spatialist.UnionCountySpatialist
type SpatialistFlatRecord = spatialist.SpatialistFlatRecord
type SpatialistFlatEnvelope = spatialist.SpatialistFlatEnvelope
type SpatialistValuation = spatialist.SpatialistValuation
type SpatialistSale = spatialist.SpatialistSale
type SpatialistAssessment = spatialist.SpatialistAssessment
type SpatialistPermit = spatialist.SpatialistPermit
type SpatialistEnricher = spatialist.SpatialistEnricher
type SpatialistTransformer = spatialist.SpatialistTransformer
type RawBatchEnricher = spatialist.RawBatchEnricher

func DefaultSpatialistConfig() SpatialistConfig {
	return spatialist.DefaultSpatialistConfig()
}

func NewUnionCountySpatialist(config SpatialistConfig) (*UnionCountySpatialist, error) {
	return spatialist.NewUnionCountySpatialist(config)
}

func FlattenSpatialistRecord(raw map[string]interface{}) (*SpatialistFlatEnvelope, error) {
	return spatialist.FlattenSpatialistRecord(raw)
}

func NewSpatialistEnricher(batchEnricher RawBatchEnricher, sourceName string, transform SpatialistTransformer) (*SpatialistEnricher, error) {
	return spatialist.NewSpatialistEnricher(batchEnricher, sourceName, transform)
}

// Re-export translator.
type UnionCountyTranslator = translator.UnionCountyTranslator

func NewUnionCountyTranslator() *UnionCountyTranslator {
	return translator.NewUnionCountyTranslator()
}

// Re-export merger.
type UnionCountyMerger = merger.UnionCountyMerger

func NewUnionCountyMerger() *UnionCountyMerger {
	return merger.NewUnionCountyMerger()
}

// Re-export shared HTTP client helpers.
type RateLimitedClient = httpclient.RateLimitedClient

func NewHTTPClient() *http.Client {
	return httpclient.NewHTTPClient()
}

func NewRateLimitedClient(client *http.Client, reqPerSecond int) *RateLimitedClient {
	return httpclient.NewRateLimitedClient(client, reqPerSecond)
}
