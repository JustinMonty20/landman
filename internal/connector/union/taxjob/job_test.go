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
