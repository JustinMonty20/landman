//go:build postgres

package parcelcrm

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/JustinMonty20/landman/internal/parcelapi"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JustinMonty20/landman/internal/models"
	"github.com/JustinMonty20/landman/internal/parcelstore"
	"github.com/jackc/pgx/v5"
)

func TestScratchpadIsolationRetryAndRefresh(t *testing.T) {
	s, a, b, parcel := fixture(t)
	ctx := context.Background()
	id := "22222222-2222-4222-8222-222222222222"
	n, e := s.createNote(ctx, a, parcel, id, "plain <script>alert(1)</script>\nentry")
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.createNote(ctx, a, parcel, id, n.Body)
	if e != nil || again.Revision != n.Revision || !again.CreatedAt.Equal(n.CreatedAt) {
		t.Fatalf("retry %+v %v", again, e)
	}
	if _, e = s.createNote(ctx, a, parcel, id, "different"); e != errConflict {
		t.Fatalf("changed retry: %v", e)
	}
	if _, e = s.createNote(ctx, b, parcel, id, n.Body); e != pgx.ErrNoRows {
		t.Fatalf("foreign token: %v", e)
	}
	if page, e := s.notes(ctx, b, parcel, "", 20); e != nil || len(page.Items) != 0 {
		t.Fatalf("foreign page: %+v %v", page, e)
	}
	if _, e = s.editNote(ctx, b, parcel, id, "guess", n.Revision); e != pgx.ErrNoRows {
		t.Fatalf("foreign edit: %v", e)
	}
	if e = s.deleteNote(ctx, b, parcel, id, n.Revision); e != pgx.ErrNoRows {
		t.Fatalf("foreign delete: %v", e)
	}
	n, e = s.editNote(ctx, a, parcel, id, "edited", n.Revision)
	if e != nil || n.Revision != 2 || n.UpdatedAt.Before(n.CreatedAt) {
		t.Fatalf("edit: %+v %v", n, e)
	}
	if _, e = s.editNote(ctx, a, parcel, id, "stale", 1); e != errConflict {
		t.Fatalf("stale edit: %v", e)
	}
	if e = s.deleteNote(ctx, a, parcel, id, 1); e != errConflict {
		t.Fatalf("stale delete: %v", e)
	}
	// The canonical parcel upsert retains the shared ID and account-owned note.
	var source string
	if e = s.DB.QueryRow(ctx, `SELECT source_parcel_id FROM parcels WHERE id=$1`, parcel).Scan(&source); e != nil {
		t.Fatal(e)
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	refreshNumber := "refresh-number"
	refreshed, e := parcelstore.SaveParcel(ctx, tx, models.GISParcel{Identity: models.ParcelIdentity{CountyID: "union", CountyCode: "union", SourceName: "union_county_gis", SourceParcelID: source, ParcelNumber: &refreshNumber}})
	if e != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil || string(refreshed) != parcel {
		t.Fatalf("refresh ID %s: %v", refreshed, e)
	}
	page, e := s.notes(ctx, a, parcel, "", 20)
	if e != nil || len(page.Items) != 1 || page.Items[0].Body != "edited" {
		t.Fatalf("refresh: %+v %v", page, e)
	}
	if _, e = s.DB.Exec(ctx, `DELETE FROM parcels WHERE id=$1`, parcel); e == nil {
		t.Fatal("parcel deletion bypassed restrictive note FK")
	}
	shared := httptest.NewRecorder()
	parcelapi.New(s.DB).ServeHTTP(shared, httptest.NewRequest(http.MethodGet, "/api/v1/parcels/"+parcel, nil))
	if shared.Code != http.StatusOK || strings.Contains(shared.Body.String(), "edited") || strings.Contains(shared.Body.String(), "user_parcel_notes") {
		t.Fatalf("shared parcel response status %d or private text leaked", shared.Code)
	}
	if e = s.deleteNote(ctx, a, parcel, id, 2); e != nil {
		t.Fatal(e)
	}
	if page, e = s.notes(ctx, a, parcel, "", 20); e != nil || len(page.Items) != 0 {
		t.Fatalf("delete: %+v %v", page, e)
	}
}
func TestScratchpadPageAndAccountCascade(t *testing.T) {
	s, a, b, parcel := fixture(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("00000000-0000-4000-8000-%012d", i)
		if _, e := s.createNote(ctx, a, parcel, id, fmt.Sprintf("entry %d", i)); e != nil {
			t.Fatal(e)
		}
	}
	seen := map[string]bool{}
	cursor := ""
	for {
		page, e := s.notes(ctx, a, parcel, cursor, 2)
		if e != nil {
			t.Fatal(e)
		}
		for _, n := range page.Items {
			if seen[n.ID] {
				t.Fatal("duplicate page entry")
			}
			seen[n.ID] = true
		}
		if page.Next == "" {
			break
		}
		cursor = page.Next
	}
	if len(seen) != 5 {
		t.Fatalf("paged %d entries", len(seen))
	}
	if _, e := s.notes(ctx, a, parcel, "", 51); e != errInvalid {
		t.Fatalf("unbounded: %v", e)
	}
	if _, e := s.notes(ctx, a, parcel, "bad", 2); e != errInvalid {
		t.Fatalf("bad cursor: %v", e)
	}
	if _, e := s.createNote(ctx, b, parcel, "33333333-3333-4333-8333-333333333333", "other user"); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DB.Exec(ctx, `DELETE FROM app_users WHERE id=$1`, string(a)); e != nil {
		t.Fatal(e)
	}
	var count int
	if e := s.DB.QueryRow(ctx, `SELECT count(*) FROM user_parcel_notes WHERE parcel_id=$1`, parcel).Scan(&count); e != nil || count != 1 {
		t.Fatalf("cascade count %d %v", count, e)
	}
}
func TestScratchpadHTTPBoundary(t *testing.T) {
	s, a, b, parcel := fixture(t)
	id := "55555555-5555-4555-8555-555555555555"
	request := func(user AccountID, method, path, body, origin string, content bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://example.test/api/me/favorites/"+parcel+"/notes"+path, strings.NewReader(body))
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if content {
			r.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		s.Serve(w, r, user, "http://example.test")
		return w
	}
	for _, body := range []string{`{"id":"` + id + `","body":"hi","extra":true}`, `{"id":"` + id + `","body":" "}`, `{"id":"` + id + `","body":"hi"} {}`, `{"id":"` + id + `","body":"` + strings.Repeat("x", 4100) + `"}`} {
		if w := request(a, "POST", "", body, "http://example.test", true); w.Code != 400 {
			t.Fatalf("invalid body status %d", w.Code)
		}
	}
	valid := `{"id":"` + id + `","body":"safe"}`
	if w := request(a, "POST", "", valid, "http://evil.test", true); w.Code != 403 {
		t.Fatalf("origin %d", w.Code)
	}
	if w := request(a, "POST", "", valid, "http://example.test", true); w.Code != 200 {
		t.Fatalf("create %d: %s", w.Code, w.Body)
	}
	if w := request(b, "POST", "", valid, "http://example.test", true); w.Code != 404 {
		t.Fatalf("foreign create token %d", w.Code)
	}
	// Escaped newlines grow in JSON but remain within the note's byte limit.
	escaped := `{"id":"66666666-6666-4666-8666-666666666666","body":"` + `text` + strings.Repeat(`\n`, 3000) + `"}`
	if w := request(a, "POST", "", escaped, "http://example.test", true); w.Code != 200 {
		t.Fatalf("escaped note %d: %s", w.Code, w.Body)
	}
	if w := request(a, "POST", "", valid, "http://example.test", true); w.Code != 200 {
		t.Fatalf("retry %d", w.Code)
	}
	for _, method := range []string{"PUT", "DELETE"} {
		body := `{"revision":1}`
		if method == "PUT" {
			body = `{"body":"guess","revision":1}`
		}
		if w := request(b, method, "/"+id, body, "http://example.test", true); w.Code != 404 {
			t.Fatalf("foreign %s: %d", method, w.Code)
		}
	}
	w := request(a, "GET", "?limit=1", "", "", false)
	var page NotePage
	if e := json.Unmarshal(w.Body.Bytes(), &page); e != nil || len(page.Items) != 1 {
		t.Fatalf("page %d %+v %v", w.Code, page, e)
	}
	if w = request(a, "PUT", "/"+id, `{"body":"edit","revision":1}`, "http://example.test", true); w.Code != 200 {
		t.Fatalf("edit %d", w.Code)
	}
	if w = request(a, "DELETE", "/"+id, `{"revision":1}`, "http://example.test", true); w.Code != 409 {
		t.Fatalf("conflict %d", w.Code)
	}
	if w = request(a, "DELETE", "/"+id, `{"revision":2}`, "http://example.test", true); w.Code != 204 {
		t.Fatalf("delete %d", w.Code)
	}
}
