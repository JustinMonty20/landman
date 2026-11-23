package parcelapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

// The published tier is usable only while its parcel fingerprint, both live
// generations, validation attestations and latest accepted source heads match.
// A stale result is unknown, even if an old row says strong_candidate.
const vacancyFresh = `parcel_boundary_usable(p.shape)
 AND a.parcel_generation=c.observation_id::text || ':' || c.normalized_hash || ':' || c.geometry_hash
 AND sv.capture_contract_version=1 AND av.capture_contract_version=1
 AND a.structure_generation=sv.generation_id AND a.assessment_generation=av.generation_id
 AND sv.normalized_sha256=sg.normalized_sha256 AND av.normalized_sha256=ag.normalized_sha256
 AND sg.capture_id=sh.capture_id AND ag.capture_id=ah.capture_id`
const vacancyTierSQL = `COALESCE((SELECT a.tier FROM vacancy_assessments a
 JOIN parcel_evidence_current c ON c.county_id=p.county_id AND c.source_name=p.source_name AND c.source_parcel_id=p.source_parcel_id
 JOIN vacancy_live_validation sv ON sv.generation_id=a.structure_generation AND sv.source_name='union_structures_171'
 JOIN vacancy_live_generations sg ON sg.id=sv.generation_id
 JOIN vacancy_latest_observations sh ON sh.source_name=sg.source_name
 JOIN vacancy_live_validation av ON av.generation_id=a.assessment_generation AND av.source_name='union_assessed_value_0'
 JOIN vacancy_live_generations ag ON ag.id=av.generation_id
 JOIN vacancy_latest_observations ah ON ah.source_name=ag.source_name
 WHERE a.parcel_id=p.id AND ` + vacancyFresh + `),'unknown')`

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
	var structureAsOf, assessmentAsOf string
	var structureRetrieved, assessmentRetrieved time.Time
	var facts []byte
	var ruleVersion int
	err = tx.QueryRow(r.Context(), `SELECT a.tier,a.reason,a.rule_version,a.parcel_generation,
 a.structure_generation::text,a.assessment_generation::text,a.facts,
 sv.source_as_of::text,av.source_as_of::text,sg.retrieved_at,ag.retrieved_at
 FROM vacancy_assessments a JOIN parcels p ON p.id=a.parcel_id
 JOIN parcel_evidence_current c ON c.county_id=p.county_id AND c.source_name=p.source_name AND c.source_parcel_id=p.source_parcel_id
 JOIN vacancy_live_validation sv ON sv.generation_id=a.structure_generation AND sv.source_name='union_structures_171'
 JOIN vacancy_live_generations sg ON sg.id=sv.generation_id
 JOIN vacancy_latest_observations sh ON sh.source_name=sg.source_name
 JOIN vacancy_live_validation av ON av.generation_id=a.assessment_generation AND av.source_name='union_assessed_value_0'
 JOIN vacancy_live_generations ag ON ag.id=av.generation_id
 JOIN vacancy_latest_observations ah ON ah.source_name=ag.source_name
 WHERE a.parcel_id=$1 AND `+vacancyFresh, id).Scan(&tier, &reason, &ruleVersion, &parcelGeneration, &structureGeneration, &assessmentGeneration, &facts, &structureAsOf, &assessmentAsOf, &structureRetrieved, &assessmentRetrieved)
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
	respond(w, map[string]any{"tier": tier, "reason": reason, "available": true, "rule_version": ruleVersion, "parcel_generation": parcelGeneration, "structure_generation": structureGeneration, "assessment_generation": assessmentGeneration, "structure_source_as_of": structureAsOf, "assessment_source_as_of": assessmentAsOf, "structures_retrieved_at": structureRetrieved, "assessment_retrieved_at": assessmentRetrieved, "facts": json.RawMessage(facts)})
}
