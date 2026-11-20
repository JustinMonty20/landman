package main

import (
	"context"
	"errors"
	"os"

	"github.com/JustinMonty20/landman/internal/connector/fema"
	"github.com/JustinMonty20/landman/internal/jobs"
	"github.com/JustinMonty20/landman/internal/taxartifact"
	"github.com/jackc/pgx/v5/pgxpool"
)

type femaInput struct {
	artifactDir string
	catalogPath string
	archivePath string
}

func manualFEMAInput(getenv func(string) string) (femaInput, error) {
	input := femaInput{
		artifactDir: getenv("UNION_FEMA_ARTIFACT_DIR"),
		catalogPath: getenv("UNION_FEMA_CATALOG_PATH"),
		archivePath: getenv("UNION_FEMA_ARCHIVE_PATH"),
	}
	for _, entry := range []struct{ name, value string }{
		{"UNION_FEMA_ARTIFACT_DIR", input.artifactDir},
		{"UNION_FEMA_CATALOG_PATH", input.catalogPath},
		{"UNION_FEMA_ARCHIVE_PATH", input.archivePath},
	} {
		if entry.value == "" {
			return femaInput{}, errors.New(entry.name + " is required")
		}
	}
	return input, nil
}

func femaDefinition(pool *pgxpool.Pool, input femaInput) (jobs.Definition, error) {
	store, err := taxartifact.NewLocal(input.artifactDir)
	if err != nil {
		return jobs.Definition{}, err
	}
	job := fema.Job{Pool: pool, Artifacts: store}
	return jobs.Definition{ID: fema.JobID, Run: func(ctx context.Context) (jobs.Result, error) {
		return job.Import(ctx, input.catalogPath, input.archivePath)
	}}, nil
}

func runFEMA(ctx context.Context, mode string) error {
	root := os.Getenv("UNION_FEMA_ARTIFACT_DIR")
	if root == "" {
		return errors.New("UNION_FEMA_ARTIFACT_DIR is required")
	}
	catalog, archive := os.Getenv("UNION_FEMA_CATALOG_PATH"), os.Getenv("UNION_FEMA_ARCHIVE_PATH")
	if mode == fema.JobID && (catalog == "" || archive == "") {
		return errors.New("UNION_FEMA_CATALOG_PATH and UNION_FEMA_ARCHIVE_PATH are required")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(ctx, jobTimeout)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()
	jobRunner, err := jobs.NewRunner(pool)
	if err != nil {
		return err
	}
	if err := jobRunner.RecoverInterrupted(ctx); err != nil {
		return err
	}
	runner := loggingRunner{jobRunner}
	if mode == "fema-replay" {
		store, err := taxartifact.NewLocal(root)
		if err != nil {
			return err
		}
		job := fema.Job{Pool: pool, Artifacts: store}
		def := jobs.Definition{ID: fema.JobID, Kind: jobs.AttemptReplay, Run: func(ctx context.Context) (jobs.Result, error) { return job.Replay(ctx, os.Args[4]) }}
		return runRequired(ctx, runner, def)
	}
	def, err := femaDefinition(pool, femaInput{artifactDir: root, catalogPath: catalog, archivePath: archive})
	if err != nil {
		return err
	}
	return runRequired(ctx, loggingRunner{jobRunner}, def)
}
