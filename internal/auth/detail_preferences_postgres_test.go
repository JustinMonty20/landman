//go:build postgres

package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestParcelDetailFields(t *testing.T) {
	f := setup(t)
	first := f.login(t, nil)
	second := f.login(t, map[string]any{"sub": "second-subject", "email": "second@example.test"})
	endpoint := "/api/me/parcel-detail-fields"
	if r := f.request("GET", endpoint); r.Code != 401 {
		t.Fatalf("anonymous GET: %d", r.Code)
	}
	checkDefault := func(cookie *http.Cookie) {
		t.Helper()
		r := f.request("GET", endpoint, cookie)
		var response detailFieldsResponse
		if r.Code != 200 || json.Unmarshal(r.Body.Bytes(), &response) != nil || !slices.Equal(response.Fields, detailFieldOrder) {
			t.Fatalf("default: %d %s", r.Code, r.Body.String())
		}
	}
	checkDefault(first)
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
	for _, body := range []string{`{}`, `{"fields":null}`, `{"fields":["soil_septic","soil_septic"]}`, `{"fields":["secrets"]}`, `{"fields":[1]}`, `{"fields":[],"account_id":"other"}`, `{"fields":[]} {}`, strings.Repeat(" ", 513) + `{"fields":[]}`} {
		if r := put(first, body, testOrigin); r.Code != 400 {
			t.Errorf("accepted invalid body %q: %d", body, r.Code)
		}
	}
	if r := put(first, `{"fields":[]}`, "https://evil.example"); r.Code != 403 {
		t.Errorf("foreign origin: %d", r.Code)
	}
	if r := put(nil, `{"fields":[]}`, testOrigin); r.Code != 401 {
		t.Errorf("anonymous PUT: %d", r.Code)
	}
	if r := put(first, `{"fields":["soil_septic","mapped_acres","delinquent_total"]}`, testOrigin); r.Code != 200 || !strings.Contains(r.Body.String(), `"fields":["mapped_acres","delinquent_total","soil_septic"]`) {
		t.Fatalf("canonical PUT: %d %s", r.Code, r.Body.String())
	}
	checkDefault(second)
	if r := put(first, `{"fields":[]}`, testOrigin); r.Code != 200 || !strings.Contains(r.Body.String(), `"fields":[]`) {
		t.Fatalf("empty selection: %d %s", r.Code, r.Body.String())
	}
	if r := f.request("GET", endpoint, first); !strings.Contains(r.Body.String(), `"fields":[]`) {
		t.Fatalf("empty was not persisted: %s", r.Body.String())
	}
}

func TestCurrentParcelOwnerRead(t *testing.T) {
	f := setup(t)
	cookie := f.login(t, nil)
	var id string
	err := f.pool.QueryRow(t.Context(), `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,current_name_1,current_name_2,mailing_line_1) VALUES('union','37179','union_county_gis','owner-fixture','County One','County Two','Private mailing line') RETURNING id::text`).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.pool.Exec(context.Background(), `DELETE FROM parcels WHERE id=$1`, id) })
	endpoint := "/api/me/parcel-owner/" + id
	if r := f.request("GET", endpoint); r.Code != 401 {
		t.Fatalf("anonymous owner read: %d", r.Code)
	}
	if r := f.request("GET", "/api/me/parcel-owner/not-an-id", cookie); r.Code != 400 {
		t.Fatalf("invalid ID: %d", r.Code)
	}
	if r := f.request("GET", "/api/me/parcel-owner/00000000-0000-4000-8000-000000000000", cookie); r.Code != 404 {
		t.Fatalf("missing parcel: %d", r.Code)
	}
	r := f.request("GET", endpoint, cookie)
	if r.Code != 200 || !strings.Contains(r.Body.String(), `"current_name_1":"County One"`) || !strings.Contains(r.Body.String(), `"current_name_2":"County Two"`) || strings.Contains(r.Body.String(), "Private mailing line") {
		t.Fatalf("owner response %d %s", r.Code, r.Body.String())
	}
}
