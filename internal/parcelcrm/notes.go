package parcelcrm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

// Note is a private, dated plain-text entry for a parcel. A client-generated ID is the creation retry token.
type Note struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	ParcelID  string    `json:"parcel_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Revision  int64     `json:"revision"`
}
type NotePage struct {
	Items []Note `json:"items"`
	Next  string `json:"next"`
}

func validNote(body string) bool {
	return utf8.ValidString(body) && strings.TrimSpace(body) != "" && len(body) <= 4000
}
func scanNote(row pgx.Row) (Note, error) {
	var n Note
	e := row.Scan(&n.ID, &n.UserID, &n.ParcelID, &n.Body, &n.CreatedAt, &n.UpdatedAt, &n.Revision)
	return n, e
}
func (s Store) notes(ctx context.Context, user AccountID, parcel, cursor string, limit int, order string) (NotePage, error) {
	page := NotePage{Items: []Note{}}
	if !uuid(parcel) || limit < 1 || limit > 50 {
		return page, errInvalid
	}
	var exists bool
	if e := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM parcels WHERE id=$1)`, parcel).Scan(&exists); e != nil {
		return page, e
	}
	if !exists {
		return page, pgx.ErrNoRows
	}
	if order != "desc" && order != "asc" {
		return page, errInvalid
	}
	var at *time.Time
	var id *string
	if cursor != "" {
		parts := strings.Split(cursor, "|")
		if len(parts) != 2 || !uuid(parts[1]) {
			return page, errInvalid
		}
		parsed, e := time.Parse(time.RFC3339Nano, parts[0])
		if e != nil {
			return page, errInvalid
		}
		at = &parsed
		id = &parts[1]
	}
	// Direction is selected from two fixed queries; user input never reaches SQL.
	query := `SELECT id::text,user_id::text,parcel_id::text,body,created_at,updated_at,revision FROM user_parcel_notes WHERE user_id=$1 AND parcel_id=$2 AND ($3::timestamptz IS NULL OR (created_at,id)<($3::timestamptz,$4::uuid)) ORDER BY created_at DESC,id DESC LIMIT $5`
	if order == "asc" {
		query = `SELECT id::text,user_id::text,parcel_id::text,body,created_at,updated_at,revision FROM user_parcel_notes WHERE user_id=$1 AND parcel_id=$2 AND ($3::timestamptz IS NULL OR (created_at,id)>($3::timestamptz,$4::uuid)) ORDER BY created_at ASC,id ASC LIMIT $5`
	}
	rows, e := s.DB.Query(ctx, query, string(user), parcel, at, id, limit+1)
	if e != nil {
		return page, e
	}
	defer rows.Close()
	for rows.Next() {
		var n Note
		if e = rows.Scan(&n.ID, &n.UserID, &n.ParcelID, &n.Body, &n.CreatedAt, &n.UpdatedAt, &n.Revision); e != nil {
			return page, e
		}
		page.Items = append(page.Items, n)
	}
	if e = rows.Err(); e != nil {
		return page, e
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		last := page.Items[limit-1]
		page.Next = last.CreatedAt.Format(time.RFC3339Nano) + "|" + last.ID
	}
	return page, nil
}
func (s Store) createNote(ctx context.Context, user AccountID, parcel, id, body string) (Note, error) {
	if !uuid(parcel) || !uuid(id) || !validNote(body) {
		return Note{}, errInvalid
	}
	n, e := scanNote(s.DB.QueryRow(ctx, `INSERT INTO user_parcel_notes(id,user_id,parcel_id,body) VALUES($1,$2,$3,$4) ON CONFLICT (id) DO NOTHING RETURNING id::text,user_id::text,parcel_id::text,body,created_at,updated_at,revision`, id, string(user), parcel, body))
	if e == nil {
		return n, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return Note{}, e
	}
	n, e = scanNote(s.DB.QueryRow(ctx, `SELECT id::text,user_id::text,parcel_id::text,body,created_at,updated_at,revision FROM user_parcel_notes WHERE id=$1 AND user_id=$2 AND parcel_id=$3`, id, string(user), parcel))
	if errors.Is(e, pgx.ErrNoRows) {
		return Note{}, pgx.ErrNoRows
	}
	if e != nil {
		return Note{}, e
	}
	if n.Body != body {
		return Note{}, errConflict
	}
	return n, nil
}
func (s Store) editNote(ctx context.Context, user AccountID, parcel, id, body string, revision int64) (Note, error) {
	if !uuid(parcel) || !uuid(id) || !validNote(body) || revision < 1 {
		return Note{}, errInvalid
	}
	n, e := scanNote(s.DB.QueryRow(ctx, `UPDATE user_parcel_notes SET body=$4,updated_at=now(),revision=revision+1 WHERE user_id=$1 AND parcel_id=$2 AND id=$3 AND revision=$5 RETURNING id::text,user_id::text,parcel_id::text,body,created_at,updated_at,revision`, string(user), parcel, id, body, revision))
	if !errors.Is(e, pgx.ErrNoRows) {
		return n, e
	}
	var exists bool
	if e = s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_parcel_notes WHERE user_id=$1 AND parcel_id=$2 AND id=$3)`, string(user), parcel, id).Scan(&exists); e != nil {
		return Note{}, e
	}
	if exists {
		return Note{}, errConflict
	}
	return Note{}, pgx.ErrNoRows
}
func (s Store) deleteNote(ctx context.Context, user AccountID, parcel, id string, revision int64) error {
	if !uuid(parcel) || !uuid(id) || revision < 1 {
		return errInvalid
	}
	tag, e := s.DB.Exec(ctx, `DELETE FROM user_parcel_notes WHERE user_id=$1 AND parcel_id=$2 AND id=$3 AND revision=$4`, string(user), parcel, id, revision)
	if e != nil {
		return e
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	var exists bool
	if e = s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_parcel_notes WHERE user_id=$1 AND parcel_id=$2 AND id=$3)`, string(user), parcel, id).Scan(&exists); e != nil {
		return e
	}
	if exists {
		return errConflict
	}
	return pgx.ErrNoRows
}
func decodeNote(w http.ResponseWriter, r *http.Request, out any) error {
	if r.Header.Get("Content-Type") != "application/json" {
		return errInvalid
	}
	// JSON escaping can expand a 4000-byte note to six bytes per input byte.
	r.Body = http.MaxBytesReader(w, r.Body, 25000)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(out); e != nil {
		return errInvalid
	}
	var extra any
	if e := d.Decode(&extra); !errors.Is(e, io.EOF) {
		return errInvalid
	}
	return nil
}

// serveNotes is called after authentication. Notes do not depend on a favorite record.
func (s Store) serveNotes(w http.ResponseWriter, r *http.Request, user AccountID, origin string, parts []string) {
	if len(parts) != 3 && len(parts) != 4 {
		http.NotFound(w, r)
		return
	}
	parcel := parts[1]
	if !uuid(parcel) || parts[2] != "notes" || (len(parts) == 4 && !uuid(parts[3])) {
		http.NotFound(w, r)
		return
	}
	if r.Method != "GET" && r.Header.Get("Origin") != origin {
		http.Error(w, "Invalid request origin", http.StatusForbidden)
		return
	}
	var e error
	switch {
	case len(parts) == 3 && r.Method == "GET":
		limit := 20
		if r.URL.Query().Has("limit") {
			limit, e = strconv.Atoi(r.URL.Query().Get("limit"))
			if e != nil {
				failure(w, errInvalid)
				return
			}
		}
		var page NotePage
		order := r.URL.Query().Get("order")
		if order == "" && !r.URL.Query().Has("order") {
			order = "desc"
		}
		page, e = s.notes(r.Context(), user, parcel, r.URL.Query().Get("cursor"), limit, order)
		if e == nil {
			send(w, page)
		}
	case len(parts) == 3 && r.Method == "POST":
		var req struct {
			ID   string `json:"id"`
			Body string `json:"body"`
		}
		if e = decodeNote(w, r, &req); e == nil {
			var n Note
			n, e = s.createNote(r.Context(), user, parcel, req.ID, req.Body)
			if e == nil {
				send(w, n)
			}
		}
	case len(parts) == 4 && r.Method == "PUT":
		var req struct {
			Body     string `json:"body"`
			Revision int64  `json:"revision"`
		}
		if e = decodeNote(w, r, &req); e == nil {
			var n Note
			n, e = s.editNote(r.Context(), user, parcel, parts[3], req.Body, req.Revision)
			if e == nil {
				send(w, n)
			}
		}
	case len(parts) == 4 && r.Method == "DELETE":
		var req struct {
			Revision int64 `json:"revision"`
		}
		if e = decodeNote(w, r, &req); e == nil {
			e = s.deleteNote(r.Context(), user, parcel, parts[3], req.Revision)
			if e == nil {
				w.WriteHeader(http.StatusNoContent)
			}
		}
	default:
		http.NotFound(w, r)
		return
	}
	if e != nil {
		failure(w, e)
	}
}
