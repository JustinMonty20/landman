package union

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"

	"github.com/JustinMonty20/landman/internal/jobs"
)

const AssessmentRangeLimit = 2048
const AssessmentRowLimit int64 = 500000

func AssessmentWhere(lo, hi int64) string {
	return fmt.Sprintf("OBJECTID >= %d AND OBJECTID <= %d", lo, hi)
}

// CaptureAssessmentRanges plans the complete range tree before requesting a feature page.
func (s *GISSource) CaptureAssessmentRanges(ctx context.Context, base *url.URL, total int64, save func(string, []byte, int, int, bool) error) error {
	label := "[union.gis.assessed_value] Assessed Value (layer 0)"
	if attempt, ok := jobs.AttemptID(ctx); ok {
		label += " attempt=" + attempt
	}
	log.Printf("%s: Capture %d source rows by OBJECTID range", label, total)
	receipts := 1
	var verifiedRows int64
	planning := true
	var census assessmentPlanningProgress
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
		if receipts%10 == 0 && !planning {
			stage := "Count ranges"
			if q.Get("resultRecordCount") != "" {
				stage = "Read range page"
			}
			log.Printf("%s: %s: checked %d/%d rows; requests=%d/%d", label, stage, verifiedRows, total, receipts, AssessmentRangeLimit)
		}
		return body, err
	}
	count := func(q url.Values) (int64, error) {
		raw, err := query(q)
		if err != nil {
			return 0, err
		}
		return AssessmentCount(raw)
	}
	program, err := planAssessment(total, count, func(progress assessmentPlanningProgress) {
		census = progress
		if progress.requests%10 == 0 {
			log.Printf("%s: Count ranges: checked %d/%d rows; requests=%d/%d; resolved leaves=%d; resolved rows=%d", label, verifiedRows, total, progress.requests, AssessmentRangeLimit, progress.resolvedLeaves, progress.resolvedRows)
		}
	})
	planning = false
	if err != nil {
		return err
	}
	log.Printf("%s: Admitted count plan: receipts=%d/%d; count requests=%d; resolved leaves=%d; resolved rows=%d; checked %d/%d rows", label, program.receipts, AssessmentRangeLimit, census.requests, census.resolvedLeaves, census.resolvedRows, verifiedRows, total)
	err = ReadAssessmentProgram(program, query, count, func([]byte) error { return nil }, func(n int64) {
		verifiedRows += n
		log.Printf("%s: Range checked: %d/%d rows; requests=%d/%d", label, verifiedRows, total, receipts, AssessmentRangeLimit)
	})
	if err != nil {
		return err
	}
	log.Printf("%s: Capture complete: checked %d/%d rows; requests=%d/%d", label, verifiedRows, total, receipts, AssessmentRangeLimit)
	return nil
}
