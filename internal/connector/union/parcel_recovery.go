package union

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/JustinMonty20/landman/internal/connector"
)

// ParcelFetchBudget limits requests and features for one explicit repair.
type ParcelFetchBudget struct{ Requests, Features int }

func DefaultParcelFetchBudget() *ParcelFetchBudget {
	return &ParcelFetchBudget{Requests: 200, Features: 10000}
}
func (b *ParcelFetchBudget) request(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if b == nil || b.Requests <= 0 {
		return fmt.Errorf("union repair: request budget exhausted")
	}
	b.Requests--
	return nil
}
func (b *ParcelFetchBudget) records(n int) error {
	if n > b.Features {
		return fmt.Errorf("union repair: feature budget exhausted")
	}
	b.Features -= n
	return nil
}

// FetchParcelParts checks the selected current OBJECTID, then reacquires the entire effective PID.
// Callers must share a budget and a two-minute fetch deadline across the attempt.
func (s *GISSource) FetchParcelParts(ctx context.Context, objectID int64, expectedPID string, budget *ParcelFetchBudget) ([]connector.RawRecord, error) {
	if objectID <= 0 || expectedPID == "" {
		return nil, fmt.Errorf("union repair: invalid selected identity")
	}
	now := time.Now()
	if err := budget.request(ctx); err != nil {
		return nil, err
	}
	selected, err := s.fetchBatch(ctx, ArcGISParams{Where: fmt.Sprintf("OBJECTID = %d", objectID), ReturnGeometry: true, ResultRecordCount: 2, Format: "json", OutFields: "*", OrderByFields: "OBJECTID"}, now)
	if err != nil {
		return nil, err
	}
	if err := budget.records(len(selected)); err != nil {
		return nil, err
	}
	if len(selected) != 1 {
		return nil, fmt.Errorf("union repair: selected OBJECTID missing or ambiguous")
	}
	id, err := ParcelRecordIdentity(selected[0])
	if err != nil {
		return nil, err
	}
	if id.ObjectID != objectID || id.PID != expectedPID {
		return nil, fmt.Errorf("union repair: selected source identity changed")
	}
	where := "PID = '" + strings.ReplaceAll(id.PID, "'", "''") + "'"
	if fallback, err := strconv.ParseInt(id.PID, 10, 64); err == nil && fallback > 0 && strconv.FormatInt(fallback, 10) == id.PID {
		where = fmt.Sprintf("(%s OR ((PID IS NULL OR PID = '') AND OBJECTID = %d))", where, fallback)
	}
	if err := budget.request(ctx); err != nil {
		return nil, err
	}
	count, err := s.queryTotalParcels(ctx, ArcGISParams{Where: where, ReturnCountOnly: true, Format: "json"})
	if err != nil {
		return nil, err
	}
	if count <= 0 || count > int64(budget.Features) {
		return nil, fmt.Errorf("union repair: invalid count or feature budget exhausted")
	}
	records := make([]connector.RawRecord, 0, int(count))
	seen := make(map[int64]bool, int(count))
	var previousObjectID int64
	for offset := 0; offset < int(count); offset += s.batchSize {
		if err := budget.request(ctx); err != nil {
			return nil, err
		}
		page, err := s.fetchBatch(ctx, ArcGISParams{Where: where, ReturnGeometry: true, ResultOffset: offset, ResultRecordCount: s.batchSize, Format: "json", OutFields: "*", OrderByFields: "OBJECTID"}, now)
		if err != nil {
			return nil, err
		}
		if err := budget.records(len(page)); err != nil {
			return nil, err
		}
		want := min(s.batchSize, int(count)-offset)
		if len(page) != want {
			return nil, fmt.Errorf("union repair: incomplete PID page")
		}
		for _, record := range page {
			part, err := ParcelRecordIdentity(record)
			if err != nil {
				return nil, err
			}
			if part.PID != id.PID || seen[part.ObjectID] || part.ObjectID <= previousObjectID {
				return nil, fmt.Errorf("union repair: conflicting or duplicate PID parts")
			}
			seen[part.ObjectID] = true
			previousObjectID = part.ObjectID
		}
		records = append(records, page...)
	}
	if !seen[objectID] {
		return nil, fmt.Errorf("union repair: selected OBJECTID absent from PID parts")
	}
	return records, nil
}
