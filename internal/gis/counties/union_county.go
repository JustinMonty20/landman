package counties

import (
	"net/http"
	"net/url"
)

type UnionCountyConnector struct {
  *BaseCountyConnector
}

type UCArcGisParams struct {
  where string
  returnCountOnly bool
  returnGeometry bool
  format string
}

// Union County Data Connector
func NewUnionCountyConnector(
  baseUrl string,
  county string,
  dataDesc string,
  client *http.Client,
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


// QueryParams for the Union County ArcGIS API
func (qp UCArcGisParams) ToUrlValues() url.Values {
  qps := url.Values{}
  
  if qp.where != "" {
    qps.Add("where", qp.where)
  }

  if qp.returnCountOnly {
    qps.Add("returnCountOnly", "true")
  }

  if qp.returnGeometry {
    qps.Add("returnGeometry", "true")
  }

  if qp.format != "" {
    qps.Add("format", qp.format)
  }
  return qps
}
