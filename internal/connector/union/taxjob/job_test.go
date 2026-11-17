package taxjob

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
}
