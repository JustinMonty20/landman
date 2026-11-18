// Package soiljob publishes captured Union county soil geometry.
package soiljob

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"time"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/county"
	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/soil"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tidwall/gjson"
)

const captureSource = "union_gis_soil"
const polygonSource = "union_county_gis"

type Job struct {
	Source    *union.GISSource
	Pool      *pgxpool.Pool
	Artifacts taxartifact.Store
}

func (j Job) Import(ctx context.Context) (jobs.Result, error) {
	if j.Source == nil || j.Pool == nil || j.Artifacts == nil {
		return jobs.Result{}, errors.New("soil capture requires source, database and artifact store")
	}
	recorder, err := ingest.NewRecorder(j.Artifacts, captureSource, time.Now().UTC())
	if err != nil {
		return jobs.Result{}, err
	}
	polygons, _, err := j.Source.CaptureSoilPolygons(ctx, func(request string, body []byte, crs int) error {
		return recorder.Record(ctx, request, "", crs, bytes.NewReader(body))
	})
	if err != nil {
		return jobs.Result{}, sealFailedCapture(ctx, j.Pool, j.Artifacts, recorder, err)
	}
	_, err = (ingest.Engine{Pool: j.Pool, Store: j.Artifacts}).Ingest(ctx, recorder, &publication{})
	return jobs.Result{Processed: len(polygons)}, err
}
func (j Job) Replay(ctx context.Context, id string) error {
	if j.Pool == nil || j.Artifacts == nil {
		return errors.New("soil replay requires database and artifact store")
	}
	return (ingest.Engine{Pool: j.Pool, Store: j.Artifacts}).Replay(ctx, id, &publication{})
}

type publication struct {
	polygons []soil.Polygon
	units    []soil.MapUnit
}

func (p *publication) Prepare(ctx context.Context, capture ingest.VerifiedCapture) error {
	manifest := capture.Manifest()
	if manifest.Source != captureSource || len(manifest.Receipts) < 1 {
		return errors.New("invalid soil capture source")
	}
	count := -1
	offset := 0
	for i, receipt := range manifest.Receipts {
		u, err := url.Parse(receipt.Request)
		if err != nil {
			return err
		}
		query := u.Query()
		if query.Get("where") != "1=1" {
			return errors.New("unexpected soil query filter")
		}
		body, err := capture.Open(ctx, i)
		if err != nil {
			return err
		}
		raw, readErr := io.ReadAll(io.LimitReader(body, 32<<20+1))
		err = errors.Join(readErr, body.Close())
		if err != nil {
			return err
		}
		if len(raw) > 32<<20 {
			return errors.New("soil response exceeds capture limit")
		}
		if i == 0 {
			if query.Get("returnCountOnly") != "true" || query.Get("f") != "json" || query.Get("outFields") != "*" || receipt.CRS != 0 || len(query) != 4 {
				return errors.New("soil capture missing count response")
			}
			value := gjson.GetBytes(raw, "count")
			if value.Type != gjson.Number || value.Raw != strconv.FormatInt(value.Int(), 10) || value.Int() < 0 || value.Int() > 1000000 {
				return errors.New("invalid soil count")
			}
			count = int(value.Int())
			continue
		}
		declared, err := union.ParcelResponseCRS(raw)
		if err != nil || declared != receipt.CRS || (declared != 3857 && declared != 102100) || query.Get("orderByFields") != "OBJECTID_1" || query.Get("outFields") != "OBJECTID_1,AREASYMBOL,SPATIALVER,MUSYM,MUKEY" || query.Get("returnGeometry") != "true" || query.Get("f") != "json" || (len(query) != 6 && len(query) != 7) {
			return errors.New("invalid soil geometry request")
		}
		at := 0
		if query.Has("resultOffset") {
			at, err = strconv.Atoi(query.Get("resultOffset"))
		}
		if (err != nil || at != offset) || (offset == 0 && query.Has("resultOffset")) || (offset > 0 && !query.Has("resultOffset")) {
			return errors.New("incomplete soil page order")
		}
		size, err := strconv.Atoi(query.Get("resultRecordCount"))
		if err != nil || size <= 0 || size > 2000 {
			return errors.New("invalid soil page size")
		}
		polygons, units, err := union.ParseSoilFeatures(raw)
		if err != nil {
			return err
		}
		expected := min(size, count-offset)
		if expected <= 0 || len(polygons) != expected {
			return fmt.Errorf("soil page incomplete at offset %d", offset)
		}
		p.polygons = append(p.polygons, polygons...)
		p.units = append(p.units, units...)
		offset += len(polygons)
	}
	if count != offset || (count > 0 && len(manifest.Receipts) < 2) {
		return errors.New("soil capture count mismatch")
	}
	return nil
}
func (p *publication) Apply(ctx context.Context, tx pgx.Tx, observation ingest.Observation) error {
	if observation.Published {
		return nil
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "soil:union:"+polygonSource); err != nil {
		return err
	}
	var newer bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ingest_captures WHERE source_name=$1 AND published_at IS NOT NULL AND observed_at >= $2 AND id<>$3::uuid)`, observation.Manifest.Source, observation.Manifest.ObservedAt, observation.ID).Scan(&newer); err != nil {
		return err
	}
	if newer {
		return ingest.ErrNoPublication
	}
	canonical := responseDigest(observation.Manifest)
	normalized := digest(p.units)
	geometry, err := geometryDigest(p.polygons)
	if err != nil {
		return err
	}
	previous, previousSet, err := previousSoilFingerprint(ctx, tx, observation.Manifest.Source)
	if err != nil {
		return err
	}
	next := ingest.Fingerprint{Source: canonical, SourceVersion: "soil-response-v1", Normalized: normalized, NormalizedVersion: "soil-unit-v1", Geometry: geometry, GeometryVersion: "soil-ewkb-v1"}
	cause := soilChangeCause(previous, next, observation.Replay)
	var setID string
	if previous.Geometry == geometry && previous.GeometryVersion == next.GeometryVersion && previousSet != "" {
		err = tx.QueryRow(ctx, `SELECT set_id::text FROM soil_polygon_set_heads WHERE county_id='union' AND source_name=$1`, polygonSource).Scan(&setID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if setID != previousSet {
			setID = ""
		}
	}
	if setID == "" {
		setID, err = soil.StoreWithSetTx(ctx, tx, county.Union, polygonSource, p.polygons, p.units)
		if err != nil {
			return err
		}
	} else if previous.Normalized != normalized || previous.NormalizedVersion != next.NormalizedVersion {
		if err := soil.StoreMapUnitsTx(ctx, tx, p.units); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO soil_ingest_publications(capture_id,source_name,canonical_version,canonical_digest,normalized_version,normalized_digest,geometry_version,geometry_digest,change_cause,published_set_id,observed_at) VALUES ($1::uuid,$2,'soil-response-v1',$3,'soil-unit-v1',$4,'soil-ewkb-v1',$5,$6,$7::uuid,$8)`, observation.ID, observation.Manifest.Source, canonical, normalized, geometry, cause, setID, observation.Manifest.ObservedAt)
	return err
}
