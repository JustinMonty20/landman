package parceljob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JustinMonty20/landman/internal/ingest"
	"github.com/JustinMonty20/landman/internal/taxartifact"
)

func TestSharedPublisherPrepareStreamsVerifiedPages(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := taxartifact.NewLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	recorder, err := ingest.NewRecorder(store, "union_county_gis", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	count := `{"count":2}`
	page := `{"spatialReference":{"wkid":3857},"features":[{"attributes":{"OBJECTID":1,"PID":"A"},"geometry":{"rings":[]}}]}`
	pages := []string{count, page, strings.Replace(page, `"OBJECTID":1`, `"OBJECTID":2`, 1)}
	requests := []string{
		"https://example.invalid/query?returnCountOnly=true",
		"https://example.invalid/query?resultOffset=0&resultRecordCount=1",
		"https://example.invalid/query?resultOffset=1&resultRecordCount=1",
	}
	for i, body := range pages {
		crs := 3857
		if i == 0 {
			crs = 0
		}
		if err := recorder.Record(ctx, requests[i], "", crs, strings.NewReader(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := recorder.BindAttempt(JobID, "attempt", "full_scan", 2); err != nil {
		t.Fatal(err)
	}
	verified, err := recorder.Verify(ctx)
	if err != nil {
		t.Fatal(err)
	}
	publisher := &sharedPublisher{}
	if err := publisher.Prepare(ctx, verified); err != nil {
		t.Fatal(err)
	}
	hashed := sha256.New()
	for _, body := range pages {
		fmt.Fprintf(hashed, "%d:%s", len(body), body)
	}
	if publisher.manifest.ArtifactHash != hex.EncodeToString(hashed.Sum(nil)) {
		t.Fatalf("ordered response hash changed: %s", publisher.manifest.ArtifactHash)
	}
	for i, receipt := range publisher.manifest.Receipts {
		if receipt.Bytes != len(pages[i]) || !receipt.Validated || receipt.SHA256 != verified.Manifest().Receipts[i].SHA256 {
			t.Fatalf("receipt %d changed: %+v", i, receipt)
		}
	}
	// The publisher retains only immutable references and small receipt metadata.
	// A changed artifact must be detected when the page is reopened for publication.
	artifact, err := verified.Artifact(1)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(artifact.Key())), []byte(strings.Replace(page, `"PID":"A"`, `"PID":"B"`, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readVerifiedParcelPage(ctx, verified, 1); err == nil {
		t.Fatal("accepted modified retained response after preparation")
	}
}
