package counties

import (
	"fmt"
	"net/http"
	"net/url"
  "net"
  "time"
  "golang.org/x/time/rate"
)

/*
all counties that I onboard onto this will use this struct
*/
type BaseCountyConnector struct {
	baseUrl  string
	county   string
	dataDesc string
	client   *RateLimitedClient 
}

/*
rate limited client for the county connector.
Unsure how the government apis will handle this many requests.
Want to add rate limiting in early to not overwhelm the api.
*/
type RateLimitedClient struct {
  client *http.Client
  limiter *rate.Limiter
}

/*
interface for all county query params
*/
type QueryParams interface {
  ToUrlValues() (url.Values, error)
}

/*
returns description of which data that will will be fetching.
*/
func (bcc BaseCountyConnector) Description() string {
	return bcc.dataDesc
}

/*
returns the name of the county that we are working within.
*/
func (bcc BaseCountyConnector) County() string {
	return bcc.county
}

/*
 builds the url for the county to query parcel data.
*/
func (bcc BaseCountyConnector) BuildUrl(qp QueryParams) (string, error) {
  queryParams, err := qp.ToUrlValues() 
  if err != nil {
    return "", err 
  }
  return fmt.Sprintf("%s?%s", bcc.baseUrl, queryParams.Encode()), nil
}

/*
  validates the baseUrl to ensure it is a valid URL
*/
func validateUrl(baseUrl string) error {
	parsed, err := url.Parse(baseUrl)

	if err != nil {
    return fmt.Errorf("invalid baseUrl! invalid format: %v", err)
	}
  // check the scheme. needs to be http or https
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("invalid baseUrl! must include scheme (http:// or https://)")
	}
  
  // check the host. needs to be a non-empty string
	if parsed.Host == "" {
		return fmt.Errorf("invalid baseUrl! must include host")
	}

	return nil
}

/*
  creates a new http client
*/
func NewHttpClient() *http.Client {
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

/*
  creates a new rate limited client
*/
func NewRateLimitedClient(
  client *http.Client,
  reqPerSecond int,
) *RateLimitedClient {
  // default to 1 req/second
  if reqPerSecond == 0 {
    reqPerSecond = 1
  }
  return &RateLimitedClient {
    client: client,
    // burst of 1
    limiter: rate.NewLimiter(rate.Limit(reqPerSecond), 1),
  }
}

/*
Creates a new county connector to be used
for any of the specific downstream counties
*/
func NewCountyConnector(
	baseUrl string,
	county string,
	dataDesc string,
	client *RateLimitedClient,
) (*BaseCountyConnector, error) {

	if baseUrl == "" {
		return nil, fmt.Errorf("baseUrl cannot be empty")
	}

	if err := validateUrl(baseUrl); err != nil {
		return nil, err 
	}

	if client == nil {
		return nil, fmt.Errorf("*http.Client cannot be empty")
	}

	return &BaseCountyConnector{
		baseUrl:  baseUrl,
		county:   county,
		dataDesc: dataDesc,
		client:   client,
	}, nil
}
