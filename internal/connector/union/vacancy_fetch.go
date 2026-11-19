package union

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

// VacancyLayer identifies one independently captured county service.
type VacancyLayer string

const (
	StructuresLayer    VacancyLayer = "structures"
	AssessedValueLayer VacancyLayer = "assessed_value"
)

// CaptureVacancyLayer retains the complete response of a bounded single-page query.
// Multi-page imports require a verified stable total ordering or snapshot cursor.
// In particular, assessed-value OBJECTID is not a unique row identity.
func (s *GISSource) CaptureVacancyLayer(ctx context.Context, layer VacancyLayer, save func(string, []byte, int) error) (int, error) {
	if save == nil {
		return 0, errors.New("capture callback required")
	}
	base, err := url.Parse(s.baseURL)
	if err != nil {
		return 0, err
	}
	switch layer {
	case StructuresLayer:
		base.Path = strings.Replace(base.Path, "/215/query", "/171/query", 1)
		if !strings.HasSuffix(base.Path, "/171/query") {
			return 0, errors.New("parcel endpoint must end in /215/query")
		}
	case AssessedValueLayer:
		base.Path = strings.Replace(base.Path, "/OperationalLayers/MapServer/215/query", "/Property_Tax_Live/Assessed_Value/MapServer/0/query", 1)
		if !strings.HasSuffix(base.Path, "/Property_Tax_Live/Assessed_Value/MapServer/0/query") {
			return 0, errors.New("parcel endpoint must be OperationalLayers/MapServer/215/query")
		}
	default:
		return 0, errors.New("unsupported vacancy layer")
	}
	base.RawQuery = ""
	q := url.Values{"where": {"1=1"}, "returnCountOnly": {"true"}, "f": {"json"}}
	base.RawQuery = q.Encode()
	countURL := base.String()
	body, err := s.get(ctx, countURL)
	if err != nil {
		return 0, err
	}
	if err := save(countURL, body, 0); err != nil {
		return 0, err
	}
	count := gjson.GetBytes(body, "count")
	if !gjson.ValidBytes(body) || count.Type != gjson.Number || count.Raw != strconv.FormatInt(count.Int(), 10) || count.Int() < 0 {
		return 0, errors.New("invalid vacancy count")
	}
	// This is deliberately not offset pagination. A count-matching offset scan is
	// insufficient when the service has repeated OIDs or changes during a scan.
	if count.Int() > 2000 {
		return 0, fmt.Errorf("%s count %d exceeds single-page safety gate; stable pagination not verified", layer, count.Int())
	}
	q = url.Values{"where": {"1=1"}, "returnGeometry": {"false"}, "outFields": {"*"}, "f": {"json"}, "resultRecordCount": {"2000"}}
	if layer == StructuresLayer {
		q.Set("returnGeometry", "true")
		q.Set("outSR", "3857")
	}
	base.RawQuery = q.Encode()
	pageURL := base.String()
	body, err = s.get(ctx, pageURL)
	if err != nil {
		return 0, err
	}
	crs := 0
	if layer == StructuresLayer {
		crs, err = ParcelResponseCRS(body)
		if err != nil {
			return 0, err
		}
	}
	if err := save(pageURL, body, crs); err != nil {
		return 0, err
	}
	features := gjson.GetBytes(body, "features")
	if gjson.GetBytes(body, "error").Exists() || !features.IsArray() || int64(len(features.Array())) != count.Int() || gjson.GetBytes(body, "exceededTransferLimit").Bool() {
		return 0, errors.New("vacancy page incomplete")
	}
	return int(count.Int()), nil
}
