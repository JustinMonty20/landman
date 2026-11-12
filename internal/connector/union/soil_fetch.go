package union

import (
	"context"
	"fmt"
	"strconv"

	"github.com/JustinMonty20/landman/internal/soil"
	"github.com/tidwall/gjson"
)

const soilPageSize = 2000

// FetchSoilPolygons reads layer 197 on the same rate-limited client as parcel queries.
func (s *GISSource) FetchSoilPolygons(ctx context.Context) ([]soil.Polygon, []soil.MapUnit, error) {
	countParams := ArcGISParams{Where: "1=1", ReturnCountOnly: true, Format: "json"}
	countURL, err := s.soilURL(countParams)
	if err != nil {
		return nil, nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	body, err := s.get(ctx, countURL)
	if err != nil {
		return nil, nil, fmt.Errorf("soil count request: %w", err)
	}
	if arcErr := gjson.GetBytes(body, "error"); arcErr.Exists() {
		return nil, nil, fmt.Errorf("soil count ArcGIS error: %s", arcErr.Raw)
	}
	countValue := gjson.GetBytes(body, "count")
	count, err := strconv.ParseInt(countValue.Raw, 10, 64)
	if !countValue.Exists() || countValue.Type != gjson.Number || err != nil || count < 0 || int64(int(count)) != count {
		return nil, nil, fmt.Errorf("soil response missing non-negative integer count")
	}

	var polygons []soil.Polygon
	var units []soil.MapUnit
	params := ArcGISParams{
		Where: "1=1", ReturnGeometry: true, ResultRecordCount: soilPageSize,
		OrderByFields: "OBJECTID_1", OutFields: "OBJECTID_1,AREASYMBOL,SPATIALVER,MUSYM,MUKEY", Format: "json",
	}
	for offset := 0; offset < int(count); offset += soilPageSize {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		params.ResultOffset = offset
		pageURL, err := s.soilURL(params)
		if err != nil {
			return nil, nil, err
		}
		body, err := s.get(ctx, pageURL)
		if err != nil {
			return nil, nil, fmt.Errorf("soil page at offset %d: %w", offset, err)
		}
		pagePolygons, pageUnits, err := ParseSoilFeatures(body)
		if err != nil {
			return nil, nil, fmt.Errorf("soil page at offset %d: %w", offset, err)
		}
		polygons = append(polygons, pagePolygons...)
		units = append(units, pageUnits...)
		if len(pagePolygons) < soilPageSize {
			break
		}
	}
	if len(polygons) != int(count) {
		return nil, nil, fmt.Errorf("soil count mismatch: fetched %d want %d", len(polygons), count)
	}
	return polygons, units, nil
}

func (s *GISSource) soilURL(params ArcGISParams) (string, error) {
	values, err := params.ToURLValues()
	if err != nil {
		return "", err
	}
	return s.soilQueryURL + "?" + values.Encode(), nil
}
