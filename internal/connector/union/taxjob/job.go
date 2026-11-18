// Package taxjob imports the four operator-supplied Union tax files in source order.
package taxjob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"

	"github.com/JustinMonty20/landman/internal/connector/union"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config identifies four local files and their declared source URLs.
type Config struct {
	ArtifactDir, DelinquentPath, BillingPath, ForeclosurePath, PipelinePath string
	BillingSourceURL, PipelineSourceURL                                     string
}

// Run validates input paths before the first typed write and publishes each complete file atomically.
func Run(ctx context.Context, pool *pgxpool.Pool, config Config) (jobs.Result, error) {
	var result jobs.Result
	if config.ArtifactDir == "" || config.BillingSourceURL == "" || config.PipelineSourceURL == "" {
		return result, errors.New("tax job requires private artifact directory and declared source URLs")
	}
	for _, raw := range []string{config.BillingSourceURL, config.PipelineSourceURL} {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
			return result, errors.New("tax job requires HTTPS declared source URLs")
		}
	}
	for _, path := range []string{config.DelinquentPath, config.BillingPath, config.ForeclosurePath, config.PipelinePath} {
		if path == "" {
			return result, errors.New("tax job requires all four file paths")
		}
		info, err := os.Stat(path)
		if err != nil {
			return result, fmt.Errorf("tax input %q: %w", filepath.Base(path), err)
		}
		if !info.Mode().IsRegular() {
			return result, errors.New("tax input must be a regular file")
		}
		file, err := os.Open(path)
		if err != nil {
			return result, fmt.Errorf("tax input %q: %w", filepath.Base(path), err)
		}
		var probe [1]byte
		_, readErr := file.Read(probe[:])
		closeErr := file.Close()
		if readErr != nil && readErr != io.EOF || closeErr != nil {
			return result, fmt.Errorf("tax input %q unreadable: %w", filepath.Base(path), errors.Join(readErr, closeErr))
		}
	}
	if _, ok := jobs.AttemptID(ctx); !ok {
		return result, errors.New("tax import requires a running runner attempt")
	}
	store, err := taxartifact.NewLocal(config.ArtifactDir)
	if err != nil {
		return result, fmt.Errorf("select private tax artifact store: %w", err)
	}
	inputs := []struct{ source, url, path string }{
		{"union_delinquent", union.DelinquentURL, config.DelinquentPath},
		{"union_billing", config.BillingSourceURL, config.BillingPath},
		{"union_foreclosure", union.ForeclosureURL, config.ForeclosurePath},
		{"union_foreclosure_pipeline", config.PipelineSourceURL, config.PipelinePath},
	}
	for _, input := range inputs {
		imported, err := ingestFile(ctx, pool, store, config.ArtifactDir, input.source, input.url, input.path)
		if err != nil {
			return result, fmt.Errorf("%s: %w", input.source, err)
		}
		result.Processed += imported.Processed
	}
	return result, nil
}
