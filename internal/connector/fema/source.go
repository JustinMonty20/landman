// Package fema captures the official effective NFHL county product.
package fema

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/JustinMonty20/landman/internal/ingest"
)

const (
	catalogURL   = "https://msc.fema.gov/portal/advanceSearch"
	archiveBase  = "https://hazards.fema.gov/nfhlv2/output/County/"
	catalogLimit = 8 << 20
	archiveLimit = 512 << 20
)

var unionArchive = regexp.MustCompile(`^37179C_[0-9]{8}\.zip$`)

type Product struct {
	ID              int64  `json:"product_ID"`
	Name            string `json:"product_NAME"`
	Filename        string `json:"product_FILE_PATH"`
	IssueMillis     int64  `json:"product_ISSUE_DATE"`
	EffectiveMillis int64  `json:"product_EFFECTIVE_DATE"`
}

// CaptureUnion records the catalog and ZIP and returns the selected product.
// The publisher must validate the retained ZIP before it publishes a generation.
func CaptureUnion(ctx context.Context, client *http.Client, recorder *ingest.Recorder) (Product, error) {
	if client == nil || recorder == nil {
		return Product{}, errors.New("FEMA capture requires an HTTP client and recorder")
	}
	c := *client
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	form := url.Values{"method": {"search"}, "selstate": {"37"}, "selcounty": {"37179"}, "selcommunity": {"37179C"}}.Encode()
	response, err := request(ctx, &c, http.MethodPost, catalogURL, strings.NewReader(form))
	if err != nil {
		return Product{}, err
	}
	catalog, err := io.ReadAll(http.MaxBytesReader(nil, response.Body, catalogLimit))
	if err := errors.Join(err, response.Body.Close()); err != nil {
		return Product{}, err
	}
	product, err := selectUnion(catalog)
	if err != nil {
		return Product{}, err
	}
	revision := fmt.Sprint(product.ID)
	if err := recorder.Record(ctx, catalogURL, revision, 0, bytes.NewReader(catalog)); err != nil {
		return Product{}, err
	}
	location := archiveBase + product.Filename
	archive, err := request(ctx, &c, http.MethodGet, location, nil)
	if err != nil {
		return Product{}, err
	}
	body := http.MaxBytesReader(nil, archive.Body, archiveLimit)
	err = recorder.Record(ctx, location, revision, 0, body)
	if err := errors.Join(err, body.Close()); err != nil {
		return Product{}, err
	}
	return product, nil
}

func selectUnion(body []byte) (Product, error) {
	var catalog struct {
		Effective struct {
			County []Product `json:"NFHL_COUNTY_DATA"`
		} `json:"EFFECTIVE"`
	}
	if err := json.Unmarshal(body, &catalog); err != nil {
		return Product{}, fmt.Errorf("FEMA catalog: %w", err)
	}
	var selected Product
	for _, product := range catalog.Effective.County {
		if product.Name != "NFHL_37179C" {
			continue
		}
		if selected.ID != 0 || product.ID <= 0 || product.IssueMillis <= 0 || product.EffectiveMillis <= 0 || !unionArchive.MatchString(product.Filename) {
			return Product{}, errors.New("invalid or ambiguous effective NFHL_37179C catalog product")
		}
		selected = product
	}
	if selected.ID == 0 {
		return Product{}, errors.New("effective NFHL_37179C catalog product not found")
	}
	return selected, nil
}

func request(ctx context.Context, client *http.Client, method, location string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, location, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept-Encoding", "identity")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	encoding := response.Header.Get("Content-Encoding")
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Range") != "" || response.Uncompressed || (encoding != "" && encoding != "identity") {
		response.Body.Close()
		return nil, fmt.Errorf("unexpected FEMA response for %s: HTTP %d, encoding %q", location, response.StatusCode, encoding)
	}
	return response, nil
}
