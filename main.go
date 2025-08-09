package main

import (
	"fmt"
  "context"
	"time"
	"github.com/JustinMonty20/landman/internal/gis/counties"
  "github.com/JustinMonty20/landman/internal/worker"
)

func main() {
  ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
  defer cancel()
	mainClient := counties.NewHttpClient()
	rateLimitedClient := counties.NewRateLimitedClient(mainClient, 1)

	ucc, err := counties.NewUnionCountyConnector(
		"https://atlas.unioncountync.gov/server/rest/services/OperationalLayers/MapServer/215/query",
		"Union",
		"Union County Parcel Data",
		rateLimitedClient,
	)

	if err != nil {
		panic(err)
	}

	params := counties.UCArcGisParams{
		Where:           "1=1",
		ReturnCountOnly: true,
		Format:          "json",
	}

	count, err := ucc.QueryTotalParcels(ctx, params)
	if err != nil {
		panic(err)
	}

	fmt.Printf("Total parcels as of %v: %d\n", time.Now().UTC(), count)

	// I would initialize my batching process here based on the the count.
  batch, err := worker.NewBatch(count, 50)
  
  if err != nil {
    panic(err)
  }
  
  fmt.Printf("Batch count: %d\n", batch.BatchCount);

  params.ReturnCountOnly = false
  params.ResultRecordCount = 50

  first50, err := ucc.GetData(ctx, params)
  if err != nil {
    panic(err)   
  }
  fmt.Printf("First 50 parcels: %v\n", first50)
}
