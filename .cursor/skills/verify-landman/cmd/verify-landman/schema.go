package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
)

const catalogSQL = `
SELECT table_name || chr(9) || column_name || chr(9) || is_nullable || chr(9) || coalesce(column_default, '')
FROM information_schema.columns
WHERE table_schema = 'public'
ORDER BY table_name, column_name`

// liveColumn is one catalog column the seed may insert.
type liveColumn struct {
	nullable   bool
	hasDefault bool
}

// schemaReport compares seed insert columns to a live catalog.
// A missing inserted column, or a NOT NULL column with no default that the
// seed omits, is one "schema: table.column ..." line.
func schemaReport(snap seedSnapshot, live map[string]map[string]liveColumn) (string, int) {
	var problems []string
	tables := make([]string, 0, len(snap.columns))
	for table := range snap.columns {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	for _, table := range tables {
		inserted := map[string]struct{}{}
		for _, column := range snap.columns[table] {
			inserted[column] = struct{}{}
			if _, ok := live[table][column]; ok {
				continue
			}
			problems = append(problems, fmt.Sprintf("schema: %s.%s missing", table, column))
		}
		names := make([]string, 0, len(live[table]))
		for column := range live[table] {
			names = append(names, column)
		}
		sort.Strings(names)
		for _, column := range names {
			col := live[table][column]
			if _, ok := inserted[column]; ok || col.nullable || col.hasDefault {
				continue
			}
			problems = append(problems, fmt.Sprintf("schema: %s.%s not null", table, column))
		}
	}
	if len(problems) == 0 {
		return "schema: ok\n", 0
	}
	return strings.Join(problems, "\n") + "\n", 1
}

func parseCatalog(raw string) map[string]map[string]liveColumn {
	live := map[string]map[string]liveColumn{}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 4 {
			continue
		}
		table := strings.TrimSpace(parts[0])
		column := strings.TrimSpace(parts[1])
		if live[table] == nil {
			live[table] = map[string]liveColumn{}
		}
		live[table][column] = liveColumn{
			nullable:   strings.EqualFold(strings.TrimSpace(parts[2]), "YES"),
			hasDefault: strings.TrimSpace(parts[3]) != "",
		}
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
