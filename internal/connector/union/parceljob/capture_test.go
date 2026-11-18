package parceljob

import (
	"github.com/JustinMonty20/landman/internal/parcelevidence"
	"testing"
)

func TestVerifyParcelResponsesRequiresEveryMultipartPage(t *testing.T) {
	bodies := [][]byte{
		[]byte(`{"count":2}`),
		[]byte(`{"spatialReference":{"wkid":3857},"features":[{"attributes":{"OBJECTID":1,"PID":"A"},"geometry":{"rings":[]}}]}`),
		[]byte(`{"spatialReference":{"wkid":3857},"features":[{"attributes":{"OBJECTID":2,"PID":"A"},"geometry":{"rings":[]}}]}`),
	}
	manifest := parcelevidence.Manifest{Kind: "full_scan", Count: 2, Receipts: []parcelevidence.Receipt{
		{Request: "https://example.invalid/query?returnCountOnly=true"},
		{Request: "https://example.invalid/query?resultOffset=0&resultRecordCount=1", DeclaredCRS: 3857},
		{Request: "https://example.invalid/query?resultOffset=1&resultRecordCount=1", DeclaredCRS: 3857},
	}}
	read := func(i int) ([]byte, error) { return bodies[i], nil }
	if err := verifyParcelResponses(manifest, read); err != nil {
		t.Fatal(err)
	}
	manifest.Receipts = manifest.Receipts[:2]
	if err := verifyParcelResponses(manifest, read); err == nil {
		t.Fatal("accepted incomplete multipart capture")
	}
}
