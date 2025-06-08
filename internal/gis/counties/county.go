package counties

import (
	"fmt"
	"net/http"
	"net/url"
)

/*
all counties that I onboard onto this will use this struct
*/
type BaseCountyConnector struct {
	baseUrl  string
	county   string
	dataDesc string
	client   *http.Client
}

/*
interface for all county query params
*/
type QueryParams interface {
  ToUrlValues() url.Values
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
func (bcc BaseCountyConnector) BuildUrl(qp QueryParams) string {
  queryParams := qp.ToUrlValues() 
  return fmt.Sprintf("%s?%s", bcc.baseUrl, queryParams.Encode())
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
Creates a new county connector to be used
for any of the specific downstream counties
*/
func NewCountyConnector(
	baseUrl string,
	county string,
	dataDesc string,
	client *http.Client,
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
