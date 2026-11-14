package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSchemaNamesSaleDateTypoWhenColumnIsNotLive(t *testing.T) {
	dir := t.TempDir()
	seedPath := filepath.Join(dir, "seed.sql")
	seed := "" +
		"INSERT INTO parcels (id, source_parcel_id, situs_line_1, likely_vacant) VALUES ('11111111-1111-1111-1111-111111111111'::uuid, '07102010', '105 CUPPED OAK DR', 't');\n" +
		"INSERT INTO parcel_sales (parcel_id, sequence, sale_date_typo) SELECT id, 1, NULL FROM parcels WHERE source_parcel_id = '07102010';\n"
	if err := os.WriteFile(seedPath, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	called := false
	text, code := checkSchema(verifyContainerName, "postgres://landman/landman", seedPath, func(ctx context.Context, container, databaseURL, sql string) (string, error) {
		called = true
		if container != verifyContainerName {
			t.Fatalf("container %s", container)
		}
		if !strings.Contains(sql, "information_schema.columns") {
			t.Fatalf("catalog SQL %s", sql)
		}
		return strings.Join([]string{
			"parcels\tid\tNO\tgen_random_uuid()",
			"parcels\tsource_parcel_id\tNO\t",
			"parcels\tsitus_line_1\tYES\t",
			"parcels\tlikely_vacant\tNO\tfalse",
			"parcel_sales\tparcel_id\tNO\t",
			"parcel_sales\tsequence\tNO\t",
			"parcel_sales\tsale_date\tYES\t",
			"",
		}, "\n"), nil
	})
	if !called {
		t.Fatal("catalog was not read")
	}
	if code != 1 {
		t.Fatalf("exit %d, text %s", code, text)
	}
	if !strings.Contains(text, "schema: parcel_sales.sale_date_typo missing") {
		t.Fatalf("report %q", text)
	}
	if strings.Contains(text, "schema: ok") {
		t.Fatalf("report %q", text)
	}
}

func TestSchemaNamesOmittedNotNullColumn(t *testing.T) {
	dir := t.TempDir()
	seedPath := filepath.Join(dir, "seed.sql")
	seed := "" +
		"INSERT INTO parcels (id, source_parcel_id, situs_line_1, likely_vacant) VALUES ('11111111-1111-1111-1111-111111111111'::uuid, '07102010', '105 CUPPED OAK DR', 't');\n" +
		"INSERT INTO parcel_sales (parcel_id, sequence, sale_date) SELECT id, 1, NULL FROM parcels WHERE source_parcel_id = '07102010';\n"
	if err := os.WriteFile(seedPath, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	text, code := checkSchema(verifyContainerName, "postgres://landman/landman", seedPath, func(ctx context.Context, container, databaseURL, sql string) (string, error) {
		return strings.Join([]string{
			"parcels\tid\tNO\tgen_random_uuid()",
			"parcels\tsource_parcel_id\tNO\t",
			"parcels\tsitus_line_1\tYES\t",
			"parcels\tlikely_vacant\tNO\tfalse",
			"parcel_sales\tparcel_id\tNO\t",
			"parcel_sales\tsequence\tNO\t",
			"parcel_sales\tsale_date\tYES\t",
			"parcel_sales\tid\tNO\tgen_random_uuid()",
			"parcel_sales\tdeed_book\tNO\t",
			"",
		}, "\n"), nil
	})
	if code != 1 || !strings.Contains(text, "schema: parcel_sales.deed_book not null") {
		t.Fatalf("exit %d report %q", code, text)
	}
	if strings.Contains(text, "parcel_sales.id") {
		t.Fatalf("defaulted id was reported %q", text)
	}
}

func TestCheckSchemaRefusesOtherContainerBeforeQuery(t *testing.T) {
	called := false
	text, code := checkSchema("landman-postgis", "postgres://landman/landman", "seed.sql", func(ctx context.Context, container, databaseURL, sql string) (string, error) {
		called = true
		return "", nil
	})
	if called {
		t.Fatal("query ran for a non-verify container")
	}
	if code == 0 || text != "" {
		t.Fatalf("exit %d text %q", code, text)
	}
}

func TestLoadedNamesParcelSalesWhenCountDiffers(t *testing.T) {
	snap := fixtureSeed(t)
	live := matchingLive(snap)
	live.Sales = snap.counts["parcel_sales"] + 1
	text, code := loadedReport(snap, live)
	if code != 1 {
		t.Fatalf("exit %d text %s", code, text)
	}
	if !strings.Contains(text, "loaded: parcel_sales") {
		t.Fatalf("report %q", text)
	}
	if !strings.Contains(text, "census parcels=") {
		t.Fatalf("missing census %q", text)
	}
}

func TestLoadedNamesParcelWhenSitusDiffers(t *testing.T) {
	snap := fixtureSeed(t)
	live := matchingLive(snap)
	sample := live.Samples["07102010"]
	sample.Situs = "1 MAIN ST"
	live.Samples["07102010"] = sample
	text, code := loadedReport(snap, live)
	if code != 1 || !strings.Contains(text, "loaded: 07102010") {
		t.Fatalf("exit %d report %q", code, text)
	}
}

func TestLoadedOkWhenLiveMatchesSeed(t *testing.T) {
	snap := fixtureSeed(t)
	text, code := loadedReport(snap, matchingLive(snap))
	if code != 0 || !strings.Contains(text, "loaded: ok") {
		t.Fatalf("exit %d report %q", code, text)
	}
	if !strings.Contains(text, "census parcel 07102010 likely_vacant=true situs=105 CUPPED OAK DR") {
		t.Fatalf("report %q", text)
	}
	if !strings.Contains(text, "census parcel 07102051 likely_vacant=false situs=203 WHITE OAK LN") {
		t.Fatalf("report %q", text)
	}
}

func TestParseLoadedReadsCountsAndSamples(t *testing.T) {
	raw := strings.Join([]string{
		"count\tparcels\t2\t-",
		"count\tvacant\t1\t-",
		"count\tbuilt\t1\t-",
		"count\tsoil_map_units\t1\t-",
		"count\tsoil_polygons\t1\t-",
		"count\tsoil_polygon_parcels\t1\t-",
		"count\tparcel_sales\t1\t-",
		"parcel\t07102010\tt\t105 CUPPED OAK DR",
		"parcel\t07102051\tf\t203 WHITE OAK LN",
	}, "\n")
	live, err := parseLoaded(raw)
	if err != nil {
		t.Fatal(err)
	}
	if live.Parcels != 2 || live.Vacant != 1 || live.Sales != 1 {
		t.Fatalf("%#v", live)
	}
	if !live.Samples["07102010"].LikelyVacant || live.Samples["07102010"].Situs != "105 CUPPED OAK DR" {
		t.Fatalf("%#v", live.Samples)
	}
	if live.Samples["07102051"].LikelyVacant {
		t.Fatalf("%#v", live.Samples["07102051"])
	}
}

func TestRealSeedParsesSampleParcelsAndSaleColumns(t *testing.T) {
	text, err := os.ReadFile("../../seed.sql")
	if err != nil {
		t.Fatal(err)
	}
	snap, err := parseSeed(string(text))
	if err != nil {
		t.Fatal(err)
	}
	vacant, ok := snap.parcel("07102010")
	if !ok || !vacant.LikelyVacant || vacant.Situs != "105 CUPPED OAK DR" {
		t.Fatalf("07102010 %#v", vacant)
	}
	built, ok := snap.parcel("07102051")
	if !ok || built.LikelyVacant || built.Situs != "203 WHITE OAK LN" {
		t.Fatalf("07102051 %#v", built)
	}
	for _, column := range []string{"parcel_id", "sequence", "sale_date", "amount", "deed_book", "deed_page"} {
		if !hasColumn(snap, "parcel_sales", column) {
			t.Fatalf("parcel_sales columns %v", snap.columns["parcel_sales"])
		}
	}
	if hasColumn(snap, "parcel_sales", "sale_date_typo") {
		t.Fatal("parser invented sale_date_typo")
	}
	unit, ok := snap.mapUnits["115330"]
	if !ok || !unit.HasName || !strings.Contains(unit.Name, "Appling") || unit.AreaName != "Union County, North Carolina" {
		t.Fatalf("map unit %#v", unit)
	}
	if snap.counts["parcels"] != len(snap.parcels) || snap.vacant+snap.built != snap.counts["parcels"] {
		t.Fatalf("counts %#v vacant %d built %d parcels %d", snap.counts, snap.vacant, snap.built, len(snap.parcels))
	}
	inserts := strings.Count(string(text), "INSERT INTO parcel_sales ")
	if snap.counts["parcel_sales"] != inserts {
		t.Fatalf("sales %d, insert lines %d", snap.counts["parcel_sales"], inserts)
	}
}

func fixtureSeed(t *testing.T) seedSnapshot {
	t.Helper()
	text := "" +
		"INSERT INTO parcels (id, source_parcel_id, situs_line_1, likely_vacant) VALUES ('11111111-1111-1111-1111-111111111111'::uuid, '07102010', '105 CUPPED OAK DR', 't');\n" +
		"INSERT INTO parcels (id, source_parcel_id, situs_line_1, likely_vacant) VALUES ('22222222-2222-2222-2222-222222222222'::uuid, '07102051', '203 WHITE OAK LN', 'f');\n" +
		"INSERT INTO soil_map_units (map_unit_key, map_unit_name, area_name) VALUES ('115330', 'Appling sandy loam, 2 to 8 percent slopes', 'Union County, North Carolina');\n" +
		"INSERT INTO soil_polygons (id, object_id) VALUES ('33333333-3333-3333-3333-333333333333'::uuid, '1');\n" +
		"INSERT INTO soil_polygon_parcels (id, overlap_acres) VALUES ('44444444-4444-4444-4444-444444444444'::uuid, 0.25);\n" +
		"INSERT INTO parcel_sales (parcel_id, sequence, sale_date, amount, deed_book, deed_page) SELECT id, 1, DATE '2026-09-30', 0.0, '9472', '0680' FROM parcels WHERE source_parcel_id = '07102010';\n"
	snap, err := parseSeed(text)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func matchingLive(snap seedSnapshot) liveLoaded {
	live := liveLoaded{
		Parcels:  snap.counts["parcels"],
		Vacant:   snap.vacant,
		Built:    snap.built,
		MapUnits: snap.counts["soil_map_units"],
		Polygons: snap.counts["soil_polygons"],
		Overlaps: snap.counts["soil_polygon_parcels"],
		Sales:    snap.counts["parcel_sales"],
		Samples:  map[string]liveParcel{},
	}
	for _, id := range sampleParcelIDs {
		parcel, _ := snap.parcel(id)
		live.Samples[id] = liveParcel{LikelyVacant: parcel.LikelyVacant, Situs: parcel.Situs}
	}
	return live
}

func hasColumn(snap seedSnapshot, table, column string) bool {
	for _, name := range snap.columns[table] {
		if name == column {
			return true
		}
	}
	return false
}
