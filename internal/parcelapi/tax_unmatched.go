package parcelapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

type unmatchedTaxRecord struct {
	ID         int64     `json:"id"`
	CaptureID  *string   `json:"capture_id"`
	AttemptID  *string   `json:"attempt_id"`
	ParcelKey  string    `json:"parcel_key"`
	TaxYear    *int16    `json:"tax_year"`
	Reason     string    `json:"reason"`
	RecordedAt time.Time `json:"recorded_at"`
	MatchedNow bool      `json:"matched_now"`
}

func (h *Handler) taxUnmatched(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if err := parse(q, "source", "import_id", "after", "limit"); err != nil {
		bad(w, err)
		return
	}
	source := q.Get("source")
	switch source {
	case "union_delinquent", "union_billing", "union_foreclosure", "union_foreclosure_pipeline":
	default:
		bad(w, errors.New("valid tax source required"))
		return
	}
	importID := q.Get("import_id")
	if importID != "" && !uuidPattern.MatchString(importID) {
		bad(w, errors.New("invalid import_id"))
		return
	}
	after := int64(0)
	if raw := q.Get("after"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 || importID == "" {
			bad(w, errors.New("after requires a nonnegative integer and import_id"))
			return
		}
		after = n
	}
	limit := 100
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 500 {
			bad(w, errors.New("limit must be between 1 and 500"))
			return
		}
		limit = n
	}
	tx, err := h.tx(r.Context())
	if err != nil {
		unavailable(w)
		return
	}
	defer tx.Rollback(context.Background())
	var sourceURL string
	var fetchedAt time.Time
	err = tx.QueryRow(r.Context(), `SELECT id::text,source_url,fetched_at FROM tax_file_imports WHERE county_id=$1 AND source_name=$2 AND (($3='' AND active) OR id::text=$3)`, countyID, source, importID).Scan(&importID, &sourceURL, &fetchedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		unavailable(w)
		return
	}
	rows, err := tx.Query(r.Context(), `SELECT u.id,u.parcel_key,NULLIF(u.tax_year,0),u.reason,u.recorded_at,u.capture_id::text,c.attempt_id::text,
 EXISTS(SELECT 1 FROM parcels p WHERE p.county_id=$1 AND p.source_name=$2 AND p.source_parcel_id=u.parcel_key)
 FROM tax_unmatched_records u LEFT JOIN ingest_captures c ON c.id=u.capture_id WHERE u.import_id=$3::uuid AND u.id>$4 ORDER BY u.id LIMIT $5`, countyID, parcelSource, importID, after, limit+1)
	if err != nil {
		unavailable(w)
		return
	}
	records := make([]unmatchedTaxRecord, 0)
	for rows.Next() {
		var item unmatchedTaxRecord
		if err := rows.Scan(&item.ID, &item.ParcelKey, &item.TaxYear, &item.Reason, &item.RecordedAt, &item.CaptureID, &item.AttemptID, &item.MatchedNow); err != nil {
			rows.Close()
			unavailable(w)
			return
		}
		records = append(records, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		unavailable(w)
		return
	}
	var next *int64
	if len(records) > limit {
		records = records[:limit]
		n := records[len(records)-1].ID
		next = &n
	}
	if err := tx.Commit(r.Context()); err != nil {
		unavailable(w)
		return
	}
	respond(w, map[string]any{"source": source, "import_id": importID, "source_url": sourceURL, "fetched_at": fetchedAt, "records": records, "next_after": next})
}
