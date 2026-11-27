// Package parcelcrm owns private favorites, lead states, and their revision checks.
package parcelcrm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AccountID string

type Store struct{ DB *pgxpool.Pool }
type State struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
	Revision int64  `json:"revision"`
	Count    int    `json:"count"`
}
type Card struct {
	ParcelID        string  `json:"parcel_id"`
	ParcelNumber    *string `json:"parcel_number"`
	Address         *string `json:"address"`
	StateID         string  `json:"state_id"`
	Favorite        bool    `json:"favorite"`
	Excluded        bool    `json:"excluded"`
	ExclusionReason string  `json:"exclusion_reason"`
	Revision        int64   `json:"revision"`
	FavoritedAt     string  `json:"favorited_at"`
}
type Board struct {
	States         []State `json:"states"`
	DefaultStateID string  `json:"default_state_id"`
	Revision       int64   `json:"revision"`
	Next           string  `json:"next"`
}

var errConflict = errors.New("revision conflict")
var errInvalid = errors.New("invalid request")

func uuid(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
func (s Store) init(ctx context.Context, user AccountID) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	// Serialize all state operations and first-use initialization on the account row.
	var id string
	if e = tx.QueryRow(ctx, `SELECT id::text FROM app_users WHERE id=$1 FOR UPDATE`, string(user)).Scan(&id); e != nil {
		return e
	}
	var exists bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_crm_settings WHERE user_id=$1)`, id).Scan(&exists); e != nil {
		return e
	}
	if !exists {
		var state string
		if e = tx.QueryRow(ctx, `INSERT INTO user_lead_states(user_id,name,position) VALUES($1,'New',0) RETURNING id::text`, id).Scan(&state); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO user_crm_settings(user_id,default_state_id) VALUES($1,$2)`, id, state); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
func (s Store) board(ctx context.Context, user AccountID) (Board, error) {
	return s.boardPage(ctx, user, "")
}
func (s Store) boardPage(ctx context.Context, user AccountID, cursor string) (Board, error) {
	var position *int
	var stateID *string
	if cursor != "" {
		parts := strings.Split(cursor, "|")
		if len(parts) != 2 || !uuid(parts[1]) {
			return Board{}, errInvalid
		}
		n, e := strconv.Atoi(parts[0])
		if e != nil || n < 0 {
			return Board{}, errInvalid
		}
		position = &n
		stateID = &parts[1]
	}
	b := Board{States: []State{}}
	if e := s.init(ctx, user); e != nil {
		return b, e
	}
	e := s.DB.QueryRow(ctx, `SELECT default_state_id::text,revision FROM user_crm_settings WHERE user_id=$1`, string(user)).Scan(&b.DefaultStateID, &b.Revision)
	if e != nil {
		return b, e
	}
	rows, e := s.DB.Query(ctx, `SELECT st.id::text,st.name,st.position,st.revision,count(p.parcel_id)::int FROM user_lead_states st LEFT JOIN user_parcels p ON p.user_id=st.user_id AND p.state_id=st.id AND p.favorite WHERE st.user_id=$1 AND ($2::integer IS NULL OR (st.position,st.id)>($2::integer,$3::uuid)) GROUP BY st.user_id,st.id ORDER BY st.position,st.id LIMIT 50`, string(user), position, stateID)
	if e != nil {
		return b, e
	}
	defer rows.Close()
	for rows.Next() {
		var st State
		if e = rows.Scan(&st.ID, &st.Name, &st.Position, &st.Revision, &st.Count); e != nil {
			return b, e
		}
		b.States = append(b.States, st)
	}
	if len(b.States) == 50 {
		last := b.States[49]
		b.Next = strconv.Itoa(last.Position) + "|" + last.ID
	}
	return b, rows.Err()
}
func (s Store) readState(ctx context.Context, user AccountID, id string) (State, error) {
	var st State
	if !uuid(id) {
		return st, errInvalid
	}
	e := s.DB.QueryRow(ctx, `SELECT st.id::text,st.name,st.position,st.revision,count(p.parcel_id)::int FROM user_lead_states st LEFT JOIN user_parcels p ON p.user_id=st.user_id AND p.state_id=st.id AND p.favorite WHERE st.user_id=$1 AND st.id=$2 GROUP BY st.user_id,st.id`, string(user), id).Scan(&st.ID, &st.Name, &st.Position, &st.Revision, &st.Count)
	return st, e
}
func (s Store) cards(ctx context.Context, user AccountID, state, cursor string, limit int, hide bool) ([]Card, error) {
	out := []Card{}
	if !uuid(state) {
		return out, errInvalid
	}
	if limit < 1 || limit > 50 {
		return out, errInvalid
	}
	var exists bool
	if e := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_lead_states WHERE user_id=$1 AND id=$2)`, string(user), state).Scan(&exists); e != nil {
		return out, e
	}
	if !exists {
		return out, pgx.ErrNoRows
	}
	var at *string
	var parcel *string
	if cursor != "" {
		parts := strings.Split(cursor, "|")
		if len(parts) != 2 || !uuid(parts[1]) {
			return out, errInvalid
		}
		at = &parts[0]
		parcel = &parts[1]
	}
	rows, e := s.DB.Query(ctx, `SELECT p.parcel_id::text,par.parcel_number,par.situs_line_1,p.state_id::text,p.favorite,p.excluded,p.exclusion_reason,p.revision,p.favorited_at::text FROM user_parcels p JOIN parcels par ON par.id=p.parcel_id WHERE p.user_id=$1 AND p.state_id=$2 AND p.favorite AND ($3::text IS NULL OR (p.favorited_at,p.parcel_id) > ($3::timestamptz,$4::uuid)) AND (NOT $5 OR NOT p.excluded) ORDER BY p.favorited_at,p.parcel_id LIMIT $6`, string(user), state, at, parcel, hide, limit)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var c Card
		if e = rows.Scan(&c.ParcelID, &c.ParcelNumber, &c.Address, &c.StateID, &c.Favorite, &c.Excluded, &c.ExclusionReason, &c.Revision, &c.FavoritedAt); e != nil {
			return out, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s Store) card(ctx context.Context, user AccountID, id string) (Card, error) {
	var c Card
	e := s.DB.QueryRow(ctx, `SELECT p.parcel_id::text,par.parcel_number,par.situs_line_1,p.state_id::text,p.favorite,p.excluded,p.exclusion_reason,p.revision,p.favorited_at::text FROM user_parcels p JOIN parcels par ON par.id=p.parcel_id WHERE p.user_id=$1 AND p.parcel_id=$2`, string(user), id).Scan(&c.ParcelID, &c.ParcelNumber, &c.Address, &c.StateID, &c.Favorite, &c.Excluded, &c.ExclusionReason, &c.Revision, &c.FavoritedAt)
	return c, e
}
func (s Store) favorite(ctx context.Context, user AccountID, id string, revision *int64) (Card, error) {
	if e := s.init(ctx, user); e != nil {
		return Card{}, e
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return Card{}, e
	}
	defer tx.Rollback(ctx)
	// Account first, card second: state deletion and card moves use the same lock order.
	var locked string
	if e = tx.QueryRow(ctx, `SELECT id::text FROM app_users WHERE id=$1 FOR UPDATE`, string(user)).Scan(&locked); e != nil {
		return Card{}, e
	}
	var inserted string
	e = tx.QueryRow(ctx, `INSERT INTO user_parcels(user_id,parcel_id,state_id) SELECT $1,p.id,settings.default_state_id FROM parcels p JOIN user_crm_settings settings ON settings.user_id=$1 WHERE p.id=$2 ON CONFLICT (user_id,parcel_id) DO NOTHING RETURNING parcel_id::text`, string(user), id).Scan(&inserted)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return Card{}, e
	}
	if errors.Is(e, pgx.ErrNoRows) {
		var active bool
		var current int64
		if e = tx.QueryRow(ctx, `SELECT favorite,revision FROM user_parcels WHERE user_id=$1 AND parcel_id=$2 FOR UPDATE`, string(user), id).Scan(&active, &current); e != nil {
			return Card{}, e
		}
		if !active {
			if revision == nil || current != *revision {
				return Card{}, errConflict
			}
			if _, e = tx.Exec(ctx, `UPDATE user_parcels SET favorite=true,revision=revision+1 WHERE user_id=$1 AND parcel_id=$2`, string(user), id); e != nil {
				return Card{}, e
			}
		}
	} else if inserted == "" {
		return Card{}, pgx.ErrNoRows
	}
	if e = tx.Commit(ctx); e != nil {
		return Card{}, e
	}
	return s.card(ctx, user, id)
}
func (s Store) mutate(ctx context.Context, user AccountID, id, action, target, reason string, revision int64) (Card, error) {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return Card{}, e
	}
	defer tx.Rollback(ctx)
	// Match favorite and state deletion lock order before touching the card.
	var locked string
	if e = tx.QueryRow(ctx, `SELECT id::text FROM app_users WHERE id=$1 FOR UPDATE`, string(user)).Scan(&locked); e != nil {
		return Card{}, e
	}
	var old string
	e = tx.QueryRow(ctx, `SELECT state_id::text FROM user_parcels WHERE user_id=$1 AND parcel_id=$2 FOR UPDATE`, string(user), id).Scan(&old)
	if e != nil {
		return Card{}, e
	}
	var tag pgconn.CommandTag
	switch action {
	case "state":
		if !uuid(target) {
			return Card{}, errInvalid
		}
		var oldName, newName string
		e = tx.QueryRow(ctx, `SELECT name FROM user_lead_states WHERE user_id=$1 AND id=$2`, string(user), old).Scan(&oldName)
		if e != nil {
			return Card{}, e
		}
		e = tx.QueryRow(ctx, `SELECT name FROM user_lead_states WHERE user_id=$1 AND id=$2`, string(user), target).Scan(&newName)
		if e != nil {
			return Card{}, e
		}
		tag, e = tx.Exec(ctx, `UPDATE user_parcels SET state_id=$3,revision=revision+1 WHERE user_id=$1 AND parcel_id=$2 AND revision=$4 AND favorite`, string(user), id, target, revision)
		if e == nil && tag.RowsAffected() > 0 && old != target {
			_, e = tx.Exec(ctx, `INSERT INTO user_parcel_state_changes(user_id,parcel_id,old_state_name,new_state_name) VALUES($1,$2,$3,$4)`, string(user), id, oldName, newName)
		}
	case "remove":
		tag, e = tx.Exec(ctx, `UPDATE user_parcels SET favorite=false,revision=revision+1 WHERE user_id=$1 AND parcel_id=$2 AND revision=$3 AND favorite`, string(user), id, revision)
	case "exclude":
		if len(strings.TrimSpace(reason)) < 1 || len(reason) > 500 {
			return Card{}, errInvalid
		}
		tag, e = tx.Exec(ctx, `UPDATE user_parcels SET excluded=true,exclusion_reason=$3,revision=revision+1 WHERE user_id=$1 AND parcel_id=$2 AND revision=$4 AND favorite`, string(user), id, strings.TrimSpace(reason), revision)
	case "restore":
		tag, e = tx.Exec(ctx, `UPDATE user_parcels SET excluded=false,exclusion_reason='',revision=revision+1 WHERE user_id=$1 AND parcel_id=$2 AND revision=$3 AND favorite`, string(user), id, revision)
	default:
		return Card{}, errInvalid
	}
	if e != nil {
		return Card{}, e
	}
	if tag.RowsAffected() == 0 {
		return Card{}, errConflict
	}
	if action == "exclude" || action == "restore" {
		_, e = tx.Exec(ctx, `INSERT INTO user_parcel_exclusion_changes(user_id,parcel_id,excluded,reason) VALUES($1,$2,$3,$4)`, string(user), id, action == "exclude", func() string {
			if action == "exclude" {
				return strings.TrimSpace(reason)
			}
			return ""
		}())
		if e != nil {
			return Card{}, e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return Card{}, e
	}
	return s.card(ctx, user, id)
}
func (s Store) state(ctx context.Context, user AccountID, id, action, name, dest string, revision int64) (Board, error) {
	if e := s.init(ctx, user); e != nil {
		return Board{}, e
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return Board{}, e
	}
	defer tx.Rollback(ctx)
	var locked string
	if e = tx.QueryRow(ctx, `SELECT id::text FROM app_users WHERE id=$1 FOR UPDATE`, string(user)).Scan(&locked); e != nil {
		return Board{}, e
	}
	if action != "create" {
		var exists bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_lead_states WHERE user_id=$1 AND id=$2)`, string(user), id).Scan(&exists); e != nil {
			return Board{}, e
		}
		if !exists {
			return Board{}, pgx.ErrNoRows
		}
	}
	name = strings.TrimSpace(name)
	if name != "" && len(name) > 80 {
		return Board{}, errInvalid
	}
	switch action {
	case "create":
		if name == "" {
			return Board{}, errInvalid
		}
		_, e = tx.Exec(ctx, `INSERT INTO user_lead_states(user_id,name,position) VALUES($1,$2,(SELECT coalesce(max(position)+1,0) FROM user_lead_states WHERE user_id=$1))`, string(user), name)
	case "rename", "reorder":
		var tag pgconn.CommandTag
		if action == "rename" {
			if name == "" {
				return Board{}, errInvalid
			}
			tag, e = tx.Exec(ctx, `UPDATE user_lead_states SET name=$3,revision=revision+1 WHERE user_id=$1 AND id=$2 AND revision=$4`, string(user), id, name, revision)
		} else {
			n, err := strconv.Atoi(name)
			if err != nil || n < 0 || n > 100000 {
				return Board{}, errInvalid
			}
			var old int
			if e = tx.QueryRow(ctx, `SELECT position FROM user_lead_states WHERE user_id=$1 AND id=$2 AND revision=$3`, string(user), id, revision).Scan(&old); e != nil {
				return Board{}, errConflict
			}
			var count int
			if e = tx.QueryRow(ctx, `SELECT count(*) FROM user_lead_states WHERE user_id=$1`, string(user)).Scan(&count); e != nil {
				return Board{}, e
			}
			if n >= count {
				return Board{}, errInvalid
			}
			if n < old {
				_, e = tx.Exec(ctx, `UPDATE user_lead_states SET position=position+1,revision=revision+1 WHERE user_id=$1 AND position >= $2 AND position < $3 AND id<>$4`, string(user), n, old, id)
			} else if n > old {
				_, e = tx.Exec(ctx, `UPDATE user_lead_states SET position=position-1,revision=revision+1 WHERE user_id=$1 AND position > $2 AND position <= $3 AND id<>$4`, string(user), old, n, id)
			}
			if e != nil {
				return Board{}, e
			}
			tag, e = tx.Exec(ctx, `UPDATE user_lead_states SET position=$3,revision=revision+1 WHERE user_id=$1 AND id=$2 AND revision=$4`, string(user), id, n, revision)
		}
		if e == nil && tag.RowsAffected() == 0 {
			return Board{}, errConflict
		}
	case "default":
		tag, err := tx.Exec(ctx, `UPDATE user_crm_settings SET default_state_id=$2,revision=revision+1 WHERE user_id=$1 AND revision=$3 AND EXISTS(SELECT 1 FROM user_lead_states WHERE user_id=$1 AND id=$2)`, string(user), id, revision)
		e = err
		if e == nil && tag.RowsAffected() == 0 {
			return Board{}, errConflict
		}
	case "delete":
		if !uuid(dest) || dest == id {
			return Board{}, errInvalid
		}
		var oldName, newName string
		if e = tx.QueryRow(ctx, `SELECT name FROM user_lead_states WHERE user_id=$1 AND id=$2 AND revision=$3`, string(user), id, revision).Scan(&oldName); e != nil {
			return Board{}, errConflict
		}
		if e = tx.QueryRow(ctx, `SELECT name FROM user_lead_states WHERE user_id=$1 AND id=$2`, string(user), dest).Scan(&newName); e != nil {
			return Board{}, e
		}
		// Only reassigned rows produce history; RETURNING and history share one statement snapshot.
		_, e = tx.Exec(ctx, `WITH moved AS (
UPDATE user_parcels SET state_id=$3,revision=revision+1 WHERE user_id=$1 AND state_id=$2
RETURNING user_id,parcel_id
) INSERT INTO user_parcel_state_changes(user_id,parcel_id,old_state_name,new_state_name)
SELECT user_id,parcel_id,$4,$5 FROM moved`, string(user), id, dest, oldName, newName)
		if e != nil {
			return Board{}, e
		}
		_, e = tx.Exec(ctx, `UPDATE user_crm_settings SET default_state_id=$3,revision=revision+1 WHERE user_id=$1 AND default_state_id=$2`, string(user), id, dest)
		if e != nil {
			return Board{}, e
		}
		_, e = tx.Exec(ctx, `DELETE FROM user_lead_states WHERE user_id=$1 AND id=$2`, string(user), id)
		if e == nil {
			_, e = tx.Exec(ctx, `WITH ordered AS (SELECT id,row_number() OVER (ORDER BY position,id)-1 AS new_pos FROM user_lead_states WHERE user_id=$1) UPDATE user_lead_states st SET position=ordered.new_pos,revision=st.revision+1 FROM ordered WHERE st.user_id=$1 AND st.id=ordered.id AND st.position<>ordered.new_pos`, string(user))
		}
	default:
		return Board{}, errInvalid
	}
	if e != nil {
		return Board{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return Board{}, e
	}
	return s.board(ctx, user)
}
func send(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func failure(w http.ResponseWriter, e error) {
	var pgErr *pgconn.PgError
	if errors.As(e, &pgErr) {
		if pgErr.Code == "23505" {
			e = errConflict
		}
		if pgErr.Code == "23503" {
			e = pgx.ErrNoRows
		}
		if pgErr.Code == "22P02" || pgErr.Code == "22007" || pgErr.Code == "22008" {
			e = errInvalid
		}
	}
	switch {
	case errors.Is(e, pgx.ErrNoRows):
		http.Error(w, "Not found", 404)
	case errors.Is(e, errConflict):
		http.Error(w, "Revision conflict. Reload and retry.", http.StatusConflict)
	case errors.Is(e, errInvalid):
		http.Error(w, "Invalid request", 400)
	default:
		http.Error(w, "CRM unavailable", http.StatusServiceUnavailable)
	}
}

// Serve receives only an authenticated account ID from the authentication boundary.
func (s Store) Serve(w http.ResponseWriter, r *http.Request, user AccountID, origin string) {
	if r.Method != "GET" {
		if r.Header.Get("Origin") != origin {
			http.Error(w, "Invalid request origin", http.StatusForbidden)
			return
		}
		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "Expected JSON", http.StatusUnsupportedMediaType)
			return
		}
	}
	q := r.URL.Query()
	path := strings.TrimPrefix(r.URL.Path, "/api/me/")
	parts := strings.Split(path, "/")
	if len(parts) >= 3 && parts[0] == "favorites" && parts[2] == "notes" {
		s.serveNotes(w, r, user, origin, parts)
		return
	}
	var req struct {
		Name          string `json:"name"`
		StateID       string `json:"state_id"`
		DestinationID string `json:"destination_id"`
		Reason        string `json:"reason"`
		Revision      int64  `json:"revision"`
		Position      *int   `json:"position"`
	}
	if r.Method != "GET" {
		r.Body = http.MaxBytesReader(w, r.Body, 2048)
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		if e := d.Decode(&req); e != nil {
			failure(w, errInvalid)
			return
		}
		var extra any
		if e := d.Decode(&extra); !errors.Is(e, io.EOF) {
			failure(w, errInvalid)
			return
		}
	}
	ctx := r.Context()
	var e error
	if path == "crm/board" && r.Method == "GET" {
		var b Board
		b, e = s.boardPage(ctx, user, q.Get("cursor"))
		if e == nil {
			send(w, b)
		}
	} else if len(parts) == 3 && parts[0] == "crm" && parts[1] == "states" && r.Method == "GET" && uuid(parts[2]) {
		var st State
		st, e = s.readState(ctx, user, parts[2])
		if e == nil {
			send(w, st)
		}
	} else if len(parts) == 3 && parts[0] == "crm" && parts[1] == "board" && r.Method == "GET" {
		limit := 20
		if q.Has("limit") {
			limit, e = strconv.Atoi(q.Get("limit"))
			if e != nil {
				failure(w, errInvalid)
				return
			}
		}
		var cards []Card
		cards, e = s.cards(ctx, user, parts[2], q.Get("cursor"), limit, q.Get("hide_excluded") == "true")
		if e == nil {
			send(w, struct {
				Items []Card `json:"items"`
				Next  string `json:"next"`
			}{cards, func() string {
				if len(cards) == limit {
					c := cards[len(cards)-1]
					return c.FavoritedAt + "|" + c.ParcelID
				}
				return ""
			}()})
		}
	} else if len(parts) >= 2 && parts[0] == "favorites" && uuid(parts[1]) {
		id := parts[1]
		var c Card
		switch {
		case len(parts) == 2 && r.Method == "GET":
			c, e = s.card(ctx, user, id)
		case len(parts) == 2 && r.Method == "PUT":
			if req.Revision != 0 {
				c, e = s.favorite(ctx, user, id, &req.Revision)
			} else {
				c, e = s.favorite(ctx, user, id, nil)
			}
		case len(parts) == 2 && r.Method == "DELETE":
			c, e = s.mutate(ctx, user, id, "remove", "", "", req.Revision)
		case len(parts) == 3 && parts[2] == "state" && r.Method == "PUT":
			c, e = s.mutate(ctx, user, id, "state", req.StateID, "", req.Revision)
		case len(parts) == 3 && parts[2] == "exclusion" && r.Method == "PUT":
			c, e = s.mutate(ctx, user, id, "exclude", "", req.Reason, req.Revision)
		case len(parts) == 3 && parts[2] == "exclusion" && r.Method == "DELETE":
			c, e = s.mutate(ctx, user, id, "restore", "", "", req.Revision)
		default:
			e = errInvalid
		}
		if e == nil {
			send(w, c)
		}
	} else if parts[0] == "crm" && len(parts) >= 2 && parts[1] == "states" && r.Method == "POST" && len(parts) == 2 {
		var b Board
		b, e = s.state(ctx, user, "", "create", req.Name, "", 0)
		if e == nil {
			send(w, b)
		}
	} else if parts[0] == "crm" && len(parts) == 3 && parts[1] == "states" && uuid(parts[2]) {
		action := ""
		switch r.Method {
		case "PATCH":
			action = "rename"
			if req.Position != nil {
				action = "reorder"
				req.Name = strconv.Itoa(*req.Position)
			}
		case "DELETE":
			action = "delete"
		}
		var b Board
		b, e = s.state(ctx, user, parts[2], action, req.Name, req.DestinationID, req.Revision)
		if e == nil {
			send(w, b)
		}
	} else if path == "crm/settings" && r.Method == "PUT" && uuid(req.StateID) {
		var b Board
		b, e = s.state(ctx, user, req.StateID, "default", "", "", req.Revision)
		if e == nil {
			send(w, b)
		}
	} else {
		http.NotFound(w, r)
		return
	}
	if e != nil {
		failure(w, e)
	}
}
