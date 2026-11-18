package union

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/ewkb"
	"github.com/twpayne/go-geom/xy"
	"github.com/twpayne/go-geom/xy/lineintersection"
	"github.com/twpayne/go-geom/xy/lineintersector"
	"github.com/twpayne/go-geom/xy/location"
)

const (
	QualificationZoning   = "zoning"
	QualificationRoads    = "roads"
	qualificationPageSize = 500
)

// QualificationFeature is a typed county feature with a generation-scoped source identity.
type QualificationFeature interface{ qualificationFeature() }
type QualificationReceipt struct {
	Count      int
	SourceSRID int
	ServiceURL string
	Metadata   json.RawMessage
}
type SourceIdentity struct {
	LayerID    int
	SourceID   int64
	SourceSRID int
	Shape      []byte
}
type ZoneFeature struct {
	SourceIdentity
	Jurisdiction, DistrictCode, Description, CaseReference, OfficialURL, SourceDate string
}
type JurisdictionFeature struct {
	SourceIdentity
	Name, SourceDate string
}
type BoundaryFeature struct{ SourceIdentity }
type RoadFeature struct {
	SourceIdentity
	Name, Class, Status string
	SourceDate          *time.Time
}

func (ZoneFeature) qualificationFeature()         {}
func (JurisdictionFeature) qualificationFeature() {}
func (BoundaryFeature) qualificationFeature()     {}
func (RoadFeature) qualificationFeature()         {}

type qualificationLayer struct {
	id                                                       int
	name, oid, code, description, caseField, dateField, kind string
}

var zoningLayers = []qualificationLayer{
	{id: 190, name: "Union County", oid: "OBJECTID", code: "ZONE", caseField: "PETITION", kind: QualificationZoning},
	{id: 173, name: "Fairview", oid: "OBJECTID_1", code: "ZONE", caseField: "PETITION", kind: QualificationZoning},
	{id: 174, name: "Indian Trail", oid: "OBJECTID", code: "NEW_ZONIN", caseField: "CASE_NU", kind: QualificationZoning},
	{id: 175, name: "Lake Park", oid: "OBJECTID", code: "LP_Zone", kind: QualificationZoning},
	{id: 176, name: "Marshville", oid: "OBJECTID", code: "Zoning", description: "ZoneDesc", dateField: "LastUpdD", kind: QualificationZoning},
	{id: 177, name: "Marvin", oid: "OBJECTID", code: "Marvin_Zon", kind: QualificationZoning},
	{id: 180, name: "Monroe", oid: "OBJECTID", code: "NEW_ZONING", kind: QualificationZoning},
	{id: 181, name: "Stallings", oid: "OBJECTID", code: "CurrentZon", caseField: "CZ_Case", kind: QualificationZoning},
	{id: 183, name: "Waxhaw", oid: "OBJECTID", code: "Zoning", description: "ZoneDesc", dateField: "LastUpdD", kind: QualificationZoning},
	{id: 184, name: "Weddington", oid: "OBJECTID", code: "ZONE", kind: QualificationZoning},
	{id: 186, name: "Wingate", oid: "OBJECTID", code: "ZONE", kind: QualificationZoning},
	{id: 194, name: "ETJ", oid: "OBJECTID", code: "NAME", kind: "jurisdiction"},
	{id: 195, name: "Municipalities", oid: "OBJECTID", code: "NAME", dateField: "LAST_UPDATE", kind: "jurisdiction"},
	{id: 221, name: "Union boundary", oid: "OBJECTID", kind: "boundary"},
	{id: 170, name: "Road Centerlines", oid: "OBJECTID", code: "FullName", dateField: "DateUpdate", kind: QualificationRoads},
}

// QualificationLayers lists the required consistency group, including its boundary.
func QualificationLayers(group string) []int {
	ids := []int{}
	for _, layer := range zoningLayers {
		if (group == QualificationZoning && layer.kind != QualificationRoads) || (group == QualificationRoads && layer.kind == QualificationRoads) {
			ids = append(ids, layer.id)
		}
	}
	return ids
}

func layerByID(id int) (qualificationLayer, error) {
	for _, l := range zoningLayers {
		if l.id == id {
			return l, nil
		}
	}
	return qualificationLayer{}, fmt.Errorf("unknown qualification layer %d", id)
}

func parseQualificationFeature(spec qualificationLayer, raw json.RawMessage, srid int) (QualificationFeature, error) {
	var value map[string]interface{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	attrs, ok := asMap(value["attributes"])
	if !ok {
		return nil, errors.New("missing attributes")
	}
	oid, ok := attrs[spec.oid].(json.Number)
	if !ok {
		return nil, fmt.Errorf("missing or noninteger %s", spec.oid)
	}
	id, err := oid.Int64()
	if err != nil || id < 0 {
		return nil, fmt.Errorf("invalid %s", spec.oid)
	}
	fields := []string{spec.code, spec.description, spec.caseField, spec.dateField}
	if spec.kind == QualificationRoads {
		fields = append(fields, "FTR_CODE", "RoadClass", "STATUS")
	}
	for _, field := range fields {
		if field == "" {
			continue
		}
		v, exists := attrs[field]
		if !exists {
			return nil, fmt.Errorf("missing required attribute %s", field)
		}
		if v == nil {
			continue
		}
		if spec.kind == QualificationRoads && field == spec.dateField {
			n, valid := v.(json.Number)
			if !valid {
				return nil, fmt.Errorf("noninteger attribute %s", field)
			}
			if _, err := n.Int64(); err != nil {
				return nil, fmt.Errorf("noninteger attribute %s: %w", field, err)
			}
		} else if _, valid := v.(string); !valid {
			return nil, fmt.Errorf("nonstring attribute %s", field)
		}
	}
	var shape geom.T
	if spec.kind == QualificationRoads {
		geo, ok := asMap(value["geometry"])
		if !ok {
			return nil, errors.New("missing road geometry")
		}
		paths, ok := geo["paths"].([]interface{})
		if !ok || len(paths) == 0 {
			return nil, errors.New("missing road paths")
		}
		multi := geom.NewMultiLineString(geom.XY).SetSRID(3857)
		for _, path := range paths {
			points, ok := path.([]interface{})
			if !ok || len(points) < 2 {
				return nil, errors.New("invalid road path")
			}
			flat := []float64{}
			for _, p := range points {
				xy, ok := p.([]interface{})
				if !ok || len(xy) < 2 {
					return nil, errors.New("invalid road point")
				}
				x, xok := floatValue(xy[0])
				y, yok := floatValue(xy[1])
				if !xok || !yok {
					return nil, errors.New("invalid road coordinate")
				}
				flat = append(flat, x, y)
			}
			if err := multi.Push(geom.NewLineStringFlat(geom.XY, flat).SetSRID(3857)); err != nil {
				return nil, err
			}
		}
		shape = multi
	} else {
		geometry, ok := asMap(value["geometry"])
		if !ok {
			return nil, errors.New("missing polygon geometry")
		}
		polygonRings, ok := rings(geometry["rings"])
		if !ok {
			return nil, errors.New("invalid polygon rings")
		}
		shells := make([][]float64, 0, len(polygonRings))
		holes := make([][]float64, 0, len(polygonRings))
		for _, ring := range polygonRings {
			if len(ring) < 4 || ring[0] != ring[len(ring)-1] {
				return nil, errors.New("invalid polygon ring closure")
			}
			flat := make([]float64, 0, 2*len(ring))
			for _, point := range ring {
				flat = append(flat, point[0], point[1])
			}
			if xy.IsRingCounterClockwise(geom.XY, flat) {
				holes = append(holes, flat)
			} else {
				shells = append(shells, flat)
			}
		}
		for _, hole := range holes {
			contained := false
			for _, shell := range shells {
				if qualificationRingInsideShell(hole, shell) {
					contained = true
					break
				}
			}
			if !contained {
				return nil, errors.New("polygon hole is not inside a source shell")
			}
		}
		polygon := geometryShapeWithRingCheck(value, qualificationRingInsideShell)
		if polygon == nil {
			return nil, errors.New("invalid polygon rings")
		}
		shape = *polygon
	}
	if srid != 3857 && srid != 102100 {
		return nil, fmt.Errorf("unexpected source SRID %d", srid)
	}
	b, err := ewkb.Marshal(shape, ewkb.NDR)
	if err != nil {
		return nil, err
	}
	identity := SourceIdentity{LayerID: spec.id, SourceID: id, SourceSRID: srid, Shape: b}
	clean := func(k string) string { return strings.TrimSpace(str(attrs, k)) }
	switch spec.kind {
	case QualificationZoning:
		link := clean("URL")
		if link != "" {
			u, e := url.Parse(link)
			if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
				link = ""
			}
		}
		return ZoneFeature{SourceIdentity: identity, Jurisdiction: spec.name, DistrictCode: clean(spec.code), Description: clean(spec.description), CaseReference: clean(spec.caseField), OfficialURL: link, SourceDate: clean(spec.dateField)}, nil
	case "jurisdiction":
		return JurisdictionFeature{SourceIdentity: identity, Name: clean(spec.code), SourceDate: clean(spec.dateField)}, nil
	case "boundary":
		return BoundaryFeature{SourceIdentity: identity}, nil
	case QualificationRoads:
		var date *time.Time
		if rawDate, ok := attrs[spec.dateField].(json.Number); ok {
			n, _ := rawDate.Int64()
			if n > 0 {
				t := time.UnixMilli(n).UTC()
				date = &t
			}
		}
		name := clean("FullName")
		if name == "" {
			name = clean("DISPLAY")
		}
		class := clean("RoadClass")
		if class == "" {
			class = clean("FTR_CODE")
		}
		return RoadFeature{SourceIdentity: identity, Name: name, Class: class, Status: clean("STATUS"), SourceDate: date}, nil
	}
	return nil, fmt.Errorf("unknown layer kind %q", spec.kind)
}

// FetchQualificationLayer validates metadata, ordered pages, source IDs, and counts before return.
// consume can stage rows; a failed scan must never publish its generation.
func (s *GISSource) FetchQualificationLayer(ctx context.Context, id int, consume func(QualificationFeature) error) (QualificationReceipt, error) {
	spec, err := layerByID(id)
	if err != nil {
		return QualificationReceipt{}, err
	}
	if consume == nil {
		return QualificationReceipt{}, errors.New("missing qualification consumer")
	}
	root := strings.TrimSuffix(s.baseURL, "/215/query")
	if root == s.baseURL {
		return QualificationReceipt{}, errors.New("GIS base URL must end with /215/query")
	}
	endpoint := root + "/" + strconv.Itoa(id)
	meta, err := s.qualificationGet(ctx, endpoint+"?f=json")
	if err != nil {
		return QualificationReceipt{}, fmt.Errorf("layer %d metadata: %w", id, err)
	}
	var m struct {
		GeometryType string                        `json:"geometryType"`
		Fields       []struct{ Name, Type string } `json:"fields"`
		Extent       struct {
			SpatialReference struct {
				WKID       int `json:"wkid"`
				LatestWKID int `json:"latestWkid"`
			} `json:"spatialReference"`
		} `json:"extent"`
		MaxRecordCount int             `json:"maxRecordCount"`
		Error          json.RawMessage `json:"error"`
	}
	if err = json.Unmarshal(meta, &m); err != nil {
		return QualificationReceipt{}, err
	}
	if len(m.Error) > 0 && string(m.Error) != "null" {
		return QualificationReceipt{}, fmt.Errorf("ArcGIS layer %d metadata error: %s", id, m.Error)
	}
	expected := "esriGeometryPolygon"
	if spec.kind == QualificationRoads {
		expected = "esriGeometryPolyline"
	}
	if m.GeometryType != expected || m.MaxRecordCount < 1 {
		return QualificationReceipt{}, fmt.Errorf("layer %d geometry or page schema changed", id)
	}
	required := []string{spec.oid}
	for _, field := range []string{spec.code, spec.description, spec.caseField, spec.dateField} {
		if field != "" {
			required = append(required, field)
		}
	}
	if spec.kind == QualificationRoads {
		required = append(required, "FTR_CODE", "RoadClass", "STATUS")
	}
	for _, field := range required {
		found := false
		for _, actual := range m.Fields {
			if actual.Name == field && ((field == spec.oid && actual.Type == "esriFieldTypeOID") || (field != spec.oid && (actual.Type == "esriFieldTypeString" || (spec.kind == QualificationRoads && field == spec.dateField && actual.Type == "esriFieldTypeDate")))) {
				found = true
				break
			}
		}
		if !found {
			return QualificationReceipt{}, fmt.Errorf("layer %d missing typed field %s", id, field)
		}
	}
	sourceSRID := m.Extent.SpatialReference.WKID
	if sourceSRID == 0 {
		sourceSRID = m.Extent.SpatialReference.LatestWKID
	}
	if sourceSRID != 3857 && sourceSRID != 102100 {
		return QualificationReceipt{}, fmt.Errorf("layer %d unexpected native SRID %d", id, sourceSRID)
	}
	countURL := endpoint + "/query?" + (url.Values{"f": {"json"}, "where": {"1=1"}, "returnCountOnly": {"true"}}).Encode()
	count, err := s.qualificationCount(ctx, countURL)
	if err != nil {
		return QualificationReceipt{}, err
	}
	size := qualificationPageSize
	if m.MaxRecordCount < size {
		size = m.MaxRecordCount
	}
	if count < size {
		size = count
	}
	seen := map[int64]bool{}
	fields := append([]string(nil), required...)
	if spec.kind == QualificationZoning && (spec.id == 190 || spec.id == 173) {
		fields = append(fields, "URL")
	}
	if spec.kind == QualificationRoads {
		fields = append(fields, "FullName", "DISPLAY")
	}
	for offset := 0; offset < count; {
		q := url.Values{"f": {"json"}, "where": {"1=1"}, "returnGeometry": {"true"}, "outFields": {strings.Join(fields, ",")}, "outSR": {"3857"}, "resultOffset": {strconv.Itoa(offset)}, "resultRecordCount": {strconv.Itoa(size)}, "orderByFields": {spec.oid}}
		body, err := s.qualificationGet(ctx, endpoint+"/query?"+q.Encode())
		if err != nil {
			if strings.Contains(err.Error(), "response exceeds size limit") && size > 1 {
				size = (size + 1) / 2
				continue
			}
			return QualificationReceipt{}, fmt.Errorf("layer %d offset %d: %w", id, offset, err)
		}
		var page struct {
			Features         []json.RawMessage `json:"features"`
			SpatialReference struct {
				WKID       int `json:"wkid"`
				LatestWKID int `json:"latestWkid"`
			} `json:"spatialReference"`
			Error    json.RawMessage `json:"error"`
			Exceeded bool            `json:"exceededTransferLimit"`
		}
		if err = json.Unmarshal(body, &page); err != nil {
			return QualificationReceipt{}, err
		}
		if len(page.Error) > 0 && string(page.Error) != "null" {
			return QualificationReceipt{}, fmt.Errorf("ArcGIS layer %d error: %s", id, page.Error)
		}
		want := size
		if count-offset < want {
			want = count - offset
		}
		if page.Features == nil {
			return QualificationReceipt{}, fmt.Errorf("layer %d offset %d response missing features", id, offset)
		}
		if len(page.Features) != want {
			if page.Exceeded && len(page.Features) < want && size > 1 {
				size = (size + 1) / 2
				continue
			}
			return QualificationReceipt{}, fmt.Errorf("layer %d offset %d page count mismatch: got %d want %d", id, offset, len(page.Features), want)
		}
		if offset+want == count && page.Exceeded {
			return QualificationReceipt{}, fmt.Errorf("layer %d final page exceeds transfer limit", id)
		}
		srid := page.SpatialReference.LatestWKID
		if srid == 0 {
			srid = page.SpatialReference.WKID
		}
		if srid != 3857 && srid != 102100 {
			return QualificationReceipt{}, fmt.Errorf("layer %d unexpected output SRID %d", id, srid)
		}
		for _, raw := range page.Features {
			if err := ctx.Err(); err != nil {
				return QualificationReceipt{}, err
			}
			f, e := parseQualificationFeature(spec, raw, sourceSRID)
			if e != nil {
				return QualificationReceipt{}, fmt.Errorf("layer %d offset %d: %w", id, offset, e)
			}
			var key SourceIdentity
			switch x := f.(type) {
			case ZoneFeature:
				key = x.SourceIdentity
			case JurisdictionFeature:
				key = x.SourceIdentity
			case BoundaryFeature:
				key = x.SourceIdentity
			case RoadFeature:
				key = x.SourceIdentity
			}
			if seen[key.SourceID] {
				return QualificationReceipt{}, fmt.Errorf("layer %d repeats source ID %d", id, key.SourceID)
			}
			seen[key.SourceID] = true
			if err := consume(f); err != nil {
				return QualificationReceipt{}, err
			}
		}
		offset += want
	}
	latest, err := s.qualificationCount(ctx, countURL)
	if err != nil {
		return QualificationReceipt{}, err
	}
	if latest != count {
		return QualificationReceipt{}, fmt.Errorf("layer %d count drift: %d to %d", id, count, latest)
	}
	return QualificationReceipt{Count: count, SourceSRID: sourceSRID, ServiceURL: endpoint, Metadata: meta}, nil
}

func (s *GISSource) qualificationCount(ctx context.Context, u string) (int, error) {
	body, err := s.qualificationGet(ctx, u)
	if err != nil {
		return 0, err
	}
	v := gjson.GetBytes(body, "error")
	if v.Exists() {
		return 0, fmt.Errorf("ArcGIS count error: %s", v.Raw)
	}
	v = gjson.GetBytes(body, "count")
	if !v.Exists() || v.Type != gjson.Number {
		return 0, errors.New("missing qualification count")
	}
	n, e := strconv.Atoi(v.Raw)
	if e != nil || n < 0 {
		return 0, errors.New("invalid qualification count")
	}
	return n, nil
}

// Retry transient transport and server failures; an access denial never retries.
func (s *GISSource) qualificationGet(ctx context.Context, u string) ([]byte, error) {
	for attempt := 0; attempt < 3; attempt++ {
		body, err := s.get(ctx, u)
		if err == nil {
			return body, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var status httpStatusError
		var network net.Error
		retry := errors.As(err, &network) || (errors.As(err, &status) && (status.code == http.StatusTooManyRequests || status.code >= 500))
		if !retry || attempt == 2 {
			return nil, err
		}
		timer := time.NewTimer(time.Duration(attempt+1) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, errors.New("qualification request exhausted")
}

// Qualification holes may contact the shell once, including in an edge interior.
// Parcel ring interpretation stays unchanged.
func qualificationRingInsideShell(ring, shell []float64) bool {
	var contact geom.Coord
	for i := 0; i < len(ring)-2; i += 2 {
		start, end := geom.Coord(ring[i:i+2]), geom.Coord(ring[i+2:i+4])
		mid := geom.Coord{(start[0] + end[0]) / 2, (start[1] + end[1]) / 2}
		for _, point := range []geom.Coord{start, mid} {
			if xy.LocatePointInRing(geom.XY, point, shell) == location.Exterior {
				return false
			}
		}
		for j := 0; j < len(shell)-2; j += 2 {
			intersection := lineintersector.LineIntersectsLine(lineintersector.RobustLineIntersector{},
				start, end, geom.Coord(shell[j:j+2]), geom.Coord(shell[j+2:j+4]))
			if !intersection.HasIntersection() {
				continue
			}
			if intersection.Type() != lineintersection.PointIntersection || len(intersection.Intersection()) != 1 {
				return false
			}
			hit := intersection.Intersection()[0]
			if contact != nil && (hit[0] != contact[0] || hit[1] != contact[1]) {
				return false
			}
			contact = hit
		}
	}
	return true
}
