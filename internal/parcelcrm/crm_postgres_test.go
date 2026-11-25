//go:build postgres

package parcelcrm

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func fixture(t *testing.T) (Store, AccountID, AccountID, string) {
	t.Helper()
	dsn := os.Getenv("CRM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("CRM_TEST_DATABASE_URL required")
	}
	db, e := pgxpool.New(context.Background(), dsn)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(db.Close)
	var name string
	if e = db.QueryRow(context.Background(), `SELECT current_database()`).Scan(&name); e != nil || !strings.HasPrefix(name, "landman_auth_test_") {
		t.Fatal("disposable database required")
	}
	stamp := fmt.Sprintf("crm-%d", time.Now().UnixNano())
	var a, b, id string
	for i, ptr := range []*string{&a, &b} {
		e = db.QueryRow(context.Background(), `INSERT INTO app_users(google_subject,email,display_name) VALUES($1,$2,'Fixture') RETURNING id::text`, fmt.Sprintf("%s-%d", stamp, i), fmt.Sprintf("%s-%d@example.test", stamp, i)).Scan(ptr)
		if e != nil {
			t.Fatal(e)
		}
	}
	e = db.QueryRow(context.Background(), `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,parcel_number) VALUES('union','union','crm_fixture',$1,$1) RETURNING id::text`, stamp).Scan(&id)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		db.Exec(ctx, `DELETE FROM app_users WHERE id IN ($1,$2)`, a, b)
		db.Exec(ctx, `DELETE FROM parcels WHERE id=$1`, id)
	})
	return Store{db}, AccountID(a), AccountID(b), id
}
func TestFocusedStateRead(t *testing.T) {
	s, a, b, _ := fixture(t)
	ctx := context.Background()
	board, e := s.board(ctx, a)
	if e != nil {
		t.Fatal(e)
	}
	id := board.DefaultStateID
	st, e := s.readState(ctx, a, id)
	if e != nil || st.Name != "New" || st.ID != id {
		t.Fatalf("own state: %+v %v", st, e)
	}
	if _, e = s.readState(ctx, b, id); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("foreign state: %v", e)
	}
	if _, e = s.readState(ctx, a, "00000000-0000-4000-8000-000000000000"); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("missing state: %v", e)
	}
	if _, e = s.readState(ctx, a, "bad"); !errors.Is(e, errInvalid) {
		t.Fatalf("invalid state: %v", e)
	}
}
func TestBoardIsolationAndRevision(t *testing.T) {
	s, a, b, id := fixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, e := s.favorite(ctx, a, id, nil); e != nil {
				t.Errorf("concurrent favorite: %v", e)
			}
		}()
	}
	wg.Wait()
	board, e := s.board(ctx, a)
	if e != nil || len(board.States) != 1 || board.States[0].Name != "New" || board.States[0].Count != 1 {
		t.Fatalf("board: %+v %v", board, e)
	}
	other, e := s.board(ctx, b)
	if e != nil || other.States[0].Count != 0 {
		t.Fatalf("other board: %+v %v", other, e)
	}
	if _, e = s.card(ctx, b, id); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("foreign card: %v", e)
	}
	if _, e = s.cards(ctx, b, board.DefaultStateID, "", 10, false); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("foreign state page: %v", e)
	}
	if _, e = s.state(ctx, b, board.DefaultStateID, "rename", "Other", "", 1); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("foreign state rename: %v", e)
	}
	if _, e = s.mutate(ctx, b, id, "state", other.DefaultStateID, "", 1); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("foreign mutation: %v", e)
	}
	card, e := s.card(ctx, a, id)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.mutate(ctx, a, id, "state", other.DefaultStateID, "", card.Revision); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("foreign state: %v", e)
	}
	board, e = s.state(ctx, a, "", "create", "Contacted", "", 0)
	if e != nil {
		t.Fatal(e)
	}
	dest := board.States[1]
	board, e = s.state(ctx, a, dest.ID, "reorder", "0", "", dest.Revision)
	if e != nil || board.States[0].ID != dest.ID || board.States[1].Position != 1 {
		t.Fatalf("reorder: %+v %v", board, e)
	}
	dest = board.States[0]
	card, e = s.mutate(ctx, a, id, "state", dest.ID, "", card.Revision)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.mutate(ctx, a, id, "state", board.DefaultStateID, "", card.Revision-1); !errors.Is(e, errConflict) {
		t.Fatalf("stale move: %v", e)
	}
	if _, e = s.mutate(ctx, a, id, "exclude", "", "", card.Revision); !errors.Is(e, errInvalid) {
		t.Fatalf("empty exclusion reason: %v", e)
	}
	card, e = s.mutate(ctx, a, id, "exclude", "", "Not a lead", card.Revision)
	if e != nil || !card.Excluded {
		t.Fatalf("exclusion: %+v %v", card, e)
	}
	card, e = s.mutate(ctx, a, id, "restore", "", "", card.Revision)
	if e != nil || card.Excluded {
		t.Fatalf("restore exclusion: %+v %v", card, e)
	}
	var exclusionEvents int
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM user_parcel_exclusion_changes WHERE user_id=$1 AND parcel_id=$2`, string(a), id).Scan(&exclusionEvents); e != nil || exclusionEvents != 2 {
		t.Fatalf("exclusion history %d %v", exclusionEvents, e)
	}
	card, e = s.mutate(ctx, a, id, "exclude", "", "Keep reason", card.Revision)
	if e != nil {
		t.Fatal(e)
	}
	card, e = s.mutate(ctx, a, id, "remove", "", "", card.Revision)
	if e != nil || card.Favorite != false {
		t.Fatalf("remove: %+v %v", card, e)
	}
	card, e = s.favorite(ctx, a, id, &card.Revision)
	if e != nil || card.StateID != dest.ID || !card.Excluded {
		t.Fatalf("restore favorite: %+v %v", card, e)
	}
	if _, e = s.state(ctx, a, dest.ID, "delete", "", other.DefaultStateID, dest.Revision); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("foreign destination: %v", e)
	}
	board, e = s.state(ctx, a, dest.ID, "delete", "", board.DefaultStateID, dest.Revision)
	if e != nil {
		t.Fatal(e)
	}
	card, e = s.card(ctx, a, id)
	if e != nil || card.StateID != board.DefaultStateID {
		t.Fatalf("reassigned: %+v %v", card, e)
	}
	var events int
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM user_parcel_state_changes WHERE user_id=$1 AND parcel_id=$2`, string(a), id).Scan(&events); e != nil || events != 2 {
		t.Fatalf("state evidence %d %v", events, e)
	}
	if _, e = s.DB.Exec(ctx, `DELETE FROM parcels WHERE id=$1`, id); e == nil {
		t.Fatal("shared parcel deletion removed private record")
	}
	if _, e = s.DB.Exec(ctx, `UPDATE parcels SET parcel_number='REFRESHED' WHERE id=$1`, id); e != nil {
		t.Fatal(e)
	}
	card, e = s.card(ctx, a, id)
	if e != nil || card.ParcelNumber == nil || *card.ParcelNumber != "REFRESHED" {
		t.Fatalf("refresh: %+v %v", card, e)
	}
}
func TestReplayAfterRemovalRequiresRevision(t *testing.T) {
	s, a, _, id := fixture(t)
	ctx := context.Background()
	initial, e := s.favorite(ctx, a, id, nil)
	if e != nil {
		t.Fatal(e)
	}
	removed, e := s.mutate(ctx, a, id, "remove", "", "", initial.Revision)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.favorite(ctx, a, id, nil); !errors.Is(e, errConflict) {
		t.Fatalf("original replay: %v", e)
	}
	if _, e = s.favorite(ctx, a, id, &initial.Revision); !errors.Is(e, errConflict) {
		t.Fatalf("stale restore: %v", e)
	}
	restored, e := s.favorite(ctx, a, id, &removed.Revision)
	if e != nil || !restored.Favorite {
		t.Fatalf("restore %+v %v", restored, e)
	}
}
func TestConcurrentDeleteAndMoveHistory(t *testing.T) {
	s, a, _, id := fixture(t)
	ctx := context.Background()
	card, e := s.favorite(ctx, a, id, nil)
	if e != nil {
		t.Fatal(e)
	}
	board, e := s.state(ctx, a, "", "create", "Second", "", 0)
	if e != nil {
		t.Fatal(e)
	}
	second := board.States[1]
	board, e = s.state(ctx, a, "", "create", "Third", "", 0)
	if e != nil {
		t.Fatal(e)
	}
	_ = board.States[2]
	// Move into the state being deleted; either lock order must leave valid history.
	lock, e := s.DB.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = lock.Exec(ctx, `SELECT id FROM app_users WHERE id=$1 FOR UPDATE`, string(a)); e != nil {
		t.Fatal(e)
	}
	type result struct {
		kind string
		err  error
	}
	done := make(chan result, 2)
	go func() {
		_, err := s.mutate(ctx, a, id, "state", second.ID, "", card.Revision)
		done <- result{"move", err}
	}()
	go func() {
		_, err := s.state(ctx, a, second.ID, "delete", "", board.DefaultStateID, second.Revision)
		done <- result{"delete", err}
	}()
	if e = lock.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	moveSucceeded := false
	for i := 0; i < 2; i++ {
		select {
		case r := <-done:
			if r.kind == "move" && r.err == nil {
				moveSucceeded = true
			}
			if r.err != nil && !(r.kind == "move" && errors.Is(r.err, pgx.ErrNoRows)) {
				t.Fatalf("%s: %v", r.kind, r.err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("deadlock")
		}
	}
	current, e := s.card(ctx, a, id)
	if e != nil {
		t.Fatal(e)
	}
	var history string
	if e = s.DB.QueryRow(ctx, `SELECT coalesce(string_agg(old_state_name || '->' || new_state_name, ',' ORDER BY id),'') FROM user_parcel_state_changes WHERE user_id=$1 AND parcel_id=$2`, string(a), id).Scan(&history); e != nil {
		t.Fatal(e)
	}
	// Delete-first: no move or history. Move-first: New -> Second -> New.
	if current.StateID != board.DefaultStateID || (moveSucceeded && history != "New->Second,Second->New") || (!moveSucceeded && history != "") {
		t.Fatalf("card=%+v history=%s", current, history)
	}
}
func TestBoundedPagesAndOrigin(t *testing.T) {
	s, a, _, id := fixture(t)
	ctx := context.Background()
	card, e := s.favorite(ctx, a, id, nil)
	if e != nil {
		t.Fatal(e)
	}
	board, e := s.board(ctx, a)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.cards(ctx, a, board.DefaultStateID, "", 51, false); !errors.Is(e, errInvalid) {
		t.Fatalf("limit: %v", e)
	}
	cards, e := s.cards(ctx, a, board.DefaultStateID, "", 1, false)
	if e != nil || len(cards) != 1 {
		t.Fatalf("page %+v %v", cards, e)
	}
	cards, e = s.cards(ctx, a, board.DefaultStateID, cards[0].FavoritedAt+"|"+id, 1, false)
	if e != nil || len(cards) != 0 {
		t.Fatalf("next page %+v %v", cards, e)
	}
	r := httptest.NewRequest("PUT", "/api/me/favorites/"+id, strings.NewReader(`{}`))
	r.Header.Set("Origin", "https://evil.test")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Serve(w, r, a, "https://landman.test")
	if w.Code != 403 {
		t.Fatalf("origin: %d", w.Code)
	}
	other, e := s.favorite(ctx, a, id, nil)
	if e != nil || other.Revision != card.Revision {
		t.Fatalf("idempotency %+v %v", other, e)
	}
}
