//go:build postgres

package main

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestGoldenUnionTaxCI(t *testing.T) {
	if os.Getenv("TAX_GOLDEN_DB") != "1" {
		t.Skip("requires an approved disposable PostgreSQL database")
	}
	dsn := os.Getenv("DATABASE_URL")
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if config.ConnConfig.Database == "landman_tax_integration" {
		if err := disposableTaxDSN(dsn); err != nil {
			t.Fatal(err)
		}
	} else if !githubActionsTaxDB(config) {
		t.Fatal("golden tax test requires the disposable CI database or a guarded local integration database")
	}
	testGoldenUnionTaxJob(t)
}

func TestGoldenCIURLGuard(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	for _, tc := range []struct {
		url  string
		want bool
	}{
		{"postgres://postgres@localhost:5432/landman_test?sslmode=disable", true},
		{"postgres://postgres@localhost:5432/landman?sslmode=disable", false},
		{"postgres://postgres@example.invalid:5432/landman_test?sslmode=disable", false},
		{"postgres://postgres@localhost:5432,example.invalid:5432/landman_test?sslmode=disable", false},
	} {
		config, err := pgxpool.ParseConfig(tc.url)
		if err != nil {
			t.Fatal(err)
		}
		if got := githubActionsTaxDB(config); got != tc.want {
			t.Errorf("CI database guard for %q = %t, want %t", tc.url, got, tc.want)
		}
	}
}

func githubActionsTaxDB(config *pgxpool.Config) bool {
	host := config.ConnConfig.Host
	return os.Getenv("GITHUB_ACTIONS") == "true" && config.ConnConfig.Database == "landman_test" &&
		(host == "localhost" || host == "127.0.0.1") && config.ConnConfig.Port == 5432 && len(config.ConnConfig.Fallbacks) == 0
}
