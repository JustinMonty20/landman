package parceljob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/JustinMonty20/landman/internal/connector"
	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/parcelevidence"
	"github.com/JustinMonty20/landman/internal/parcelstore"
)

func ReprocessLimit(text string) (int, error) {
	n, err := strconv.Atoi(text)
	if err != nil || n < 1 || n > 100 {
		return 0, fmt.Errorf("reprocess limit must be between 1 and 100")
	}
	return n, nil
}

func stageParcelEvidenceRecords(ctx context.Context, writer *parcelstore.ShapeWriter, quarantine *jobs.QuarantineStore, records []connector.RawRecord, result *jobs.Result, receiptRef string, replay bool) error {
	for index, record := range records {
		id, err := union.ParcelRecordIdentity(record)
		if err != nil {
			return err
		}
		rawKey := strconv.FormatInt(id.ObjectID, 10)
		if err := writer.ObserveIdentity(ctx, rawKey, id.PID); err != nil {
			return err
		}
		if len(id.PID) > maxIndexedPIDBytes {
			if !replay {
				if err := persistInvalid(ctx, quarantine, record, &union.InvalidParcelRecord{Identity: id, Reason: unsupportedPIDReason}, fmt.Sprintf("%s#%d", receiptRef, index)); err != nil {
					return err
				}
			}
			result.Quarantined++
			continue
		}
		parcel, vacant, err := union.NormalizeParcel(record)
		if err != nil {
			var invalid *union.InvalidParcelRecord
			if !errors.As(err, &invalid) {
				return err
			}
			if !replay {
				if err := persistInvalid(ctx, quarantine, record, invalid, fmt.Sprintf("%s#%d", receiptRef, index)); err != nil {
					return err
				}
			}
			if err := writer.RejectPID(ctx, invalid.Identity.PID); err != nil {
				return err
			}
			result.Quarantined++
			continue
		}
		attrs, _ := record.RawData["attributes"].(map[string]interface{})
		if err := parcelevidence.QuantizeSourceNumbers(attrs, &parcel); err != nil {
			return err
		}
		feature := make(map[string]interface{}, len(record.RawData))
		for key, value := range record.RawData {
			if key != "source_declared_crs" {
				feature[key] = value
			}
		}
		payload, err := json.Marshal(feature)
		if err != nil {
			return err
		}
		if err = writer.StageEvidencePart(ctx, parcel, vacant, rawKey, payload); err != nil {
			return err
		}
	}
	return nil
}

const JobID = "union.gis.parcels"

// A larger natural key cannot be indexed safely by PostgreSQL B-trees.
// Preserve it in the receipt and quarantine identity, never in serving indexes.
const maxIndexedPIDBytes = 2048
const unsupportedPIDReason = "parcel identity exceeds indexed key limit"

func persistInvalid(ctx context.Context, quarantine *jobs.QuarantineStore, record connector.RawRecord, invalid *union.InvalidParcelRecord, reference string) error {
	payload, err := json.Marshal(map[string]interface{}{"attributes": map[string]interface{}{"PID": invalid.Identity.PID, "OBJECTID": invalid.Identity.ObjectID}})
	if err != nil {
		return err
	}
	if len(payload) > 8*1024 {
		payload = nil
	}
	if err := quarantine.Quarantine(ctx, jobs.QuarantineRecord{JobID: JobID, CountyID: "union", SourceName: "union_county_gis", RawRecordKey: strconv.FormatInt(invalid.Identity.ObjectID, 10), SourceParcelID: invalid.Identity.PID, Payload: payload, PayloadReference: reference, Reason: invalid.Reason}); err != nil {
		return fmt.Errorf("persist parcel quarantine: %w", err)
	}
	return nil
}

func quarantineCaptureBatch(ctx context.Context, quarantine *jobs.QuarantineStore, records []connector.RawRecord, reference string) error {
	for i, record := range records {
		id, err := union.ParcelRecordIdentity(record)
		if err != nil {
			return err
		}
		if len(id.PID) > maxIndexedPIDBytes {
			if err := persistInvalid(ctx, quarantine, record, &union.InvalidParcelRecord{Identity: id, Reason: unsupportedPIDReason}, fmt.Sprintf("%s#%d", reference, i)); err != nil {
				return err
			}
			continue
		}
		_, _, err = union.NormalizeParcel(record)
		if err == nil {
			continue
		}
		var invalid *union.InvalidParcelRecord
		if !errors.As(err, &invalid) {
			return err
		}
		if err := persistInvalid(ctx, quarantine, record, invalid, fmt.Sprintf("%s#%d", reference, i)); err != nil {
			return err
		}
	}
	return nil
}

// Job owns Union parcel capture, staging, repair and replay.
type Job struct {
	Root       string
	DSN        string
	Source     *union.GISSource
	Quarantine *jobs.QuarantineStore
}

func (j Job) newSnapshot(kind string) (*parcelevidence.Snapshot, error) {
	return parcelevidence.New(j.Root, kind)
}
func (j Job) Import(ctx context.Context) (jobs.Result, error) {
	snapshot, err := j.newSnapshot("full_scan")
	if err != nil {
		return jobs.Result{}, err
	}
	total, err := j.Source.CaptureParcelPages(ctx, snapshot.Save, func(records []connector.RawRecord) error {
		receipt := snapshot.Manifest.Receipts[len(snapshot.Manifest.Receipts)-1]
		return quarantineCaptureBatch(ctx, j.Quarantine, records, snapshot.Manifest.ID+"/"+receipt.Path+"@"+receipt.SHA256)
	})
	if err != nil {
		return jobs.Result{}, fmt.Errorf("capture parcels: %w", err)
	}
	if err = snapshot.Complete(total); err != nil {
		return jobs.Result{}, err
	}
	if err := parcelstore.RegisterCapture(ctx, j.DSN, snapshot.Manifest); err != nil {
		return jobs.Result{}, err
	}
	return j.publish(ctx, snapshot, nil, false)
}

// Replay verifies local evidence without contacting the source.
func (j Job) Replay(ctx context.Context, id string) (jobs.Result, error) {
	snapshot, err := parcelevidence.Open(j.Root, id)
	if err != nil {
		return jobs.Result{}, err
	}
	return j.publish(ctx, snapshot, nil, true)
}
func (j Job) publish(ctx context.Context, snapshot *parcelevidence.Snapshot, selected []jobs.PendingRecord, correction bool) (jobs.Result, error) {
	snapshot, err := parcelevidence.Open(j.Root, snapshot.Manifest.ID)
	if err != nil {
		return jobs.Result{}, err
	}
	if err = verifyParcelCapture(snapshot); err != nil {
		return jobs.Result{}, err
	}
	writer, err := parcelstore.NewShapeWriter(ctx, j.DSN)
	if err != nil {
		return jobs.Result{}, err
	}
	defer writer.Close()
	var result jobs.Result
	var count int
	for i, receipt := range snapshot.Manifest.Receipts {
		if receipt.DeclaredCRS == 0 {
			continue
		}
		if snapshot.Manifest.Kind == "reprocess" {
			u, err := url.Parse(receipt.Request)
			if err != nil {
				return result, err
			}
			if strings.HasPrefix(u.Query().Get("where"), "OBJECTID = ") {
				continue
			}
		}
		body, err := snapshot.Read(i)
		if err != nil {
			return result, err
		}
		records, err := union.DecodeParcelPage(body, snapshot.Manifest.CreatedAt, "union_county_gis")
		if err != nil {
			return result, err
		}
		count += len(records)
		if err = stageParcelEvidenceRecords(ctx, writer, j.Quarantine, records, &result, snapshot.Manifest.ID+"/"+receipt.Path+"@"+receipt.SHA256, true); err != nil {
			return result, err
		}
	}
	if count != snapshot.Manifest.Count {
		return result, fmt.Errorf("snapshot feature count %d differs from manifest %d", count, snapshot.Manifest.Count)
	}
	result.Processed, err = writer.PublishEvidence(ctx, j.Quarantine, JobID, selected, snapshot.Manifest, correction)
	return result, err
}

func (j Job) Reprocess(ctx context.Context, limit int) (jobs.Result, error) {
	if limit < 1 || limit > 100 {
		return jobs.Result{}, fmt.Errorf("reprocess limit must be between 1 and 100")
	}
	pending, err := j.Quarantine.Pending(ctx, JobID, limit)
	if err != nil {
		return jobs.Result{}, err
	}
	if len(pending) == 0 {
		return jobs.Result{}, nil
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	budget := union.DefaultParcelFetchBudget()
	snapshot, err := j.newSnapshot("reprocess")
	if err != nil {
		return jobs.Result{}, err
	}
	var result jobs.Result
	var captured int
	groups := make(map[string][]jobs.PendingRecord)
	var pids []string
	for _, entry := range pending {
		if len(groups[entry.PID]) == 0 {
			pids = append(pids, entry.PID)
		}
		groups[entry.PID] = append(groups[entry.PID], entry)
	}
	for _, pid := range pids {
		entries := groups[pid]
		oid, err := strconv.ParseInt(entries[0].RawRecordKey, 10, 64)
		if err != nil {
			return result, fmt.Errorf("reprocess: invalid stored OBJECTID")
		}
		records, err := j.Source.CaptureParcelParts(fetchCtx, oid, pid, budget, snapshot.Save, func(page []connector.RawRecord) error {
			receipt := snapshot.Manifest.Receipts[len(snapshot.Manifest.Receipts)-1]
			return quarantineCaptureBatch(fetchCtx, j.Quarantine, page, snapshot.Manifest.ID+"/"+receipt.Path+"@"+receipt.SHA256)
		})
		if err != nil {
			return result, err
		}
		present := make(map[string]bool, len(records))
		for _, record := range records {
			id, err := union.ParcelRecordIdentity(record)
			if err != nil {
				return result, err
			}
			present[strconv.FormatInt(id.ObjectID, 10)] = true
		}
		for _, entry := range entries {
			if !present[entry.RawRecordKey] {
				return result, fmt.Errorf("reprocess: selected OBJECTID missing from current PID")
			}
		}
		captured += len(records)
	}
	if err = snapshot.Complete(captured); err != nil {
		return result, err
	}
	if err := parcelstore.RegisterCapture(ctx, j.DSN, snapshot.Manifest); err != nil {
		return result, err
	}
	return j.publish(ctx, snapshot, pending, false)
}
