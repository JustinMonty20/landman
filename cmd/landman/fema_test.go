package main

import "testing"

func TestManualFEMAInputRequiresAllPaths(t *testing.T) {
	values := map[string]string{
		"UNION_FEMA_ARTIFACT_DIR": "artifacts/fema",
		"UNION_FEMA_CATALOG_PATH": "artifacts/fema/catalog.json",
		"UNION_FEMA_ARCHIVE_PATH": "artifacts/fema/archive.zip",
	}
	getenv := func(name string) string { return values[name] }

	for _, name := range []string{"UNION_FEMA_ARTIFACT_DIR", "UNION_FEMA_CATALOG_PATH", "UNION_FEMA_ARCHIVE_PATH"} {
		values[name] = ""
		if _, err := manualFEMAInput(getenv); err == nil {
			t.Fatalf("manualFEMAInput accepted missing %s", name)
		}
		values[name] = map[string]string{
			"UNION_FEMA_ARTIFACT_DIR": "artifacts/fema",
			"UNION_FEMA_CATALOG_PATH": "artifacts/fema/catalog.json",
			"UNION_FEMA_ARCHIVE_PATH": "artifacts/fema/archive.zip",
		}[name]
	}

	input, err := manualFEMAInput(getenv)
	if err != nil {
		t.Fatal(err)
	}
	if input.artifactDir != values["UNION_FEMA_ARTIFACT_DIR"] || input.catalogPath != values["UNION_FEMA_CATALOG_PATH"] || input.archivePath != values["UNION_FEMA_ARCHIVE_PATH"] {
		t.Fatalf("manualFEMAInput = %+v", input)
	}
}
