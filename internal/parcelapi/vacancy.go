package parcelapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

// vacancy is an evidence-only read. Legacy parcel filters remain unchanged
// until a validated three-generation backfill and reader cutover.
func (h *Handler) vacancy(w http.ResponseWriter, r *http.Request, id string) {
	tx, err := h.tx(r.Context())
	if err != nil {
		unavailable(w)
		return
	}
	defer tx.Rollback(context.Background())
	ok, err := exists(r.Context(), tx, id)
	if err != nil {
		unavailable(w)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	var tier, reason, parcelGeneration, structureGeneration, assessmentGeneration string
	var facts []byte
	var ruleVersion int
	err = tx.QueryRow(r.Context(), `SELECT a.tier,a.reason,a.rule_version,a.parcel_generation,
 a.structure_generation::text,a.assessment_generation::text,a.facts
 FROM vacancy_assessments a WHERE a.parcel_id=$1`, id).Scan(&tier, &reason, &ruleVersion, &parcelGeneration, &structureGeneration, &assessmentGeneration, &facts)
	if errors.Is(err, pgx.ErrNoRows) {
		if err = tx.Commit(r.Context()); err != nil {
			unavailable(w)
			return
		}
		respond(w, map[string]any{"tier": "unknown", "reason": "validated_evidence_not_published", "available": false})
		return
	}
	if err != nil {
		unavailable(w)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		unavailable(w)
		return
	}
	respond(w, map[string]any{"tier": tier, "reason": reason, "available": true, "rule_version": ruleVersion, "parcel_generation": parcelGeneration, "structure_generation": structureGeneration, "assessment_generation": assessmentGeneration, "facts": json.RawMessage(facts)})
}
