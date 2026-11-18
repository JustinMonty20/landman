package parceljob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
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

func stageParcelEvidenceRecords(ctx context.Context, writer *parcelstore.ShapeWriter, records []connector.RawRecord, result *jobs.Result) error {
	for _, record := range records {
		id, err := union.ParcelRecordIdentity(record)
		if err != nil {
			return err
		}
		rawKey := strconv.FormatInt(id.ObjectID, 10)
		if err := writer.ObserveIdentity(ctx, rawKey, id.PID); err != nil {
			return err
		}
		if len(id.PID) > maxIndexedPIDBytes {
			result.Quarantined++
			continue
		}
		parcel, vacant, err := union.NormalizeParcel(record)
		if err != nil {
			var invalid *union.InvalidParcelRecord
			if !errors.As(err, &invalid) {
				return err
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

func (j Job) Import(ctx context.Context) (jobs.Result, error) {
	store, err := j.sharedStore()
	if err != nil {
		return jobs.Result{}, err
	}
	recorder, err := j.newRecorder(store)
	if err != nil {
		return jobs.Result{}, err
	}
	total, err := j.Source.CaptureParcelPages(ctx, recordParcelResponse(ctx, recorder), func(records []connector.RawRecord) error {
		return j.captureQuarantine(ctx, recorder, records)
	})
	if err != nil {
		return jobs.Result{}, errors.Join(fmt.Errorf("capture parcels: %w", err), j.sealPartial(ctx, store, recorder, "full_scan"))
	}
	return j.publishShared(ctx, store, recorder, nil, total)
}

// Replay accepts only a registered shared capture; it never contacts the source.
func (j Job) Replay(ctx context.Context, id string) (jobs.Result, error) {
	return j.replayShared(ctx, id)
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
	store, err := j.sharedStore()
	if err != nil {
		return jobs.Result{}, err
	}
	recorder, err := j.newRecorder(store)
	if err != nil {
		return jobs.Result{}, err
	}
	budget := union.DefaultParcelFetchBudget()
	groups := make(map[string][]jobs.PendingRecord)
	var pids []string
	for _, entry := range pending {
		if len(groups[entry.PID]) == 0 {
			pids = append(pids, entry.PID)
		}
		groups[entry.PID] = append(groups[entry.PID], entry)
	}
	count := 0
	for _, pid := range pids {
		entries := groups[pid]
		oid, err := strconv.ParseInt(entries[0].RawRecordKey, 10, 64)
		if err != nil {
			return jobs.Result{}, fmt.Errorf("reprocess: invalid stored OBJECTID")
		}
		n, err := captureReprocessPID(fetchCtx, j, recorder, oid, pid, budget, entries)
		if err != nil {
			return jobs.Result{}, errors.Join(err, j.sealPartial(ctx, store, recorder, "reprocess"))
		}
		count += n
	}
	return j.publishShared(ctx, store, recorder, pending, count)
}
