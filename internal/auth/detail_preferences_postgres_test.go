//go:build postgres

package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParcelDetailGroups(t *testing.T) {
	f := setup(t)
	first := f.login(t, nil)
	second := f.login(t, map[string]any{"sub": "second-subject", "email": "second@example.test"})
	endpoint := "/api/me/parcel-detail-groups"
	if r := f.request("GET", endpoint); r.Code != 401 {
		t.Fatalf("anonymous GET: %d", r.Code)
	}
	if r := f.request("GET", endpoint, first); r.Code != 200 || !strings.Contains(r.Body.String(), `"groups":["acreage","soil"]`) {
		t.Fatalf("default: %d %s", r.Code, r.Body.String())
	}
	put := func(cookie *http.Cookie, body, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("PUT", testOrigin+endpoint, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		return w
	}
	for _, body := range []string{`{}`, `{"groups":null}`, `{"groups":["soil","soil"]}`, `{"groups":["secrets"]}`, `{"groups":[1]}`, `{"groups":[],"account_id":"other"}`, `{"groups":[]} {}`, strings.Repeat(" ", 513) + `{"groups":[]}`} {
		if r := put(first, body, testOrigin); r.Code != 400 {
			t.Errorf("accepted invalid body %q: %d", body, r.Code)
		}
	}
	if r := put(first, `{"groups":[]}`, "https://evil.example"); r.Code != 403 {
		t.Errorf("foreign origin: %d", r.Code)
	}
	if r := put(nil, `{"groups":[]}`, testOrigin); r.Code != 401 {
		t.Errorf("anonymous PUT: %d", r.Code)
	}
	if r := put(first, `{"groups":["soil","acreage","tax"]}`, testOrigin); r.Code != 200 || !strings.Contains(r.Body.String(), `"groups":["acreage","tax","soil"]`) {
		t.Fatalf("canonical PUT: %d %s", r.Code, r.Body.String())
	}
	if r := f.request("GET", endpoint, second); !strings.Contains(r.Body.String(), `"groups":["acreage","soil"]`) {
		t.Fatalf("second account changed: %s", r.Body.String())
	}
	if r := put(first, `{"groups":[]}`, testOrigin); r.Code != 200 || !strings.Contains(r.Body.String(), `"groups":[]`) {
		t.Fatalf("empty selection: %d %s", r.Code, r.Body.String())
	}
	if r := f.request("GET", endpoint, first); !strings.Contains(r.Body.String(), `"groups":[]`) {
		t.Fatalf("empty was not persisted: %s", r.Body.String())
	}
}
