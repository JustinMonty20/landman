package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var sampleParcelIDs = []string{"07102010", "07102051"}

const loadedSQL = `
SELECT 'count', 'parcels', count(*)::text, '-' FROM parcels
UNION ALL
SELECT 'count', 'vacant', (count(*) FILTER (WHERE likely_vacant))::text, '-' FROM parcels
UNION ALL
SELECT 'count', 'built', (count(*) FILTER (WHERE NOT likely_vacant))::text, '-' FROM parcels
UNION ALL
SELECT 'count', 'soil_map_units', count(*)::text, '-' FROM soil_map_units
UNION ALL
SELECT 'count', 'soil_polygons', count(*)::text, '-' FROM soil_polygons
UNION ALL
SELECT 'count', 'soil_polygon_parcels', count(*)::text, '-' FROM soil_polygon_parcels
UNION ALL
SELECT 'count', 'parcel_sales', count(*)::text, '-' FROM parcel_sales
UNION ALL
SELECT 'parcel', source_parcel_id,
       CASE WHEN likely_vacant THEN 't' ELSE 'f' END,
       coalesce(situs_line_1, '')
FROM parcels
WHERE source_parcel_id IN ('07102010', '07102051')`

type liveLoaded struct {
	Parcels  int
	Vacant   int
	Built    int
	MapUnits int
	Polygons int
	Overlaps int
	Sales    int
	Samples  map[string]liveParcel
}

type liveParcel struct {
	LikelyVacant bool
	Situs        string
}

// loadedReport prints the seed census and whether the live database matches it.
// On a miss, the loaded line names the table or the source_parcel_id.
func loadedReport(snap seedSnapshot, live liveLoaded) (string, int) {
	problem := loadedProblem(snap, live)
	text := censusText(snap)
	if problem == "" {
		return text + "loaded: ok\n", 0
	}
	return text + "loaded: " + problem + "\n", 1
}

func censusText(snap seedSnapshot) string {
	var b strings.Builder
	fmt.Fprintf(&b, "census parcels=%d vacant=%d built=%d soil_map_units=%d soil_polygons=%d soil_polygon_parcels=%d parcel_sales=%d\n",
		snap.counts["parcels"], snap.vacant, snap.built,
		snap.counts["soil_map_units"], snap.counts["soil_polygons"],
		snap.counts["soil_polygon_parcels"], snap.counts["parcel_sales"])
	for _, id := range sampleParcelIDs {
		parcel, ok := snap.parcel(id)
		if !ok {
			fmt.Fprintf(&b, "census parcel %s missing\n", id)
			continue
		}
		flag := "false"
		if parcel.LikelyVacant {
			flag = "true"
		}
		fmt.Fprintf(&b, "census parcel %s likely_vacant=%s situs=%s\n", id, flag, parcel.Situs)
	}
	return b.String()
}

func loadedProblem(snap seedSnapshot, live liveLoaded) string {
	checks := []struct {
		table string
		want  int
		got   int
	}{
		{"parcels", snap.counts["parcels"], live.Parcels},
		{"parcels", snap.vacant, live.Vacant},
		{"parcels", snap.built, live.Built},
		{"soil_map_units", snap.counts["soil_map_units"], live.MapUnits},
		{"soil_polygons", snap.counts["soil_polygons"], live.Polygons},
		{"soil_polygon_parcels", snap.counts["soil_polygon_parcels"], live.Overlaps},
		{"parcel_sales", snap.counts["parcel_sales"], live.Sales},
	}
	for _, check := range checks {
		if check.want != check.got {
			return check.table
		}
	}
	for _, id := range sampleParcelIDs {
		want, ok := snap.parcel(id)
		got, liveOK := live.Samples[id]
		if !ok || !liveOK || want.LikelyVacant != got.LikelyVacant || want.Situs != got.Situs {
			return id
		}
	}
	return ""
}

func parseLoaded(raw string) (liveLoaded, error) {
	live := liveLoaded{Samples: map[string]liveParcel{}}
	counts := map[string]int{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 4 {
			return liveLoaded{}, fmt.Errorf("loaded row %q", line)
		}
		switch fields[0] {
		case "count":
			n, err := strconv.Atoi(strings.TrimSpace(fields[2]))
			if err != nil {
				return liveLoaded{}, fmt.Errorf("loaded count %s: %w", fields[1], err)
			}
			counts[fields[1]] = n
		case "parcel":
			live.Samples[fields[1]] = liveParcel{
				LikelyVacant: fields[2] == "t" || fields[2] == "true",
				Situs:        fields[3],
			}
		default:
			return liveLoaded{}, fmt.Errorf("loaded row %q", line)
		}
	}
	var err error
	if live.Parcels, err = requiredCount(counts, "parcels"); err != nil {
		return liveLoaded{}, err
	}
	if live.Vacant, err = requiredCount(counts, "vacant"); err != nil {
		return liveLoaded{}, err
	}
	if live.Built, err = requiredCount(counts, "built"); err != nil {
		return liveLoaded{}, err
	}
	if live.MapUnits, err = requiredCount(counts, "soil_map_units"); err != nil {
		return liveLoaded{}, err
	}
	if live.Polygons, err = requiredCount(counts, "soil_polygons"); err != nil {
		return liveLoaded{}, err
	}
	if live.Overlaps, err = requiredCount(counts, "soil_polygon_parcels"); err != nil {
		return liveLoaded{}, err
	}
	if live.Sales, err = requiredCount(counts, "parcel_sales"); err != nil {
		return liveLoaded{}, err
	}
	return live, nil
}

func requiredCount(counts map[string]int, name string) (int, error) {
	n, ok := counts[name]
	if !ok {
		return 0, fmt.Errorf("loaded rows missing %s", name)
	}
	return n, nil
}

func checkLoaded(container, gooseURL, seedPath string, query sqlQuery) (string, int) {
	if err := refuseContainer(container); err != nil {
		fmt.Fprintf(os.Stderr, "check-loaded: %v\n", err)
		return "", 1
	}
	text, err := os.ReadFile(seedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-loaded: %v\n", err)
		return "", 1
	}
	snap, err := parseSeed(string(text))
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-loaded: %v\n", err)
		return "", 1
	}
	raw, err := query(context.Background(), container, gooseURL, loadedSQL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-loaded: %v\n", err)
		return "", 1
	}
	live, err := parseLoaded(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-loaded: %v\n", err)
		return "", 1
	}
	return loadedReport(snap, live)
}
