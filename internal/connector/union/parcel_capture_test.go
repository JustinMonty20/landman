package union_test

import (
	"github.com/JustinMonty20/landman/internal/connector/union"
	"testing"
	"time"
)

func TestParcelResponseCRSRequiresOriginalDeclaration(t *testing.T) {
	for _, input := range []string{`{"features":[]}`, `{"spatialReference":{"wkid":4326},"features":[]}`, `{"spatialReference":{"wkid":102100,"latestWkid":26917},"features":[]}`, `{"spatialReference":{"wkid":3857},"features":[{"geometry":{"spatialReference":{"wkid":4326}}}]}`} {
		if _, err := union.ParcelResponseCRS([]byte(input)); err == nil {
			t.Fatalf("unsupported CRS accepted: %s", input)
		}
	}
	for declared, want := range map[string]int{`{"spatialReference":{"wkid":3857},"features":[]}`: 3857, `{"spatialReference":{"wkid":102100,"latestWkid":3857},"features":[]}`: 102100} {
		got, err := union.ParcelResponseCRS([]byte(declared))
		if err != nil || got != want {
			t.Fatalf("declared CRS %d, expected %d: %v", got, want, err)
		}
	}
}

func TestParcelResponseCRSGeometryLatestAndIdentity(t *testing.T) {
	for _, text := range []string{
		`{"spatialReference":{"wkid":3857},"features":[{"geometry":{"spatialReference":{"latestWkid":3857.5}}}]}`,
		`{"spatialReference":{"wkid":3857},"features":[{"geometry":{"spatialReference":{"wkid":102100.5}}}]}`,
		`{"spatialReference":{"wkid":3857.1},"features":[]}`,
	} {
		if _, err := union.ParcelResponseCRS([]byte(text)); err == nil {
			t.Fatalf("invalid CRS accepted: %s", text)
		}
	}
	for _, number := range []string{"1", "1.0", "1e0"} {
		body := []byte(`{"spatialReference":{"wkid":3857,"latestWkid":102100},"features":[{"attributes":{"OBJECTID":` + number + `},"geometry":{"spatialReference":{"wkid":102100}}}]}`)
		records, err := union.DecodeParcelPage(body, time.Now(), "union_county_gis")
		if err != nil || len(records) != 1 || records[0].ParcelID != "1" {
			t.Fatalf("OBJECTID %s: %v %+v", number, err, records)
		}
	}
}
