package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// saleSelectSQL is the seeded-parcel sale read. psql has no bound $1,
// so saleQuerySQL inlines the id array in that position.
const saleSelectSQL = `
SELECT p.source_parcel_id, s.sequence, s.sale_date::text, s.amount::text, s.deed_book, s.deed_page
FROM parcels p
LEFT JOIN parcel_sales s ON s.parcel_id = p.id
WHERE p.county_id = 'union' AND p.source_name = 'union_county_gis'
  AND p.source_parcel_id = ANY($1::text[])
ORDER BY 1, 2`

const committedIncludingQuarantineSQL = `
SELECT count(*)::text
FROM job_attempts
WHERE job_id = 'union.gis.parcels'
  AND attempt_kind = 'full_scan'
  AND outcome IN ('success', 'success_with_quarantine')`

const postImportSQL = `
SELECT 'county_parcels', count(*)::text
FROM parcels
WHERE county_id = 'union' AND source_name = 'union_county_gis'
UNION ALL
SELECT 'soil_polygons', count(*)::text
FROM soil_polygons
WHERE county_id = 'union' AND source_name = 'union_county_gis'
UNION ALL
SELECT 'overlaps', count(*)::text
FROM soil_polygon_parcels AS spp
JOIN soil_polygons AS s ON s.id = spp.soil_polygon_id
WHERE s.county_id = 'union' AND s.source_name = 'union_county_gis'
UNION ALL
SELECT 'parcel_jobs', count(*)::text
FROM job_attempts
WHERE job_id = 'union.gis.parcels' AND outcome = 'success' AND attempt_kind = 'full_scan'
UNION ALL
SELECT 'soil_jobs', count(*)::text
FROM job_attempts
WHERE job_id = 'union.gis.soil' AND outcome = 'success' AND attempt_kind = 'full_scan'
UNION ALL
SELECT 'screening_jobs', count(*)::text
FROM job_attempts
WHERE job_id = 'union.soil.screening' AND outcome = 'success' AND attempt_kind = 'full_scan'
UNION ALL
SELECT 'heads', count(*)::text
FROM soil_screening_heads
WHERE county_id = 'union' AND source_name = 'union_county_gis'
UNION ALL
SELECT 'assessments', count(*)::text
FROM soil_parcel_assessments AS a
JOIN soil_screening_heads AS h ON h.run_id = a.run_id
WHERE h.county_id = 'union' AND h.source_name = 'union_county_gis'`

func saleQuerySQL(ids []string) (string, error) {
	array, err := sqlTextArray(ids)
	if err != nil {
		return "", err
	}
	if !strings.Contains(saleSelectSQL, "ANY($1::text[])") {
		return "", fmt.Errorf("sale query lost its parameter")
	}
	return strings.Replace(saleSelectSQL, "$1::text[]", array+"::text[]", 1), nil
}

func sqlTextArray(ids []string) (string, error) {
	if len(ids) == 0 {
		return "", fmt.Errorf("no parcel ids")
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		if !safeParcelID(id) {
			return "", fmt.Errorf("parcel id %q", id)
		}
		parts[i] = "'" + id + "'"
	}
	return "ARRAY[" + strings.Join(parts, ",") + "]", nil
}

func safeParcelID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func readSaleComparison(ctx context.Context, get httpGetter, query sqlQuery, container, databaseURL string, ids []string) ([]saleSlot, []saleSlot, error) {
	expected, err := expectedSlotsFromCounty(ctx, get, ids)
	if err != nil {
		return nil, nil, err
	}
	sqlText, err := saleQuerySQL(ids)
	if err != nil {
		return nil, nil, err
	}
	raw, err := query(ctx, container, databaseURL, sqlText)
	if err != nil {
		return nil, nil, err
	}
	actual, err := parseSaleRows(raw)
	if err != nil {
		return nil, nil, err
	}
	return expected, actual, nil
}

func expectedSlotsFromCounty(ctx context.Context, get httpGetter, ids []string) ([]saleSlot, error) {
	status, body, err := get(ctx, parcelLayer+"?f=json")
	if err != nil {
		return nil, fmt.Errorf("parcel metadata: %w", err)
	}
	if status != 200 {
		return nil, fmt.Errorf("parcel metadata HTTP %d", status)
	}
	pageSize, err := maxRecordCount(body)
	if err != nil {
		return nil, err
	}
	where, err := pidIn(ids)
	if err != nil {
		return nil, err
	}
	var slots []saleSlot
	seen := map[string]struct{}{}
	for offset := 0; ; offset += pageSize {
		if pageSize == 0 || offset/pageSize > len(ids) {
			return nil, fmt.Errorf("parcel sale pages did not end")
		}
		status, body, err = get(ctx, parcelSalePageURL(where, pageSize, offset))
		if err != nil {
			return nil, fmt.Errorf("parcel sale page: %w", err)
		}
		if status != 200 {
			return nil, fmt.Errorf("parcel sale page HTTP %d", status)
		}
		pageSlots, pids, err := slotsFromPage(body)
		if err != nil {
			return nil, err
		}
		for _, pid := range pids {
			if _, dup := seen[pid]; dup {
				return nil, fmt.Errorf("duplicate parcel %s", pid)
			}
			seen[pid] = struct{}{}
		}
		slots = append(slots, pageSlots...)
		if len(pids) < pageSize {
			break
		}
	}
	for _, id := range ids {
		if _, ok := seen[id]; !ok {
			return nil, fmt.Errorf("county response missing %s", id)
		}
	}
	return slots, nil
}

func maxRecordCount(body []byte) (int, error) {
	var meta struct {
		MaxRecordCount int             `json:"maxRecordCount"`
		Error          json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &meta); err != nil {
		return 0, fmt.Errorf("parcel metadata: %w", err)
	}
	if err := arcGISError(meta.Error); err != nil {
		return 0, err
	}
	if meta.MaxRecordCount <= 0 {
		return 0, fmt.Errorf("layer metadata missing maxRecordCount")
	}
	return meta.MaxRecordCount, nil
}

func pidIn(ids []string) (string, error) {
	parts := make([]string, len(ids))
	for i, id := range ids {
		if !safeParcelID(id) {
			return "", fmt.Errorf("parcel id %q", id)
		}
		parts[i] = "'" + id + "'"
	}
	return "PID IN (" + strings.Join(parts, ",") + ")", nil
}

func parcelSalePageURL(where string, pageSize, offset int) string {
	values := url.Values{}
	values.Set("where", where)
	values.Set("outFields", saleOutFields())
	values.Set("returnGeometry", "false")
	values.Set("orderByFields", "OBJECTID")
	values.Set("resultRecordCount", strconv.Itoa(pageSize))
	values.Set("resultOffset", strconv.Itoa(offset))
	values.Set("f", "json")
	return parcelLayer + "/query?" + values.Encode()
}

func saleOutFields() string {
	fields := []string{"PID", "OBJECTID"}
	for _, prefix := range []string{"s1", "s2", "s3"} {
		fields = append(fields,
			prefix+"_SALEDATE",
			prefix+"_SALESAMT",
			prefix+"_DEED_BOOK",
			prefix+"_DEED_PAGE",
			prefix+"_DEEDTYPE",
			prefix+"_DEEDQUAL_CODEDESC",
			prefix+"_grantor",
			prefix+"_grantor2",
		)
	}
	return strings.Join(fields, ",")
}

func slotsFromPage(body []byte) ([]saleSlot, []string, error) {
	var page struct {
		Features []struct {
			Attributes map[string]any `json:"attributes"`
		} `json:"features"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, nil, fmt.Errorf("parcel page: %w", err)
	}
	if err := arcGISError(page.Error); err != nil {
		return nil, nil, err
	}
	slots := []saleSlot{}
	pids := make([]string, 0, len(page.Features))
	for _, feature := range page.Features {
		pid := attrText(feature.Attributes["PID"])
		if pid == "" {
			return nil, nil, fmt.Errorf("parcel feature missing PID")
		}
		pids = append(pids, pid)
		slots = append(slots, expectedSaleSlots(pid, feature.Attributes)...)
	}
	return slots, pids, nil
}

func arcGISError(raw json.RawMessage) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return fmt.Errorf("arcgis %s", raw)
}

func parseSaleRows(raw string) ([]saleSlot, error) {
	var slots []saleSlot
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 6 {
			return nil, fmt.Errorf("sale row %q", line)
		}
		for i := range fields {
			fields[i] = strings.TrimSpace(fields[i])
		}
		if fields[1] == "" {
			continue
		}
		sequence, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, fmt.Errorf("sale sequence %q", fields[1])
		}
		slots = append(slots, saleSlot{
			SourceParcelID: fields[0],
			Sequence:       sequence,
			SaleDate:       fields[2],
			Amount:         fields[3],
			DeedBook:       fields[4],
			DeedPage:       fields[5],
		})
	}
	return slots, nil
}

func parcelPublished(ctx context.Context, query sqlQuery, container, databaseURL string) (bool, error) {
	raw, err := query(ctx, container, databaseURL, committedIncludingQuarantineSQL)
	if err != nil {
		return false, err
	}
	line := strings.TrimSpace(raw)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	n, err := strconv.Atoi(line)
	if err != nil {
		return false, fmt.Errorf("parcel publication count %q", strings.TrimSpace(raw))
	}
	return n > 0, nil
}

func readPostImport(ctx context.Context, query sqlQuery, container, databaseURL string, snap seedSnapshot) (postImport, error) {
	raw, err := query(ctx, container, databaseURL, postImportSQL)
	if err != nil {
		return postImport{}, err
	}
	counts := map[string]int{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, value, ok := strings.Cut(line, "\t")
		if !ok {
			return postImport{}, fmt.Errorf("post-import row %q", line)
		}
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return postImport{}, fmt.Errorf("post-import %s: %w", name, err)
		}
		counts[name] = n
	}
	post := postImport{
		SeedParcels:      snap.counts["parcels"],
		SeedSoilPolygons: snap.counts["soil_polygons"],
		SeedOverlaps:     snap.counts["soil_polygon_parcels"],
	}
	var missing error
	if post.CountyParcels, missing = requiredCount(counts, "county_parcels"); missing != nil {
		return postImport{}, missing
	}
	if post.SoilPolygons, missing = requiredCount(counts, "soil_polygons"); missing != nil {
		return postImport{}, missing
	}
	if post.Overlaps, missing = requiredCount(counts, "overlaps"); missing != nil {
		return postImport{}, missing
	}
	if post.ParcelJobSuccesses, missing = requiredCount(counts, "parcel_jobs"); missing != nil {
		return postImport{}, missing
	}
	if post.SoilJobSuccesses, missing = requiredCount(counts, "soil_jobs"); missing != nil {
		return postImport{}, missing
	}
	if post.ScreeningJobSuccesses, missing = requiredCount(counts, "screening_jobs"); missing != nil {
		return postImport{}, missing
	}
	if post.ScreeningHeads, missing = requiredCount(counts, "heads"); missing != nil {
		return postImport{}, missing
	}
	if post.Assessments, missing = requiredCount(counts, "assessments"); missing != nil {
		return postImport{}, missing
	}
	return post, nil
}
