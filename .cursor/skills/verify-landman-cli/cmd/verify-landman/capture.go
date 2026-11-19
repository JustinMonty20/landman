package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/soil"
)

const captureBudget = 3 * time.Minute

type sqlExec func(ctx context.Context, container, databaseURL, sql string) error

type captureDeps struct {
	get   httpGetter
	exec  sqlExec
	query sqlQuery
	now   time.Time
}

// runCapture refreshes seed.sql from the county layers.
// A 30 second request or a missed 3 minute budget leaves the current seed in place.
func runCapture(container, postgresURL, seedPath string, deps captureDeps) (string, int) {
	if err := refuseContainer(container); err != nil {
		fmt.Fprintf(os.Stderr, "capture: %v\n", err)
		return "", 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), captureBudget)
	defer cancel()
	err := captureSeed(ctx, container, postgresURL, seedPath, deps)
	return finishCapture(seedPath+".capture-tmp", err)
}

func finishCapture(tmp string, err error) (string, int) {
	if err == nil {
		return "", 0
	}
	_ = os.Remove(tmp)
	if countySlowErr(err) {
		return "capture: left seed.sql in place (county slow)\n", 2
	}
	fmt.Fprintf(os.Stderr, "capture: %v\n", err)
	return "", 1
}

func countySlowErr(err error) bool {
	return timeoutErr(err) || errors.Is(err, context.DeadlineExceeded)
}

func captureSeed(ctx context.Context, container, postgresURL, seedPath string, deps captureDeps) error {
	text, err := os.ReadFile(seedPath)
	if err != nil {
		return err
	}
	previous, err := parseSeed(string(text))
	if err != nil {
		return err
	}
	when := deps.now
	if when.IsZero() {
		when = time.Now()
	}
	when = when.UTC()
	records, err := fetchParcelRecords(ctx, deps.get, previous.parcelIDs())
	if err != nil {
		return err
	}
	parcels, err := shapeParcels(previous, records)
	if err != nil {
		return err
	}
	bounds, err := parcelBounds(parcels)
	if err != nil {
		return err
	}
	polygons, units, err := fetchSoil(ctx, deps.get, bounds)
	if err != nil {
		return err
	}
	renderedSoils, err := shapeSoils(polygons, units)
	if err != nil {
		return err
	}
	overlaps, err := overlapInCaptureDB(ctx, deps, container, postgresURL, parcels, renderedSoils)
	if err != nil {
		return err
	}
	body, err := renderSeed(previous, parcels, renderedSoils, overlaps, when)
	if err != nil {
		return err
	}
	tmp := seedPath + ".capture-tmp"
	if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
		return err
	}
	return publishCapturedSeed(tmp, seedPath, previous.parcelIDs())
}

func publishCapturedSeed(tmpPath, seedPath string, wantIDs []string) error {
	text, err := os.ReadFile(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	snap, err := parseSeed(string(text))
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if !sameIDs(snap.parcelIDs(), wantIDs) {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("captured parcel ids do not match the current seed")
	}
	return os.Rename(tmpPath, seedPath)
}

func sameIDs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	counts := map[string]int{}
	for _, id := range want {
		counts[id]++
	}
	for _, id := range got {
		counts[id]--
		if counts[id] < 0 {
			return false
		}
	}
	for _, n := range counts {
		if n != 0 {
			return false
		}
	}
	return true
}

func parcelLikelyVacant(record connector.RawRecord) bool {
	vacant := union.IsLikelyVacantGISRecord(record)
	attrs, _ := record.RawData["attributes"].(map[string]any)
	pid, _ := attrs["PID"].(string)
	if pid == "" {
		return false
	}
	return vacant
}

func shapeParcels(previous seedSnapshot, records []connector.RawRecord) ([]renderedParcel, error) {
	translator := union.NewGISTranslator()
	byID := map[string]renderedParcel{}
	for _, record := range records {
		vacant := parcelLikelyVacant(record)
		parcel, err := translator.Translate(record)
		if err != nil {
			return nil, err
		}
		id := parcel.Identity.SourceParcelID
		if _, dup := byID[id]; dup {
			return nil, fmt.Errorf("duplicate parcel %s", id)
		}
		stored, ok := previous.parcel(id)
		if !ok {
			return nil, fmt.Errorf("county returned parcel %s that is not in the seed", id)
		}
		byID[id] = renderedParcel{ID: stored.ID, Vacant: vacant, Parcel: parcel}
	}
	rows := make([]renderedParcel, 0, len(previous.parcels))
	for _, stored := range previous.parcels {
		row, ok := byID[stored.SourceParcelID]
		if !ok {
			return nil, fmt.Errorf("county response missing %s", stored.SourceParcelID)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func shapeSoils(polygons []soil.Polygon, units []soil.MapUnit) ([]renderedSoil, error) {
	if len(polygons) != len(units) {
		return nil, fmt.Errorf("soil polygons and map units differ")
	}
	rows := make([]renderedSoil, 0, len(polygons))
	seen := map[string]struct{}{}
	for i, polygon := range polygons {
		if _, dup := seen[polygon.ObjectID]; dup {
			return nil, fmt.Errorf("duplicate soil object %s", polygon.ObjectID)
		}
		seen[polygon.ObjectID] = struct{}{}
		id, err := newUUID()
		if err != nil {
			return nil, err
		}
		rows = append(rows, renderedSoil{ID: id, Polygon: polygon, Unit: units[i]})
	}
	return rows, nil
}

func fetchParcelRecords(ctx context.Context, get httpGetter, ids []string) ([]connector.RawRecord, error) {
	where, err := pidIn(ids)
	if err != nil {
		return nil, err
	}
	features, err := fetchPages(ctx, get, parcelLayer+"?f=json", func(pageSize, offset int) string {
		return parcelIDPageURL(where, pageSize, offset)
	})
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	records := make([]connector.RawRecord, 0, len(features))
	for _, feature := range features {
		attrs, _ := feature["attributes"].(map[string]any)
		records = append(records, connector.RawRecord{
			SourceName: "union_county_gis",
			ParcelID:   attrText(attrs["PID"]),
			FetchedAt:  now,
			RawData:    feature,
		})
	}
	return records, nil
}

func fetchSoil(ctx context.Context, get httpGetter, bounds geomBounds) ([]soil.Polygon, []soil.MapUnit, error) {
	var polygons []soil.Polygon
	var units []soil.MapUnit
	_, err := fetchPages(ctx, get, soilLayer+"?f=json", func(pageSize, offset int) string {
		return soilEnvelopeURL(bounds, pageSize, offset)
	}, func(body []byte) (int, error) {
		pagePolygons, pageUnits, err := union.ParseSoilFeatures(body)
		if err != nil {
			return 0, err
		}
		polygons = append(polygons, pagePolygons...)
		units = append(units, pageUnits...)
		return len(pagePolygons), nil
	})
	if err != nil {
		return nil, nil, err
	}
	return polygons, units, nil
}

type pageSink func(body []byte) (int, error)

func fetchPages(ctx context.Context, get httpGetter, metaURL string, pageURL func(pageSize, offset int) string, sink ...pageSink) ([]map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	status, body, err := get(ctx, metaURL)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("metadata HTTP %d", status)
	}
	pageSize, err := maxRecordCount(body)
	if err != nil {
		return nil, err
	}
	var features []map[string]any
	for offset := 0; ; offset += pageSize {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if offset/pageSize > 10000 {
			return nil, fmt.Errorf("layer pages did not end")
		}
		status, body, err = get(ctx, pageURL(pageSize, offset))
		if err != nil {
			return nil, err
		}
		if status != 200 {
			return nil, fmt.Errorf("layer page HTTP %d", status)
		}
		if len(sink) > 0 {
			n, err := sink[0](body)
			if err != nil {
				return nil, err
			}
			if n < pageSize {
				break
			}
			continue
		}
		pageFeatures, err := decodeFeatures(body)
		if err != nil {
			return nil, err
		}
		features = append(features, pageFeatures...)
		if len(pageFeatures) < pageSize {
			break
		}
	}
	return features, nil
}

func decodeFeatures(body []byte) ([]map[string]any, error) {
	var page struct {
		Features []map[string]any `json:"features"`
		Error    json.RawMessage  `json:"error"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, err
	}
	if err := arcGISError(page.Error); err != nil {
		return nil, err
	}
	if page.Features == nil {
		return nil, fmt.Errorf("layer page missing features")
	}
	return page.Features, nil
}

func parcelIDPageURL(where string, pageSize, offset int) string {
	values := url.Values{}
	values.Set("where", where)
	values.Set("outFields", "*")
	values.Set("returnGeometry", "true")
	values.Set("orderByFields", "OBJECTID")
	values.Set("resultRecordCount", strconv.Itoa(pageSize))
	values.Set("resultOffset", strconv.Itoa(offset))
	values.Set("f", "json")
	return parcelLayer + "/query?" + values.Encode()
}

type geomBounds struct {
	minX, minY, maxX, maxY float64
}

func parcelBounds(parcels []renderedParcel) (geomBounds, error) {
	var bounds geomBounds
	for i, row := range parcels {
		shape := row.Parcel.Geometry.Shape
		if shape == nil || *shape == nil {
			return geomBounds{}, fmt.Errorf("parcel %s has no shape", row.Parcel.Identity.SourceParcelID)
		}
		box := (*shape).Bounds()
		if i == 0 {
			bounds = geomBounds{minX: box.Min(0), minY: box.Min(1), maxX: box.Max(0), maxY: box.Max(1)}
			continue
		}
		if box.Min(0) < bounds.minX {
			bounds.minX = box.Min(0)
		}
		if box.Min(1) < bounds.minY {
			bounds.minY = box.Min(1)
		}
		if box.Max(0) > bounds.maxX {
			bounds.maxX = box.Max(0)
		}
		if box.Max(1) > bounds.maxY {
			bounds.maxY = box.Max(1)
		}
	}
	if len(parcels) == 0 {
		return geomBounds{}, fmt.Errorf("no parcels")
	}
	return bounds, nil
}

func soilEnvelopeURL(bounds geomBounds, pageSize, offset int) string {
	geometry := fmt.Sprintf(`{"xmin":%s,"ymin":%s,"xmax":%s,"ymax":%s,"spatialReference":{"wkid":3857}}`,
		strconv.FormatFloat(bounds.minX, 'f', -1, 64),
		strconv.FormatFloat(bounds.minY, 'f', -1, 64),
		strconv.FormatFloat(bounds.maxX, 'f', -1, 64),
		strconv.FormatFloat(bounds.maxY, 'f', -1, 64))
	values := url.Values{}
	values.Set("geometry", geometry)
	values.Set("geometryType", "esriGeometryEnvelope")
	values.Set("inSR", "3857")
	values.Set("spatialRel", "esriSpatialRelIntersects")
	values.Set("outFields", "OBJECTID_1,AREASYMBOL,SPATIALVER,MUSYM,MUKEY")
	values.Set("returnGeometry", "true")
	values.Set("orderByFields", "OBJECTID_1")
	values.Set("resultRecordCount", strconv.Itoa(pageSize))
	values.Set("resultOffset", strconv.Itoa(offset))
	values.Set("f", "json")
	return soilLayer + "/query?" + values.Encode()
}

// overlapInCaptureDB loads shapes into landman_capture, reads overlap acres, and drops that database.
func overlapInCaptureDB(ctx context.Context, deps captureDeps, container, postgresURL string, parcels []renderedParcel, soils []renderedSoil) ([]renderedOverlap, error) {
	captureURL, err := withDatabase(postgresURL, "landman_capture")
	if err != nil {
		return nil, err
	}
	created := false
	defer func() {
		if !created {
			return
		}
		dropCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = deps.exec(dropCtx, container, postgresURL, "DROP DATABASE IF EXISTS landman_capture;")
	}()
	created = true
	if err := deps.exec(ctx, container, postgresURL, `
SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'landman_capture' AND pid <> pg_backend_pid();
DROP DATABASE IF EXISTS landman_capture;
CREATE DATABASE landman_capture;
`); err != nil {
		return nil, err
	}
	var script strings.Builder
	script.WriteString("CREATE EXTENSION IF NOT EXISTS postgis;\n")
	script.WriteString("CREATE TABLE parcels (id uuid PRIMARY KEY, shape geometry(Geometry, 3857) NOT NULL);\n")
	script.WriteString("CREATE TABLE soil_polygons (id uuid PRIMARY KEY, shape geometry(Geometry, 3857) NOT NULL);\n")
	for _, row := range parcels {
		shape, err := sqlShape(row.Parcel.Geometry.Shape)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&script, "INSERT INTO parcels (id, shape) VALUES (%s, %s);\n", sqlUUID(row.ID), shape)
	}
	for _, row := range soils {
		shape, err := sqlShape(row.Polygon.Shape)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&script, "INSERT INTO soil_polygons (id, shape) VALUES (%s, %s);\n", sqlUUID(row.ID), shape)
	}
	if err := deps.exec(ctx, container, captureURL, script.String()); err != nil {
		return nil, err
	}
	raw, err := deps.query(ctx, container, captureURL, overlapSelectSQL())
	if err != nil {
		return nil, err
	}
	return overlapsFromRows(raw)
}

func overlapsFromRows(raw string) ([]renderedOverlap, error) {
	var rows []renderedOverlap
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			return nil, fmt.Errorf("overlap row %q", line)
		}
		if _, ok := ratFromDecimal(fields[2]); !ok {
			return nil, fmt.Errorf("overlap acres %q", fields[2])
		}
		id, err := newUUID()
		if err != nil {
			return nil, err
		}
		rows = append(rows, renderedOverlap{
			ID: id, SoilID: fields[0], ParcelID: fields[1], Acres: fields[2],
		})
	}
	return rows, nil
}

func withDatabase(rawURL, name string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	parsed.Path = "/" + name
	return parsed.String(), nil
}

func newUUID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16]), nil
}
