package union

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net/url"
	"strconv"

	"github.com/tidwall/gjson"
)

// AssessmentRangeLimit bounds the number of retained partition and page receipts.
const AssessmentRangeLimit = 2048

// AssessmentRowLimit bounds a full assessed-value capture and its partition counts.
const AssessmentRowLimit int64 = 500000

func AssessmentWhere(lo, hi int64) string {
	return fmt.Sprintf("OBJECTID >= %d AND OBJECTID <= %d", lo, hi)
}

// CaptureAssessmentRanges partitions numeric OBJECTIDs without treating them as row keys.
// Count checks detect observed drift, not a county snapshot or live query support.
func (s *GISSource) CaptureAssessmentRanges(ctx context.Context, base *url.URL, total int64, save func(string, []byte, int, int, bool) error) error {
	const label = "[union.gis.assessed_value] Assessed Value (layer 0)"
	log.Printf("%s: Capture %d source rows by OBJECTID range", label, total)
	var processed int64
	receipts := 1 // the initial global count
	query := func(q url.Values) ([]byte, error) {
		if receipts >= AssessmentRangeLimit {
			return nil, errors.New("assessment partition receipt limit")
		}
		base.RawQuery = q.Encode()
		request := base.String()
		body, status, err := s.getVacancy(ctx, request)
		if len(body) != 0 {
			if e := save(request, body, 0, status, err == nil); e != nil {
				return nil, e
			}
		}
		receipts++
		if receipts%10 == 0 {
			stage := "Count ranges"
			if q.Get("returnCountOnly") != "true" {
				stage = "Read range page"
			}
			log.Printf("%s: %s: checked %d/%d rows; requests=%d/%d", label, stage, processed, total, receipts, AssessmentRangeLimit)
		}
		return body, err
	}
	count := func(where string) (int64, error) {
		body, err := query(url.Values{"f": {"json"}, "where": {where}, "returnCountOnly": {"true"}})
		if err != nil {
			return 0, err
		}
		v := gjson.GetBytes(body, "count")
		if gjson.GetBytes(body, "error").Exists() || gjson.GetBytes(body, "exceededTransferLimit").Bool() || v.Type != gjson.Number || v.Raw != strconv.FormatInt(v.Int(), 10) || v.Int() < 0 || v.Int() > AssessmentRowLimit {
			return 0, errors.New("invalid assessment partition count")
		}
		return v.Int(), nil
	}
	var walk func(int64, int64, int64) error
	walk = func(lo, hi, expected int64) error {
		where := AssessmentWhere(lo, hi)
		n, err := count(where)
		if err != nil {
			return err
		}
		if n != expected {
			return errors.New("assessment partition count drift")
		}
		if n > 2000 {
			if lo == hi {
				return errors.New("indivisible assessment range exceeds page limit")
			}
			mid := lo + (hi-lo)/2
			left, err := count(AssessmentWhere(lo, mid))
			if err != nil {
				return err
			}
			rightExpected := n - left
			if rightExpected < 0 {
				return errors.New("assessment partition count drift")
			}
			// The left count is checked again at its leaf or subtree entry.
			if err := walk(lo, mid, left); err != nil {
				return err
			}
			return walk(mid+1, hi, rightExpected)
		}
		body, err := query(url.Values{"f": {"json"}, "where": {where}, "returnGeometry": {"false"}, "outFields": {"*"}, "resultRecordCount": {"2000"}})
		if err != nil {
			return err
		}
		features, decodeErr := DecodeAssessmentPage(body)
		if decodeErr != nil {
			return fmt.Errorf("decode assessment page for OBJECTID range %d–%d: %w", lo, hi, decodeErr)
		}
		if gjson.GetBytes(body, "error").Exists() || gjson.GetBytes(body, "exceededTransferLimit").Bool() || int64(len(features)) != n {
			return fmt.Errorf("assessment page incomplete for OBJECTID range %d–%d: got %d rows, expected %d", lo, hi, len(features), n)
		}
		for i, feature := range features {
			oid, err := AssessmentObjectID(feature)
			if err != nil {
				return fmt.Errorf("read assessment identity in range %d–%d, row %d: %w", lo, hi, i+1, err)
			}
			if oid < lo || oid > hi {
				return fmt.Errorf("assessment OBJECTID %d at row %d is outside range %d–%d", oid, i+1, lo, hi)
			}
		}
		after, err := count(where)
		if err != nil {
			return err
		}
		if after != n {
			return fmt.Errorf("assessment partition %d–%d count drift: got %d after capture, expected %d", lo, hi, after, n)
		}
		previous := processed
		processed += n
		if processed/10000 > previous/10000 || (n > 0 && processed == total) {
			log.Printf("%s: Range checked: %d/%d rows; requests=%d/%d", label, processed, total, receipts, AssessmentRangeLimit)
		}
		return nil
	}
	if err := walk(0, math.MaxInt64, total); err != nil {
		return err
	}
	log.Printf("%s: Verify final county count: checked %d/%d rows; requests=%d/%d", label, processed, total, receipts, AssessmentRangeLimit)
	final, err := count("1=1")
	if err != nil {
		return err
	}
	if final != total {
		return errors.New("assessment total count drift")
	}
	log.Printf("%s: Capture complete: checked %d/%d rows; requests=%d/%d", label, processed, total, receipts, AssessmentRangeLimit)
	return nil
}
