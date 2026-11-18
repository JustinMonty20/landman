package union

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/ewkb"
)

func fixture(t *testing.T, id int, kind string) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join("testdata", "qualification", fmt.Sprintf("%d-%s.json", id, kind)))
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func TestCapturedQualificationFields(t *testing.T) {
	for _, spec := range zoningLayers {
		t.Run(strconv.Itoa(spec.id), func(t *testing.T) {
			var sample struct {
				Features         []json.RawMessage `json:"features"`
				SpatialReference struct {
					LatestWKID int `json:"latestWkid"`
				} `json:"spatialReference"`
			}
			if e := json.Unmarshal(fixture(t, spec.id, "sample"), &sample); e != nil {
				t.Fatal(e)
			}
			if sample.SpatialReference.LatestWKID != 3857 || len(sample.Features) == 0 {
				t.Fatal("captured response missing geometry or SRID")
			}
			for _, raw := range sample.Features {
				f, e := parseQualificationFeature(spec, raw, 102100)
				if e != nil {
					t.Fatal(e)
				}
				var shape []byte
				switch x := f.(type) {
				case ZoneFeature:
					shape = x.Shape
					if x.Jurisdiction != spec.name {
						t.Fatalf("jurisdiction=%q", x.Jurisdiction)
					}
					if spec.id == 176 || spec.id == 183 {
						if x.DistrictCode != "" || x.Description == "" {
							t.Fatalf("municipal sample code=%q description=%q", x.DistrictCode, x.Description)
						}
					}
				case JurisdictionFeature:
					shape = x.Shape
					if x.Name == "" {
						t.Fatal("missing captured jurisdiction name")
					}
				case BoundaryFeature:
					shape = x.Shape
				case RoadFeature:
					shape = x.Shape
					if x.Name == "" || x.Class != "Secondary" || x.Status != "Active" || x.SourceDate == nil {
						t.Fatalf("road mapping: %+v", x)
					}
				default:
					t.Fatalf("unknown feature %T", f)
				}
				g, e := ewkb.Unmarshal(shape)
				if e != nil || g.SRID() != 3857 {
					t.Fatalf("geometry %T: %v", g, e)
				}
				if spec.kind == QualificationRoads {
					if _, ok := g.(*geom.MultiLineString); !ok {
						t.Fatalf("lost multi-line geometry: %T", g)
					}
				}
			}
		})
	}
}

func qualificationServer(t *testing.T, id int, handler func(*http.Request) (int, string)) (*GISSource, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, body := handler(r)
		w.WriteHeader(status)
		fmt.Fprint(w, body)
	}))
	cfg := DefaultGISSourceConfig()
	cfg.BaseURL = server.URL + "/server/rest/services/OperationalLayers/MapServer/215/query"
	cfg.RateLimit = 1000
	src, e := NewGISSource(cfg)
	if e != nil {
		server.Close()
		t.Fatal(e)
	}
	_ = id
	return src, server.Close
}
func fixtureHandler(t *testing.T, id int, mutate func(*http.Request, int, string) (int, string)) func(*http.Request) (int, string) {
	t.Helper()
	meta := string(fixture(t, id, "metadata"))
	sample := string(fixture(t, id, "sample"))
	var data struct {
		Features []json.RawMessage `json:"features"`
	}
	if e := json.Unmarshal([]byte(sample), &data); e != nil {
		t.Fatal(e)
	}
	var calls atomic.Int32
	return func(r *http.Request) (int, string) {
		n := int(calls.Add(1))
		var body string
		if r.URL.Path == fmt.Sprintf("/server/rest/services/OperationalLayers/MapServer/%d", id) {
			body = meta
		} else if r.URL.Query().Get("returnCountOnly") == "true" {
			body = `{"count":2}`
		} else {
			off, _ := strconv.Atoi(r.URL.Query().Get("resultOffset"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("resultRecordCount"))
			end := off + limit
			if end > len(data.Features) {
				end = len(data.Features)
			}
			if off > end {
				off = end
			}
			page := map[string]any{"features": data.Features[off:end], "spatialReference": map[string]int{"latestWkid": 3857}}
			b, _ := json.Marshal(page)
			body = string(b)
		}
		if mutate != nil {
			return mutate(r, n, body)
		}
		return http.StatusOK, body
	}
}
func TestQualificationPointTouchHoleAndUnknownName(t *testing.T) {
	for _, tc := range []struct {
		name, rings string
		valid       bool
	}{
		{"point-touch", `[[[0,0],[0,10],[10,10],[10,0],[0,0]],[[0,0],[2,1],[1,2.5],[0,0]]]`, true},
		{"edge-cross", `[[[0,0],[0,10],[10,10],[10,0],[0,0]],[[2,2],[12,2],[12,4],[2,4],[2,2]]]`, false},
		{"concave-cross", `[[[0,0],[0,10],[3,10],[3,3],[7,3],[7,10],[10,10],[10,0],[0,0]],[[2,2],[8,2],[8,5],[2,5],[2,2]]]`, false},
		{"exterior-hole", `[[[0,0],[0,10],[10,10],[10,0],[0,0]],[[20,20],[30,20],[30,30],[20,30],[20,20]]]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec, _ := layerByID(195)
			raw := json.RawMessage(fmt.Sprintf(`{"attributes":{"OBJECTID":1,"NAME":null,"LAST_UPDATE":null},"geometry":{"rings":%s}}`, tc.rings))
			feature, err := parseQualificationFeature(spec, raw, 3857)
			if !tc.valid {
				if err == nil {
					t.Fatalf("accepted invalid hole %T", feature)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			jurisdiction := feature.(JurisdictionFeature)
			if jurisdiction.Name != "" {
				t.Fatalf("null NAME converted to authority %q", jurisdiction.Name)
			}
			shape, err := ewkb.Unmarshal(jurisdiction.Shape)
			if err != nil {
				t.Fatal(err)
			}
			polygon, ok := shape.(*geom.Polygon)
			if !ok || polygon.NumLinearRings() != 2 || math.Abs(polygon.Area()) != 98 {
				t.Fatalf("point-touch hole lost: %T rings=%d area=%g", shape, polygon.NumLinearRings(), polygon.Area())
			}
		})
	}
}

func TestQualificationRejectsUncontainedHole(t *testing.T) {
	spec, err := layerByID(221)
	if err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"attributes":{"OBJECTID":1},"geometry":{"rings":[[[0,0],[0,10],[10,10],[10,0],[0,0]],[[20,20],[30,20],[30,30],[20,30],[20,20]]]}}`)
	if _, err := parseQualificationFeature(spec, raw, 102100); err == nil {
		t.Fatal("uncontained counterclockwise ring was silently converted into a shell")
	}
}

func TestQualificationPagingAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*http.Request, int, string) (int, string)
		want   string
	}{
		{"valid", nil, ""},
		{"count drift", func(r *http.Request, n int, b string) (int, string) {
			if n == 4 {
				return 200, `{"count":3}`
			}
			return 200, b
		}, "count drift"},
		{"transfer truncation retry", func(r *http.Request, n int, b string) (int, string) {
			if n == 3 {
				return 200, `{"features":[],"exceededTransferLimit":true,"spatialReference":{"latestWkid":3857}}`
			}
			return 200, b
		}, ""},
		{"short page", func(r *http.Request, n int, b string) (int, string) {
			if n == 3 {
				return 200, `{"features":[],"spatialReference":{"latestWkid":3857}}`
			}
			return 200, b
		}, "page count mismatch"},
		{"duplicate IDs", func(r *http.Request, n int, b string) (int, string) {
			if n == 3 {
				var v struct {
					Features []json.RawMessage `json:"features"`
				}
				json.Unmarshal([]byte(b), &v)
				return 200, fmt.Sprintf(`{"features":[%s,%s],"spatialReference":{"latestWkid":3857}}`, v.Features[0], v.Features[0])
			}
			return 200, b
		}, "repeats source ID"},
		{"ArcGIS 200 error", func(r *http.Request, n int, b string) (int, string) {
			if n == 3 {
				return 200, `{"error":{"code":400,"message":"denied"}}`
			}
			return 200, b
		}, "ArcGIS"},
		{"schema drift", func(r *http.Request, n int, b string) (int, string) {
			if n == 1 {
				return 200, strings.Replace(b, `"esriFieldTypeOID"`, `"esriFieldTypeString"`, 1)
			}
			return 200, b
		}, "missing typed field"},
		{"missing geometry", func(r *http.Request, n int, b string) (int, string) {
			if n == 3 {
				return 200, strings.Replace(b, `"rings"`, `"missingRings"`, 1)
			}
			return 200, b
		}, "invalid polygon rings"},
		{"transfer limit", func(r *http.Request, n int, b string) (int, string) {
			if n == 3 {
				return 200, strings.TrimSuffix(b, "}") + `,"exceededTransferLimit":true}`
			}
			return 200, b
		}, "exceeds transfer limit"},
		{"output SRID drift", func(r *http.Request, n int, b string) (int, string) {
			if n == 3 {
				return 200, strings.Replace(b, `"latestWkid":3857`, `"latestWkid":4326`, 1)
			}
			return 200, b
		}, "unexpected output SRID"},
		{"access denial", func(r *http.Request, n int, b string) (int, string) { return 403, "denied" }, "403"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src, close := qualificationServer(t, 190, fixtureHandler(t, 190, tc.mutate))
			defer close()
			count, e := src.FetchQualificationLayer(context.Background(), 190, func(QualificationFeature) error { return nil })
			if tc.want == "" {
				if e != nil || count.Count != 2 {
					t.Fatalf("count=%d error=%v", count.Count, e)
				}
			} else if e == nil || !strings.Contains(e.Error(), tc.want) {
				t.Fatalf("error=%v, want %s", e, tc.want)
			}
		})
	}
}
func TestQualificationCancellationAndRetry(t *testing.T) {
	t.Run("cancel", func(t *testing.T) {
		src, close := qualificationServer(t, 170, fixtureHandler(t, 170, func(r *http.Request, n int, b string) (int, string) {
			if n == 3 {
				time.Sleep(60 * time.Millisecond)
			}
			return 200, b
		}))
		defer close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		_, e := src.FetchQualificationLayer(ctx, 170, func(QualificationFeature) error { return nil })
		if e == nil || ctx.Err() == nil {
			t.Fatalf("error=%v", e)
		}
	})
	t.Run("retry transient", func(t *testing.T) {
		var attempts atomic.Int32
		src, close := qualificationServer(t, 170, fixtureHandler(t, 170, func(r *http.Request, n int, b string) (int, string) {
			if n == 1 {
				attempts.Add(1)
				return 502, "unavailable"
			}
			if n == 2 {
				attempts.Add(1)
			}
			return 200, b
		}))
		defer close()
		count, e := src.FetchQualificationLayer(context.Background(), 170, func(QualificationFeature) error { return nil })
		if e != nil || count.Count != 2 || attempts.Load() != 2 {
			t.Fatalf("count=%d attempts=%d err=%v", count.Count, attempts.Load(), e)
		}
	})
}
