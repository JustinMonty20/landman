package parcelevidence_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JustinMonty20/landman/internal/parcelevidence"
)

func TestSourceCanonicalContract(t *testing.T) {
	equivalent := [][2]string{
		{`{"attributes":{"OBJECTID":9007199254740993,"amount":1.00000000000000000001},"geometry":{"rings":[[[0,0],[0,1],[1,1],[1,0],[0,0]]]}}`, `{"geometry":{"rings":[[[1.0,1],[1,0],[0,0],[0,1],[1,1]]]},"attributes":{"amount":1.000000000000000000010,"OBJECTID":9007199254740993}}`},
		{`{"value":1e3}`, `{"value":1000.00}`},
		{`{"geometry":{"rings":[[[0,0],[0,1],[1,1],[1,0],[0,0]],[[2,0],[2,1],[3,1],[3,0],[2,0]]]}}`, `{"geometry":{"rings":[[[3,1],[3,0],[2,0],[2,1],[3,1]],[[1,1],[1,0],[0,0],[0,1],[1,1]]]}}`},
	}
	for _, pair := range equivalent {
		a, err := parcelevidence.SourceHash([]byte(pair[0]))
		if err != nil {
			t.Fatal(err)
		}
		b, err := parcelevidence.SourceHash([]byte(pair[1]))
		if err != nil {
			t.Fatal(err)
		}
		if a != b {
			t.Fatalf("equivalent source JSON differs: %q %q", pair[0], pair[1])
		}
	}
	distinct := []string{`{}`, `{"v":null}`, `{"v":0}`, `{"v":0.0000000000000000000000000000001}`, `{"v":0.0000000000000000000000000000002}`, `{"geometry":{"rings":[[[0,0],[0,1],[1.00000000001,1],[1,0],[0,0]]]}}`, `{"geometry":{"rings":[[[0,0],[0,1],[1,1],[1,0],[0,0]]]}}`}
	seen := map[string]bool{}
	for _, v := range distinct {
		h, err := parcelevidence.SourceHash([]byte(v))
		if err != nil {
			t.Fatal(err)
		}
		if seen[h] {
			t.Fatalf("lost source distinction: %s", v)
		}
		seen[h] = true
	}
}
func TestSavedEvidenceCorruptionIsRejected(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	snapshot, err := parcelevidence.New(dir, "full_scan")
	if err != nil {
		t.Fatal(err)
	}
	if err = snapshot.Save("http://local/?returnCountOnly=true", []byte(`{"count":1}`), 0); err != nil {
		t.Fatal(err)
	}
	if err = snapshot.Save("http://local/?resultOffset=0", []byte(`{"spatialReference":{"wkid":3857},"features":[{}]}`), 3857); err != nil {
		t.Fatal(err)
	}
	if err = snapshot.Complete(1); err != nil {
		t.Fatal(err)
	}
	if _, err = parcelevidence.Open(dir, snapshot.Manifest.ID); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, snapshot.Manifest.ID, "000001.json"), []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = parcelevidence.Open(dir, snapshot.Manifest.ID); err == nil {
		t.Fatal("corrupt source response accepted")
	}
}

func TestArcGISRingDirectionChangesSourceHash(t *testing.T) {
	a := []byte(`{"geometry":{"rings":[[[0,0],[0,4],[4,4],[4,0],[0,0]],[[1,1],[2,1],[2,2],[1,2],[1,1]]]}}`)
	b := []byte(`{"geometry":{"rings":[[[0,0],[0,4],[4,4],[4,0],[0,0]],[[1,1],[1,2],[2,2],[2,1],[1,1]]]}}`)
	first, err := parcelevidence.SourceHash(a)
	if err != nil {
		t.Fatal(err)
	}
	second, err := parcelevidence.SourceHash(b)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("shell/hole direction change was erased")
	}
}

func TestEvidenceRejectsPublicSymlinkAndOversizedFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	s, err := parcelevidence.New(dir, "full_scan")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Save("http://local/?returnCountOnly=true", []byte(`{"count":0}`), 0); err != nil {
		t.Fatal(err)
	}
	if err = s.Complete(0); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, s.Manifest.ID, "000000.json")
	if err = os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = parcelevidence.Open(dir, s.Manifest.ID); err == nil {
		t.Fatal("public response accepted")
	}
	if err = os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(filepath.Join(dir, s.Manifest.ID, "manifest.json"), path); err != nil {
		t.Fatal(err)
	}
	if _, err = parcelevidence.Open(dir, s.Manifest.ID); err == nil {
		t.Fatal("response symlink accepted")
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, make([]byte, 32<<20+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = parcelevidence.Open(dir, s.Manifest.ID); err == nil {
		t.Fatal("oversized response accepted")
	}
}
