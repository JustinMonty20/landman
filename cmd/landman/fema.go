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
	store, err := taxartifact.NewLocal(root)
	if err != nil {
		return err
	}
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
	job := fema.Job{Pool: pool, Artifacts: store}
	def := jobs.Definition{ID: fema.JobID, Run: func(ctx context.Context) (jobs.Result, error) { return job.Import(ctx, catalog, archive) }}
	if mode == "fema-replay" {
		def.Kind = jobs.AttemptReplay
		def.Run = func(ctx context.Context) (jobs.Result, error) { return job.Replay(ctx, os.Args[4]) }
	}
	return runRequired(ctx, loggingRunner{jobRunner}, def)
}
