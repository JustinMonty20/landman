package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestGISRunnerPreservesSourceExit(t *testing.T) {
	script, err := filepath.Abs(filepath.Join("..", "..", "run-gis.sh"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name         string
		soil, parcel int
		want         int
	}{
		{"soil fails", 7, 0, 7},
		{"parcel fails", 0, 9, 9},
		{"both succeed", 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			binary := filepath.Join(dir, "landman")
			stub := "#!/usr/bin/env bash\nprintf '%s\\n' \"$*\"\nprintf '%s\\n' \"$*\" >&2\ncase \"$2\" in\nunion.gis.soil) exit \"$SOIL_STATUS\" ;;\nunion.gis.parcels) exit \"$PARCEL_STATUS\" ;;\n*) exit 99 ;;\nesac\n"
			if err := os.WriteFile(binary, []byte(stub), 0700); err != nil {
				t.Fatal(err)
			}
			evidence := filepath.Join(dir, "evidence")
			cmd := exec.Command("bash", script)
			cmd.Env = append(os.Environ(), "HOST_DATABASE_URL=unused-test-url", "EVIDENCE="+evidence,
				"VERIFY_LANDMAN_BINARY="+binary, "SOIL_STATUS="+strconv.Itoa(tc.soil), "PARCEL_STATUS="+strconv.Itoa(tc.parcel))
			output, runErr := cmd.CombinedOutput()
			status := 0
			if runErr != nil {
				exit, ok := runErr.(*exec.ExitError)
				if !ok {
					t.Fatal(runErr)
				}
				status = exit.ExitCode()
			}
			if status != tc.want {
				t.Fatalf("exit %d, want %d: %s", status, tc.want, output)
			}
			for _, file := range []string{"import.exit.txt", "soil.exit.txt"} {
				want := tc.want
				if file == "soil.exit.txt" {
					want = tc.soil
				}
				body, err := os.ReadFile(filepath.Join(evidence, file))
				if err != nil || strings.TrimSpace(string(body)) != strconv.Itoa(want) {
					t.Fatalf("%s = %q, %v; want %d", file, body, err, want)
				}
			}
			stdout, err := os.ReadFile(filepath.Join(evidence, "import.stdout.txt"))
			if err != nil {
				t.Fatal(err)
			}
			want := "run union.gis.soil\n"
			if tc.soil == 0 {
				want += "run union.gis.parcels\n"
			} else if _, err := os.Stat(filepath.Join(evidence, "parcels.exit.txt")); !os.IsNotExist(err) {
				t.Fatalf("parcels ran after soil failure: %v", err)
			}
			if string(stdout) != want {
				t.Fatalf("source sequence %q, want %q", stdout, want)
			}
			stderr, err := os.ReadFile(filepath.Join(evidence, "import.stderr.txt"))
			if err != nil || string(stderr) != want {
				t.Fatalf("stderr %q, %v; want %q", stderr, err, want)
			}
		})
	}
}
