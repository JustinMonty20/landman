package parcelevidence_test

import (
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
