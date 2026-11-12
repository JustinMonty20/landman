package soil

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const soilDataAccessURL = "https://sdmdataaccess.sc.egov.usda.gov/Tabular/post.rest"

type nameResult struct {
	key, symbol, name, areaSymbol, areaName string
	surveyDate                              *time.Time
}

// fetchNames retrieves missing map-unit names in one independent USDA request.
func fetchNames(ctx context.Context, client *http.Client, endpoint string, keys []string) (map[string]nameResult, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	quoted := make([]string, len(keys))
	for i, key := range keys {
		quoted[i] = "'" + strings.ReplaceAll(key, "'", "''") + "'"
	}
	query := fmt.Sprintf(`SELECT m.mukey, m.musym, m.muname, l.areasymbol, s.areaname, s.saverest FROM mapunit AS m JOIN legend AS l ON m.lkey = l.lkey LEFT JOIN sacatalog AS s ON l.areasymbol = s.areasymbol WHERE m.mukey IN (%s)`, strings.Join(quoted, ","))
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
		return nil, fmt.Errorf("USDA names: HTTP %d", resp.StatusCode)
	}
	var data struct {
		Table [][]any `json:"Table"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
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
