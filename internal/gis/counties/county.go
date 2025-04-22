package counties

import (
  "net/http"
)

/*
  all counties that I onboard onto this will use this struct 
*/
type BaseCountyConnector struct {
   client *http.Client
   baseUrl string
   county string
   dataDesc string 
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
