// Package vacancyjob captures independent Union building and assessment observations.
package vacancyjob

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"sort"
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
	n, err := j.Source.CaptureVacancyLayer(ctx, j.Layer, func(request string, body []byte, crs, status int, complete bool) error {
		return recorder.RecordHTTP(ctx, request, "", crs, status, complete, bytes.NewReader(body))
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
	layer            union.VacancyLayer
	structures       []structure
	assessments      []assessment
	count            int
	sourceURL        string
	legacyStructures bool
}

func (p *publisher) validationNote() string {
	switch {
	case p.layer == union.AssessedValueLayer:
		return "bounded numeric-range assessment observation; county snapshot, query support, coverage and effective year unverified"
	case p.legacyStructures:
		return "bounded single-page structures observation; source ID membership and county coverage unverified"
	default:
		return "paged source-ID-reconciled structures observation; county snapshot and coverage unverified"
	}
}

func (p *publisher) Prepare(ctx context.Context, capture ingest.VerifiedCapture) error {
	_, source, err := identity(p.layer)
	if err != nil {
		return err
	}
	m := capture.Manifest()
	if m.Source != source || m.Kind != "full_scan" || len(m.Receipts) < 2 {
		return errors.New("invalid vacancy capture manifest")
	}
	if p.layer == union.AssessedValueLayer && len(m.Receipts) > 2 {
		return p.prepareAssessmentRanges(ctx, capture)
	}
	structure := p.layer == union.StructuresLayer
	var ids []int64
	var count int
	pageIndex := 0
	legacyStructures := structure && len(m.Receipts) == 2
	p.legacyStructures = legacyStructures
	var aggregateBytes int64
	seenLegacy := make(map[string]bool)
	for i, r := range m.Receipts {
		if r.HTTPStatus < 200 || r.HTTPStatus >= 300 || !r.ResponseComplete {
			return errors.New("vacancy response lacks successful complete HTTP outcome")
		}
		u, e := url.Parse(r.Request)
		if e != nil || u.Scheme != "https" && u.Scheme != "http" || u.Host == "" {
			return errors.New("invalid vacancy request")
		}
		target := u.Scheme + "://" + u.Host + u.Path
		if i == 0 {
			p.sourceURL = target
		} else if target != p.sourceURL {
			return errors.New("vacancy requests target different layers")
		}
		path := "/171/query"
		if !structure {
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
		if q.Get("where") != "1=1" || q.Get("f") != "json" {
			return errors.New("unexpected vacancy query")
		}
		body, e := capture.Open(ctx, i)
		if e != nil {
			return e
		}
		raw, readErr := io.ReadAll(io.LimitReader(body, 32<<20+1))
		if e = errors.Join(readErr, body.Close()); e != nil {
			return e
		}
		aggregateBytes += int64(len(raw))
		if aggregateBytes > union.VacancyCaptureByteLimit {
			return errors.New("vacancy capture exceeds aggregate byte limit")
		}
		if len(raw) > 32<<20 || !gjson.ValidBytes(raw) || gjson.GetBytes(raw, "error").Exists() {
			return errors.New("invalid vacancy response")
		}
		if i == 0 {
			if len(q) != 3 || q.Get("returnCountOnly") != "true" || r.CRS != 0 {
				return errors.New("missing vacancy count request")
			}
			v := gjson.GetBytes(raw, "count")
			if gjson.GetBytes(raw, "exceededTransferLimit").Bool() || v.Type != gjson.Number || v.Raw != strconv.FormatInt(v.Int(), 10) || v.Int() < 0 || v.Int() > 250000 || (!structure || legacyStructures) && v.Int() > 2000 {
				return errors.New("vacancy count exceeds safe gate")
			}
			count = int(v.Int())
			continue
		}
		if structure && !legacyStructures && i == len(m.Receipts)-1 {
			if len(q) != 3 || q.Get("returnCountOnly") != "true" || r.CRS != 0 {
				return errors.New("unexpected final structure count request")
			}
			final := gjson.GetBytes(raw, "count")
			if gjson.GetBytes(raw, "exceededTransferLimit").Bool() || final.Type != gjson.Number || final.Raw != strconv.Itoa(count) {
				return errors.New("structure count drift")
			}
			continue
		}
		if structure && !legacyStructures && (i == 1 || i == len(m.Receipts)-2) {
			if len(q) != 3 || q.Get("returnIdsOnly") != "true" || r.CRS != 0 {
				return errors.New("unexpected structure ID request")
			}
			list, e := union.StructureIDs(raw)
			if e != nil || len(list) != count {
				return errors.New("structure ID count or membership invalid")
			}
			if i == 1 {
				ids = list
			} else {
				if pageIndex != max(1, (count+1999)/2000) {
					return errors.New("structure page count mismatch")
				}
				for n := range ids {
					if ids[n] != list[n] {
						return errors.New("structure membership drift")
					}
				}
			}
			continue
		}
		if structure && !legacyStructures && (i < 2 || i >= len(m.Receipts)-2) {
			return errors.New("invalid structure receipt sequence")
		}
		expectedParams := 5
		if structure {
			expectedParams = 8
			if legacyStructures {
				expectedParams = 6
			}
		}
		if len(q) != expectedParams || q.Get("resultRecordCount") != "2000" || q.Get("outFields") != "*" || q.Get("returnGeometry") != strconv.FormatBool(structure) {
			return errors.New("unexpected vacancy page request")
		}
		if structure {
			if q.Get("outSR") != "3857" || (!legacyStructures && (q.Get("orderByFields") != "OBJECTID ASC" || q.Get("resultOffset") != strconv.Itoa(pageIndex*2000))) || r.CRS != 3857 && r.CRS != 102100 {
				return errors.New("unsupported structure page request")
			}
			crs, e := union.ParcelResponseCRS(raw)
			if e != nil || crs != r.CRS {
				return errors.New("structure response CRS mismatch")
			}
		} else if r.CRS != 0 || i != 1 {
			return errors.New("unexpected assessment page")
		}
		if gjson.GetBytes(raw, "exceededTransferLimit").Bool() {
			return errors.New("truncated vacancy page")
		}
		var page struct {
			Features []json.RawMessage `json:"features"`
		}
		expected := count - pageIndex*2000
		if expected > 2000 {
			expected = 2000
		}
		if e = json.Unmarshal(raw, &page); e != nil || page.Features == nil || len(page.Features) != expected {
			return errors.New("vacancy page count mismatch")
		}
		for n, feature := range page.Features {
			if structure {
				v, e := parseStructure(feature, r.CRS)
				if e != nil {
					return e
				}
				if legacyStructures {
					if seenLegacy[v.ObjectID] {
						return errors.New("duplicate structure OBJECTID")
					}
					seenLegacy[v.ObjectID] = true
				} else if v.ObjectID != strconv.FormatInt(ids[pageIndex*2000+n], 10) {
					return errors.New("structure page source ID membership drift")
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
		pageIndex++
	}
	if structure && !legacyStructures && (len(m.Receipts) != max(1, (count+1999)/2000)+4 || len(p.structures) != count) || structure && legacyStructures && (pageIndex != 1 || len(p.structures) != count) || !structure && (len(m.Receipts) != 2 || len(p.assessments) != count) {
		return errors.New("incomplete vacancy capture")
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

// normalizedFingerprint hashes canonical rows as a sorted multiset. Repeated assessment rows remain repeated.
func (p *publisher) normalizedFingerprint() (string, error) {
	rows := make([]string, 0, p.count)
	if p.layer == union.StructuresLayer {
		for _, v := range p.structures {
			var geometry any
			if err := json.Unmarshal(v.Geometry, &geometry); err != nil {
				return "", err
			}
			b, err := json.Marshal([]any{v.ObjectID, v.OutlineID, v.PIN, v.Status, geometry, v.CRS})
			if err != nil {
				return "", err
			}
			rows = append(rows, string(b))
		}
	} else {
		for _, v := range p.assessments {
			b, err := json.Marshal(v)
			if err != nil {
				return "", err
			}
			rows = append(rows, string(b))
		}
	}
	sort.Strings(rows)
	h := sha256.New()
	for i, row := range rows {
		if i != 0 {
			_, _ = h.Write([]byte("\n"))
		}
		_, _ = io.WriteString(h, row)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

const normalizerVersion = "vacancy-rows-v1"

func (p *publisher) Apply(ctx context.Context, tx pgx.Tx, ob ingest.Observation) error {
	fingerprint, err := p.normalizedFingerprint()
	if err != nil {
		return err
	}
	var latestID, latestCapture, latestHash, latestVersion string
	var latestAt time.Time
	err = tx.QueryRow(ctx, `SELECT id::text,capture_id::text,retrieved_at,normalized_sha256,normalizer_version FROM vacancy_live_generations WHERE source_name=$1 ORDER BY retrieved_at DESC,id DESC LIMIT 1`, ob.Manifest.Source).Scan(&latestID, &latestCapture, &latestAt, &latestHash, &latestVersion)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err == nil && latestCapture != ob.ID && (ob.Published || !ob.Manifest.ObservedAt.After(latestAt)) {
		return ingest.ErrNoPublication
	}
	var acceptedID string
	var acceptedAt time.Time
	cursorErr := tx.QueryRow(ctx, `SELECT capture_id::text,observed_at FROM vacancy_latest_observations WHERE source_name=$1`, ob.Manifest.Source).Scan(&acceptedID, &acceptedAt)
	if errors.Is(cursorErr, pgx.ErrNoRows) && err == nil {
		acceptedID, acceptedAt = latestCapture, latestAt
	} else if cursorErr != nil && !errors.Is(cursorErr, pgx.ErrNoRows) {
		return cursorErr
	}
	if acceptedID != "" && acceptedID != ob.ID && !ob.Manifest.ObservedAt.After(acceptedAt) {
		return ingest.ErrNoPublication
	}
	if !ob.Replay {
		if _, err := tx.Exec(ctx, `INSERT INTO vacancy_latest_observations(source_name,capture_id,observed_at) VALUES($1,$2::uuid,$3)
ON CONFLICT (source_name) DO UPDATE SET capture_id=EXCLUDED.capture_id,observed_at=EXCLUDED.observed_at`, ob.Manifest.Source, ob.ID, ob.Manifest.ObservedAt); err != nil {
			return err
		}
	}
	if latestID != "" && latestHash == fingerprint && latestVersion == normalizerVersion {
		return ingest.ErrObservationOnly
	}
	id := latestID
	if latestCapture == ob.ID {
		_, err = tx.Exec(ctx, `DELETE FROM vacancy_live_structures WHERE generation_id=$1`, id)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM vacancy_live_assessments WHERE generation_id=$1`, id)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE vacancy_live_generations SET normalized_sha256=$2,normalizer_version=$3,observed_count=$4,validation_note=$5 WHERE id=$1`, id, fingerprint, normalizerVersion, p.count, p.validationNote())
	} else {
		err = tx.QueryRow(ctx, `INSERT INTO vacancy_live_generations(source_name,capture_id,retrieved_at,observed_count,validation_note,normalized_sha256,normalizer_version) VALUES($1,$2::uuid,$3,$4,$5,$6,$7) RETURNING id::text`, ob.Manifest.Source, ob.ID, ob.Manifest.ObservedAt, p.count, p.validationNote(), fingerprint, normalizerVersion).Scan(&id)
	}
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
