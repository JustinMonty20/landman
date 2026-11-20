package vacancy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"

	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const JobID = "union.gis.vacancy_fixture"
const captureSource = "union_vacancy_fixture"

// Import retains the exact offline bytes in the shared artifact registry before
// atomic typed publication. This path does not validate countywide coverage.
func Import(ctx context.Context, pool *pgxpool.Pool, path, artifactDir string) (jobs.Result, error) {
	var result jobs.Result
	attempt, ok := jobs.AttemptID(ctx)
	if !ok {
		return result, errors.New("vacancy import requires runner attempt")
	}
	info, err := os.Stat(path)
	if err != nil {
		return result, err
	}
	if !info.Mode().IsRegular() {
		return result, errors.New("vacancy snapshot must be a local regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer file.Close()
	log.Printf("[%s] Read local vacancy snapshot", JobID)
	s, _, err := ReadSnapshot(file)
	if err != nil {
		return result, fmt.Errorf("read vacancy snapshot: %w", err)
	}
	log.Printf("[%s] Retain snapshot: structures=%d assessments=%d", JobID, len(s.Structures), len(s.Assessments))
	for _, raw := range []string{s.Manifest.StructureURL, s.Manifest.AssessmentURL} {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
			return result, errors.New("snapshot provenance requires HTTPS URLs")
		}
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return result, err
	}
	store, err := taxartifact.NewLocal(artifactDir)
	if err != nil {
		return result, err
	}
	recorder, err := ingest.NewRecorder(store, captureSource, s.Manifest.RetrievedAt)
	if err != nil {
		return result, err
	}
	if err = recorder.Record(ctx, s.Manifest.StructureURL+" | "+s.Manifest.AssessmentURL, "", 4326, file); err != nil {
		return result, err
	}
	if err = recorder.BindAttempt(JobID, attempt, "full_scan", s.Manifest.ExpectedCount); err != nil {
		return result, err
	}
	_, err = (ingest.Engine{Pool: pool, Store: store}).Ingest(ctx, recorder, &publisher{})
	if err != nil {
		return result, err
	}
	result.Processed = s.Manifest.ExpectedCount
	return result, nil
}

type publisher struct {
	snapshot Snapshot
	hash     string
}

func (p *publisher) Prepare(ctx context.Context, capture ingest.VerifiedCapture) error {
	manifest := capture.Manifest()
	if manifest.Source != captureSource || len(manifest.Receipts) != 1 {
		return errors.New("unexpected vacancy capture")
	}
	r, err := capture.Open(ctx, 0)
	if err != nil {
		return err
	}
	defer r.Close()
	p.snapshot, p.hash, err = ReadSnapshot(r)
	if err != nil {
		return err
	}
	if manifest.Count != p.snapshot.Manifest.ExpectedCount || manifest.Receipts[0].Request != p.snapshot.Manifest.StructureURL+" | "+p.snapshot.Manifest.AssessmentURL {
		return errors.New("vacancy manifest does not match retained input")
	}
	return nil
}
func (p *publisher) Apply(ctx context.Context, tx pgx.Tx, observation ingest.Observation) error {
	s := p.snapshot
	var structuresID, assessmentsID string
	for _, source := range []struct {
		name, url string
		count     int
		id        *string
	}{
		{"vacancy_fixture_structures", s.Manifest.StructureURL, len(s.Structures), &structuresID},
		{"vacancy_fixture_assessed_value", s.Manifest.AssessmentURL, len(s.Assessments), &assessmentsID},
	} {
		err := tx.QueryRow(ctx, `INSERT INTO vacancy_evidence_generations(source_name,source_url,artifact_sha256,retrieved_at,effective_at,observed_count,validation_note)
   VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(source_name,artifact_sha256) DO UPDATE SET id=vacancy_evidence_generations.id RETURNING id::text`, source.name, source.url, p.hash, s.Manifest.RetrievedAt, s.Manifest.EffectiveAt, source.count, s.Manifest.ValidationNote).Scan(source.id)
		if err != nil {
			return fmt.Errorf("register generation: %w", err)
		}
	}
	for _, v := range s.Structures {
		_, err := tx.Exec(ctx, `INSERT INTO vacancy_structures(generation_id,feature_id,pin,status,shape)
    VALUES($1,$2,$3,$4,ST_Transform(ST_SetSRID(ST_GeomFromGeoJSON($5),4326),3857))
    ON CONFLICT(generation_id,feature_id) DO NOTHING`, structuresID, v.ID, v.PIN, v.Status, string(v.Geometry))
		if err != nil {
			return fmt.Errorf("structure %q: %w", v.ID, err)
		}
	}
	for _, v := range s.Assessments {
		_, err := tx.Exec(ctx, `INSERT INTO vacancy_assessment_rows(generation_id,row_id,parcel_number,assessment_year,category,building_value)
    VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(generation_id,row_id) DO NOTHING`, assessmentsID, v.ID, v.ParcelNumber, v.Year, v.Category, v.BuildingValue)
		if err != nil {
			return fmt.Errorf("assessment %q: %w", v.ID, err)
		}
	}
	return nil
}
