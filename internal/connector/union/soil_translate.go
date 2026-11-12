package union

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/JustinMonty20/landman/internal/county"
	"github.com/JustinMonty20/landman/internal/soil"
)

// ParseSoilFeature translates a single ArcGIS feature into shared soil values.
func ParseSoilFeature(data []byte) (soil.Polygon, soil.MapUnit, error) {
	var feature map[string]interface{}
	if err := json.Unmarshal(data, &feature); err != nil {
		return soil.Polygon{}, soil.MapUnit{}, fmt.Errorf("decode soil feature: %w", err)
	}
	if feature == nil || feature["error"] != nil {
		return soil.Polygon{}, soil.MapUnit{}, fmt.Errorf("invalid ArcGIS soil feature: %v", feature["error"])
	}
	attrs, ok := asMap(feature["attributes"])
	if !ok {
		return soil.Polygon{}, soil.MapUnit{}, fmt.Errorf("soil feature missing attributes")
	}
	objectID, ok := intValue(attrs["OBJECTID_1"])
	if !ok {
		return soil.Polygon{}, soil.MapUnit{}, fmt.Errorf("soil feature missing OBJECTID_1")
	}
	key := strings.TrimSpace(str(attrs, "MUKEY"))
	symbol := strings.TrimSpace(str(attrs, "MUSYM"))
	if key == "" || symbol == "" {
		return soil.Polygon{}, soil.MapUnit{}, fmt.Errorf("soil feature missing MUKEY or MUSYM")
	}
	shape := geometryShape(feature)
	if shape == nil {
		return soil.Polygon{}, soil.MapUnit{}, fmt.Errorf("soil feature has no rings")
	}
	polygon := soil.Polygon{
		County: county.Union, SourceName: "union_county_gis",
		ObjectID: strconv.FormatInt(objectID, 10), MapUnitKey: key, Shape: shape,
	}
	if version, ok := floatValue(attrs["SPATIALVER"]); ok {
		polygon.SpatialVersion = &version
	}
	unit := soil.MapUnit{MapUnitKey: key, MapUnitSymbol: symbol, AreaSymbol: str(attrs, "AREASYMBOL")}
	return polygon, unit, nil
}

// ParseSoilFeatures translates an ArcGIS query response, including an empty page.
func ParseSoilFeatures(data []byte) ([]soil.Polygon, []soil.MapUnit, error) {
	var response struct {
		Features []json.RawMessage `json:"features"`
		Error    json.RawMessage   `json:"error"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, nil, fmt.Errorf("decode soil response: %w", err)
	}
	if len(response.Error) > 0 && string(response.Error) != "null" {
		return nil, nil, fmt.Errorf("ArcGIS soil error: %s", response.Error)
	}
	if response.Features == nil {
		return nil, nil, fmt.Errorf("soil response missing features")
	}
	polygons := make([]soil.Polygon, 0, len(response.Features))
	units := make([]soil.MapUnit, 0, len(response.Features))
	for _, raw := range response.Features {
		polygon, unit, err := ParseSoilFeature(raw)
		if err != nil {
			return nil, nil, err
		}
		polygons = append(polygons, polygon)
		units = append(units, unit)
	}
	return polygons, units, nil
}
