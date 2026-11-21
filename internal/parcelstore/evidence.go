package parcelstore

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	return w.stagePart(ctx, parcel, vacant, rawKey, feature, &hash)
}

// ApplyEvidence serializes source evidence, history and current rows in the caller's publication transaction.
func (w *ShapeWriter) ApplyEvidence(ctx context.Context, quarantine *jobs.QuarantineStore, jobID string, selected []jobs.PendingRecord, manifest parcelevidence.Manifest, correction bool) (int, error) {
	if _, err := w.tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('union.parcel.evidence.publication'))`); err != nil {
		return 0, err
	}
	if verifyParcelGeometryRepair {
		var disposable bool
		if err := w.tx.QueryRow(ctx, `SELECT current_database() = 'landman_parcel_repair_test'`).Scan(&disposable); err != nil {
			return 0, fmt.Errorf("check repair verification database: %w", err)
		}
		if !disposable || !correction {
			return 0, fmt.Errorf("parcel repair verification requires replay in landman_parcel_repair_test")
		}
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
	registrationErr := w.tx.QueryRow(ctx, `SELECT c.source_name=$3 AND c.observed_at=$4 AND c.attempt_kind=$5,
   a.outcome,c.attempt_id::text FROM ingest_captures c JOIN job_attempts a ON a.id=c.attempt_id
   WHERE c.id=$1::uuid AND a.job_id=$2`, manifest.ID, jobID, manifest.Source, manifest.CreatedAt, manifest.Kind).Scan(&registered, &originatingOutcome, &originatingAttempt)
	if registrationErr != nil {
		return 0, registrationErr
	}
	if !registered {
		return 0, fmt.Errorf("capture differs from registered source attempt")
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
	// A refusal does not advance current source evidence, but it still fences older input.
	if _, err := w.tx.Exec(ctx, `CREATE TEMP TABLE parcel_evidence_groups ON COMMIT DROP AS
 WITH candidates AS (
   SELECT p.source_parcel_id,min(p.ordinal) ordinal,false invalid_geometry FROM parcel_import_parts p
   WHERE NOT EXISTS (SELECT 1 FROM parcel_import_rejected r WHERE r.source_parcel_id=p.source_parcel_id) GROUP BY p.source_parcel_id
   UNION ALL SELECT source_parcel_id,NULL,true FROM parcel_import_rejected WHERE invalid_geometry
 ) SELECT c.source_parcel_id,row_number() OVER (ORDER BY ordinal,c.source_parcel_id) first_ordinal,invalid_geometry
 FROM candidates c WHERE NOT $3::boolean OR NOT EXISTS (SELECT 1 FROM parcel_geometry_decisions d JOIN parcel_evidence_observations o USING(artifact_id)
 WHERE d.source_parcel_id=c.source_parcel_id AND d.outcome<>'repaired' AND d.artifact_id<>$2 AND o.observed_at >= $1)`, manifest.CreatedAt, manifest.ID, correction); err != nil {
		return 0, err
	}
	if _, err := w.tx.Exec(ctx, `CREATE UNIQUE INDEX ON parcel_evidence_groups(first_ordinal); ANALYZE parcel_evidence_groups`); err != nil {
		return 0, err
	}
	if !correction {
		var conflict bool
		if err := w.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM parcel_geometry_decisions d JOIN parcel_evidence_observations o USING(artifact_id) JOIN parcel_evidence_groups g USING(source_parcel_id) WHERE d.outcome<>'repaired' AND o.observed_at >= $1)`, manifest.CreatedAt).Scan(&conflict); err != nil {
			return 0, err
		}
		if conflict {
			return 0, fmt.Errorf("source observation is not newer than geometry refusal")
		}
	}
	if observationID == "" && correction && publicationKind == "offline_retry" {
		var eligible bool
		if err := w.tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM parcel_evidence_groups p LEFT JOIN parcel_evidence_current c ON c.county_id='union' AND c.source_name='union_county_gis' AND c.source_parcel_id=p.source_parcel_id LEFT JOIN parcel_evidence_observations o ON o.id=c.observation_id WHERE o.observed_at IS NULL OR o.observed_at < $1)`, manifest.CreatedAt).Scan(&eligible); err != nil {
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
	publication := evidencePublication{artifactID: manifest.ID, observedAt: manifest.CreatedAt, observationID: observationID, kind: publicationKind, attemptID: attemptID, replay: correction, quarantine: quarantine, jobID: jobID}
	processed, groupsChecked := 0, 0
	var cursor int64
	for {
		rows, err := w.tx.Query(ctx, `SELECT source_parcel_id,first_ordinal,invalid_geometry FROM parcel_evidence_groups WHERE first_ordinal>$1 ORDER BY first_ordinal LIMIT 200`, cursor)
		if err != nil {
			return 0, err
		}
		type group struct {
			pid             string
			ordinal         int64
			invalidGeometry bool
		}
		batch := make([]group, 0, 200)
		for rows.Next() {
			var g group
			if err = rows.Scan(&g.pid, &g.ordinal, &g.invalidGeometry); err != nil {
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
		pids := make([]string, 0, len(batch))
		for _, g := range batch {
			cursor = g.ordinal
			n, err := w.publishEvidencePID(ctx, g.pid, g.invalidGeometry, publication)
			if err != nil {
				return 0, fmt.Errorf("write parcel PID %q (group %d, publication order %d): %w", g.pid, groupsChecked+1, g.ordinal, err)
			}
			groupsChecked++
			if groupsChecked%10000 == 0 {
				log.Printf("[%s] capture=%s: Checked %d parcel groups in publication transaction", jobID, manifest.ID, groupsChecked)
			}
			processed += n
			if n > 0 {
				pids = append(pids, g.pid)
			}
		}
		// A replay has no retained selection tokens or original scan cutoff.
		// Leave unverifiable quarantine occurrences pending for a live repair.
		if !correction {
			keyRows, err := w.tx.Query(ctx, `SELECT raw_key FROM parcel_import_parts WHERE source_parcel_id=ANY($1::text[]) ORDER BY ordinal`, pids)
			if err != nil {
				return 0, err
			}
			var keys []string
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
			if err = quarantine.ResolveObserved(ctx, w.tx, jobID, keys, selected); err != nil {
				return 0, err
			}
		}
	}
	if w.geometryChanged {
		if _, err := w.tx.Exec(ctx, `INSERT INTO parcel_derived_handoff(artifact_id,screening_pending,qualification_pending) VALUES ($1,true,true) ON CONFLICT (artifact_id) DO UPDATE SET requested_at=clock_timestamp(),request_generation=parcel_derived_handoff.request_generation+1,screening_pending=true,qualification_pending=true`, manifest.ID); err != nil {
			return 0, err
		}
	}
	return processed, nil
}

type evidencePublication struct {
	artifactID, observationID, kind, attemptID, jobID string
	observedAt                                        time.Time
	replay                                            bool
	quarantine                                        *jobs.QuarantineStore
}

func (w *ShapeWriter) publishEvidencePID(ctx context.Context, pid string, invalidGeometry bool, publication evidencePublication) (int, error) {
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
	var sourceHash string
	if !invalidGeometry {
		sourceHash, err = parcelevidence.SourcePartHash(features)
		if err != nil {
			return 0, err
		}
	}
	var previousObservation, oldHash, oldGeometry, oldNormalizer, oldSourceHash, oldSourceVersion, oldHashVersion, oldGeometryVersion *string
	var shapeMissing bool
	err = w.tx.QueryRow(ctx, `SELECT p.shape IS NULL, observation_id::text,normalized_hash,geometry_hash,normalizer_version,source_hash,source_hash_version,normalized_hash_version,geometry_hash_version FROM parcel_evidence_current c JOIN parcels p USING(county_id,source_name,source_parcel_id) WHERE county_id='union' AND source_name='union_county_gis' AND source_parcel_id=$1 FOR UPDATE OF c`, pid).Scan(&shapeMissing, &previousObservation, &oldHash, &oldGeometry, &oldNormalizer, &oldSourceHash, &oldSourceVersion, &oldHashVersion, &oldGeometryVersion)
	if err != nil && err != pgx.ErrNoRows {
		return 0, err
	}
	if publication.replay && publication.kind == "" {
		if previousObservation == nil || *previousObservation != publication.observationID {
			return 0, nil
		}
		if oldSourceHash == nil || *oldSourceHash != sourceHash || oldSourceVersion == nil || *oldSourceVersion != parcelevidence.SourceHashVersion {
			return 0, fmt.Errorf("trusted source hash mismatch for PID %s", pid)
		}
	} else if previousObservation != nil {
		var newer bool
		err = w.tx.QueryRow(ctx, `SELECT o.observed_at >= $2 FROM parcel_evidence_observations o WHERE o.id=$1::uuid`, *previousObservation, publication.observedAt).Scan(&newer)
		if err != nil {
			return 0, err
		}
		if newer {
			if publication.replay {
				return 0, nil
			}
			return 0, fmt.Errorf("source observation is not newer for %s", pid)
		}
	}
	shape, err := w.applyGeometry(ctx, pid, invalidGeometry, count, publication)
	if err != nil || shape == nil {
		return 0, err
	}
	geometryHash := parcelevidence.GeometryHash(shape)
	normalizedHash, err := parcelevidence.NormalizedHash(last, vacant, geometryHash)
	if err != nil {
		return 0, err
	}
	unchanged := oldHash != nil && *oldHash == normalizedHash && oldHashVersion != nil && *oldHashVersion == parcelevidence.NormalizedHashVersion && oldGeometryVersion != nil && *oldGeometryVersion == parcelevidence.GeometryHashVersion
	if !unchanged && oldHash != nil && oldHashVersion != nil && *oldHashVersion != parcelevidence.NormalizedHashVersion && oldGeometry != nil && *oldGeometry == geometryHash && oldGeometryVersion != nil && *oldGeometryVersion == parcelevidence.GeometryHashVersion {
		unchanged, err = w.matchesStoredProjection(ctx, pid, last, shape, vacant)
		if err != nil {
			return 0, err
		}
	}
	// The last accepted fingerprint survives refusal. Restore the boundary even if all facts match.
	if shapeMissing && oldGeometry != nil && *oldGeometry == geometryHash {
		tag, err := w.tx.Exec(ctx, `UPDATE parcels SET shape=ST_GeomFromEWKB($2), updated_at=now() WHERE county_id='union' AND source_name='union_county_gis' AND source_parcel_id=$1 AND shape IS NULL`, pid, shape)
		if err != nil {
			return 0, err
		}
		if tag.RowsAffected() > 0 {
			w.geometryChanged = true
		}
	}
	if unchanged {
		var missingID models.ParcelID
		err = w.tx.QueryRow(ctx, `SELECT p.id FROM parcels p WHERE p.county_id='union' AND p.source_name='union_county_gis' AND p.source_parcel_id=$1 AND NOT EXISTS (SELECT 1 FROM parcel_owner_labels l WHERE l.parcel_id=p.id)`, pid).Scan(&missingID)
		if err == nil {
			if err = saveOwnerLabels(ctx, w.tx, missingID, last.Owner, last.Mailing); err != nil {
				return 0, err
			}
		} else if err != pgx.ErrNoRows {
			return 0, err
		}
		if !publication.replay || publication.kind == "offline_retry" || *oldHashVersion != parcelevidence.NormalizedHashVersion {
			if err = w.updateCurrentEvidence(ctx, pid, publication.observationID, sourceHash, normalizedHash, geometryHash, features); err != nil {
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
	if err = w.saveParcelAndSales(ctx, last, vacant); err != nil {
		return 0, err
	}
	if err = w.analyzeAfterSavedPart(ctx); err != nil {
		return 0, err
	}
	cause := "source_observation"
	if previousObservation != nil && oldSourceHash != nil && *oldSourceHash == sourceHash && oldSourceVersion != nil && *oldSourceVersion == parcelevidence.SourceHashVersion {
		cause = "normalization_correction"
	} else if publication.replay {
		cause = publication.kind
	}
	if _, err = w.tx.Exec(ctx, `INSERT INTO parcel_evidence_history(county_id,source_name,source_parcel_id,old_observation_id,new_observation_id,cause,source_hash_version,old_source_hash_version,normalizer_version,old_normalizer_version,old_source_hash,new_source_hash,old_normalized_hash,new_normalized_hash,old_geometry_hash,new_geometry_hash,old_fields,new_fields,attempt_id,normalized_hash_version,old_normalized_hash_version,geometry_hash_version,old_geometry_hash_version) VALUES ('union','union_county_gis',$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17::uuid,$18,$19,$20,$21)`, pid, previousObservation, publication.observationID, cause, parcelevidence.SourceHashVersion, oldSourceVersion, parcelevidence.NormalizerVersion, oldNormalizer, oldSourceHash, sourceHash, oldHash, normalizedHash, oldGeometry, geometryHash, old, nextFields, publication.attemptID, parcelevidence.NormalizedHashVersion, oldHashVersion, parcelevidence.GeometryHashVersion, oldGeometryVersion); err != nil {
		return 0, err
	}
	if err = w.updateCurrentEvidence(ctx, pid, publication.observationID, sourceHash, normalizedHash, geometryHash, features); err != nil {
		return 0, err
	}
	// A cached lookup can keep scanning the whole table as an empty table grows.
	if w.partsSaved == parcelAnalyzeInterval {
		if _, err := w.tx.Exec(ctx, `ANALYZE parcel_evidence_current (county_id, source_name, source_parcel_id)`); err != nil {
			return 0, fmt.Errorf("parcelstore: analyze current evidence keys: %w", err)
		}
	}
	if oldGeometry == nil || *oldGeometry != geometryHash {
		w.geometryChanged = true
	}
	return count, nil
}

// Compare the complete mapped serving projection without invoking business DML.
// PostgreSQL casts JSON decimal tokens to NUMERIC at the same column precision
// as the writer. Generated IDs and content timestamps are excluded.
func (w *ShapeWriter) matchesStoredProjection(ctx context.Context, pid string, candidate models.GISParcel, shape []byte, vacant bool) (bool, error) {
	p := candidate
	if err := parcelevidence.NormalizeStorageNumbers(&p); err != nil {
		return false, err
	}
	num := func(key string, fallback *float64) any { return historyNumber(p.StorageNumbers[key], fallback) }
	extras := p.SourceExtras
	if len(extras) == 0 {
		extras = json.RawMessage(`{}`)
	}
	fields := map[string]any{
		"county_id": p.Identity.CountyID, "county_code": p.Identity.CountyCode,
		"source_name": p.Identity.SourceName, "source_parcel_id": p.Identity.SourceParcelID,
		"parcel_number": p.Identity.ParcelNumber, "account_number": p.Identity.AccountNumber, "parcel_year": p.Identity.ParcelYear,
		"current_name_1": p.Owner.CurrentName1, "current_name_2": p.Owner.CurrentName2,
		"tax_name_1": p.Owner.TaxName1, "tax_name_2": p.Owner.TaxName2,
		"tax_owner_id_1": p.Owner.TaxOwnerID1, "tax_owner_id_2": p.Owner.TaxOwnerID2,
		"mailing_line_1": p.Mailing.Line1, "mailing_line_2": p.Mailing.Line2, "mailing_city": p.Mailing.City,
		"mailing_state": p.Mailing.State, "mailing_postal_code": p.Mailing.PostalCode,
		"situs_line_1": p.Situs.Line1, "situs_line_2": p.Situs.Line2, "situs_city": p.Situs.City,
		"situs_state": p.Situs.State, "situs_postal_code": p.Situs.PostalCode,
		"legal_description": p.Legal.Description, "plat_book": p.Legal.PlatBook, "plat_page": p.Legal.PlatPage,
		"deed_book": p.Legal.DeedBook, "deed_page": p.Legal.DeedPage,
		"land_code": p.Land.LandCode, "land_type": p.Land.LandType, "property_use": p.Land.PropertyUse,
		"subdivision":  p.Land.Subdivision,
		"mapped_acres": num("mapped_acres", p.Land.MappedAcres), "gross_acres": num("gross_acres", p.Land.GrossAcres),
		"valued_acres": num("VAL_AC", p.Land.ValuedAcres), "valued_sqft": num("VAL_SQFT", p.Land.ValuedSqFt),
		"year_built": p.Building.YearBuilt, "sqft": num("SQFT", p.Building.SqFt),
		"basement": p.Building.Basement, "quality_code": p.Building.QualityCode,
		"structure": p.Building.Structure, "structure_style": p.Building.StructureStyle,
		"valuation_land": num("FMV_LAND", p.Valuation.Land), "valuation_improvement": num("FMV_IMPRV", p.Valuation.Improvement),
		"valuation_total": num("FMV_TOTAL", p.Valuation.Total), "valuation_assessed": num("TOTVAL", p.Valuation.Assessed),
		"srid": defaultStorageSRID, "source_srid": p.Geometry.SourceSRID,
		"source_area": num("SHAPE.STArea()", p.Geometry.SourceArea), "source_length": num("SHAPE.STLength()", p.Geometry.SourceLength),
		"source_extras": extras, "likely_vacant": vacant,
	}
	sales := make([]map[string]any, 0, len(p.Sales))
	for _, sale := range p.Sales {
		var day any
		if sale.SaleDate != nil {
			day = sale.SaleDate.UTC().Format("2006-01-02")
		}
		sales = append(sales, map[string]any{
			"sequence": sale.Sequence, "sale_date": day, "og_sale_date": sale.OGSaleDate,
			"amount":    num(fmt.Sprintf("s%d_SALESAMT", sale.Sequence), sale.Amount),
			"deed_book": sale.DeedBook, "deed_page": sale.DeedPage, "deed_type": sale.DeedType,
			"deed_quality": sale.DeedQuality, "grantor": sale.Grantor, "grantor_2": sale.Grantor2,
		})
	}
	parcelJSON, err := json.Marshal(fields)
	if err != nil {
		return false, err
	}
	salesJSON, err := json.Marshal(sales)
	if err != nil {
		return false, err
	}
	var same bool
	err = w.tx.QueryRow(ctx, `SELECT
		(to_jsonb(p)-'id'-'created_at'-'updated_at'-'shape') =
		(to_jsonb(jsonb_populate_record(NULL::parcels,$2::jsonb))-'id'-'created_at'-'updated_at'-'shape')
		AND ST_Equals(p.shape,ST_GeomFromEWKB($4)) IS TRUE
		AND coalesce((SELECT jsonb_agg(to_jsonb(s)-'id'-'parcel_id'-'created_at'-'updated_at' ORDER BY s.sequence)
			FROM parcel_sales s WHERE s.parcel_id=p.id),'[]'::jsonb) =
		coalesce((SELECT jsonb_agg(to_jsonb(s)-'id'-'parcel_id'-'created_at'-'updated_at' ORDER BY s.sequence)
			FROM jsonb_populate_recordset(NULL::parcel_sales,$3::jsonb) s),'[]'::jsonb)
		FROM parcels p WHERE p.county_id='union' AND p.source_name='union_county_gis' AND p.source_parcel_id=$1`, pid, parcelJSON, salesJSON, shape).Scan(&same)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	return same, err
}

func (w *ShapeWriter) updateCurrentEvidence(ctx context.Context, pid, observationID, sourceHash, normalizedHash, geometryHash string, features []json.RawMessage) error {
	sourceJSON, err := json.Marshal(features)
	if err != nil {
		return err
	}
	_, err = w.tx.Exec(ctx, `INSERT INTO parcel_evidence_current(county_id,source_name,source_parcel_id,observation_id,source_hash,source_hash_version,normalized_hash,normalizer_version,geometry_hash,source_features,normalized_hash_version,geometry_hash_version) VALUES ('union','union_county_gis',$1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(county_id,source_name,source_parcel_id) DO UPDATE SET observation_id=EXCLUDED.observation_id,source_hash=EXCLUDED.source_hash,source_hash_version=EXCLUDED.source_hash_version,normalized_hash=EXCLUDED.normalized_hash,normalizer_version=EXCLUDED.normalizer_version,geometry_hash=EXCLUDED.geometry_hash,source_features=EXCLUDED.source_features,normalized_hash_version=EXCLUDED.normalized_hash_version,geometry_hash_version=EXCLUDED.geometry_hash_version`, pid, observationID, sourceHash, parcelevidence.SourceHashVersion, normalizedHash, parcelevidence.NormalizerVersion, geometryHash, sourceJSON, parcelevidence.NormalizedHashVersion, parcelevidence.GeometryHashVersion)
	return err
}
