//go:build postgres

package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAuthenticatedParcelTaskRoute(t *testing.T) {
	f := setup(t)
	buyer := f.login(t, nil)
	other := f.login(t, map[string]any{"sub": "second-subject", "email": "second@example.test"})
	var parcel string
	if err := f.pool.QueryRow(t.Context(), `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id) VALUES('union','37179','union_county_gis',$1) RETURNING id::text`, fmt.Sprintf("task-route-%d", time.Now().UnixNano())).Scan(&parcel); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM user_parcels WHERE parcel_id=$1`, parcel)
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM parcels WHERE id=$1`, parcel)
	})
	path := "/api/me/parcels/" + parcel + "/tasks"
	if got := f.request("GET", path).Code; got != http.StatusUnauthorized {
		t.Fatalf("anonymous task route: %d", got)
	}
	if got := f.request("GET", path, buyer).Code; got != http.StatusOK {
		t.Fatalf("authenticated task route: %d", got)
	}
	body := fmt.Sprintf(`{"id":"644ea46d-e02a-4395-baff-f35d54d2813b","message":"Private fixture","due_at":%q,"timezone":"UTC"}`, time.Now().Add(time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339))
	create := func(origin string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", testOrigin+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		return w
	}
	if got := create("https://evil.example", buyer).Code; got != http.StatusForbidden {
		t.Fatalf("cross-origin task create: %d", got)
	}
	if got := create(testOrigin, buyer).Code; got != http.StatusOK {
		t.Fatalf("authenticated nonfavorite task create: %d", got)
	}
	if got := f.request("GET", path, other); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"items":[]`) {
		t.Fatalf("other account task list: %d %s", got.Code, got.Body.String())
	}
	if got := f.request("GET", path+"/clear", buyer); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"count":1`) {
		t.Fatalf("clear confirmation route: %d %s", got.Code, got.Body.String())
	}
	if got := f.request("GET", "/api/me/favorites/"+parcel+"/tasks", buyer).Code; got != http.StatusNotFound {
		t.Fatalf("retired task route: %d", got)
	}
}
