package fema

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/taxartifact"
)

const fixture = `{"EFFECTIVE":{"NFHL_COUNTY_DATA":[{"product_NAME":"NFHL_37119C"},{"product_NAME":"NFHL_37179C","product_ID":17,"product_FILE_PATH":"37179C_20260914.zip","product_ISSUE_DATE":1789423200000,"product_EFFECTIVE_DATE":1542409200000}]}}`

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCaptureUnion(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusPartialContent, http.StatusFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			ctx := context.Background()
			store, err := taxartifact.NewLocal(filepath.Join(t.TempDir(), "artifacts"))
			if err != nil {
				t.Fatal(err)
			}
			recorder, err := ingest.NewRecorder(store, "fema_union", time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
				calls++
				h := http.Header{}
				switch r.URL.String() {
				case catalogURL:
					if r.Method != http.MethodPost || r.ParseForm() != nil || r.Form.Get("selcommunity") != "37179C" {
						t.Fatal("wrong catalog request")
					}
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(fixture)), Header: h}, nil
				case archiveBase + "37179C_20260914.zip":
					if r.Method != http.MethodGet {
						t.Fatal("wrong archive request")
					}
					if status == http.StatusFound {
						h.Set("Location", "https://example.invalid/file.zip")
					}
					return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("fixture archive bytes")), Header: h}, nil
				default:
					t.Fatalf("unexpected request: %s", r.URL)
					return nil, nil
				}
			})}
			product, err := CaptureUnion(ctx, client, recorder)
			if calls != 2 {
				t.Fatalf("requests: %d", calls)
			}
			if status != http.StatusOK {
				if err == nil || len(recorder.Manifest().Receipts) != 1 {
					t.Fatalf("archive accepted: %v", err)
				}
				return
			}
			if err != nil || product.ID != 17 {
				t.Fatalf("product: %+v: %v", product, err)
			}
			capture, err := recorder.Verify(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for i, want := range []string{fixture, "fixture archive bytes"} {
				body, err := capture.Open(ctx, i)
				if err != nil {
					t.Fatal(err)
				}
				got, err := io.ReadAll(body)
				closeErr := body.Close()
				if err != nil || closeErr != nil || string(got) != want {
					t.Fatalf("receipt %d differs: %v %v", i, err, closeErr)
				}
			}
		})
	}
}

func TestSelectUnionRejectsUnexpectedProduct(t *testing.T) {
	for _, input := range []string{
		strings.Replace(fixture, "37179C_20260914.zip", "https://example.invalid/37179C_20260914.zip", 1),
		strings.Replace(fixture, "NFHL_37179C", "NFHL_37171C", 1),
		strings.Replace(fixture, `"EFFECTIVE"`, `"PRELIM"`, 1),
		strings.Replace(fixture, `{"product_NAME":"NFHL_37119C"}`, fixture[strings.Index(fixture, `{"product_NAME":"NFHL_37179C"`):strings.LastIndex(fixture, "]")], 1),
	} {
		if _, err := selectUnion([]byte(input)); err == nil {
			t.Fatalf("accepted unexpected product: %s", input)
		}
	}
}
