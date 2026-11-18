package soil

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const soilDataAccessURL = "https://sdmdataaccess.sc.egov.usda.gov/Tabular/post.rest"
const nameLookupTimeout = 5 * time.Second

func usdaQuery(ctx context.Context, client *http.Client, endpoint, query string) ([]byte, error) {
	form := url.Values{"query": {query}, "format": {"JSON"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("USDA: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (16<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > 16<<20 {
		return nil, fmt.Errorf("USDA response exceeds 16 MiB")
	}
	return body, nil
}

type nameResult struct {
	key, symbol, name, areaSymbol, areaName string
	surveyDate                              *time.Time
}

// fetchNames retrieves missing map-unit names in one independent USDA request.
func fetchNames(ctx context.Context, client *http.Client, endpoint string, keys []string) (map[string]nameResult, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	query := NameQuery(keys)
	lookupCtx, cancel := context.WithTimeout(ctx, nameLookupTimeout)
	defer cancel()
	raw, err := usdaQuery(lookupCtx, client, endpoint, query)
	if err != nil {
		return nil, err
	}
	return ParseNameResponse(raw)
}

// NameQuery identifies the optional USDA map-unit-name response.
func NameQuery(keys []string) string {
	quoted := make([]string, len(keys))
	for i, key := range keys {
		quoted[i] = "'" + strings.ReplaceAll(key, "'", "''") + "'"
	}
	query := fmt.Sprintf(`SELECT m.mukey, m.musym, m.muname, l.areasymbol, s.areaname, s.saverest FROM mapunit AS m JOIN legend AS l ON m.lkey = l.lkey LEFT JOIN sacatalog AS s ON l.areasymbol = s.areasymbol WHERE m.mukey IN (%s)`, strings.Join(quoted, ","))
	return query
}

// ParseNameResponse validates the retained USDA response before publication.
func ParseNameResponse(raw []byte) (map[string]nameResult, error) {
	var data struct {
		Table [][]any `json:"Table"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("USDA names: %w", err)
	}
	if data.Table == nil {
		return nil, fmt.Errorf("USDA names: missing Table")
	}
	results := make(map[string]nameResult, len(data.Table))
	for _, row := range data.Table {
		if len(row) != 6 {
			return nil, fmt.Errorf("USDA names: malformed row")
		}
		fields := make([]string, 6)
		for i, value := range row {
			if value != nil {
				text, ok := value.(string)
				if !ok {
					return nil, fmt.Errorf("USDA names: non-string field")
				}
				fields[i] = text
			}
		}
		var date *time.Time
		if fields[5] != "" {
			parsed, err := time.Parse("1/2/2006 3:04:05 PM", fields[5])
			if err != nil {
				return nil, fmt.Errorf("USDA names: invalid survey date: %w", err)
			}
			date = &parsed
		}
		results[fields[0]] = nameResult{fields[0], fields[1], fields[2], fields[3], fields[4], date}
	}
	return results, nil
}

// CaptureNames keeps optional map-unit labels within the same evidence boundary.
func CaptureNames(ctx context.Context, client *http.Client, endpoint string, keys []string, save func(string, []byte) error) error {
	if len(keys) == 0 {
		return nil
	}
	query := NameQuery(keys)
	lookupCtx, cancel := context.WithTimeout(ctx, nameLookupTimeout)
	defer cancel()
	raw, err := usdaQuery(lookupCtx, client, endpoint, query)
	if err != nil {
		return err
	}
	if save != nil {
		if err := save(query, raw); err != nil {
			return err
		}
	}
	_, err = ParseNameResponse(raw)
	return err
}

// StoreCapturedNamesTx applies optional labels only after their bytes are verified.
func StoreCapturedNamesTx(ctx context.Context, tx pgx.Tx, raw []byte, observedAt time.Time) error {
	names, err := ParseNameResponse(raw)
	if err != nil {
		return err
	}
	for key, name := range names {
		if _, err := tx.Exec(ctx, `UPDATE soil_map_units SET map_unit_name=NULLIF($2,''),map_unit_symbol=$3,area_symbol=$4,area_name=NULLIF($5,''),fetched_at=$6 WHERE map_unit_key=$1 AND (map_unit_name IS DISTINCT FROM NULLIF($2,'') OR map_unit_symbol IS DISTINCT FROM $3 OR area_symbol IS DISTINCT FROM $4 OR area_name IS DISTINCT FROM NULLIF($5,''))`, key, name.name, name.symbol, name.areaSymbol, name.areaName, observedAt); err != nil {
			return err
		}
	}
	return nil
}
