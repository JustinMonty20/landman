package parceljob

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/parcelevidence"
	"github.com/JustinMonty20/landman/internal/parcelstore"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func (j Job) sharedStore() (taxartifact.Store, error) { return taxartifact.NewLocal(j.Root) }

func (j Job) newRecorder(store taxartifact.Store) (*ingest.Recorder, error) {
	return ingest.NewRecorder(store, "union_county_gis", time.Now().UTC().Truncate(time.Microsecond))
}

func recordParcelResponse(ctx context.Context, recorder *ingest.Recorder) func(string, []byte, int) error {
	return func(request string, body []byte, crs int) error {
		return recorder.Record(ctx, request, "", crs, bytes.NewReader(body))
	}
}

func (j Job) captureQuarantine(ctx context.Context, recorder *ingest.Recorder, records []connector.RawRecord) error {
	receipt := recorder.Manifest().Receipts[len(recorder.Manifest().Receipts)-1]
	return quarantineCaptureBatch(ctx, j.Quarantine, records, receipt.ArtifactKey+"@"+receipt.SHA256)
}

func (j Job) publishShared(ctx context.Context, store taxartifact.Store, recorder *ingest.Recorder, selected []jobs.PendingRecord, count int) (jobs.Result, error) {
	pool, err := pgxpool.New(ctx, j.DSN)
	if err != nil {
		return jobs.Result{}, err
	}
	defer pool.Close()
	attempt, ok := jobs.AttemptID(ctx)
	if !ok {
		return jobs.Result{}, fmt.Errorf("parcel capture requires a running attempt")
	}
	kind := "full_scan"
	if selected != nil {
		kind = "reprocess"
	}
	if err = recorder.BindAttempt(JobID, attempt, kind, count); err != nil {
		return jobs.Result{}, err
	}
	publisher := &sharedPublisher{quarantine: j.Quarantine, selected: selected}
	_, err = (ingest.Engine{Pool: pool, Store: store}).Ingest(ctx, recorder, publisher)
	if err == nil {
		log.Printf("[%s] Parcel publication complete: processed=%d quarantined=%d", JobID, publisher.result.Processed, publisher.result.Quarantined)
	}
	return publisher.result, err
}

// sealPartial binds all retained responses even when a later page or parser fails.
// An incomplete capture is never published: replay verifies full page membership.
func (j Job) sealPartial(ctx context.Context, store taxartifact.Store, recorder *ingest.Recorder, kind string) error {
	if len(recorder.Manifest().Receipts) == 0 {
		return nil
	}
	attempt, ok := jobs.AttemptID(ctx)
	if !ok {
		return fmt.Errorf("partial capture requires a running attempt")
	}
	if err := recorder.BindAttempt(JobID, attempt, kind, 0); err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, j.DSN)
	if err != nil {
		return err
	}
	defer pool.Close()
	_, err = (ingest.Engine{Pool: pool, Store: store}).Seal(ctx, recorder)
	return err
}

type sharedPublisher struct {
	quarantine *jobs.QuarantineStore
	selected   []jobs.PendingRecord
	verified   ingest.VerifiedCapture
	manifest   parcelevidence.Manifest
	result     jobs.Result
}

func (p *sharedPublisher) Prepare(ctx context.Context, verified ingest.VerifiedCapture) error {
	m := verified.Manifest()
	if m.Source != "union_county_gis" || (m.Kind != "full_scan" && m.Kind != "reprocess") || m.Count < 0 {
		return fmt.Errorf("invalid parcel capture metadata")
	}
	evidence := parcelevidence.Manifest{Version: 1, Source: m.Source, Kind: m.Kind, Count: m.Count, CreatedAt: m.ObservedAt, ArtifactHashVersion: "ordered-response-bytes-v1"}
	h := sha256.New()
	for _, receipt := range m.Receipts {
		evidence.Receipts = append(evidence.Receipts, parcelevidence.Receipt{Path: receipt.ArtifactKey, Request: receipt.Request, SHA256: receipt.SHA256, DeclaredCRS: receipt.CRS, Validated: true})
	}
	if err := verifyParcelResponses(evidence, func(i int) ([]byte, error) {
		body, err := readVerifiedParcelPage(ctx, verified, i)
		if err != nil {
			return nil, err
		}
		if err := appendParcelHash(h, body); err != nil {
			return nil, err
		}
		evidence.Receipts[i].Bytes = len(body)
		return body, nil
	}); err != nil {
		return err
	}
	evidence.ArtifactHash = hex.EncodeToString(h.Sum(nil))
	p.manifest = evidence
	p.verified = verified
	return nil
}
func readVerifiedParcelPage(ctx context.Context, verified ingest.VerifiedCapture, i int) ([]byte, error) {
	r, err := verified.Open(ctx, i)
	if err != nil {
		return nil, err
	}
	body, readErr := io.ReadAll(io.LimitReader(r, 32<<20+1))
	closeErr := r.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(body) == 0 || len(body) > 32<<20 {
		return nil, fmt.Errorf("parcel receipt outside evidence size limit")
	}
	return body, nil
}

func parcelPublicationPage(kind string, receipt parcelevidence.Receipt) (bool, error) {
	if receipt.DeclaredCRS == 0 {
		return false, nil
	}
	if kind != "reprocess" {
		return true, nil
	}
	u, err := url.Parse(receipt.Request)
	if err != nil {
		return false, err
	}
	return !strings.HasPrefix(u.Query().Get("where"), "OBJECTID = "), nil
}

func appendParcelHash(h hash.Hash, body []byte) error {
	if _, err := fmt.Fprintf(h, "%d:", len(body)); err != nil {
		return err
	}
	_, err := h.Write(body)
	return err
}
func (p *sharedPublisher) Apply(ctx context.Context, tx pgx.Tx, observation ingest.Observation) error {
	p.manifest.ID = observation.ID
	writer, err := parcelstore.NewShapeWriterTx(ctx, tx)
	if err != nil {
		return err
	}
	log.Printf("[%s] capture=%s: Stage %d parcel features", JobID, observation.ID, p.manifest.Count)
	count := 0
	for i, receipt := range p.manifest.Receipts {
		publish, err := parcelPublicationPage(p.manifest.Kind, receipt)
		if err != nil {
			return err
		}
		if !publish {
			continue
		}
		body, err := readVerifiedParcelPage(ctx, p.verified, i)
		if err != nil {
			return err
		}
		records, err := union.DecodeParcelPage(body, p.manifest.CreatedAt, p.manifest.Source)
		if err != nil {
			return err
		}
		count += len(records)
		if err := stageParcelEvidenceRecords(ctx, writer, records, &p.result); err != nil {
			return fmt.Errorf("stage parcel receipt %d/%d: %w", i+1, len(p.manifest.Receipts), err)
		}
		if i%10 == 0 || count == p.manifest.Count {
			log.Printf("[%s] capture=%s: Staged %d/%d parcel features", JobID, observation.ID, count, p.manifest.Count)
		}
	}
	if count != p.manifest.Count {
		return fmt.Errorf("snapshot feature count %d differs from manifest %d", count, p.manifest.Count)
	}
	log.Printf("[%s] capture=%s: Publish complete parcel groups", JobID, observation.ID)
	p.result.Processed, err = writer.ApplyEvidence(ctx, p.quarantine, JobID, p.selected, p.manifest, observation.Replay)
	if err != nil {
		return fmt.Errorf("publish complete parcel groups: %w", err)
	}
	if observation.Published {
		return nil
	}
	var recorded bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM parcel_evidence_observations WHERE artifact_id=$1)`, observation.ID).Scan(&recorded); err != nil {
		return err
	}
	if !recorded {
		return ingest.ErrNoPublication
	}
	return nil
}

func (j Job) replayShared(ctx context.Context, id string) (jobs.Result, error) {
	store, err := j.sharedStore()
	if err != nil {
		return jobs.Result{}, err
	}
	pool, err := pgxpool.New(ctx, j.DSN)
	if err != nil {
		return jobs.Result{}, err
	}
	defer pool.Close()
	publisher := &sharedPublisher{quarantine: j.Quarantine}
	if err := (ingest.Engine{Pool: pool, Store: store}).Replay(ctx, id, JobID, publisher); err != nil {
		return jobs.Result{}, err
	}
	return publisher.result, nil
}

func captureReprocessPID(ctx context.Context, j Job, recorder *ingest.Recorder, oid int64, pid string, budget *union.ParcelFetchBudget, entries []jobs.PendingRecord) (int, error) {
	records, err := j.Source.CaptureParcelParts(ctx, oid, pid, budget, recordParcelResponse(ctx, recorder), func(page []connector.RawRecord) error {
		return j.captureQuarantine(ctx, recorder, page)
	})
	if err != nil {
		return 0, err
	}
	present := make(map[string]bool, len(records))
	for _, record := range records {
		id, err := union.ParcelRecordIdentity(record)
		if err != nil {
			return 0, err
		}
		present[strconv.FormatInt(id.ObjectID, 10)] = true
	}
	for _, entry := range entries {
		if !present[entry.RawRecordKey] {
			return 0, fmt.Errorf("reprocess: selected OBJECTID missing from current PID")
		}
	}
	return len(records), nil
}
