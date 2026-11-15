package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarkdownOmitsStaleJobClaims(t *testing.T) {
	phrases := []string{
		"required soil screening:",
		"source import did not complete cleanly:",
		"after the soil fetch",
		"after that fetch returns",
	}
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "cmd" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		assertNoStaleSales(t, path, string(body), phrases)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMarkdownOmitsStaleSalesClaims(t *testing.T) {
	phrases := []string{
		"does not write parcel_sales",
		"does not call SaveSales",
		"parcel_sales is still",
		"and 79 sales",
	}
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == "cmd" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		assertNoStaleSales(t, path, string(body), phrases)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := os.ReadFile(filepath.Join(root, "seed.sql"))
	if err != nil {
		t.Fatal(err)
	}
	header, _, _ := strings.Cut(string(seed), "\nBEGIN;")
	assertNoStaleSales(t, "seed.sql", header, phrases)
}

func assertNoStaleSales(t *testing.T, name, text string, phrases []string) {
	t.Helper()
	for _, phrase := range phrases {
		if strings.Contains(text, phrase) {
			t.Errorf("%s contains %q", name, phrase)
		}
	}
}
