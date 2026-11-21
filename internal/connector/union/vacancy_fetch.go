package union

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
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

// VacancyCaptureByteLimit bounds retained response bytes across one capture attempt.
const VacancyCaptureByteLimit int64 = 256 << 20

// CaptureVacancyLayer retains bounded responses. Structures pages reconcile source IDs;
// assessed-value OBJECTID is not a unique row identity and cannot safely page.
func (s *GISSource) CaptureVacancyLayer(ctx context.Context, layer VacancyLayer, saveReceipt func(string, []byte, int, int, bool) error) (int, error) {
	if saveReceipt == nil {
		return 0, errors.New("capture callback required")
	}
	var aggregateBytes int64
	save := func(request string, body []byte, crs, status int, complete bool) error {
		aggregateBytes += int64(len(body))
		if aggregateBytes > VacancyCaptureByteLimit {
			return errors.New("vacancy capture exceeds aggregate byte limit")
		}
		return saveReceipt(request, body, crs, status, complete)
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
	body, status, responseErr := s.getVacancy(ctx, countURL)
	if len(body) != 0 {
		if err := save(countURL, body, 0, status, responseErr == nil); err != nil {
			return 0, err
		}
	}
	if responseErr != nil {
		return 0, responseErr
	}
	count := gjson.GetBytes(body, "count")
	if !gjson.ValidBytes(body) || gjson.GetBytes(body, "error").Exists() || gjson.GetBytes(body, "exceededTransferLimit").Bool() || count.Type != gjson.Number || count.Raw != strconv.FormatInt(count.Int(), 10) || count.Int() < 0 {
		return 0, errors.New("invalid vacancy count")
	}
	limit := int64(250000)
	if layer == AssessedValueLayer {
		limit = AssessmentRowLimit
	}
	if count.Int() > limit {
		return 0, fmt.Errorf("%s count %d exceeds safe capture gate %d", layer, count.Int(), limit)
	}
	if layer == AssessedValueLayer {
		return int(count.Int()), s.CaptureAssessmentRanges(ctx, base, count.Int(), save)
	}
	var ids []int64
	if layer == StructuresLayer {
		ids, err = s.vacancyIDs(ctx, base, save)
		if err != nil {
			return 0, err
		}
		if len(ids) != int(count.Int()) {
			return 0, errors.New("structure count and source IDs disagree")
		}
	}
	q = url.Values{"where": {"1=1"}, "returnGeometry": {"false"}, "outFields": {"*"}, "f": {"json"}, "resultRecordCount": {"2000"}}
	if layer == StructuresLayer {
		q.Set("returnGeometry", "true")
		q.Set("outSR", "3857")
		q.Set("orderByFields", "OBJECTID ASC")
	}
	for offset := int64(0); offset < count.Int() || offset == 0; offset += 2000 {
		if layer == StructuresLayer {
			q.Set("resultOffset", strconv.FormatInt(offset, 10))
		}
		base.RawQuery = q.Encode()
		pageURL := base.String()
		body, status, responseErr = s.getVacancy(ctx, pageURL)
		crs := 0
		if layer == StructuresLayer {
			crs, _ = ParcelResponseCRS(body)
		}
		if len(body) != 0 {
			if err := save(pageURL, body, crs, status, responseErr == nil); err != nil {
				return 0, err
			}
		}
		if responseErr != nil {
			return 0, responseErr
		}
		if layer == StructuresLayer && crs == 0 {
			return 0, fmt.Errorf("structure page at offset %d: missing or invalid CRS", offset)
		}
		if err := ValidateStructurePage(body, int(offset), int(count.Int()), ids); err != nil {
			return 0, err
		}
		checked := int(offset) + min(2000, int(count.Int()-offset))
		log.Printf("[union.gis.structures] Structures (layer 171): checked %d/%d records", checked, count.Int())
	}
	if layer == StructuresLayer {
		after, err := s.vacancyIDs(ctx, base, save)
		if err != nil {
			return 0, err
		}
		if len(after) != len(ids) {
			return 0, errors.New("structure source ID membership drift")
		}
		for i := range ids {
			if after[i] != ids[i] {
				return 0, errors.New("structure source ID membership drift")
			}
		}
		base.RawQuery = url.Values{"where": {"1=1"}, "returnCountOnly": {"true"}, "f": {"json"}}.Encode()
		request := base.String()
		body, status, err := s.getVacancy(ctx, request)
		if len(body) != 0 {
			if e := save(request, body, 0, status, err == nil); e != nil {
				return 0, e
			}
		}
		if err != nil {
			return 0, err
		}
		final := gjson.GetBytes(body, "count")
		if gjson.GetBytes(body, "error").Exists() || gjson.GetBytes(body, "exceededTransferLimit").Bool() || final.Type != gjson.Number || final.Raw != count.Raw {
			return 0, errors.New("structure count drift")
		}
	}
	return int(count.Int()), nil
}

func (s *GISSource) vacancyIDs(ctx context.Context, base *url.URL, save func(string, []byte, int, int, bool) error) ([]int64, error) {
	base.RawQuery = url.Values{"where": {"1=1"}, "returnIdsOnly": {"true"}, "f": {"json"}}.Encode()
	request := base.String()
	body, status, err := s.getVacancy(ctx, request)
	if len(body) != 0 {
		if e := save(request, body, 0, status, err == nil); e != nil {
			return nil, e
		}
	}
	if err != nil {
		return nil, err
	}
	return StructureIDs(body)
}

// ValidateStructurePage checks one ordered structures page against the full source ID list.
// A full intermediate page can report that more records remain. A short or final
// flagged page cannot establish complete coverage and must fail.
func ValidateStructurePage(body []byte, offset, total int, ids []int64) error {
	if offset < 0 || total < 0 || offset > total || len(ids) != total {
		return fmt.Errorf("structure page at offset %d: invalid source membership", offset)
	}
	expected := min(2000, total-offset)
	features := gjson.GetBytes(body, "features")
	if !gjson.ValidBytes(body) || gjson.GetBytes(body, "error").Exists() || !features.IsArray() || len(features.Array()) != expected {
		return fmt.Errorf("structure page at offset %d incomplete: got %d rows, expected %d", offset, len(features.Array()), expected)
	}
	if gjson.GetBytes(body, "exceededTransferLimit").Bool() && offset+expected >= total {
		return fmt.Errorf("structure page at offset %d: final page reports transfer limit", offset)
	}
	for i, feature := range features.Array() {
		v := feature.Get("attributes.OBJECTID")
		if v.Type != gjson.Number || v.Raw != strconv.FormatInt(ids[offset+i], 10) {
			return fmt.Errorf("structure source ID membership drift at offset %d, row %d (expected OBJECTID %d)", offset, i+1, ids[offset+i])
		}
	}
	return nil
}

// StructureIDs validates the complete unique source membership response.
func StructureIDs(body []byte) ([]int64, error) {
	if gjson.GetBytes(body, "error").Exists() || gjson.GetBytes(body, "exceededTransferLimit").Bool() || gjson.GetBytes(body, "objectIdFieldName").String() != "OBJECTID" {
		return nil, errors.New("invalid structure ID response")
	}
	raw := gjson.GetBytes(body, "objectIds")
	if !raw.IsArray() || len(raw.Array()) > 250000 {
		return nil, errors.New("missing or oversized structure ID list")
	}
	ids := make([]int64, 0, len(raw.Array()))
	for _, v := range raw.Array() {
		if v.Type != gjson.Number || v.Raw != strconv.FormatInt(v.Int(), 10) || v.Int() < 0 {
			return nil, errors.New("invalid structure source ID")
		}
		ids = append(ids, v.Int())
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for i := 1; i < len(ids); i++ {
		if ids[i] == ids[i-1] {
			return nil, errors.New("duplicate structure source ID")
		}
	}
	return ids, nil
}

// getVacancy returns bounded response bytes even when status, JSON, or size is rejected.
func (s *GISSource) getVacancy(ctx context.Context, rawURL string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "ParcelDataCollection/0.0.1")
	resp, err := s.httpClient.Do(ctx, req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	const limit = 32 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return body, resp.StatusCode, err
	}
	if len(body) > limit {
		return body, resp.StatusCode, errors.New("vacancy response exceeds 32 MiB limit")
	}
	if err := unexpectedHTTPStatus(resp.StatusCode); err != nil {
		return body, resp.StatusCode, err
	}
	if !gjson.ValidBytes(body) {
		return body, resp.StatusCode, errors.New("vacancy response invalid JSON")
	}
	return body, resp.StatusCode, nil
}
