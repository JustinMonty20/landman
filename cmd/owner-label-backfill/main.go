// Command owner-label-backfill classifies already stored Union GIS parcels without source requests.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/JustinMonty20/landman/internal/parcelstore"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	if err = pool.Ping(ctx); err != nil {
		log.Fatal(err)
	}
	processed, total, err := parcelstore.BackfillOwnerLabels(ctx, pool)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("classified %d stored parcels; %d of %d Union GIS parcels labeled\n", processed, total, total)
}
