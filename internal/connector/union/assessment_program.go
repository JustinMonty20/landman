package union

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"

	"github.com/tidwall/gjson"
)

// AssessmentProtocolRevision identifies count-first, admitted range captures.
const AssessmentProtocolRevision = "assessment-count-first-v1"

type assessmentRange struct{ low, high, count int64 }
type assessmentNode struct {
	rangeEvidence assessmentRange
	left, right   *assessmentNode
}

type AssessmentProgram struct {
	root     *assessmentNode
	total    int64
	receipts int
}

func AssessmentCount(raw []byte) (int64, error) {
	v := gjson.GetBytes(raw, "count")
	if !gjson.ValidBytes(raw) || gjson.GetBytes(raw, "error").Exists() || gjson.GetBytes(raw, "exceededTransferLimit").Bool() || v.Type != gjson.Number || v.Int() < 0 || v.Int() > AssessmentRowLimit || v.Raw != strconv.FormatInt(v.Int(), 10) {
		return 0, errors.New("invalid assessment partition count")
	}
	return v.Int(), nil
}

func assessmentCountQuery(where string) url.Values {
	return url.Values{"f": {"json"}, "where": {where}, "returnCountOnly": {"true"}}
}
func assessmentPageQuery(where string) url.Values {
	return url.Values{"f": {"json"}, "where": {where}, "returnGeometry": {"false"}, "outFields": {"*"}, "resultRecordCount": {"2000"}}
}

// PlanAssessment counts the root and every child exactly once, without requesting pages.
// The returned program is an admission token; a failed census never returns one.
func PlanAssessment(total int64, count func(url.Values) (int64, error)) (AssessmentProgram, error) {
	if total < 0 || total > AssessmentRowLimit {
		return AssessmentProgram{}, errors.New("assessment count exceeds safe gate")
	}
	const initialGlobalCountReceipts = 1
	receipts := initialGlobalCountReceipts
	query := func(lo, hi int64) (int64, error) {
		if receipts >= AssessmentRangeLimit {
			return 0, errors.New("assessment partition receipt limit")
		}
		n, err := count(assessmentCountQuery(AssessmentWhere(lo, hi)))
		if err != nil {
			return 0, err
		}
		receipts++
		return n, nil
	}
	rootCount, err := query(0, math.MaxInt64)
	if err != nil {
		return AssessmentProgram{}, err
	}
	if rootCount != total {
		return AssessmentProgram{}, errors.New("assessment partition count drift")
	}
	var build func(assessmentRange) (*assessmentNode, error)
	build = func(r assessmentRange) (*assessmentNode, error) {
		node := &assessmentNode{rangeEvidence: r}
		if r.count <= 2000 {
			return node, nil
		}
		if r.low == r.high {
			return nil, errors.New("indivisible assessment range exceeds page limit")
		}
		mid := r.low + (r.high-r.low)/2
		left, err := query(r.low, mid)
		if err != nil {
			return nil, err
		}
		right, err := query(mid+1, r.high)
		if err != nil {
			return nil, err
		}
		if left > r.count || right > r.count || left+right != r.count {
			return nil, errors.New("assessment partition sum mismatch")
		}
		node.left, err = build(assessmentRange{r.low, mid, left})
		if err != nil {
			return nil, err
		}
		node.right, err = build(assessmentRange{mid + 1, r.high, right})
		if err != nil {
			return nil, err
		}
		return node, nil
	}
	root, err := build(assessmentRange{0, math.MaxInt64, total})
	if err != nil {
		return AssessmentProgram{}, err
	}
	var pages int
	var leaves func(*assessmentNode)
	leaves = func(n *assessmentNode) {
		if n.left != nil {
			leaves(n.left)
			leaves(n.right)
		} else if n.rangeEvidence.count > 0 {
			pages++
		}
	}
	leaves(root)
	required := receipts + 2*pages + 1
	if required > AssessmentRangeLimit {
		return AssessmentProgram{}, fmt.Errorf("assessment partition receipt limit: need %d, limit %d", required, AssessmentRangeLimit)
	}
	return AssessmentProgram{root: root, total: total, receipts: required}, nil
}

// emit receives validated feature bytes in source order, including exact duplicates.
func ReadAssessmentProgram(program AssessmentProgram, page func(url.Values) ([]byte, error), count func(url.Values) (int64, error), emit func([]byte) error, verified func(int64)) error {
	if program.root == nil || emit == nil {
		return errors.New("assessment program not admitted")
	}
	var walk func(*assessmentNode) error
	walk = func(node *assessmentNode) error {
		if node.left != nil {
			if err := walk(node.left); err != nil {
				return err
			}
			return walk(node.right)
		}
		r := node.rangeEvidence
		if r.count == 0 {
			return nil
		}
		where := AssessmentWhere(r.low, r.high)
		raw, err := page(assessmentPageQuery(where))
		if err != nil {
			return err
		}
		features, err := DecodeAssessmentPage(raw)
		if err != nil {
			return fmt.Errorf("decode assessment page for OBJECTID range %d–%d: %w", r.low, r.high, err)
		}
		if gjson.GetBytes(raw, "error").Exists() || gjson.GetBytes(raw, "exceededTransferLimit").Bool() || int64(len(features)) != r.count {
			return fmt.Errorf("assessment page incomplete for OBJECTID range %d–%d: got %d rows, expected %d", r.low, r.high, len(features), r.count)
		}
		for i, feature := range features {
			oid, err := AssessmentObjectID(feature)
			if err != nil {
				return fmt.Errorf("read assessment identity in range %d–%d, row %d: %w", r.low, r.high, i+1, err)
			}
			if oid < r.low || oid > r.high {
				return fmt.Errorf("assessment OBJECTID %d at row %d is outside range %d–%d", oid, i+1, r.low, r.high)
			}
			if err := emit(feature); err != nil {
				return err
			}
		}
		after, err := count(assessmentCountQuery(where))
		if err != nil {
			return err
		}
		if after != r.count {
			return fmt.Errorf("assessment partition %d–%d count drift: got %d after capture, expected %d", r.low, r.high, after, r.count)
		}
		if verified != nil {
			verified(r.count)
		}
		return nil
	}
	if err := walk(program.root); err != nil {
		return err
	}
	final, err := count(assessmentCountQuery("1=1"))
	if err != nil {
		return err
	}
	if final != program.total {
		return errors.New("assessment total count drift")
	}
	return nil
}
