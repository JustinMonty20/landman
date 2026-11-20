package fema

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twpayne/go-geom/encoding/ewkb"
)

const JobID = "union.fema"
const sourceName = "fema_union"

type Job struct {
	Pool      *pgxpool.Pool
	Artifacts taxartifact.Store
}

func (j Job) Import(ctx context.Context, catalogPath, archivePath string) (jobs.Result, error) {
	attempt, ok := jobs.AttemptID(ctx)
	if !ok {
		return jobs.Result{}, errors.New("FEMA import requires a running job attempt")
	}
	log.Print("[union.fema] Read local FEMA catalog")
	catalog, err := regularFile(catalogPath)
	if err != nil {
		return jobs.Result{}, fmt.Errorf("open FEMA catalog: %w", err)
	}
	defer catalog.Close()
	body, err := io.ReadAll(io.LimitReader(catalog, catalogLimit+1))
	if err != nil {
		return jobs.Result{}, err
	}
	if len(body) > catalogLimit {
		return jobs.Result{}, errors.New("FEMA catalog exceeds size limit")
	}
	product, err := selectUnion(body)
	if err != nil {
		return jobs.Result{}, err
	}
	if filepath.Base(archivePath) != product.Filename {
		return jobs.Result{}, errors.New("ZIP filename does not match the effective FEMA catalog product")
	}
	log.Printf("[union.fema] Retain effective product %s (product ID %d, archive %s)", product.Name, product.ID, product.Filename)
	archive, err := regularFile(archivePath)
	if err != nil {
		return jobs.Result{}, fmt.Errorf("open FEMA archive: %w", err)
	}
	defer archive.Close()
	recorder, err := ingest.NewRecorder(j.Artifacts, sourceName, time.Now().UTC())
	if err != nil {
		return jobs.Result{}, err
	}
	if err := recorder.BindAttempt(JobID, attempt, string(jobs.AttemptFullScan), 0); err != nil {
		return jobs.Result{}, err
	}
	rev := fmt.Sprint(product.ID)
	if err := recorder.Record(ctx, catalogURL, rev, 0, bytes.NewReader(body)); err != nil {
		return jobs.Result{}, err
	}
	if err := recorder.Record(ctx, archiveBase+product.Filename, rev, 0, &boundedReader{r: archive, remaining: archiveLimit}); err != nil {
		return jobs.Result{}, err
	}
	p := &publication{}
	_, err = (ingest.Engine{Pool: j.Pool, Store: j.Artifacts}).Ingest(ctx, recorder, p)
	return jobs.Result{Processed: p.processed}, err
}

func (j Job) Replay(ctx context.Context, id string) (jobs.Result, error) {
	p := &publication{}
	err := (ingest.Engine{Pool: j.Pool, Store: j.Artifacts}).Replay(ctx, id, JobID, p)
	return jobs.Result{Processed: p.processed}, err
}

func regularFile(filename string) (*os.File, error) {
	info, err := os.Stat(filename)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("FEMA inputs must be regular files")
	}
	return os.Open(filename)
}

type boundedReader struct {
	r         io.Reader
	remaining int64
}

func (r *boundedReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p[:min(int64(len(p)), r.remaining+1)])
	r.remaining -= int64(n)
	if r.remaining < 0 {
		return 0, errors.New("FEMA archive exceeds size limit")
	}
	return n, err
}

type publication struct {
	product     Product
	release     Release
	archiveHash string
	processed   int
}

func (p *publication) Prepare(ctx context.Context, capture ingest.VerifiedCapture) error {
	m := capture.Manifest()
	if m.Source != sourceName || len(m.Receipts) != 2 || m.Receipts[0].Request != catalogURL || m.Receipts[0].Size > catalogLimit || m.Receipts[1].Size > archiveLimit {
		return errors.New("invalid FEMA capture manifest")
	}
	reader, err := capture.Open(ctx, 0)
	if err != nil {
		return err
	}
	data, readErr := io.ReadAll(reader)
	if err := errors.Join(readErr, reader.Close()); err != nil {
		return err
	}
	product, err := selectUnion(data)
	if err != nil {
		return err
	}
	rev := fmt.Sprint(product.ID)
	if m.Receipts[1].Request != archiveBase+product.Filename || m.Receipts[0].Revision != rev || m.Receipts[1].Revision != rev {
		return errors.New("FEMA capture does not match catalog product")
	}
	rc, err := capture.Open(ctx, 1)
	if err != nil {
		return err
	}
	defer rc.Close()
	file, err := os.CreateTemp("", "landman-fema-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	size, err := io.Copy(file, rc)
	if err != nil {
		return err
	}
	if size != m.Receipts[1].Size {
		return errors.New("FEMA archive size mismatch")
	}
	p.release, err = decodeArchive(ctx, file, size)
	if err != nil {
		return err
	}
	p.product = product
	p.archiveHash = m.Receipts[1].SHA256
	return nil
}

func (p *publication) Apply(ctx context.Context, tx pgx.Tx, o ingest.Observation) error {
	if o.Published {
		return ingest.ErrNoPublication
	}
	issued := time.UnixMilli(p.product.IssueMillis).UTC()
	var activeID, activeHash string
	var activeProduct int64
	var activeIssue, latest time.Time
	err := tx.QueryRow(ctx, `SELECT h.capture_id::text,r.archive_sha256,r.product_id,r.issued_at,h.latest_observed_at FROM fema_release_head h JOIN fema_releases r ON r.capture_id=h.capture_id WHERE h.study_id='37179C'`).Scan(&activeID, &activeHash, &activeProduct, &activeIssue, &latest)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if activeID != "" {
		if !o.Manifest.ObservedAt.After(latest) || issued.Before(activeIssue) {
			if o.Replay {
				return ingest.ErrNoPublication
			}
			return errors.New("FEMA product or observation predates the current release")
		}
		if issued.Equal(activeIssue) {
			if activeHash != p.archiveHash || activeProduct != p.product.ID {
				return errors.New("conflicting archive for the current FEMA issue date")
			}
			if o.Replay {
				return ingest.ErrNoPublication
			}
			if _, err := tx.Exec(ctx, `UPDATE fema_release_head SET latest_observation_id=$1,latest_observed_at=$2 WHERE study_id='37179C'`, o.ID, o.Manifest.ObservedAt); err != nil {
				return err
			}
			return ingest.ErrObservationOnly
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO fema_releases(capture_id,study_id,product_id,issued_at,catalog_effective_at,index_effective_date,database_revision_date,archive_sha256,source_srid,observed_at) VALUES($1,'37179C',$2,$3,$4,$5,$6,$7,4269,$8)`, o.ID, p.product.ID, issued, time.UnixMilli(p.product.EffectiveMillis).UTC(), p.release.IndexEffective, p.release.DatabaseRevision, p.archiveHash, o.Manifest.ObservedAt)
	if err != nil {
		return err
	}
	batch := &pgx.Batch{}
	for _, z := range p.release.Zones {
		shape, err := ewkb.Marshal(z.Shape, ewkb.NDR)
		if err != nil {
			return err
		}
		batch.Queue(zoneInsert, o.ID, z.ID, shape, z.Code, z.Subtype, z.SFHA)
	}
	for _, panel := range p.release.Panels {
		shape, err := ewkb.Marshal(panel.Shape, ewkb.NDR)
		if err != nil {
			return err
		}
		batch.Queue(panelInsert, o.ID, panel.ID, shape, panel.Number, panel.Effective)
	}
	log.Printf("[union.fema] capture=%s: Store %d flood zones and %d map panels", o.ID, len(p.release.Zones), len(p.release.Panels))
	results := tx.SendBatch(ctx, batch)
	for i := 0; i < batch.Len(); i++ {
		tag, e := results.Exec()
		if e != nil || tag.RowsAffected() != 1 {
			closeErr := results.Close()
			var kind, id string
			if i < len(p.release.Zones) {
				kind, id = "flood zone", p.release.Zones[i].ID
			} else {
				kind, id = "map panel", p.release.Panels[i-len(p.release.Zones)].ID
			}
			if e != nil {
				return errors.Join(fmt.Errorf("store FEMA %s %q (row %d/%d): %w", kind, id, i+1, batch.Len(), e), closeErr)
			}
			return errors.Join(fmt.Errorf("FEMA %s %q (row %d/%d) failed the repair limit", kind, id, i+1, batch.Len()), closeErr)
		}
	}
	if err := results.Close(); err != nil {
		return err
	}
	log.Printf("[union.fema] capture=%s: Calculate panel coverage", o.ID)
	if _, err := tx.Exec(ctx, coverageInsert, o.ID); err != nil {
		return fmt.Errorf("calculate FEMA panel coverage: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO fema_release_head(study_id,capture_id,latest_observation_id,latest_observed_at) VALUES('37179C',$1,$1,$2) ON CONFLICT (study_id) DO UPDATE SET capture_id=EXCLUDED.capture_id,latest_observation_id=EXCLUDED.latest_observation_id,latest_observed_at=EXCLUDED.latest_observed_at`, o.ID, o.Manifest.ObservedAt)
	if err == nil {
		p.processed = len(p.release.Zones) + len(p.release.Panels)
	}
	return err
}

const repair = `WITH input AS (SELECT ST_GeomFromEWKB($3::bytea) AS raw), candidate AS MATERIALIZED (
 SELECT raw, NOT ST_IsValid(raw) AS repaired, ST_Multi(ST_CollectionExtract(ST_MakeValid(raw),3)) AS fixed FROM input
), measured AS MATERIALIZED (
 SELECT fixed,repaired, CASE WHEN repaired THEN abs(ST_Area(ST_Transform(raw,2264))-ST_Area(ST_Transform(fixed,2264)))*0.3048006096012192^2 ELSE 0 END AS delta FROM candidate
) `
const zoneInsert = repair + `INSERT INTO fema_zones(capture_id,source_id,zone_code,zone_subtype,sfha_flag,shape,repaired,repair_delta_m2) SELECT $1,$2,$4,$5,$6,fixed,repaired,delta FROM measured WHERE NOT ST_IsEmpty(fixed) AND ST_IsValid(fixed) AND ST_Area(fixed)>0 AND delta<=1`
const panelInsert = repair + `INSERT INTO fema_panels(capture_id,source_id,panel_number,effective_date,shape,repaired,repair_delta_m2) SELECT $1,$2,$4,$5,fixed,repaired,delta FROM measured WHERE NOT ST_IsEmpty(fixed) AND ST_IsValid(fixed) AND ST_Area(fixed)>0 AND delta<=1`
const coverageInsert = `WITH extents AS (
 SELECT (SELECT ST_Union(shape) FROM fema_panels WHERE capture_id=$1) AS panels,
        (SELECT ST_Union(shape) FROM fema_zones WHERE capture_id=$1) AS mapped
) INSERT INTO fema_coverage(capture_id,panel_extent,mapped_extent,panel_gap)
 SELECT $1,ST_Multi(panels),ST_Multi(mapped),ST_Multi(ST_CollectionExtract(ST_Difference(panels,mapped),3)) FROM extents`
