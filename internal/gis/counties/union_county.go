package counties

import (
	"fmt"
	"io"
  "context"	
  "net/http"
	"net/url"
  "github.com/tidwall/gjson"
)

type UnionCountyConnector struct {
	*BaseCountyConnector
}

// Union Counties ArcGis Query Params for requests.
type UCArcGisParams struct {
	Where             string
	ReturnCountOnly   bool
	ReturnGeometry    bool
	ResultOffset      int // pagination mechanism
	ResultRecordCount int // batch size
	Format            string
  OutFields         string
}

// Union County Data Connector
func NewUnionCountyConnector(
	baseUrl string,
	county string,
	dataDesc string,
	client *RateLimitedClient,
) (*UnionCountyConnector, error) {
	baseCountyConnector, err := NewCountyConnector(
		baseUrl,
		county,
		dataDesc,
		client,
	)

	if err != nil {
		return nil, err
	}

	return &UnionCountyConnector{
		baseCountyConnector,
	}, nil
}

func (ucc *UnionCountyConnector) QueryTotalParcels(ctx context.Context, qp UCArcGisParams) (int64, error) {
	url, err := ucc.BuildUrl(qp)
	if err != nil {
		return 0, err
	}
	// don't like the client.client refactor eventually.
	req, err := http.NewRequestWithContext(ctx,"GET", url, nil)
	if err != nil {
		return 0, err
	}

	req.Header.Set("User-Agent", "ParcelDataCollection/0.0.1")
	resp, err := ucc.Client.Do(ctx, req)
	if err != nil {
		return 0, err
	}

	// close the response body
	// sick go defer really like this for resource cleanup
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	// parse the response
	json := string(body)
	// fmt.Println(json)
	result := gjson.Get(json, "count")
	return result.Int(), nil
}

// QueryParams for the Union County ArcGIS A}
func (qp UCArcGisParams) ToUrlValues() (url.Values, error) {
	qps := url.Values{}

	if qp.Where != "" {
		qps.Add("where", qp.Where)
	}

	if qp.ReturnCountOnly {
		qps.Add("returnCountOnly", "true")
	}

	if qp.ReturnGeometry {
		qps.Add("returnGeometry", "true")
	}

	if qp.ResultOffset != 0 {
		qps.Add("resultOffset", fmt.Sprintf("%d", qp.ResultOffset))
	}

	if qp.ResultRecordCount != 0 {
		qps.Add("resultRecordCount", fmt.Sprintf("%d", qp.ResultRecordCount))
	}

	if qp.Format != "json" && qp.Format != "geojson" {
		return nil, fmt.Errorf("Invalid format. Must be json or geojson")
	} else {
		qps.Add("f", qp.Format)
	}

  if qp.OutFields == "" {
    qps.Add("outFields", "*")
  }

	return qps, nil
}

func (ucc *UnionCountyConnector) GetData(ctx context.Context, params UCArcGisParams) ([]gjson.Result, error) {
  url, err := ucc.BuildUrl(params); 
  if err != nil {
    return nil, err
  }
  req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
  if err != nil {
    return nil, err
  }
  req.Header.Set("User-Agent", "ParcelDataCollection/0.0.1")
  resp, err := ucc.Client.Do(ctx, req)
  // if something goes wrong with the request we want to return the error. 
  // we probably want to state of the params so I can store the request to be retried later.
  if err != nil {
    return nil, err 
  }
  defer resp.Body.Close()
  body, err := io.ReadAll(resp.Body)
  if err != nil {
    return nil, err
  }
  json := string(body)
  result := gjson.Get(json, "features|@pretty").Array()

  return result, nil
}
