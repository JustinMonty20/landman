package taxjob

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/JustinMonty20/landman/internal/taxsource"
	"strings"
	"testing"
)

func TestStageRowsInBoundedBatches(t *testing.T) {
	ctx := context.Background()
	file, _, err := stageRows(ctx, t.TempDir(), func(_ context.Context, yield func(string) error) (taxsource.Meta, error) {
		for i := 0; i < 10000; i++ {
			if err := yield(fmt.Sprintf("key-%d", i)); err != nil {
				return taxsource.Meta{}, err
			}
		}
		return taxsource.Meta{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer releaseRows(file)
	batches := 0
	count, err := saveRows(ctx, nil, file, func(rows []string) (int, error) {
		batches++
		if len(rows) > 512 {
			t.Fatal("unbounded batch")
		}
		return len(rows), nil
	})
	if err != nil || count != 10000 || batches < 2 {
		t.Fatalf("count=%d batches=%d err=%v", count, batches, err)
	}
}

func TestRunRejectsMissingInputsBeforeWriting(t *testing.T) {
	root := t.TempDir()
	cfg := Config{ArtifactDir: filepath.Join(root, "private"), BillingSourceURL: "https://example.org/billing", PipelineSourceURL: "https://example.org/pipeline"}
	if _, err := Run(context.Background(), nil, cfg); err == nil || !strings.Contains(err.Error(), "four file paths") {
		t.Fatalf("missing paths: %v", err)
	}
	if _, err := os.Stat(cfg.ArtifactDir); !os.IsNotExist(err) {
		t.Fatalf("created artifact directory for invalid job: %v", err)
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg.DelinquentPath, cfg.BillingPath, cfg.ForeclosurePath, cfg.PipelinePath = file, file, file, filepath.Join(root, "missing")
	if _, err := Run(context.Background(), nil, cfg); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("unreadable path: %v", err)
	}
	if _, err := os.Stat(cfg.ArtifactDir); !os.IsNotExist(err) {
		t.Fatalf("created artifact directory for unreadable input: %v", err)
	}
	cfg.PipelinePath = root
	if _, err := Run(context.Background(), nil, cfg); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("directory as input: %v", err)
	}
	cfg.PipelinePath = file
	if err := os.Chmod(file, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(file, 0600)
	if probe, err := os.Open(file); err == nil {
		probe.Close()
		t.Skip("filesystem permits opening mode-000 file")
	}
	if _, err := Run(context.Background(), nil, cfg); err == nil || !strings.Contains(err.Error(), "tax input") {
		t.Fatalf("unreadable file: %v", err)
	}
	if _, err := os.Stat(cfg.ArtifactDir); !os.IsNotExist(err) {
		t.Fatalf("created artifact directory for unreadable file: %v", err)
	}
}
