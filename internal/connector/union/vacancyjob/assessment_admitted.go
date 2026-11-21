package vacancyjob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/tidwall/gjson"
)

func (p *publisher) prepareAdmittedAssessment(ctx context.Context, capture ingest.VerifiedCapture) error {
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
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || !strings.HasSuffix(u.Path, "/Property_Tax_Live/Assessed_Value/MapServer/0/query") || u.RawQuery != q.Encode() || r.Revision != union.AssessmentProtocolRevision || r.CRS != 0 || r.HTTPStatus < 200 || r.HTTPStatus >= 300 || !r.ResponseComplete {
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
	count := func(q url.Values) (int64, error) {
		raw, err := next(q)
		if err != nil {
			return 0, err
		}
		return union.AssessmentCount(raw)
	}
	total, err := count(url.Values{"f": {"json"}, "where": {"1=1"}, "returnCountOnly": {"true"}})
	if err != nil {
		return err
	}
	program, err := union.PlanAssessment(total, count)
	if err != nil {
		return err
	}
	err = union.ReadAssessmentProgram(program, next, count, func(feature []byte) error {
		oid, err := union.AssessmentObjectID(feature)
		if err != nil {
			return err
		}
		row, err := parseAssessment(feature)
		if err != nil {
			return fmt.Errorf("decode assessment OBJECTID %d: %w", oid, err)
		}
		if row.ObjectID != strconv.FormatInt(oid, 10) {
			return errors.New("assessment published OBJECTID differs from partition identity")
		}
		p.assessments = append(p.assessments, row)
		return nil
	}, nil)
	if err != nil {
		return err
	}
	if index != len(m.Receipts) || len(p.assessments) != int(total) {
		return errors.New("incomplete assessment partition capture")
	}
	p.count = int(total)
	return nil
}
