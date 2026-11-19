// Package vacancyjob captures independent Union building and assessment observations.
package vacancyjob

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tidwall/gjson"
)

const (
	StructuresID    = "union.gis.structures"
	AssessedValueID = "union.gis.assessed_value"
)

type Job struct {
	Source    *union.GISSource
	Pool      *pgxpool.Pool
	Artifacts taxartifact.Store
	Layer     union.VacancyLayer
}

func identity(layer union.VacancyLayer) (string, string, error) {
	switch layer {
	case union.StructuresLayer:
		return StructuresID, "union_structures_171", nil
	case union.AssessedValueLayer:
		return AssessedValueID, "union_assessed_value_0", nil
	default:
		return "", "", errors.New("unknown vacancy layer")
	}
}
func (j Job) Import(ctx context.Context) (jobs.Result, error) {
	id, source, err := identity(j.Layer)
	if err != nil {
		return jobs.Result{}, err
	}
	if j.Source == nil || j.Pool == nil || j.Artifacts == nil {
		return jobs.Result{}, errors.New("vacancy capture requires client, database and artifacts")
	}
	attempt, ok := jobs.AttemptID(ctx)
	if !ok {
		return jobs.Result{}, errors.New("vacancy capture requires runner attempt")
	}
	recorder, err := ingest.NewRecorder(j.Artifacts, source, time.Now().UTC())
	if err != nil {
		return jobs.Result{}, err
	}
	if err = recorder.BindAttempt(id, attempt, "full_scan", 0); err != nil {
		return jobs.Result{}, err
	}
	n, err := j.Source.CaptureVacancyLayer(ctx, j.Layer, func(request string, body []byte, crs int) error {
		return recorder.Record(ctx, request, "", crs, bytes.NewReader(body))
	})
	if err != nil {
		// Keep partial and rejected responses registered for offline diagnosis.
		if len(recorder.Manifest().Receipts) > 0 {
			sealCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			_, sealErr := (ingest.Engine{Pool: j.Pool, Store: j.Artifacts}).Seal(sealCtx, recorder)
			return jobs.Result{}, errors.Join(err, sealErr)
		}
		return jobs.Result{}, err
	}
	_, err = (ingest.Engine{Pool: j.Pool, Store: j.Artifacts}).Ingest(ctx, recorder, &publisher{layer: j.Layer})
	return jobs.Result{Processed: n}, err
}
func (j Job) Replay(ctx context.Context, captureID string) error {
	id, _, err := identity(j.Layer)
	if err != nil {
		return err
	}
	if j.Pool == nil || j.Artifacts == nil {
		return errors.New("vacancy replay requires database and artifacts")
	}
	return (ingest.Engine{Pool: j.Pool, Store: j.Artifacts}).Replay(ctx, captureID, id, &publisher{layer: j.Layer})
}

type structure struct {
	ObjectID, OutlineID, PIN, Status string
	Geometry                         json.RawMessage
	CRS                              int
}
type assessment struct {
	ObjectID, ParcelNumber, Category string
	Year                             *int
	Building, Land, Total            *string
}
type publisher struct {
	layer       union.VacancyLayer
	structures  []structure
	assessments []assessment
	count       int
	sourceURL   string
}

func (p *publisher) Prepare(ctx context.Context, capture ingest.VerifiedCapture) error {
	_, source, err := identity(p.layer)
	if err != nil {
		return err
	}
	m := capture.Manifest()
	if m.Source != source || m.Kind != "full_scan" || len(m.Receipts) != 2 {
		return errors.New("invalid vacancy capture manifest")
	}
	var count int
	for i, r := range m.Receipts {
		u, e := url.Parse(r.Request)
		if e != nil || u.Scheme != "https" && u.Scheme != "http" || u.Host == "" {
			return errors.New("invalid vacancy request")
		}
		if i == 1 && (u.Scheme+"://"+u.Host+u.Path != p.sourceURL) {
			return errors.New("vacancy requests target different layers")
		}
		if i == 0 {
			p.sourceURL = u.Scheme + "://" + u.Host + u.Path
		}
		path := "/171/query"
		if p.layer == union.AssessedValueLayer {
			path = "/Property_Tax_Live/Assessed_Value/MapServer/0/query"
		}
		if !strings.HasSuffix(u.Path, path) {
			return errors.New("wrong vacancy layer")
		}
		q := u.Query()
		for _, values := range q {
			if len(values) != 1 {
				return errors.New("ambiguous vacancy request parameter")
			}
		}
		if q.Get("where") != "1=1" || q.Get("f") != "json" || len(q) != 3 && i == 0 || len(q) != 5 && i == 1 && p.layer == union.AssessedValueLayer || len(q) != 6 && i == 1 && p.layer == union.StructuresLayer {
			return errors.New("unexpected vacancy query")
		}
		if i == 0 && (q.Get("returnCountOnly") != "true" || r.CRS != 0) {
			return errors.New("missing vacancy count request")
		}
		if i == 1 && (q.Get("resultRecordCount") != "2000" || q.Get("outFields") != "*" || q.Get("returnGeometry") != strconv.FormatBool(p.layer == union.StructuresLayer)) {
			return errors.New("unexpected vacancy page request")
		}
		if i == 1 && p.layer == union.StructuresLayer && (q.Get("outSR") != "3857" || r.CRS != 3857 && r.CRS != 102100) {
			return errors.New("unsupported structure geometry CRS")
		}
		if i == 1 && p.layer == union.AssessedValueLayer && r.CRS != 0 {
			return errors.New("unexpected assessment geometry")
		}
		body, e := capture.Open(ctx, i)
		if e != nil {
			return e
		}
		raw, readErr := io.ReadAll(io.LimitReader(body, 32<<20+1))
		e = errors.Join(readErr, body.Close())
		if e != nil {
			return e
		}
		if len(raw) > 32<<20 {
			return errors.New("vacancy response too large")
		}
		if !gjson.ValidBytes(raw) || gjson.GetBytes(raw, "error").Exists() {
			return errors.New("invalid vacancy response")
		}
		if i == 0 {
			v := gjson.GetBytes(raw, "count")
			if v.Type != gjson.Number || v.Raw != strconv.FormatInt(v.Int(), 10) || v.Int() < 0 || v.Int() > 2000 {
				return errors.New("vacancy count exceeds safe single-page gate")
			}
			count = int(v.Int())
			continue
		}
		if gjson.GetBytes(raw, "exceededTransferLimit").Bool() {
			return errors.New("truncated vacancy page")
		}
		if p.layer == union.StructuresLayer {
			crs, e := union.ParcelResponseCRS(raw)
			if e != nil || crs != r.CRS {
				return errors.New("structure response CRS mismatch")
			}
		}
		var page struct {
			Features []json.RawMessage `json:"features"`
		}
		if e = json.Unmarshal(raw, &page); e != nil || page.Features == nil || len(page.Features) != count {
			return errors.New("vacancy page count mismatch")
		}
		for _, feature := range page.Features {
			if p.layer == union.StructuresLayer {
				v, e := parseStructure(feature, r.CRS)
				if e != nil {
					return e
				}
				p.structures = append(p.structures, v)
			} else {
				v, e := parseAssessment(feature)
				if e != nil {
					return e
				}
				p.assessments = append(p.assessments, v)
			}
		}
	}
	p.count = count
	return nil
}
func attributes(feature json.RawMessage) (map[string]json.RawMessage, json.RawMessage, error) {
	var v struct {
		Attributes map[string]json.RawMessage `json:"attributes"`
		Geometry   json.RawMessage            `json:"geometry"`
	}
	if err := json.Unmarshal(feature, &v); err != nil || v.Attributes == nil {
		return nil, nil, errors.New("feature missing attributes")
	}
	return v.Attributes, v.Geometry, nil
}
func text(attrs map[string]json.RawMessage, key string) (string, error) {
	raw, ok := attrs[key]
	if !ok || string(raw) == "null" {
		return "", nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, nil
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String(), nil
	}
	return "", fmt.Errorf("invalid %s", key)
}
func parseStructure(raw json.RawMessage, crs int) (structure, error) {
	attrs, geom, err := attributes(raw)
	if err != nil {
		return structure{}, err
	}
	oid, err := text(attrs, "OBJECTID")
	if err != nil || oid == "" {
		return structure{}, errors.New("structure missing OBJECTID")
	}
	outline, err := text(attrs, "outline_id")
	if err != nil {
		return structure{}, err
	}
	pin, err := text(attrs, "PIN_Num")
	if err != nil {
		return structure{}, err
	}
	status, err := text(attrs, "status")
	if err != nil {
		return structure{}, err
	}
	if !gjson.GetBytes(geom, "rings").IsArray() {
		return structure{}, errors.New("structure missing polygon rings")
	}
	return structure{oid, outline, pin, status, geom, crs}, nil
}
func money(attrs map[string]json.RawMessage, key string) (*string, error) {
	raw, ok := attrs[key]
	if !ok || string(raw) == "null" {
		return nil, nil
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err != nil {
		return nil, fmt.Errorf("invalid %s", key)
	}
	value, ok := new(big.Rat).SetString(n.String())
	if !ok || value.Sign() < 0 || !value.IsInt() && value.Denom().Cmp(big.NewInt(100)) > 0 {
		return nil, fmt.Errorf("invalid %s", key)
	}
	formatted := value.FloatString(2)
	return &formatted, nil
}
func parseAssessment(raw json.RawMessage) (assessment, error) {
	attrs, _, err := attributes(raw)
	if err != nil {
		return assessment{}, err
	}
	oid, err := text(attrs, "OBJECTID")
	if err != nil {
		return assessment{}, err
	}
	parcel, err := text(attrs, "parcel_number")
	if err != nil || parcel == "" {
		return assessment{}, errors.New("assessment missing parcel number")
	}
	category, err := text(attrs, "assess_category")
	if err != nil {
		return assessment{}, err
	}
	yearText, err := text(attrs, "parcel_year")
	if err != nil {
		return assessment{}, err
	}
	var year *int
	if yearText != "" {
		n, e := strconv.Atoi(yearText)
		if e != nil || n < 0 || n > 9999 {
			return assessment{}, errors.New("invalid assessment year")
		}
		year = &n
	}
	building, err := money(attrs, "market_building")
	if err != nil {
		return assessment{}, err
	}
	land, err := money(attrs, "market_land")
	if err != nil {
		return assessment{}, err
	}
	total, err := money(attrs, "market_total")
	if err != nil {
		return assessment{}, err
	}
	return assessment{oid, parcel, category, year, building, land, total}, nil
}
func (p *publisher) Apply(ctx context.Context, tx pgx.Tx, ob ingest.Observation) error {
	var previous bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vacancy_live_generations WHERE source_name=$1 AND retrieved_at >= $2 AND capture_id<>$3::uuid)`, ob.Manifest.Source, ob.Manifest.ObservedAt, ob.ID).Scan(&previous)
	if err != nil {
		return err
	}
	if previous {
		return ingest.ErrNoPublication
	}
	var same bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vacancy_live_generations g JOIN ingest_captures c ON c.id=g.capture_id WHERE g.source_name=$1 AND (convert_from(c.manifest,'UTF8')::jsonb)->'receipts'->1->>'sha256'=$2)`, ob.Manifest.Source, ob.Manifest.Receipts[1].SHA256).Scan(&same)
	if err != nil {
		return err
	}
	if same {
		return ingest.ErrObservationOnly
	}
	var id string
	err = tx.QueryRow(ctx, `INSERT INTO vacancy_live_generations(source_name,capture_id,retrieved_at,observed_count,validation_note) VALUES($1,$2::uuid,$3,$4,$5) RETURNING id::text`, ob.Manifest.Source, ob.ID, ob.Manifest.ObservedAt, p.count, "single page reconciled; county coverage, assessment year and live pagination unverified").Scan(&id)
	if err != nil {
		return err
	}
	for i, v := range p.structures {
		_, err = tx.Exec(ctx, `INSERT INTO vacancy_live_structures(generation_id,ordinal,object_id,outline_id,pin,status,geometry,source_crs) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, i, v.ObjectID, v.OutlineID, v.PIN, v.Status, v.Geometry, v.CRS)
		if err != nil {
			return err
		}
	}
	for i, v := range p.assessments {
		_, err = tx.Exec(ctx, `INSERT INTO vacancy_live_assessments(generation_id,ordinal,object_id,parcel_number,parcel_year,assess_category,market_building,market_land,market_total) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, i, v.ObjectID, v.ParcelNumber, v.Year, v.Category, v.Building, v.Land, v.Total)
		if err != nil {
			return err
		}
	}
	return nil
}
