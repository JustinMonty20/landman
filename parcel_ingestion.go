package main

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
	"github.com/JustinMonty20/landman/internal/parcelstore"
)

func reprocessLimit(text string) (int, error) {
	n, err := strconv.Atoi(text)
	if err != nil || n < 1 || n > 100 {
		return 0, fmt.Errorf("reprocess limit must be between 1 and 100")
	}
	return n, nil
}

func stageParcelRecords(ctx context.Context, writer *parcelstore.ShapeWriter, quarantine *jobs.QuarantineStore, records []connector.RawRecord, result *jobs.Result) error {
	for _, record := range records {
		id, err := union.ParcelRecordIdentity(record)
		if err != nil {
			return err
		}
		rawKey := strconv.FormatInt(id.ObjectID, 10)
		if err := writer.ObserveIdentity(ctx, rawKey, id.PID); err != nil {
			return err
		}
		parcel, vacant, err := union.NormalizeParcel(record)
		if err != nil {
			var invalid *union.InvalidParcelRecord
			if !errors.As(err, &invalid) {
				return err
			}
			payload, err := json.Marshal(record.RawData)
			if err != nil {
				return err
			}
			if err := quarantine.Quarantine(ctx, jobs.QuarantineRecord{JobID: parcelJobID, CountyID: "union", SourceName: "union_county_gis", RawRecordKey: strconv.FormatInt(invalid.Identity.ObjectID, 10), Payload: payload, Reason: invalid.Reason}); err != nil {
				return fmt.Errorf("persist parcel quarantine: %w", err)
			}
			if err := writer.RejectPID(ctx, invalid.Identity.PID); err != nil {
				return err
			}
			result.Quarantined++
			continue
		}
		if err := writer.StagePart(ctx, parcel, vacant, rawKey); err != nil {
			return err
		}
	}
	return nil
}

func importParcels(ctx context.Context, dsn string, source *union.GISSource, quarantine *jobs.QuarantineStore) (jobs.Result, error) {
	writer, err := parcelstore.NewShapeWriter(ctx, dsn)
	if err != nil {
		return jobs.Result{}, err
	}
	defer writer.Close()
	var result jobs.Result
	if err := source.FetchBatches(ctx, func(records []connector.RawRecord) error {
		return stageParcelRecords(ctx, writer, quarantine, records, &result)
	}); err != nil {
		return result, fmt.Errorf("import parcels: %w", err)
	}
	result.Processed, err = writer.Publish(ctx, quarantine, parcelJobID, nil)
	return result, err
}

func reprocessParcels(ctx context.Context, dsn string, source *union.GISSource, quarantine *jobs.QuarantineStore, limit int) (jobs.Result, error) {
	if limit < 1 || limit > 100 {
		return jobs.Result{}, fmt.Errorf("reprocess limit must be between 1 and 100")
	}
	pending, err := quarantine.Pending(ctx, parcelJobID, limit)
	if err != nil {
		return jobs.Result{}, err
	}
	if len(pending) == 0 {
		return jobs.Result{}, nil
	}
	fetchCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	budget := union.DefaultParcelFetchBudget()
	writer, err := parcelstore.NewShapeWriter(ctx, dsn)
	if err != nil {
		return jobs.Result{}, err
	}
	defer writer.Close()
	var result jobs.Result
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
		records, err := source.FetchParcelParts(fetchCtx, oid, pid, budget)
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
		if err := stageParcelRecords(ctx, writer, quarantine, records, &result); err != nil {
			return result, err
		}
	}
	result.Processed, err = writer.Publish(ctx, quarantine, parcelJobID, pending)
	return result, err
}
