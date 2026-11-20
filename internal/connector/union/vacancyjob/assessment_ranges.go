package vacancyjob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"strconv"
	"strings"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/tidwall/gjson"
)

func (p *publisher) prepareAssessmentRanges(ctx context.Context, capture ingest.VerifiedCapture) error {
	m := capture.Manifest()
	if len(m.Receipts) > union.AssessmentRangeLimit {
		return errors.New("assessment partition receipt limit")
	}
	index := 0
	var bytesRead int64
	var target string
	next := func(q url.Values) ([]byte, error) {
		if index >= len(m.Receipts) {
			return nil, errors.New("missing assessment partition receipt")
		}
		r := m.Receipts[index]
		u, err := url.Parse(r.Request)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || !strings.HasSuffix(u.Path, "/Property_Tax_Live/Assessed_Value/MapServer/0/query") || u.RawQuery != q.Encode() || r.CRS != 0 || r.HTTPStatus < 200 || r.HTTPStatus >= 300 || !r.ResponseComplete {
			return nil, errors.New("unsupported assessment partition receipt")
		}
		endpoint := u.Scheme + "://" + u.Host + u.Path
		if target == "" {
			target = endpoint
			p.sourceURL = endpoint
		} else if target != endpoint {
			return nil, errors.New("assessment partition target drift")
		}
		body, err := capture.Open(ctx, index)
		if err != nil {
			return nil, err
		}
		raw, readErr := io.ReadAll(io.LimitReader(body, 32<<20+1))
		if err = errors.Join(readErr, body.Close()); err != nil {
			return nil, err
		}
		index++
		bytesRead += int64(len(raw))
		if len(raw) > 32<<20 || bytesRead > union.VacancyCaptureByteLimit || !gjson.ValidBytes(raw) || gjson.GetBytes(raw, "error").Exists() || gjson.GetBytes(raw, "exceededTransferLimit").Bool() {
			return nil, errors.New("invalid assessment partition response")
		}
		return raw, nil
	}
	count := func(where string) (int64, error) {
		raw, err := next(url.Values{"f": {"json"}, "where": {where}, "returnCountOnly": {"true"}})
		if err != nil {
			return 0, err
		}
		v := gjson.GetBytes(raw, "count")
		if v.Type != gjson.Number || v.Raw != strconv.FormatInt(v.Int(), 10) || v.Int() < 0 || v.Int() > 250000 {
			return 0, errors.New("invalid assessment partition count")
		}
		return v.Int(), nil
	}
	total, err := count("1=1")
	if err != nil {
		return err
	}
	var walk func(int64, int64, int64) error
	walk = func(lo, hi, expected int64) error {
		where := union.AssessmentWhere(lo, hi)
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
			left, err := count(union.AssessmentWhere(lo, mid))
			if err != nil {
				return err
			}
			if left > n {
				return errors.New("assessment partition sum mismatch")
			}
			if err := walk(lo, mid, left); err != nil {
				return err
			}
			return walk(mid+1, hi, n-left)
		}
		raw, err := next(url.Values{"f": {"json"}, "where": {where}, "returnGeometry": {"false"}, "outFields": {"*"}, "resultRecordCount": {"2000"}})
		if err != nil {
			return err
		}
		features, err := union.DecodeAssessmentPage(raw)
		if err != nil || int64(len(features)) != n {
			return errors.New("assessment partition page count mismatch")
		}
		for i, feature := range features {
			oid, err := union.AssessmentObjectID(feature)
			if err != nil {
				return fmt.Errorf("read assessment identity at partition %d–%d row %d: %w", lo, hi, i+1, err)
			}
			if oid < lo || oid > hi {
				return fmt.Errorf("assessment OBJECTID %d outside partition %d–%d", oid, lo, hi)
			}
			row, err := parseAssessment(feature)
			if err != nil {
				return fmt.Errorf("decode assessment OBJECTID %d at partition %d–%d row %d: %w", oid, lo, hi, i+1, err)
			}
			if row.ObjectID != strconv.FormatInt(oid, 10) {
				return errors.New("assessment published OBJECTID differs from partition identity")
			}
			p.assessments = append(p.assessments, row)
		}
		after, err := count(where)
		if err != nil {
			return err
		}
		if after != n {
			return errors.New("assessment partition count drift")
		}
		return nil
	}
	if err := walk(0, math.MaxInt64, total); err != nil {
		return err
	}
	final, err := count("1=1")
	if err != nil {
		return err
	}
	if final != total || index != len(m.Receipts) || len(p.assessments) != int(total) {
		return errors.New("incomplete assessment partition capture")
	}
	p.count = int(total)
	return nil
}
