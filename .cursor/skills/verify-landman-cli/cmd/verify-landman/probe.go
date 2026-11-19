package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"
)

const (
	probeTimeout = 30 * time.Second
	parcelLayer  = "https://atlas.unioncountync.gov/server/rest/services/OperationalLayers/MapServer/215"
	soilLayer    = "https://atlas.unioncountync.gov/server/rest/services/OperationalLayers/MapServer/197"
)

var countyClient = &http.Client{Timeout: probeTimeout}

type httpGetter func(ctx context.Context, rawURL string) (status int, body []byte, err error)

func httpGet(ctx context.Context, rawURL string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("User-Agent", "landman-verify")
	resp, err := countyClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

func timedGet(ctx context.Context, get httpGetter, rawURL string) timedCall {
	started := time.Now()
	status, _, err := get(ctx, rawURL)
	call := timedCall{HTTPStatus: status, DurationMS: time.Since(started).Milliseconds()}
	if err != nil {
		if timeoutErr(err) {
			call.Error = "timeout"
		} else {
			call.Error = err.Error()
		}
	}
	return call
}

func timeoutErr(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// runProbe times layer metadata and one data page for each layer.
// It writes page-timings.json. A slow data page still exits 0 when both
// metadata calls returned HTTP 200.
func runProbe(evidence string, get httpGetter) int {
	ctx := context.Background()
	timings := pageTimings{
		ParcelMetadata: timedGet(ctx, get, parcelLayer+"?f=json"),
		SoilMetadata:   timedGet(ctx, get, soilLayer+"?f=json"),
		ParcelPage:     timedGet(ctx, get, parcelPageURL()),
		SoilPage:       timedGet(ctx, get, soilPageURL()),
	}
	if err := writeJSON(evidence+"/page-timings.json", timings); err != nil {
		fmt.Fprintf(os.Stderr, "probe: %v\n", err)
		return 1
	}
	if timings.ParcelMetadata.HTTPStatus == 200 && timings.ParcelMetadata.Error == "" &&
		timings.SoilMetadata.HTTPStatus == 200 && timings.SoilMetadata.Error == "" {
		return 0
	}
	fmt.Fprintf(os.Stderr, "probe: metadata HTTP %d and %d\n", timings.ParcelMetadata.HTTPStatus, timings.SoilMetadata.HTTPStatus)
	return 1
}

func parcelPageURL() string {
	values := url.Values{}
	values.Set("where", "1=1")
	values.Set("outFields", "*")
	values.Set("returnGeometry", "true")
	values.Set("orderByFields", "OBJECTID")
	values.Set("resultRecordCount", "2000")
	values.Set("f", "json")
	return parcelLayer + "/query?" + values.Encode()
}

func soilPageURL() string {
	values := url.Values{}
	values.Set("where", "1=1")
	values.Set("outFields", "OBJECTID_1,AREASYMBOL,SPATIALVER,MUSYM,MUKEY")
	values.Set("returnGeometry", "true")
	values.Set("orderByFields", "OBJECTID_1")
	values.Set("resultRecordCount", "2000")
	values.Set("f", "json")
	return soilLayer + "/query?" + values.Encode()
}

func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}
