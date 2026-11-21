//go:build postgres

package vacancyjob

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAssessmentProgressIncludesAttemptAndRequestCount(t *testing.T) {
	ctx := context.Background()
	dsn := os.Getenv("API_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("API_TEST_DATABASE_URL missing")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var name string
	if err := pool.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || !strings.HasPrefix(name, "landman_api_test_") {
		t.Fatal("refusing non-disposable database")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := taxartifact.NewLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := jobs.NewRunner(pool)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"count":2001}`)
	}))
	defer server.Close()
	source, err := union.NewGISSource(union.GISSourceConfig{BaseURL: server.URL + "/server/rest/services/OperationalLayers/MapServer/215/query", RateLimit: 1000})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	logger := log.Default()
	previous := logger.Writer()
	logger.SetOutput(&output)
	t.Cleanup(func() { logger.SetOutput(previous) })
	job := Job{Source: source, Pool: pool, Artifacts: store, Layer: union.AssessedValueLayer}
	_, _, err = runner.Run(ctx, jobs.Definition{ID: AssessedValueID, Run: job.Import})
	if err == nil {
		t.Fatal("expected indivisible range failure")
	}
	start := regexp.MustCompile(`Capture county records attempt=([a-f0-9-]{36})`).FindStringSubmatch(output.String())
	if len(start) != 2 {
		t.Fatal("missing capture attempt ID")
	}
	want := "attempt=" + start[1] + ": Count ranges: checked 0/2001 rows; requests=10/2048"
	if !strings.Contains(output.String(), want) {
		t.Fatalf("missing attempt-linked progress line: %s", want)
	}
}
