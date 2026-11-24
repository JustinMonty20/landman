// Package ingest coordinates immutable source capture and transactional publication.
// Source adapters own their typed records, identity rules, and business SQL.
package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Receipt binds the exact response bytes to the request and source revision.
type Receipt struct {
	Request          string `json:"request"`
	Revision         string `json:"revision,omitempty"`
	CRS              int    `json:"crs,omitempty"`
	HTTPStatus       int    `json:"http_status,omitempty"`
	ResponseComplete bool   `json:"response_complete,omitempty"`
	ArtifactKey      string `json:"artifact_key"`
	Size             int64  `json:"size"`
	SHA256           string `json:"sha256"`
}

// Manifest is the durable, ordered evidence for a single complete capture.
type Manifest struct {
	Source     string    `json:"source"`
	ObservedAt time.Time `json:"observed_at"`
	Version    int       `json:"version"`
	Receipts   []Receipt `json:"receipts"`
	Kind       string    `json:"kind,omitempty"`
	Count      int       `json:"count,omitempty"`
}

// VerifiedCapture cannot be constructed by an adapter; it is issued after
// checking every retained response against a persisted manifest.
type VerifiedCapture struct {
	manifest  Manifest
	store     taxartifact.Store
	artifacts []taxartifact.Artifact
}

func (v VerifiedCapture) Manifest() Manifest { return copyManifest(v.manifest) }

// Artifact returns a checked receipt for typed adapters; callers cannot mint one from a reference.
func (v VerifiedCapture) Artifact(i int) (taxartifact.Artifact, error) {
	if i < 0 || i >= len(v.artifacts) {
		return taxartifact.Artifact{}, fmt.Errorf("capture receipt %d out of range", i)
	}
	return v.artifacts[i], nil
}
func copyManifest(m Manifest) Manifest {
	m.Receipts = append([]Receipt(nil), m.Receipts...)
	return m
}

func (v VerifiedCapture) Open(ctx context.Context, i int) (io.ReadCloser, error) {
	if i < 0 || i >= len(v.artifacts) {
		return nil, fmt.Errorf("capture receipt %d out of range", i)
	}
	return v.store.Get(ctx, v.artifacts[i])
}

// Recorder retains exact responses using the selected artifact-store backend.
type Recorder struct {
	store            taxartifact.Store
	manifest         Manifest
	artifacts        []taxartifact.Artifact
	attemptID, jobID string
}

// BindAttempt makes a complete capture attributable to its original running job.
func (r *Recorder) BindAttempt(jobID, id, kind string, count int) error {
	if jobID == "" || id == "" || (kind != "full_scan" && kind != "reprocess") || count < 0 {
		return errors.New("invalid source attempt binding")
	}
	r.jobID, r.attemptID = jobID, id
	r.manifest.Kind, r.manifest.Count = kind, count
	return nil
}

func NewRecorder(store taxartifact.Store, source string, observedAt time.Time) (*Recorder, error) {
	if store == nil || source == "" || observedAt.IsZero() {
		return nil, errors.New("capture requires source, observation time and artifact store")
	}
	return &Recorder{store: store, manifest: Manifest{Source: source, ObservedAt: observedAt, Version: 1}}, nil
}
func (r *Recorder) Record(ctx context.Context, request, revision string, crs int, body io.Reader) error {
	return r.record(ctx, request, revision, crs, 0, false, body)
}

// RecordHTTP retains HTTP outcome alongside the exact response bytes.
func (r *Recorder) RecordHTTP(ctx context.Context, request, revision string, crs, status int, complete bool, body io.Reader) error {
	return r.record(ctx, request, revision, crs, status, complete, body)
}

func (r *Recorder) record(ctx context.Context, request, revision string, crs, status int, complete bool, body io.Reader) error {
	if request == "" || body == nil || crs < 0 || status < 0 || status > 599 {
		return errors.New("invalid capture response")
	}
	artifact, err := r.store.Put(ctx, r.manifest.Source, body)
	if err != nil {
		return err
	}
	if !artifact.Verified() || artifact.Mode() != r.store.Mode() {
		return errors.New("unverified capture response")
	}
	digest := artifact.SHA256()
	r.manifest.Receipts = append(r.manifest.Receipts, Receipt{Request: request, Revision: revision, CRS: crs, HTTPStatus: status, ResponseComplete: complete, ArtifactKey: artifact.Key(), Size: artifact.Size(), SHA256: hex.EncodeToString(digest[:])})
	r.artifacts = append(r.artifacts, artifact)
	return nil
}
func (r *Recorder) Manifest() Manifest { return copyManifest(r.manifest) }

// SetCount binds the source-reported complete capture count after the last receipt.
func (r *Recorder) SetCount(count int) error {
	if count < 0 {
		return errors.New("invalid capture count")
	}
	r.manifest.Count = count
	return nil
}

// Verify checks responses already retained by this recorder before preparation.
// Replay still obtains its manifest from the database, never from a caller.
func (r *Recorder) Verify(ctx context.Context) (VerifiedCapture, error) {
	return (Engine{Store: r.store}).verify(ctx, r.Manifest())
}

// Publisher prepares source-specific typed records before the engine begins a
// publication transaction. Apply performs all business DML in that transaction.
type Publisher interface {
	Prepare(context.Context, VerifiedCapture) error
	Apply(context.Context, pgx.Tx, Observation) error
}

// Observation carries the persisted identity of this publication.
type Observation struct {
	ID        string
	Replay    bool
	Published bool
	Manifest  Manifest
}

// ErrNoPublication leaves a verified capture registered without advancing freshness.
// A publisher returns it when a replay finds no eligible source observation.
var ErrNoPublication = errors.New("capture has no eligible publication")

// ErrObservationOnly commits an accepted observation without changing the
// published capture or active business freshness.
var ErrObservationOnly = errors.New("capture accepted without publication")

type Engine struct {
	Pool  *pgxpool.Pool
	Store taxartifact.Store
}

// Ingest seals evidence in a separate transaction before preparing or publishing.
// A failed prepare or publication leaves the capture available for replay.
func (e Engine) Ingest(ctx context.Context, recorder *Recorder, publisher Publisher) (string, error) {
	if recorder == nil || recorder.store != e.Store || publisher == nil {
		return "", errors.New("ingestion requires selected store, recorder and publisher")
	}
	log.Printf("[ingest] source=%s: Verify and register capture (%d responses)", recorder.manifest.Source, len(recorder.artifacts))
	id, err := e.Seal(ctx, recorder)
	if err != nil {
		return "", fmt.Errorf("source=%s: verify and register capture: %w", recorder.manifest.Source, err)
	}
	return id, e.publish(ctx, id, false, publisher)
}

// Seal registers a verified complete capture before any business write.
func (e Engine) Seal(ctx context.Context, recorder *Recorder) (string, error) {
	if e.Pool == nil || e.Store == nil || recorder == nil || recorder.store != e.Store || len(recorder.artifacts) == 0 {
		return "", errors.New("incomplete capture")
	}
	manifest := recorder.Manifest()
	if _, err := e.verify(ctx, manifest); err != nil {
		return "", err
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	var id string
	if recorder.attemptID != "" {
		err = e.Pool.QueryRow(ctx, `INSERT INTO ingest_captures(source_name,observed_at,manifest,manifest_sha256,storage_mode,attempt_id,attempt_kind)
  SELECT $1,$2,$3,$4,$5,id,$6 FROM job_attempts WHERE id=$7::uuid AND job_id=$8 AND outcome='running' AND attempt_kind=$6 RETURNING ingest_captures.id::text`, manifest.Source, manifest.ObservedAt, data, hash[:], e.Store.Mode(), manifest.Kind, recorder.attemptID, recorder.jobID).Scan(&id)
		if err == pgx.ErrNoRows {
			return "", errors.New("capture registration requires running source attempt")
		}
	} else {
		err = e.Pool.QueryRow(ctx, `INSERT INTO ingest_captures(source_name,observed_at,manifest,manifest_sha256,storage_mode) VALUES ($1,$2,$3,$4,$5) RETURNING id::text`, manifest.Source, manifest.ObservedAt, data, hash[:], e.Store.Mode()).Scan(&id)
	}
	return id, err
}

// Replay uses only a trusted database manifest and retained bytes. It never
// asks a source adapter to fetch a live response.
func (e Engine) Replay(ctx context.Context, id, jobID string, publisher Publisher) error {
	if jobID == "" {
		return errors.New("replay requires source job ID")
	}
	if attempt, ok := jobs.AttemptID(ctx); ok {
		if e.Pool == nil {
			return errors.New("replay requires database")
		}
		var registered string
		err := e.Pool.QueryRow(ctx, `INSERT INTO ingest_replay_attempts(capture_id,attempt_id)
SELECT c.id,a.id FROM ingest_captures c JOIN job_attempts a ON a.id=$2::uuid
WHERE c.id=$1::uuid AND a.job_id=$3 AND a.outcome='running' AND a.attempt_kind='replay'
RETURNING capture_id::text`, id, attempt, jobID).Scan(&registered)
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("replay requires running replay attempt and registered capture")
		}
		if err != nil {
			return err
		}
	}
	return e.publish(ctx, id, true, publisher)
}
func (e Engine) publish(ctx context.Context, id string, replay bool, publisher Publisher) (err error) {
	stage := "load capture manifest"
	defer func() {
		if err != nil {
			err = fmt.Errorf("capture=%s: %s: %w", id, stage, err)
		}
	}()
	if e.Pool == nil || e.Store == nil || publisher == nil {
		return errors.New("ingestion requires store, database and publisher")
	}
	var data, savedHash []byte
	var mode string
	err = e.Pool.QueryRow(ctx, `SELECT manifest,manifest_sha256,storage_mode FROM ingest_captures WHERE id=$1::uuid`, id).Scan(&data, &savedHash, &mode)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(data)
	if mode != e.Store.Mode() || string(savedHash) != string(hash[:]) {
		return errors.New("capture differs from registered manifest or store")
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	stage = "verify saved responses"
	log.Printf("[ingest] source=%s capture=%s replay=%t: Verify saved responses", manifest.Source, id, replay)
	verified, err := e.verify(ctx, manifest)
	if err != nil {
		return err
	}
	stage = "prepare source data"
	log.Printf("[ingest] source=%s capture=%s: Prepare source data", manifest.Source, id)
	if err := publisher.Prepare(ctx, verified); err != nil {
		return err
	}
	stage = "run publication transaction"
	log.Printf("[ingest] source=%s capture=%s: Run publication transaction", manifest.Source, id)
	err = e.transaction(ctx, manifest.Source, func(ctx context.Context, tx pgx.Tx) error {
		var published bool
		if err := tx.QueryRow(ctx, `SELECT published_at IS NOT NULL FROM ingest_captures WHERE id=$1::uuid FOR UPDATE`, id).Scan(&published); err != nil {
			return err
		}
		if published && !replay {
			return errors.New("capture already published")
		}
		if err := publisher.Apply(ctx, tx, Observation{ID: id, Replay: replay, Published: published, Manifest: manifest}); err != nil {
			if errors.Is(err, ErrNoPublication) {
				return ErrNoPublication
			}
			if errors.Is(err, ErrObservationOnly) {
				return nil
			}
			return err
		}
		if !published {
			if _, err := tx.Exec(ctx, `UPDATE ingest_captures SET published_at=clock_timestamp() WHERE id=$1::uuid`, id); err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, ErrNoPublication) {
		log.Printf("[ingest] source=%s capture=%s: No publication needed", manifest.Source, id)
		return nil
	}
	if err == nil {
		log.Printf("[ingest] source=%s capture=%s: Publication transaction committed", manifest.Source, id)
	}
	return err
}
func (e Engine) verify(ctx context.Context, manifest Manifest) (VerifiedCapture, error) {
	if manifest.Version != 1 || manifest.Source == "" || manifest.ObservedAt.IsZero() || len(manifest.Receipts) == 0 {
		return VerifiedCapture{}, errors.New("invalid capture manifest")
	}
	verified := VerifiedCapture{manifest: copyManifest(manifest), store: e.Store}
	for _, receipt := range manifest.Receipts {
		if receipt.Request == "" || receipt.CRS < 0 {
			return VerifiedCapture{}, errors.New("invalid capture receipt")
		}
		digest, err := hex.DecodeString(receipt.SHA256)
		if err != nil || len(digest) != sha256.Size {
			return VerifiedCapture{}, errors.New("invalid capture digest")
		}
		var sum [sha256.Size]byte
		copy(sum[:], digest)
		artifact, err := taxartifact.ReferenceForSource(manifest.Source, receipt.ArtifactKey, receipt.Size, sum)
		if err != nil {
			return VerifiedCapture{}, err
		}
		checked, err := taxartifact.VerifiedReference(ctx, e.Store, artifact)
		if err != nil {
			return VerifiedCapture{}, err
		}
		verified.artifacts = append(verified.artifacts, checked)
	}
	return verified, nil
}

func (e Engine) transaction(ctx context.Context, source string, apply func(context.Context, pgx.Tx) error) error {
	tx, err := e.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "ingest:"+source); err != nil {
		return fmt.Errorf("acquire publication lock: %w", err)
	}
	if err = apply(ctx, tx); err != nil {
		return fmt.Errorf("apply source data: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit publication: %w", err)
	}
	return nil
}

// ChangeCause distinguishes a new observation from a normalizer correction.
type ChangeCause string

const (
	SourceObservation       ChangeCause = "source_observation"
	NormalizationCorrection ChangeCause = "normalization_correction"
	OfflineRetry            ChangeCause = "offline_retry"
)

// Fingerprint versions are independent. A version mismatch never proves the
// source changed and requires an adapter's read-only projection comparison.
type Fingerprint struct{ Source, SourceVersion, Normalized, NormalizedVersion, Geometry, GeometryVersion string }

type Decision struct {
	Cause             ChangeCause
	WriteBusiness     bool
	CompareProjection bool
}

func Classify(previous, next Fingerprint, replay bool) Decision {
	if previous.NormalizedVersion != next.NormalizedVersion || previous.GeometryVersion != next.GeometryVersion {
		return Decision{CompareProjection: true}
	}
	if previous.Normalized == next.Normalized && previous.Geometry == next.Geometry {
		return Decision{}
	}
	if previous.Source != "" && previous.SourceVersion == next.SourceVersion && previous.Source == next.Source {
		return Decision{Cause: NormalizationCorrection, WriteBusiness: true}
	}
	if replay {
		return Decision{Cause: OfflineRetry, WriteBusiness: true}
	}
	return Decision{Cause: SourceObservation, WriteBusiness: true}
}
