package counties

import (
	"fmt"
	"github.com/tidwall/gjson"
	"io"
	"net/http"
	"net/url"
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

func (ucc *UnionCountyConnector) QueryTotalParcels(qp UCArcGisParams) (int64, error) {
	url, err := ucc.BuildUrl(qp)
	if err != nil {
		return 0, err
	}
	// don't like the client.client refactor eventually.
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}

	req.Header.Set("User-Agent", "ParcelDataCollection/0.0.1")
	resp, err := ucc.client.client.Do(req)
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

	return qps, nil
}
