package soiljob

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/JustinMonty20/landman/internal/county"
	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/soil"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const interpretationSource = "union_usda_soil_interpretations"
const usdaRevision = "sda-tabular-v1"
const namesRevision = "sda-names-v1"
const setRequestPrefix = "soil://union/union_county_gis/"

type ScreeningJob struct {
	Pool      *pgxpool.Pool
	Artifacts taxartifact.Store
	Client    *http.Client
	Interval  time.Duration
}

func (j ScreeningJob) Import(ctx context.Context) (result jobs.Result, runErr error) {
	if j.Pool == nil || j.Artifacts == nil || j.Client == nil || j.Interval <= 0 {
		return jobs.Result{}, errors.New("USDA screening requires database, artifact store, client and freshness interval")
	}
	setID, polygons, _, err := soil.StoredSet(ctx, j.Pool, county.Union, polygonSource)
	if err != nil {
		return jobs.Result{}, err
	}
	var previousHead string
	if err := j.Pool.QueryRow(ctx, `SELECT COALESCE((SELECT run_id::text FROM soil_screening_heads WHERE county_id='union' AND source_name=$1),'')`, polygonSource).Scan(&previousHead); err != nil {
		return jobs.Result{}, err
	}
	defer func() {
		if runErr == nil {
			return
		}
		failureCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		runErr = errors.Join(runErr, soil.RecordScreeningFailure(failureCtx, j.Pool, county.Union, polygonSource, previousHead, runErr))
	}()
	keys := make([]string, 0, len(polygons))
	seen := map[string]bool{}
	for _, polygon := range polygons {
		if !seen[polygon.MapUnitKey] {
			keys = append(keys, polygon.MapUnitKey)
			seen[polygon.MapUnitKey] = true
		}
	}
	sort.Strings(keys)
	recorder, err := ingest.NewRecorder(j.Artifacts, interpretationSource, time.Now().UTC())
	if err != nil {
		return jobs.Result{}, err
	}
	nameRows, err := j.Pool.Query(ctx, `SELECT map_unit_key FROM soil_map_units WHERE map_unit_key=ANY($1) AND map_unit_name IS NULL ORDER BY map_unit_key`, keys)
	if err != nil {
		return jobs.Result{}, err
	}
	nameKeys := []string{}
	for nameRows.Next() {
		var key string
		if err := nameRows.Scan(&key); err != nil {
			nameRows.Close()
			return jobs.Result{}, err
		}
		nameKeys = append(nameKeys, key)
	}
	err = nameRows.Err()
	nameRows.Close()
	if err != nil {
		return jobs.Result{}, err
	}
	keyBytes, err := json.Marshal(struct{ Keys, NameKeys []string }{keys, nameKeys})
	if err != nil {
		return jobs.Result{}, err
	}
	if err := recorder.Record(ctx, setRequestPrefix+setID, "", 0, bytes.NewReader(keyBytes)); err != nil {
		return jobs.Result{}, err
	}
	var retentionError error
	err = soil.CaptureNames(ctx, j.Client, soil.USDAEndpoint, nameKeys, func(query string, body []byte) error {
		retentionError = recorder.Record(ctx, usdaRequest(query), namesRevision, 0, bytes.NewReader(body))
		return retentionError
	})
	if retentionError != nil {
		return jobs.Result{}, sealFailedCapture(ctx, j.Pool, j.Artifacts, recorder, retentionError)
	}
	if ctx.Err() != nil {
		return jobs.Result{}, sealFailedCapture(ctx, j.Pool, j.Artifacts, recorder, ctx.Err())
	}
	// Only unreceived optional names may be skipped; malformed retained names cannot be published.
	if err != nil && len(recorder.Manifest().Receipts) > 1 {
		return jobs.Result{}, sealFailedCapture(ctx, j.Pool, j.Artifacts, recorder, err)
	}
	_, err = soil.CaptureInterpretations(ctx, j.Client, soil.USDAEndpoint, keys, func(query string, body []byte) error {
		return recorder.Record(ctx, usdaRequest(query), usdaRevision, 0, bytes.NewReader(body))
	})
	if err != nil {
		return jobs.Result{}, sealFailedCapture(ctx, j.Pool, j.Artifacts, recorder, err)
	}
	_, err = (ingest.Engine{Pool: j.Pool, Store: j.Artifacts}).Ingest(ctx, recorder, &screeningPublication{interval: j.Interval})
	return jobs.Result{Processed: len(polygons)}, err
}

func (j ScreeningJob) Replay(ctx context.Context, captureID string) error {
	if j.Pool == nil || j.Artifacts == nil || j.Interval <= 0 {
		return errors.New("USDA replay requires database, artifact store and freshness interval")
	}
	return (ingest.Engine{Pool: j.Pool, Store: j.Artifacts}).Replay(ctx, captureID, &screeningPublication{interval: j.Interval})
}

func usdaRequest(query string) string {
	return "POST " + soil.USDAEndpoint + "?query=" + url.QueryEscape(query) + "&format=JSON"
}

type screeningPublication struct {
	setID    string
	keys     []string
	values   []soil.Interpretation
	interval time.Duration
	names    []byte
}

func (p *screeningPublication) Prepare(ctx context.Context, capture ingest.VerifiedCapture) error {
	manifest := capture.Manifest()
	if manifest.Source != interpretationSource || len(manifest.Receipts) == 0 || !strings.HasPrefix(manifest.Receipts[0].Request, setRequestPrefix) || manifest.Receipts[0].CRS != 0 || manifest.Receipts[0].Revision != "" {
		return errors.New("invalid USDA screening capture")
	}
	p.setID = strings.TrimPrefix(manifest.Receipts[0].Request, setRequestPrefix)
	if p.setID == "" {
		return errors.New("missing captured soil set")
	}
	body, err := capture.Open(ctx, 0)
	if err != nil {
		return err
	}
	raw, readErr := io.ReadAll(io.LimitReader(body, 1<<20))
	err = errors.Join(readErr, body.Close())
	if err != nil {
		return err
	}
	var input struct{ Keys, NameKeys []string }
	if err := json.Unmarshal(raw, &input); err != nil {
		return err
	}
	keys := input.Keys
	if keys == nil {
		return errors.New("missing USDA map unit keys")
	}
	if len(keys) > 1_000_000 || !sort.StringsAreSorted(keys) || !sort.StringsAreSorted(input.NameKeys) {
		return errors.New("unordered USDA keys")
	}
	for i, key := range keys {
		if key == "" || (i > 0 && keys[i-1] == key) {
			return errors.New("duplicate or empty USDA key")
		}
	}
	for i, key := range input.NameKeys {
		if key == "" || (i > 0 && input.NameKeys[i-1] == key) || !slices.Contains(keys, key) {
			return errors.New("invalid USDA name key")
		}
	}
	p.keys = keys
	index := 1
	if len(manifest.Receipts) > 1 && manifest.Receipts[1].Revision == namesRevision {
		receipt := manifest.Receipts[1]
		if len(input.NameKeys) == 0 || receipt.Request != usdaRequest(soil.NameQuery(input.NameKeys)) || receipt.CRS != 0 {
			return errors.New("invalid USDA name request")
		}
		r, e := capture.Open(ctx, 1)
		if e != nil {
			return e
		}
		p.names, e = io.ReadAll(io.LimitReader(r, 16<<20+1))
		e = errors.Join(e, r.Close())
		if e != nil {
			return e
		}
		if len(p.names) > 16<<20 {
			return errors.New("USDA names response exceeds capture limit")
		}
		index++
	}
	p.values, err = soil.ParseInterpretations(ctx, soil.USDAEndpoint, keys, func(ctx context.Context, query string) ([]byte, error) {
		if index >= len(manifest.Receipts) {
			return nil, errors.New("missing USDA response")
		}
		receipt := manifest.Receipts[index]
		if receipt.Request != usdaRequest(query) || receipt.Revision != usdaRevision || receipt.CRS != 0 {
			return nil, fmt.Errorf("USDA receipt %d request or revision differs", index)
		}
		reader, e := capture.Open(ctx, index)
		if e != nil {
			return nil, e
		}
		index++
		raw, e := io.ReadAll(io.LimitReader(reader, 16<<20+1))
		e = errors.Join(e, reader.Close())
		if e != nil {
			return nil, e
		}
		if len(raw) > 16<<20 {
			return nil, errors.New("USDA response exceeds capture limit")
		}
		return raw, nil
	})
	if err != nil {
		return err
	}
	if index != len(manifest.Receipts) {
		return errors.New("unconsumed USDA responses")
	}
	for i := range p.values {
		p.values[i].FetchedAt = manifest.ObservedAt
		// Missing measurements originate from a map; stabilize their source positions for replay.
		measurements := append([]soil.Measurement(nil), p.values[i].Measurements...)
		sort.SliceStable(measurements, func(a, b int) bool {
			left, right := measurements[a], measurements[b]
			if left.Kind != right.Kind {
				return left.Kind < right.Kind
			}
			if left.ComponentKey != right.ComponentKey {
				return left.ComponentKey < right.ComponentKey
			}
			if left.SourceKey != right.SourceKey {
				return left.SourceKey < right.SourceKey
			}
			return left.Position < right.Position
		})
		for j := range measurements {
			measurements[j].Position = j
			measurements[j].FetchedAt = manifest.ObservedAt
		}
		p.values[i].Measurements = measurements
	}
	return nil
}

func (p *screeningPublication) Apply(ctx context.Context, tx pgx.Tx, observation ingest.Observation) error {
	if observation.Published {
		return nil
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "soil:union:"+polygonSource); err != nil {
		return err
	}
	var newer bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ingest_captures WHERE source_name=$1 AND published_at IS NOT NULL AND observed_at >= $2 AND id<>$3::uuid)`, interpretationSource, observation.Manifest.ObservedAt, observation.ID).Scan(&newer); err != nil {
		return err
	}
	if newer {
		return ingest.ErrNoPublication
	}
	if len(p.names) > 0 {
		if err := soil.StoreCapturedNamesTx(ctx, tx, p.names, observation.Manifest.ObservedAt); err != nil {
			return err
		}
	}
	// Serialize against parcel geometry writes before deciding whether the generation is current.
	if _, err := tx.Exec(ctx, `LOCK TABLE parcels IN SHARE MODE`); err != nil {
		return err
	}
	setID, polygons, _, err := soil.StoredSet(ctx, tx, county.Union, polygonSource)
	if err != nil {
		return err
	}
	if setID != p.setID {
		return errors.New("soil set changed after USDA capture")
	}
	currentKeys := make([]string, 0, len(polygons))
	seen := map[string]bool{}
	for _, polygon := range polygons {
		if !seen[polygon.MapUnitKey] {
			currentKeys = append(currentKeys, polygon.MapUnitKey)
			seen[polygon.MapUnitKey] = true
		}
	}
	sort.Strings(currentKeys)
	if !slices.Equal(currentKeys, p.keys) {
		return errors.New("USDA capture does not cover current soil map units")
	}
	canonical := responseDigest(observation.Manifest)
	normalizedValues := append([]soil.Interpretation(nil), p.values...)
	for i := range normalizedValues {
		normalizedValues[i].FetchedAt = time.Time{}
		normalizedValues[i].Measurements = append([]soil.Measurement(nil), normalizedValues[i].Measurements...)
		for j := range normalizedValues[i].Measurements {
			normalizedValues[i].Measurements[j].FetchedAt = time.Time{}
		}
	}
	normalized := digest(normalizedValues)
	geometry := digest(p.setID)
	previous, _, err := previousSoilFingerprint(ctx, tx, interpretationSource)
	if err != nil {
		return err
	}
	next := ingest.Fingerprint{Source: canonical, SourceVersion: "usda-response-v1", Normalized: normalized, NormalizedVersion: "usda-interpretation-v1", Geometry: geometry, GeometryVersion: "soil-set-v1"}
	cause := soilChangeCause(previous, next, observation.Replay)
	var runID *string
	if previous.Normalized != normalized || previous.NormalizedVersion != next.NormalizedVersion || previous.Geometry != geometry || previous.GeometryVersion != next.GeometryVersion {
		if err := soil.PublishScreeningTx(ctx, tx, county.Union, polygonSource, p.setID, polygons, p.values, soil.USDAEndpoint, p.interval, observation.Manifest.ObservedAt); err != nil {
			return err
		}
		var id string
		if err := tx.QueryRow(ctx, `SELECT run_id::text FROM soil_screening_heads WHERE county_id='union' AND source_name=$1`, polygonSource).Scan(&id); err != nil {
			return err
		}
		runID = &id
	} else {
		due, err := soil.NeedsScreenTx(ctx, tx, county.Union, polygonSource)
		if err != nil {
			return err
		}
		if due {
			if err := soil.PublishScreeningTx(ctx, tx, county.Union, polygonSource, p.setID, polygons, p.values, soil.USDAEndpoint, p.interval, observation.Manifest.ObservedAt); err != nil {
				return err
			}
			var id string
			if err := tx.QueryRow(ctx, `SELECT run_id::text FROM soil_screening_heads WHERE county_id='union' AND source_name=$1`, polygonSource).Scan(&id); err != nil {
				return err
			}
			runID = &id
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO soil_ingest_publications(capture_id,source_name,canonical_version,canonical_digest,normalized_version,normalized_digest,geometry_version,geometry_digest,change_cause,published_set_id,published_run_id,observed_at) VALUES ($1::uuid,$2,'usda-response-v1',$3,'usda-interpretation-v1',$4,'soil-set-v1',$5,$6,$7::uuid,$8::uuid,$9)`, observation.ID, interpretationSource, canonical, normalized, geometry, cause, p.setID, runID, observation.Manifest.ObservedAt)
	return err
}
