package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/JustinMonty20/landman/internal/auth"
	"github.com/JustinMonty20/landman/internal/parcelapi"
	"github.com/jackc/pgx/v5/pgxpool"
)

func run() error {
	config, err := auth.LoadConfig(os.Getenv)
	if err != nil {
		return err
	}
	if os.Getenv("DATABASE_URL") == "" {
		return errors.New("DATABASE_URL is required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startup, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(startup, os.Getenv("DATABASE_URL"))
	if err != nil {
		return errors.New("invalid database configuration")
	}
	defer pool.Close()
	if err := auth.CheckSchema(startup, pool); err != nil {
		return err
	}
	handler, err := auth.New(startup, pool, config, parcelapi.New(pool))
	if err != nil {
		return err
	}
	server := &http.Server{Addr: "127.0.0.1:8080", Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: time.Minute, MaxHeaderBytes: 16 << 10}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	log.Print("Application API: 127.0.0.1:8080. Open the UI at the configured redirect origin.")
	err = server.ListenAndServe()
	stop()
	<-shutdownDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return errors.New("application API stopped")
}

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
