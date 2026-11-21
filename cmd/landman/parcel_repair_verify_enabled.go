//go:build parcelrepairverify

package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func checkParcelRepairVerificationMode(ctx context.Context, mode, dsn string) error {
	const database = "landman_parcel_repair_test"
	if mode != "replay" || dsn == "" {
		return fmt.Errorf("repair verification build only supports parcel replay in %s", database)
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("invalid repair verification database connection")
	}
	if (cfg.Host != "127.0.0.1" && cfg.Host != "::1") || cfg.Database != database || len(cfg.Fallbacks) > 1 {
		return fmt.Errorf("repair verification requires an explicit loopback connection to %s", database)
	}
	return nil
}
