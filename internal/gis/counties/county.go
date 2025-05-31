package counties

import (
	"net/http"
  "fmt"
  "net/url"
)

/*
  all counties that I onboard onto this will use this struct
*/
type BaseCountyConnector struct {
   baseUrl string
   county string
   dataDesc string 
   client *http.Client  
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

  if _, err := url.Parse(baseUrl); err != nil {
    return nil, fmt.Errorf("invalid baseUrl! needs to be a valid URL")
  }

  if client == nil {
    return nil, fmt.Errorf("*http.Client cannot be empty")
  }
  
  return &BaseCountyConnector{
    baseUrl: baseUrl,
    county: county,
    dataDesc: dataDesc,
    client: client, 
  }, nil
}
