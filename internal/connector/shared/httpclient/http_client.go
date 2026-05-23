package httpclient

import (
	"context"
	"net"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// RateLimitedClient wraps an HTTP client with rate limiting
// This is specific to Union County's needs right now, but could be extracted
// to a shared package when other counties need it.
//
// NOTE: When adding more counties, if they also need rate limiting,
// consider moving this to pkg/http/ or internal/connector/shared/
type RateLimitedClient struct {
	Client  *http.Client
	limiter *rate.Limiter
}

// Do executes an HTTP request with rate limiting
func (rlc *RateLimitedClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if err := rlc.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	return rlc.Client.Do(req)
}

// NewHTTPClient creates a production-ready HTTP client with proper timeouts
// and connection pooling.
//
// NOTE: This is intentionally shared across connectors.
// If per-county tuning is needed later, wrap this in county-specific constructors.
func NewHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			// Timeouts
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 15 * time.Second,
			// KeepAlive
			DisableKeepAlives:     false,
			ExpectContinueTimeout: 1 * time.Second,
			DialContext: (&net.Dialer{
				Timeout: 5 * time.Second,
			}).DialContext,
		},
	}
}

// NewRateLimitedClient creates a new rate-limited HTTP client
// Respects government API rate limits to avoid overwhelming their servers.
//
// Union County: We conservatively use 1 req/second
// Other counties might have different rate limits:
// - Mecklenburg might allow 5 req/second
// - Wake County might have no published limit (default to 1 req/second)
func NewRateLimitedClient(client *http.Client, reqPerSecond int) *RateLimitedClient {
	// default to 1 req/second if not specified
	if reqPerSecond == 0 {
		reqPerSecond = 1
	}
	return &RateLimitedClient{
		Client:  client,
		limiter: rate.NewLimiter(rate.Limit(reqPerSecond), 1), // burst of 1
	}
}
