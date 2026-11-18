package parcelstore

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/models"
	"github.com/JustinMonty20/landman/internal/parcelevidence"
	"github.com/jackc/pgx/v5"
	"github.com/twpayne/go-geom/encoding/ewkb"
)

var digest = regexp.MustCompile(`^[0-9a-f]{64}$`)

// StageEvidencePart keeps the complete original feature alongside the normalized part.
func (w *ShapeWriter) StageEvidencePart(ctx context.Context, parcel models.GISParcel, vacant bool, rawKey string, feature json.RawMessage) error {
	hash, err := parcelevidence.SourceHash(feature)
	if err != nil {
		return err
	}
	if err := w.StagePart(ctx, parcel, vacant, rawKey); err != nil {
		return err
	}
	tag, err := w.tx.Exec(ctx, `UPDATE parcel_import_parts SET source_feature=$1,source_hash=$2 WHERE ordinal=(SELECT max(ordinal) FROM parcel_import_parts)`, feature, hash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("missing staged evidence part")
	}
	return nil
}

// PublishEvidence serializes the complete candidate comparison, source links,
// history and current business rows in the same publication transaction.
func (w *ShapeWriter) PublishEvidence(ctx context.Context, quarantine *jobs.QuarantineStore, jobID string, selected []jobs.PendingRecord, manifest parcelevidence.Manifest, correction bool) (int, error) {
	if _, err := w.tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('union.parcel.evidence.publication'))`); err != nil {
		return 0, err
	}
	if manifest.ArtifactHashVersion != "ordered-response-bytes-v1" || !digest.MatchString(manifest.ArtifactHash) {
		return 0, fmt.Errorf("invalid artifact digest")
	}
	if _, err := w.tx.Exec(ctx, `ANALYZE parcel_import_parts, parcel_import_rejected`); err != nil {
		return 0, err
	}
	var observationID, publicationKind string
	attemptID, ok := jobs.AttemptID(ctx)
	if !ok {
		return 0, fmt.Errorf("parcel publication requires a job attempt")
	}
	var registered bool
	var originatingOutcome, originatingAttempt string
	registrationErr := w.tx.QueryRow(ctx, `SELECT c.artifact_hash=$2 AND c.manifest=$3::jsonb AND c.attempt_kind=$4,
		a.outcome,c.attempt_id::text FROM parcel_evidence_captures c JOIN job_attempts a ON a.id=c.attempt_id
		WHERE c.artifact_id=$1 AND a.job_id=$5`, manifest.ID, manifest.ArtifactHash, manifest, manifest.Kind, jobID).Scan(&registered, &originatingOutcome, &originatingAttempt)
	if registrationErr != nil && registrationErr != pgx.ErrNoRows {
		return 0, registrationErr
	}
	if registrationErr == nil && !registered {
		return 0, fmt.Errorf("snapshot differs registered capture")
	}
	if correction {
		var trusted []byte
		err := w.tx.QueryRow(ctx, `SELECT id::text,manifest FROM parcel_evidence_observations WHERE artifact_id=$1`, manifest.ID).Scan(&observationID, &trusted)
		if err != nil && err != pgx.ErrNoRows {
			return 0, err
		}
		if err == nil {
			saved, e := json.Marshal(manifest)
			if e != nil {
				return 0, e
			}
			var lhs, rhs any
			if e = json.Unmarshal(trusted, &lhs); e != nil {
				return 0, e
			}
			if e = json.Unmarshal(saved, &rhs); e != nil {
				return 0, e
			}
			left, _ := json.Marshal(lhs)
			right, _ := json.Marshal(rhs)
			if string(left) != string(right) {
				return 0, fmt.Errorf("snapshot differs from trusted observation manifest")
			}
		} else {
			var populated bool
			if e := w.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM parcels WHERE county_id='union' AND source_name='union_county_gis')`).Scan(&populated); e != nil {
				return 0, e
			}
			if populated {
				if !registered || originatingOutcome != "failed" {
					return 0, fmt.Errorf("populated replay requires its own failed registered source attempt")
				}
				publicationKind = "offline_retry"
			} else {
				var hasEvidence bool
				if e := w.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM parcel_evidence_observations WHERE county_id='union' AND source_name='union_county_gis')`).Scan(&hasEvidence); e != nil {
					return 0, e
				}
				if hasEvidence && (!registered || originatingOutcome != "failed") {
					return 0, fmt.Errorf("rebuild requires empty source evidence")
				}
				publicationKind = "offline_rebuild"
				if registered && originatingOutcome == "failed" {
					publicationKind = "offline_retry"
				}
			}
		}
	} else {
		if !registered || originatingOutcome != "running" || originatingAttempt != attemptID {
			return 0, fmt.Errorf("source publication requires its registered running capture")
		}
		publicationKind = "observed"
	}
	if observationID == "" && correction && publicationKind == "offline_retry" {
		var eligible bool
		if err := w.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM (SELECT DISTINCT p.source_parcel_id FROM parcel_import_parts p WHERE NOT EXISTS (SELECT 1 FROM parcel_import_rejected r WHERE r.source_parcel_id=p.source_parcel_id)) p LEFT JOIN parcel_evidence_current c ON c.county_id='union' AND c.source_name='union_county_gis' AND c.source_parcel_id=p.source_parcel_id LEFT JOIN parcel_evidence_observations o ON o.id=c.observation_id WHERE o.observed_at IS NULL OR o.observed_at < $1)`, manifest.CreatedAt).Scan(&eligible); err != nil {
			return 0, err
		}
		if !eligible {
			return 0, nil
		}
	}
	if observationID == "" {
		err := w.tx.QueryRow(ctx, `INSERT INTO parcel_evidence_observations(artifact_id,observed_at,attempt_kind,manifest,publication_kind,attempt_id) VALUES ($1,$2,$3,$4,$5,$6::uuid) RETURNING id::text`, manifest.ID, manifest.CreatedAt, manifest.Kind, manifest, publicationKind, attemptID).Scan(&observationID)
		if err != nil {
			return 0, err
		}
	}
	if _, err := w.tx.Exec(ctx, `CREATE TEMP TABLE parcel_evidence_groups ON COMMIT DROP AS SELECT p.source_parcel_id,min(p.ordinal) first_ordinal FROM parcel_import_parts p WHERE NOT EXISTS (SELECT 1 FROM parcel_import_rejected r WHERE r.source_parcel_id=p.source_parcel_id) GROUP BY p.source_parcel_id; CREATE UNIQUE INDEX ON parcel_evidence_groups(first_ordinal); ANALYZE parcel_evidence_groups`); err != nil {
		return 0, err
	}
	processed := 0
	var cursor int64
	for {
		rows, err := w.tx.Query(ctx, `SELECT source_parcel_id,first_ordinal FROM parcel_evidence_groups WHERE first_ordinal>$1 ORDER BY first_ordinal LIMIT 200`, cursor)
		if err != nil {
			return 0, err
		}
		type group struct {
			pid     string
			ordinal int64
		}
		batch := make([]group, 0, 200)
		for rows.Next() {
			var g group
			if err = rows.Scan(&g.pid, &g.ordinal); err != nil {
				break
			}
			batch = append(batch, g)
		}
		rows.Close()
		if err != nil {
			return 0, err
		}
		if err = rows.Err(); err != nil {
			return 0, err
		}
		if len(batch) == 0 {
			break
		}
		for _, g := range batch {
			pid := g.pid
			cursor = g.ordinal
			n, err := w.publishEvidencePID(ctx, pid, observationID, manifest.CreatedAt, manifest.ID, correction, publicationKind, attemptID)
			if err != nil {
				return 0, err
			}
			processed += n
			var keys []string
			keyRows, err := w.tx.Query(ctx, `SELECT raw_key FROM parcel_import_parts WHERE source_parcel_id=$1 ORDER BY ordinal`, pid)
			if err != nil {
				return 0, err
			}
			for keyRows.Next() {
				var key string
				if err = keyRows.Scan(&key); err != nil {
					break
				}
				keys = append(keys, key)
			}
			keyRows.Close()
			if err != nil {
				return 0, err
			}
			if err = keyRows.Err(); err != nil {
				return 0, err
			}
			if !correction || (publicationKind == "offline_retry" && n > 0) {
				if err = quarantine.ResolveObserved(ctx, w.tx, jobID, keys, selected); err != nil {
					return 0, err
				}
			}
		}
	}
	if correction && processed > 0 {
		if _, err := w.tx.Exec(ctx, `INSERT INTO parcel_derived_handoff(artifact_id) VALUES ($1) ON CONFLICT (artifact_id) DO UPDATE SET requested_at=now(),screening_pending=true,qualification_pending=true`, manifest.ID); err != nil {
			return 0, err
		}
	}
	if err := w.tx.Commit(ctx); err != nil {
		return 0, err
	}
	return processed, nil
}

func (w *ShapeWriter) publishEvidencePID(ctx context.Context, pid, observationID string, observedAt time.Time, artifactID string, correction bool, publicationKind, attemptID string) (int, error) {
	rows, err := w.tx.Query(ctx, `SELECT payload,likely_vacant,source_feature,source_hash FROM parcel_import_parts WHERE source_parcel_id=$1 ORDER BY ordinal`, pid)
	if err != nil {
		return 0, err
	}
	var last models.GISParcel
	var vacant bool
	var features []json.RawMessage
	count := 0
	sourceBytes := 0
	for rows.Next() {
		var payload, feature []byte
		var hash string
		var partVacant bool
		if err = rows.Scan(&payload, &partVacant, &feature, &hash); err != nil {
			break
		}
		if feature == nil || hash == "" {
			err = fmt.Errorf("missing source evidence for PID %s", pid)
			break
		}
		last = models.GISParcel{}
		if err = json.Unmarshal(payload, &last); err != nil {
			break
		}
		sourceBytes += len(feature)
		if sourceBytes > 64<<20 {
			err = fmt.Errorf("PID %s exceeds source evidence memory limit", pid)
			break
		}
		features = append(features, feature)
		_ = hash
		vacant = vacant || partVacant
		count++
	}
	rows.Close()
	if err != nil {
		return 0, err
	}
	if err = rows.Err(); err != nil {
		return 0, err
	}
	var shape []byte
	err = w.tx.QueryRow(ctx, `SELECT ST_AsEWKB(ST_Normalize(ST_Multi(ST_Union(ST_GeomFromEWKB(shape))))) FROM parcel_import_parts WHERE source_parcel_id=$1`, pid).Scan(&shape)
	if err != nil {
		return 0, err
	}
	geometryHash := parcelevidence.GeometryHash(shape)
	normalizedHash, err := parcelevidence.NormalizedHash(last, vacant, geometryHash)
	if err != nil {
		return 0, err
	}
	sourceHash, err := parcelevidence.SourcePartHash(features)
	if err != nil {
		return 0, err
	}
	var previousObservation, oldHash, oldGeometry, oldNormalizer, oldSourceHash, oldSourceVersion, oldHashVersion, oldGeometryVersion *string
	err = w.tx.QueryRow(ctx, `SELECT observation_id::text,normalized_hash,geometry_hash,normalizer_version,source_hash,source_hash_version,normalized_hash_version,geometry_hash_version FROM parcel_evidence_current WHERE county_id='union' AND source_name='union_county_gis' AND source_parcel_id=$1 FOR UPDATE`, pid).Scan(&previousObservation, &oldHash, &oldGeometry, &oldNormalizer, &oldSourceHash, &oldSourceVersion, &oldHashVersion, &oldGeometryVersion)
	if err != nil && err != pgx.ErrNoRows {
		return 0, err
	}
	if correction && publicationKind == "" {
		if previousObservation == nil {
			return 0, nil
		}
		var currentID string
		if err = w.tx.QueryRow(ctx, `SELECT artifact_id FROM parcel_evidence_observations WHERE id=$1::uuid`, *previousObservation).Scan(&currentID); err != nil {
			return 0, err
		}
		if currentID != artifactID {
			return 0, nil
		}
		observationID = *previousObservation
		if oldSourceHash == nil || *oldSourceHash != sourceHash || oldSourceVersion == nil || *oldSourceVersion != parcelevidence.SourceHashVersion {
			return 0, fmt.Errorf("trusted source hash mismatch for PID %s", pid)
		}
	} else if previousObservation != nil {
		var newer bool
		err = w.tx.QueryRow(ctx, `SELECT o.observed_at >= $2 FROM parcel_evidence_observations o WHERE o.id=$1::uuid`, *previousObservation, observedAt).Scan(&newer)
		if err != nil {
			return 0, err
		}
		if newer {
			if correction {
				return 0, nil
			}
			return 0, fmt.Errorf("source observation is not newer for %s", pid)
		}
	}
	unchanged := oldHash != nil && *oldHash == normalizedHash && oldHashVersion != nil && *oldHashVersion == parcelevidence.NormalizedHashVersion && oldGeometryVersion != nil && *oldGeometryVersion == parcelevidence.GeometryHashVersion
	if !unchanged && oldHash != nil && oldHashVersion != nil && *oldHashVersion != parcelevidence.NormalizedHashVersion && oldGeometry != nil && *oldGeometry == geometryHash && oldGeometryVersion != nil && *oldGeometryVersion == parcelevidence.GeometryHashVersion {
		unchanged, err = w.matchesStoredProjection(ctx, pid, last, shape, vacant)
		if err != nil {
			return 0, err
		}
	}
	if unchanged {
		if !correction || publicationKind == "offline_retry" || *oldHashVersion != parcelevidence.NormalizedHashVersion {
			if err = w.updateCurrentEvidence(ctx, pid, observationID, sourceHash, normalizedHash, geometryHash, features); err != nil {
				return 0, err
			}
		}
		return count, nil
	}
	var oldFields []byte
	err = w.tx.QueryRow(ctx, `SELECT jsonb_build_object('current_name_1',current_name_1,'current_name_2',current_name_2,'tax_name_1',tax_name_1,'tax_name_2',tax_name_2,'tax_owner_id_1',tax_owner_id_1,'tax_owner_id_2',tax_owner_id_2,'mapped_acres',mapped_acres,'gross_acres',gross_acres,'valued_acres',valued_acres,'valued_sqft',valued_sqft,'land_code',land_code,'land_type',land_type,'property_use',property_use,'subdivision',subdivision,'valuation_land',valuation_land,'valuation_improvement',valuation_improvement,'valuation_total',valuation_total,'valuation_assessed',valuation_assessed) FROM parcels WHERE county_id='union' AND source_name='union_county_gis' AND source_parcel_id=$1`, pid).Scan(&oldFields)
	if err != nil && err != pgx.ErrNoRows {
		return 0, err
	}
	old := json.RawMessage(oldFields)
	projection := last
	if err = parcelevidence.NormalizeStorageNumbers(&projection); err != nil {
		return 0, err
	}
	nextFields, err := json.Marshal(map[string]interface{}{"current_name_1": projection.Owner.CurrentName1, "current_name_2": projection.Owner.CurrentName2, "tax_name_1": projection.Owner.TaxName1, "tax_name_2": projection.Owner.TaxName2, "tax_owner_id_1": projection.Owner.TaxOwnerID1, "tax_owner_id_2": projection.Owner.TaxOwnerID2, "mapped_acres": historyNumber(projection.StorageNumbers["mapped_acres"], projection.Land.MappedAcres), "gross_acres": historyNumber(projection.StorageNumbers["gross_acres"], projection.Land.GrossAcres), "valued_acres": historyNumber(projection.StorageNumbers["VAL_AC"], projection.Land.ValuedAcres), "valued_sqft": historyNumber(projection.StorageNumbers["VAL_SQFT"], projection.Land.ValuedSqFt), "land_code": projection.Land.LandCode, "land_type": projection.Land.LandType, "property_use": projection.Land.PropertyUse, "subdivision": projection.Land.Subdivision, "valuation_land": historyNumber(projection.StorageNumbers["FMV_LAND"], projection.Valuation.Land), "valuation_improvement": historyNumber(projection.StorageNumbers["FMV_IMPRV"], projection.Valuation.Improvement), "valuation_total": historyNumber(projection.StorageNumbers["FMV_TOTAL"], projection.Valuation.Total), "valuation_assessed": historyNumber(projection.StorageNumbers["TOTVAL"], projection.Valuation.Assessed)})
	if err != nil {
		return 0, err
	}
	decoded, err := ewkb.Unmarshal(shape)
	if err != nil {
		return 0, err
	}
	last.Geometry.Shape = &decoded
	if err = w.SavePart(ctx, last, vacant); err != nil {
		return 0, err
	}
	if _, err = w.tx.Exec(ctx, `UPDATE parcels SET shape=ST_GeomFromEWKB($2),likely_vacant=$3,updated_at=now() WHERE county_id='union' AND source_name='union_county_gis' AND source_parcel_id=$1`, pid, shape, vacant); err != nil {
		return 0, err
	}
	cause := "source_observation"
	if previousObservation != nil && oldSourceHash != nil && *oldSourceHash == sourceHash && oldSourceVersion != nil && *oldSourceVersion == parcelevidence.SourceHashVersion {
		cause = "normalization_correction"
	} else if correction {
		cause = publicationKind
	}
	if _, err = w.tx.Exec(ctx, `INSERT INTO parcel_evidence_history(county_id,source_name,source_parcel_id,old_observation_id,new_observation_id,cause,source_hash_version,old_source_hash_version,normalizer_version,old_normalizer_version,old_source_hash,new_source_hash,old_normalized_hash,new_normalized_hash,old_geometry_hash,new_geometry_hash,old_fields,new_fields,attempt_id,normalized_hash_version,old_normalized_hash_version,geometry_hash_version,old_geometry_hash_version) VALUES ('union','union_county_gis',$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17::uuid,$18,$19,$20,$21)`, pid, previousObservation, observationID, cause, parcelevidence.SourceHashVersion, oldSourceVersion, parcelevidence.NormalizerVersion, oldNormalizer, oldSourceHash, sourceHash, oldHash, normalizedHash, oldGeometry, geometryHash, old, nextFields, attemptID, parcelevidence.NormalizedHashVersion, oldHashVersion, parcelevidence.GeometryHashVersion, oldGeometryVersion); err != nil {
		return 0, err
	}
	if err = w.updateCurrentEvidence(ctx, pid, observationID, sourceHash, normalizedHash, geometryHash, features); err != nil {
		return 0, err
	}
	return count, nil
}

const businessProjectionSQL = `SELECT (to_jsonb(p)-'updated_at'-'shape'||jsonb_build_object(
	'shape',encode(ST_AsEWKB(ST_Normalize(p.shape)),'hex'),
	'sales',coalesce((SELECT jsonb_agg(to_jsonb(s)-'updated_at' ORDER BY s.sequence)
	FROM parcel_sales s WHERE s.parcel_id=p.id),'[]'::jsonb)))::text
	FROM parcels p WHERE p.county_id='union' AND p.source_name='union_county_gis' AND p.source_parcel_id=$1`

// A savepoint lets PostgreSQL compare the entire stored projection, including
// sales and exact NUMERIC values, when two hash versions are incomparable.
func (w *ShapeWriter) matchesStoredProjection(ctx context.Context, pid string, candidate models.GISParcel, shape []byte, vacant bool) (bool, error) {
	var before, after string
	if err := w.tx.QueryRow(ctx, businessProjectionSQL, pid).Scan(&before); err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	decoded, err := ewkb.Unmarshal(shape)
	if err != nil {
		return false, err
	}
	candidate.Geometry.Shape = &decoded
	if _, err := w.tx.Exec(ctx, `SAVEPOINT parcel_projection_comparison`); err != nil {
		return false, err
	}
	partsSaved := w.partsSaved
	if err := w.SavePart(ctx, candidate, vacant); err != nil {
		return false, err
	}
	if _, err := w.tx.Exec(ctx, `UPDATE parcels SET shape=ST_GeomFromEWKB($2),likely_vacant=$3 WHERE county_id='union' AND source_name='union_county_gis' AND source_parcel_id=$1`, pid, shape, vacant); err != nil {
		return false, err
	}
	if err := w.tx.QueryRow(ctx, businessProjectionSQL, pid).Scan(&after); err != nil {
		return false, err
	}
	if _, err := w.tx.Exec(ctx, `ROLLBACK TO SAVEPOINT parcel_projection_comparison`); err != nil {
		return false, err
	}
	if _, err := w.tx.Exec(ctx, `RELEASE SAVEPOINT parcel_projection_comparison`); err != nil {
		return false, err
	}
	w.partsSaved = partsSaved
	return before == after, nil
}

func (w *ShapeWriter) updateCurrentEvidence(ctx context.Context, pid, observationID, sourceHash, normalizedHash, geometryHash string, features []json.RawMessage) error {
	sourceJSON, err := json.Marshal(features)
	if err != nil {
		return err
	}
	_, err = w.tx.Exec(ctx, `INSERT INTO parcel_evidence_current(county_id,source_name,source_parcel_id,observation_id,source_hash,source_hash_version,normalized_hash,normalizer_version,geometry_hash,source_features,normalized_hash_version,geometry_hash_version) VALUES ('union','union_county_gis',$1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(county_id,source_name,source_parcel_id) DO UPDATE SET observation_id=EXCLUDED.observation_id,source_hash=EXCLUDED.source_hash,source_hash_version=EXCLUDED.source_hash_version,normalized_hash=EXCLUDED.normalized_hash,normalizer_version=EXCLUDED.normalizer_version,geometry_hash=EXCLUDED.geometry_hash,source_features=EXCLUDED.source_features,normalized_hash_version=EXCLUDED.normalized_hash_version,geometry_hash_version=EXCLUDED.geometry_hash_version`, pid, observationID, sourceHash, parcelevidence.SourceHashVersion, normalizedHash, parcelevidence.NormalizerVersion, geometryHash, sourceJSON, parcelevidence.NormalizedHashVersion, parcelevidence.GeometryHashVersion)
	return err
}
