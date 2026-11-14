package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
)

const catalogSQL = `
SELECT table_name || chr(9) || column_name
FROM information_schema.columns
WHERE table_schema = 'public'
ORDER BY table_name, column_name`

// schemaReport compares seed insert columns to a live catalog.
// The text is "schema: ok" or one "schema: table.column missing" line per gap.
func schemaReport(snap seedSnapshot, live map[string]map[string]struct{}) (string, int) {
	var problems []string
	tables := make([]string, 0, len(snap.columns))
	for table := range snap.columns {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	for _, table := range tables {
		for _, column := range snap.columns[table] {
			if _, ok := live[table][column]; ok {
				continue
			}
			problems = append(problems, fmt.Sprintf("schema: %s.%s missing", table, column))
		}
	}
	if len(problems) == 0 {
		return "schema: ok\n", 0
	}
	return strings.Join(problems, "\n") + "\n", 1
}

func parseCatalog(raw string) map[string]map[string]struct{} {
	live := map[string]map[string]struct{}{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		table, column, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		table = strings.TrimSpace(table)
		column = strings.TrimSpace(column)
		if live[table] == nil {
			live[table] = map[string]struct{}{}
		}
		live[table][column] = struct{}{}
	}
	return live
}

func checkSchema(container, gooseURL, seedPath string, query sqlQuery) (string, int) {
	if err := refuseContainer(container); err != nil {
		fmt.Fprintf(os.Stderr, "check-schema: %v\n", err)
		return "", 1
	}
	text, err := os.ReadFile(seedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-schema: %v\n", err)
		return "", 1
	}
	snap, err := parseSeed(string(text))
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-schema: %v\n", err)
		return "", 1
	}
	raw, err := query(context.Background(), container, gooseURL, catalogSQL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-schema: %v\n", err)
		return "", 1
	}
	return schemaReport(snap, parseCatalog(raw))
}
