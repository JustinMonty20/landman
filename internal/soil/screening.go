package soil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/JustinMonty20/landman/internal/county"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twpayne/go-geom/encoding/ewkb"
	"golang.org/x/time/rate"
)

// Use identifies a distinct USDA interpretation, not a parcel approval decision.
type Use string

const (
	ConventionalSeptic      Use = "conventional-septic"
	DwellingWithoutBasement Use = "dwelling-without-basement"
	DwellingWithBasement    Use = "dwelling-with-basement"
)

type rule struct {
	use                  Use
	attribute, key, name string
}

var screeningRules = []rule{
	{ConventionalSeptic, "18", "208", "ENG - Septic Tank Absorption Fields"},
	{DwellingWithoutBasement, "69", "340", "ENG - Dwellings W/O Basements"},
	{DwellingWithBasement, "80", "393", "ENG - Dwellings With Basements"},
}

const screeningMethod = "USDA Dominant Condition; tie break 1; no component cutoff"

type Availability string

const (
	Rated       Availability = "rated"
	NotRated    Availability = "not-rated"
	Unavailable Availability = "unavailable"
)

// ComponentResult is a component's rating or reason. Percent describes map-unit
// composition, not a component's location or acreage inside a parcel.
type ComponentResult struct {
	Facts          map[string]Fact `json:"facts,omitempty"`
	Sequence       int             `json:"sequence"`
	Availability   Availability    `json:"availability,omitempty"`
	OriginalRating string          `json:"original_rating,omitempty"`
	Key            string          `json:"key"`
	Name           string          `json:"name"`
	Percent        *float64        `json:"percent"`
	Rating         string          `json:"rating,omitempty"`
	ReasonKey      string          `json:"reason_key,omitempty"`
	ReasonName     string          `json:"reason_name,omitempty"`
	Reason         string          `json:"reason,omitempty"`
	Severity       *float64        `json:"severity,omitempty"`
}

// Measurement is a source-scoped observation, not a parcel-located soil condition.
// SourceKey identifies the month or horizon when available; position retains
// horizon/restriction row identity when SDA exposes no row key.
type Measurement struct {
	ComponentKey string    `json:"component_key"`
	Kind         string    `json:"kind"`
	SourceKey    string    `json:"source_key"`
	Month        string    `json:"month,omitempty"`
	Position     int       `json:"position"`
	TopCM        *float64  `json:"top_cm,omitempty"`
	BottomCM     *float64  `json:"bottom_cm,omitempty"`
	Value        Fact      `json:"value"`
	SourceQuery  string    `json:"source_query"`
	FetchedAt    time.Time `json:"fetched_at"`
}

type SourceRevision struct {
	RuleVersion    *string    `json:"rule_version"`
	AreaSymbol     string     `json:"area_symbol"`
	SurveyVersion  *string    `json:"survey_version"`
	SurveySavedOn  *time.Time `json:"survey_saved_on"`
	SurveySavedRaw string     `json:"survey_saved_raw,omitempty"`
	Query          string     `json:"query"`
}

type Fact struct {
	RawValue      *string      `json:"raw_value,omitempty"`
	Value         *string      `json:"value"`
	Unit          string       `json:"unit,omitempty"`
	Availability  Availability `json:"availability"`
	MissingReason string       `json:"missing_reason,omitempty"`
}

// Interpretation retains original map-unit ratings separately from component
// reasons and raw ancillary values whose units or domains remain unverified.
type Interpretation struct {
	ID                      string             `json:"id"`
	Availability            Availability       `json:"availability"`
	OriginalRating          string             `json:"original_rating,omitempty"`
	RuleName                string             `json:"rule_name"`
	Revision                SourceRevision     `json:"revision"`
	Facts                   map[string]Fact    `json:"facts"`
	ComponentQuery          string             `json:"component_query"`
	MetadataQuery           string             `json:"metadata_query"`
	Warnings                []string           `json:"warnings"`
	MapUnitKey              string             `json:"map_unit_key"`
	Use                     Use                `json:"use"`
	RuleKey                 string             `json:"rule_key"`
	Aggregation             string             `json:"aggregation"`
	Rating                  string             `json:"rating,omitempty"`
	NotRated                bool               `json:"not_rated"`
	Components              []ComponentResult  `json:"components"`
	Measurements            []Measurement      `json:"measurements"`
	UnratedComponentPercent *float64           `json:"unrated_component_percent,omitempty"`
	Evidence                map[string]*string `json:"evidence"`
	SourceQuery             string             `json:"source_query"`
	SourceURL               string             `json:"source_url"`
	FetchedAt               time.Time          `json:"fetched_at"`
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func componentFact(name, unit, raw string) (Fact, error) {
	if raw == "" {
		return Fact{Availability: Unavailable, MissingReason: "USDA returned no value"}, nil
	}
	valid := false
	switch name {
	case "agricultural_capability_class":
		valid = len(raw) == 1 && raw[0] >= '1' && raw[0] <= '8'
	case "agricultural_capability_subclass":
		valid = raw == "e" || raw == "w" || raw == "s" || raw == "c"
	case "drainage_class":
		switch raw {
		case "Excessively drained", "Somewhat excessively drained", "Well drained", "Moderately well drained", "Somewhat poorly drained", "Poorly drained", "Very poorly drained":
			valid = true
		}
	case "slope_percent":
		value, err := strconv.ParseFloat(raw, 64)
		valid = err == nil && !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 100
	}
	if !valid {
		return Fact{}, fmt.Errorf("unsupported %s value %q", name, raw)
	}
	return Fact{RawValue: &raw, Value: &raw, Unit: unit, Availability: Rated}, nil
}
func quotedKeys(keys []string) string {
	quoted := make([]string, len(keys))
	for i, key := range keys {
		quoted[i] = "'" + strings.ReplaceAll(key, "'", "''") + "'"
	}
	return strings.Join(quoted, ",")
}

func field(row []any, i int) (string, error) {
	if row[i] == nil {
		return "", nil
	}
	s, ok := row[i].(string)
	if !ok {
		return "", fmt.Errorf("USDA field %d is not text", i)
	}
	return s, nil
}
func number(row []any, i int) (*float64, error) {
	s, err := field(row, i)
	if err != nil || s == "" {
		return nil, err
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return nil, fmt.Errorf("invalid USDA number %q", s)
	}
	return &v, nil
}
func rating(s string) (string, bool, error) {
	switch s {
	case "Not limited", "Somewhat limited", "Very limited":
		return s, false, nil
	case "Not rated", "":
		return "", true, nil
	default:
		return "", false, fmt.Errorf("unsupported USDA rating %q", s)
	}
}
func parseTable(data []byte, columns int) ([][]any, error) {
	var envelope struct {
		Table [][]any `json:"Table"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	if envelope.Table == nil {
		return nil, errors.New("USDA response has no Table")
	}
	for _, row := range envelope.Table {
		if len(row) != columns {
			return nil, fmt.Errorf("USDA response row has %d columns, want %d", len(row), columns)
		}
		for _, cell := range row {
			if cell != nil {
				if _, ok := cell.(string); !ok {
					return nil, errors.New("USDA response has non-text cell")
				}
			}
		}
	}
	return envelope.Table, nil
}

// FetchInterpretations validates every requested map unit and all three uses before
// returning any evidence. Requests scale with distinct keys in batches of 50.
func FetchInterpretations(ctx context.Context, client *http.Client, endpoint string, keys []string) ([]Interpretation, error) {
	unique := map[string]bool{}
	for _, key := range keys {
		if key == "" {
			return nil, errors.New("empty USDA map unit key")
		}
		unique[key] = true
	}
	keys = make([]string, 0, len(unique))
	for key := range unique {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return []Interpretation{}, nil
	}
	limiter := rate.NewLimiter(rate.Every(time.Second), 1)
	query := func(sql string, columns int) ([][]any, error) {
		if err := limiter.Wait(ctx); err != nil {
			return nil, err
		}
		raw, err := usdaQuery(ctx, client, endpoint, sql)
		if err != nil {
			return nil, err
		}
		return parseTable(raw, columns)
	}
	metadataSQL := `SELECT attributekey,nasisrulename,algorithmname,tiebreakrule,componentpercentcutoff,notratedphrase FROM sdvattribute WHERE attributekey IN (18,69,80) ORDER BY attributekey`
	rows, err := query(metadataSQL, 6)
	if err != nil {
		return nil, err
	}
	if len(rows) != len(screeningRules) {
		return nil, errors.New("USDA rule metadata incomplete")
	}
	for i, r := range screeningRules {
		id, _ := field(rows[i], 0)
		name, _ := field(rows[i], 1)
		method, _ := field(rows[i], 2)
		tie, _ := field(rows[i], 3)
		cutoff, _ := field(rows[i], 4)
		missing, _ := field(rows[i], 5)
		if id != r.attribute || name != r.name || method != "Dominant Condition" || tie != "1" || cutoff != "" || missing != "Not rated" {
			return nil, fmt.Errorf("USDA rule %s changed identity or aggregation", r.attribute)
		}
	}
	results := make([]Interpretation, 0, len(keys)*3)
	for start := 0; start < len(keys); start += 50 {
		batch := keys[start:min(start+50, len(keys))]
		expected := map[string]bool{}
		for _, key := range batch {
			expected[key] = true
		}
		quoted := quotedKeys(batch)
		ratingSQL := fmt.Sprintf(`SELECT m.mukey,a.engstafdcd,a.engdwobdcd,a.engdwbdcd,a.niccdcd,a.niccdcdpct,a.slopegradwta,a.wtdepannmin,a.flodfreqdcd,a.pondfreqprs FROM mapunit m LEFT JOIN muaggatt a ON a.mukey=m.mukey WHERE m.mukey IN (%s) ORDER BY m.mukey`, quoted)
		rows, err = query(ratingSQL, 10)
		if err != nil {
			return nil, err
		}
		found := map[string]bool{}
		batchStart := len(results)
		for _, row := range rows {
			key, _ := field(row, 0)
			if !expected[key] || found[key] {
				return nil, fmt.Errorf("unexpected or duplicate USDA map unit %q", key)
			}
			found[key] = true
			evidence := map[string]*string{}
			facts := map[string]Fact{}
			names := []string{"agricultural_capability_class", "agricultural_capability_dominant_percent", "slope_percent", "water_table_depth_cm", "flood_frequency", "ponding_presence"}
			for i, name := range names {
				text, _ := field(row, i+4)
				fact := Fact{Availability: Unavailable, MissingReason: "USDA returned no value"}
				if text != "" {
					fact.RawValue = &text
					fact.MissingReason = "Field units/domain metadata not captured; raw source value retained without interpretation"
				}
				facts[name] = fact
				evidence[name] = fact.RawValue
			}
			for _, name := range []string{"drainage_class", "shrink_swell_measurement", "restrictive_layer_depth", "agricultural_capability_subclass"} {
				facts[name] = Fact{Availability: Unavailable, MissingReason: "No verified measured-field lookup; component limitation reasons are retained separately"}
				evidence[name] = nil
			}
			for i, r := range screeningRules {
				text, _ := field(row, i+1)
				value, notRated, e := rating(text)
				if e != nil {
					return nil, e
				}
				availability := Rated
				if text == "" {
					availability = Unavailable
				} else if notRated {
					availability = NotRated
				}
				results = append(results, Interpretation{MapUnitKey: key, Use: r.use, RuleKey: r.key, RuleName: r.name, Aggregation: screeningMethod, Rating: value, OriginalRating: text, NotRated: availability == NotRated, Availability: availability, Components: []ComponentResult{}, Evidence: evidence, Facts: facts, SourceURL: endpoint, SourceQuery: ratingSQL, MetadataQuery: metadataSQL, FetchedAt: time.Now().UTC(), Warnings: []string{"Mapped survey evidence, not site verified", "Rule revision unavailable"}})
			}
		}
		for _, key := range batch {
			if !found[key] {
				return nil, fmt.Errorf("USDA omitted requested map unit %s", key)
			}
		}
		catalogSQL := fmt.Sprintf(`SELECT m.mukey,l.areasymbol,s.saversion,s.saverest FROM mapunit m JOIN legend l ON m.lkey=l.lkey LEFT JOIN sacatalog s ON l.areasymbol=s.areasymbol WHERE m.mukey IN (%s) ORDER BY m.mukey`, quoted)
		catalogRows, e := query(catalogSQL, 4)
		if e != nil {
			return nil, e
		}
		revisions := map[string]SourceRevision{}
		for _, row := range catalogRows {
			key, _ := field(row, 0)
			if _, ok := revisions[key]; !expected[key] || ok {
				return nil, fmt.Errorf("unexpected or duplicate USDA catalog key %s", key)
			}
			area, _ := field(row, 1)
			version, _ := field(row, 2)
			saved, _ := field(row, 3)
			revision := SourceRevision{AreaSymbol: area, SurveySavedRaw: saved, Query: catalogSQL}
			if version != "" {
				revision.SurveyVersion = &version
			}
			if saved != "" {
				date, e := time.Parse("1/2/2006 3:04:05 PM", saved)
				if e != nil {
					return nil, fmt.Errorf("USDA survey date: %w", e)
				}
				calendar := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
				revision.SurveySavedOn = &calendar
			}
			revisions[key] = revision
		}
		for _, key := range batch {
			if _, ok := revisions[key]; !ok {
				return nil, fmt.Errorf("USDA omitted catalog key %s", key)
			}
		}
		ancillarySQL := fmt.Sprintf(`SELECT c.mukey,c.cokey,c.compname,c.comppct_r,c.nirrcapcl,c.nirrcapscl,c.drainagecl,c.slope_r FROM component c WHERE c.mukey IN (%s) ORDER BY c.mukey,c.cokey`, quoted)
		ancillaryRows, e := query(ancillarySQL, 8)
		if e != nil {
			return nil, e
		}
		ancillary := map[string]map[string]Fact{}
		for _, row := range ancillaryRows {
			key, _ := field(row, 0)
			componentKey, _ := field(row, 1)
			if !expected[key] || componentKey == "" || ancillary[key+":"+componentKey] != nil {
				return nil, fmt.Errorf("unexpected or duplicate USDA component %s/%s", key, componentKey)
			}
			facts := map[string]Fact{}
			for i, spec := range []struct{ name, unit string }{
				{"agricultural_capability_class", ""}, {"agricultural_capability_subclass", ""},
				{"drainage_class", ""}, {"slope_percent", "percent"},
			} {
				text, _ := field(row, i+4)
				fact, err := componentFact(spec.name, spec.unit, text)
				if err != nil {
					return nil, fmt.Errorf("USDA component %s/%s: %w", key, componentKey, err)
				}
				facts[spec.name] = fact
			}
			ancillary[key+":"+componentKey] = facts
		}
		measurements, e := fetchMeasurements(batch, ancillary, query)
		if e != nil {
			return nil, e
		}
		componentsSQL := fmt.Sprintf(`SELECT c.mukey,c.cokey,c.compname,c.comppct_r,i.mrulekey,i.seqnum,i.rulekey,i.rulename,i.interphr,i.interphrc FROM component c JOIN cointerp i ON i.cokey=c.cokey WHERE c.mukey IN (%s) AND i.mrulekey IN ('208','340','393') ORDER BY c.mukey,c.cokey,i.mrulekey,i.seqnum`, quoted)
		componentRows, e := query(componentsSQL, 10)
		if e != nil {
			return nil, e
		}
		index := map[string]*Interpretation{}
		for j := batchStart; j < len(results); j++ {
			v := &results[j]
			v.ComponentQuery = componentsSQL
			v.Measurements = measurements[v.MapUnitKey]
			v.Revision = revisions[v.MapUnitKey]
			if v.Revision.SurveyVersion == nil || v.Revision.SurveySavedOn == nil {
				v.Warnings = append(v.Warnings, "USDA survey revision or date unavailable")
			}
			index[v.MapUnitKey+":"+v.RuleKey] = v
		}
		sequences := map[string]bool{}
		for _, row := range componentRows {
			key, _ := field(row, 0)
			ruleKey, _ := field(row, 4)
			target := index[key+":"+ruleKey]
			if target == nil {
				return nil, fmt.Errorf("unexpected USDA component for %s rule %s", key, ruleKey)
			}
			componentKey, _ := field(row, 1)
			name, _ := field(row, 2)
			pct, e := number(row, 3)
			if e != nil {
				return nil, e
			}
			if componentKey == "" || (pct != nil && *pct > 100) {
				return nil, errors.New("invalid USDA component identity or percent")
			}
			seq, _ := field(row, 5)
			sequence, e := strconv.Atoi(seq)
			identity := key + ":" + ruleKey + ":" + componentKey + ":" + seq
			if e != nil || sequence < 0 || sequences[identity] {
				return nil, errors.New("invalid or duplicate USDA component sequence")
			}
			sequences[identity] = true
			reasonKey, _ := field(row, 6)
			reasonName, _ := field(row, 7)
			severity, e := number(row, 8)
			if e != nil {
				return nil, e
			}
			if severity != nil && *severity > 1 {
				return nil, errors.New("USDA component severity outside fuzzy-rating domain")
			}
			value, _ := field(row, 9)
			item := ComponentResult{Key: componentKey, Name: name, Percent: pct, Sequence: sequence}
			if sequence == 0 {
				facts, ok := ancillary[key+":"+componentKey]
				if !ok {
					return nil, fmt.Errorf("USDA missing ancillary component %s/%s", key, componentKey)
				}
				item.Facts = facts
			}
			if sequence == 0 {
				item.Rating, _, e = rating(value)
				if e != nil {
					return nil, e
				}
				item.OriginalRating = value
				item.Availability = Rated
				if value == "" {
					item.Availability = Unavailable
				} else if value == "Not rated" {
					item.Availability = NotRated
				}
				if reasonKey != ruleKey || reasonName != target.RuleName {
					return nil, errors.New("USDA component rule identity changed")
				}
			} else {
				if reasonKey == "" || reasonName == "" {
					return nil, errors.New("USDA reason identity missing")
				}
				item.ReasonKey = reasonKey
				item.ReasonName = reasonName
				item.Reason = value
				item.Severity = severity
			}
			target.Components = append(target.Components, item)
		}
		for j := batchStart; j < len(results); j++ {
			result := &results[j]
			headers := map[string]ComponentResult{}
			sum, unrated := 0.0, 0.0
			knownPercent := true
			for _, c := range result.Components {
				if c.ReasonKey != "" {
					continue
				}
				headers[c.Key] = c
				if c.Percent == nil {
					knownPercent = false
					continue
				}
				sum += *c.Percent
				if c.Availability != Rated {
					unrated += *c.Percent
				}
			}
			for _, c := range result.Components {
				if c.ReasonKey == "" {
					continue
				}
				header, ok := headers[c.Key]
				if !ok || header.Name != c.Name || (header.Percent == nil) != (c.Percent == nil) || (header.Percent != nil && *header.Percent != *c.Percent) {
					return nil, errors.New("USDA component reason has no matching component rating")
				}
			}
			for _, m := range result.Measurements {
				if _, ok := headers[m.ComponentKey]; !ok {
					return nil, fmt.Errorf("USDA measurement has no rated component %s/%s", result.MapUnitKey, m.ComponentKey)
				}
			}
			if sum > 100.001 {
				return nil, fmt.Errorf("USDA component percentages exceed 100 for %s", result.MapUnitKey)
			}
			if knownPercent {
				unrated += math.Max(0, 100-sum)
				result.UnratedComponentPercent = &unrated
			} else {
				result.Warnings = append(result.Warnings, "Component percentage unavailable; unrated share unknown")
			}
			if sum < 100 || len(headers) == 0 {
				result.Warnings = append(result.Warnings, "Component information incomplete; component percentages are not located parcel acreage")
			}
		}
	}
	return results, nil
}

// ScreeningOptions binds screening to a committed soil set and limits evidence age.
// Every explicit Screening invocation refreshes evidence.
type ScreeningOptions struct {
	FreshnessInterval time.Duration
	SoilSetID         string
}

const DefaultFreshnessInterval = 30 * 24 * time.Hour

// Screening publishes interpretations and assessments for the set returned by
// StoreWithSet. A replaced set or required fetch failure preserves the prior head.
func Screening(ctx context.Context, connectionString string, countyID county.ID, source string, polygons []Polygon, units []MapUnit, options ...ScreeningOptions) error {
	pool, err := pgxpool.New(ctx, connectionString)
	if err != nil {
		return err
	}
	defer pool.Close()
	return screen(ctx, pool, http.DefaultClient, soilDataAccessURL, countyID, source, polygons, units, options...)
}

func screen(ctx context.Context, pool *pgxpool.Pool, client *http.Client, endpoint string, countyID county.ID, source string, polygons []Polygon, units []MapUnit, options ...ScreeningOptions) (runErr error) {
	var previousHead string
	defer func() {
		if runErr != nil {
			failureCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			failureErr := recordScreeningFailure(failureCtx, pool, countyID, source, previousHead, runErr)
			runErr = errors.Join(runErr, failureErr)
		}
	}()
	if err := pool.QueryRow(ctx, `SELECT COALESCE((SELECT run_id::text FROM soil_screening_heads WHERE county_id=$1 AND source_name=$2),'')`, countyID.String(), source).Scan(&previousHead); err != nil {
		return err
	}
	if err := validateMembership(countyID, source, polygons, units); err != nil {
		return err
	}
	interval := DefaultFreshnessInterval
	if len(options) != 1 || options[0].SoilSetID == "" {
		return errors.New("soil: screening requires the committed soil set ID")
	}
	if options[0].FreshnessInterval != 0 {
		interval = options[0].FreshnessInterval
	}
	if interval <= 0 {
		return errors.New("soil: freshness interval must be positive")
	}
	keys := make([]string, 0, len(polygons))
	for _, polygon := range polygons {
		keys = append(keys, polygon.MapUnitKey)
	}
	values, err := FetchInterpretations(ctx, client, endpoint, keys)
	if err != nil {
		return err
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "soil:"+countyID.String()+":"+source); err != nil {
		return err
	}
	var setID string
	if err = tx.QueryRow(ctx, `SELECT set_id FROM soil_polygon_set_heads WHERE county_id=$1 AND source_name=$2`, countyID.String(), source).Scan(&setID); err != nil {
		return fmt.Errorf("soil: no committed polygon set: %w", err)
	}
	if setID != options[0].SoilSetID {
		return errors.New("soil: a newer polygon set replaced the source import")
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM soil_polygon_set_members WHERE set_id=$1`, setID).Scan(&count); err != nil {
		return err
	}
	if count != len(polygons) {
		return errors.New("soil: screening membership differs from committed set")
	}
	var runID string
	err = tx.QueryRow(ctx, `INSERT INTO soil_screening_runs(county_id,source_name,source_url,method,soil_set_id,assessment_method,fresh_until) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`, countyID.String(), source, endpoint, screeningMethod, setID, assessmentMethod, time.Now().UTC().Add(interval)).Scan(&runID)
	if err != nil {
		return err
	}
	for _, polygon := range polygons {
		shape, e := ewkb.Marshal(*polygon.Shape, ewkb.NDR)
		if e != nil {
			return e
		}
		tag, e := tx.Exec(ctx, `INSERT INTO soil_screening_members(run_id,polygon_id)
SELECT $1,m.polygon_id FROM soil_polygon_set_members m JOIN soil_polygons p ON p.id=m.polygon_id WHERE m.set_id=$2 AND p.county_id=$3 AND p.source_name=$4 AND p.object_id=$5 AND m.map_unit_key=$6 AND ST_AsEWKB(m.shape)=$7 AND m.spatial_version IS NOT DISTINCT FROM $8`, runID, setID, countyID.String(), source, polygon.ObjectID, polygon.MapUnitKey, shape, polygon.SpatialVersion)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("soil: polygon %s differs from committed set", polygon.ObjectID)
		}
	}
	for _, v := range values {
		components, e := json.Marshal(v.Components)
		if e != nil {
			return e
		}
		evidence, e := json.Marshal(struct {
			Facts                   map[string]*string `json:"facts"`
			UnratedComponentPercent *float64           `json:"unrated_component_percent"`
		}{v.Evidence, v.UnratedComponentPercent})
		if e != nil {
			return e
		}
		provenance, e := json.Marshal(v)
		if e != nil {
			return e
		}
		var rated any
		if v.Availability == Rated {
			rated = v.Rating
		}
		var interpretationID string
		if e = tx.QueryRow(ctx, `INSERT INTO soil_interpretation_keys(map_unit_key,rule_key,aggregation,survey_version,rule_version) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (map_unit_key,rule_key,aggregation,survey_version,rule_version) DO UPDATE SET map_unit_key=excluded.map_unit_key RETURNING id`, v.MapUnitKey, v.RuleKey, v.Aggregation, v.Revision.SurveyVersion, v.Revision.RuleVersion).Scan(&interpretationID); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO soil_screening_ratings(run_id,map_unit_key,rule_key,aggregation,rating,availability,components,evidence,source_query,fetched_at,provenance,interpretation_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, runID, v.MapUnitKey, v.RuleKey, v.Aggregation, rated, v.Availability, components, evidence, v.SourceQuery, v.FetchedAt, provenance, interpretationID); e != nil {
			return e
		}
		for _, m := range v.Measurements {
			payload, e := json.Marshal(m)
			if e != nil {
				return e
			}
			if _, e = tx.Exec(ctx, `INSERT INTO soil_measurements(run_id,interpretation_id,component_key,kind,source_key,position,payload) VALUES ($1,$2,$3,$4,$5,$6,$7)`, runID, interpretationID, m.ComponentKey, m.Kind, m.SourceKey, m.Position, payload); e != nil {
				return e
			}
		}
		for _, component := range v.Components {
			facts, e := json.Marshal(component.Facts)
			if e != nil {
				return e
			}
			var availability any
			if component.Sequence == 0 {
				availability = component.Availability
			}
			if _, e = tx.Exec(ctx, `INSERT INTO soil_component_facts(run_id,interpretation_id,component_key,sequence,name,percent,availability,original_rating,rating,reason_key,reason_name,reason,severity,facts) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, runID, interpretationID, component.Key, component.Sequence, component.Name, component.Percent, availability, nullableString(component.OriginalRating), nullableString(component.Rating), nullableString(component.ReasonKey), nullableString(component.ReasonName), nullableString(component.Reason), component.Severity, facts); e != nil {
				return e
			}
		}
	}
	if err = assessParcels(ctx, tx, runID, setID, countyID, source); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO soil_screening_heads(county_id,source_name,run_id) VALUES ($1,$2,$3) ON CONFLICT(county_id,source_name) DO UPDATE SET run_id=excluded.run_id,refresh_failed_at=NULL,refresh_error=NULL`, countyID.String(), source, runID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM soil_screening_refresh_status WHERE county_id=$1 AND source_name=$2`, countyID.String(), source); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// A late failure must not mark a newer successful screening run as failed.
func recordScreeningFailure(ctx context.Context, pool *pgxpool.Pool, countyID county.ID, source, previousHead string, failure error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "soil:"+countyID.String()+":"+source); err != nil {
		return err
	}
	var currentHead string
	if err = tx.QueryRow(ctx, `SELECT COALESCE((SELECT run_id::text FROM soil_screening_heads WHERE county_id=$1 AND source_name=$2),'')`, countyID.String(), source).Scan(&currentHead); err != nil {
		return err
	}
	if currentHead != previousHead {
		return tx.Commit(ctx)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO soil_screening_refresh_status(county_id,source_name,failed_at,error) VALUES ($1,$2,now(),$3) ON CONFLICT(county_id,source_name) DO UPDATE SET failed_at=excluded.failed_at,error=excluded.error`, countyID.String(), source, failure.Error()); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE soil_screening_heads SET refresh_failed_at=now(),refresh_error=$3 WHERE county_id=$1 AND source_name=$2`, countyID.String(), source, failure.Error()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// CurrentInterpretations returns only the complete published generation. Failure
// status is independent of whether this map unit has evidence in the prior head.
func CurrentInterpretations(ctx context.Context, pool *pgxpool.Pool, countyID county.ID, source, key string) ([]Interpretation, *time.Time, error) {
	var failed *time.Time
	if err := pool.QueryRow(ctx, `SELECT (SELECT failed_at FROM soil_screening_refresh_status WHERE county_id=$1 AND source_name=$2)`, countyID.String(), source).Scan(&failed); err != nil {
		return nil, nil, err
	}
	rows, err := pool.Query(ctx, `SELECT r.provenance,r.interpretation_id::text, (SELECT COALESCE(jsonb_agg(jsonb_build_object('sequence',c.sequence,'key',c.component_key,'name',c.name,'percent',c.percent,'availability',c.availability,'original_rating',c.original_rating,'rating',c.rating,'reason_key',c.reason_key,'reason_name',c.reason_name,'reason',c.reason,'severity',c.severity,'facts',c.facts) ORDER BY c.component_key,c.sequence),'[]'::jsonb) FROM soil_component_facts c WHERE c.run_id=r.run_id AND c.interpretation_id=r.interpretation_id), (SELECT COALESCE(jsonb_agg(m.payload ORDER BY m.component_key,m.kind,m.position),'[]'::jsonb) FROM soil_measurements m WHERE m.run_id=r.run_id AND m.interpretation_id=r.interpretation_id) FROM soil_screening_heads h JOIN soil_screening_ratings r ON r.run_id=h.run_id WHERE h.county_id=$1 AND h.source_name=$2 AND r.map_unit_key=$3 ORDER BY r.rule_key`, countyID.String(), source, key)
	if err != nil {
		return nil, failed, err
	}
	defer rows.Close()
	result := []Interpretation{}
	for rows.Next() {
		var raw, components, measurements []byte
		var id string
		if err = rows.Scan(&raw, &id, &components, &measurements); err != nil {
			return nil, failed, err
		}
		var v Interpretation
		if err = json.Unmarshal(raw, &v); err != nil {
			return nil, failed, err
		}
		v.ID = id
		if err = json.Unmarshal(components, &v.Components); err != nil {
			return nil, failed, err
		}
		if err = json.Unmarshal(measurements, &v.Measurements); err != nil {
			return nil, failed, err
		}
		result = append(result, v)
	}
	return result, failed, rows.Err()
}
