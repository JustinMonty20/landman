package main

import (
  "fmt"
  "time"
	"github.com/JustinMonty20/landman/internal/gis/counties"
)

func main() {
	mainClient := counties.NewHttpClient()
	rateLimitedClient := counties.NewRateLimitedClient(mainClient, 1)

	ucc, err := counties.NewUnionCountyConnector(
		"https://ucwater.unioncountync.gov/arcgis/rest/services/GoMaps/UnionGoMaps/MapServer/10/query",
		"Union",
		"Union County Parcel Data",
		rateLimitedClient,
	)

	if err != nil {
		panic(err)
	}

	params := counties.UCArcGisParams{
    Where: "1=1",
		ReturnCountOnly: true,
    Format: "geojson",
	}

	count, err := ucc.QueryTotalParcels(params)
  if err != nil {
    panic(err) 
  }

  fmt.Printf("Total parcels as of %v: %d", time.Now().UTC(), count) 

  // I would initialize my batching process here based on the the count. 
}
