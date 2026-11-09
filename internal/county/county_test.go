package county

import (
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    ID
		wantErr bool
	}{
		{name: "union", raw: "union", want: Union},
		{name: "wake", raw: "wake", wantErr: true},
		{name: "UNION", raw: "UNION", wantErr: true},
		{name: "Union", raw: "Union", wantErr: true},
		{name: "empty", raw: "", wantErr: true},
		{name: "leading space", raw: " union", wantErr: true},
		{name: "trailing space", raw: "union ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.raw)
			if tt.wantErr {
				if got != "" {
					t.Fatalf("Parse(%q) = %q, want zero ID", tt.raw, got)
				}
				if !errors.Is(err, ErrUnknown) {
					t.Fatalf("Parse(%q) error = %v, want ErrUnknown", tt.raw, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("Parse(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestValid(t *testing.T) {
	tests := []struct {
		name string
		id   ID
		want bool
	}{
		{name: "Union", id: Union, want: true},
		{name: "converted union", id: ID("union"), want: true},
		{name: "wake", id: ID("wake"), want: false},
		{name: "zero", id: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.id.Valid(); got != tt.want {
				t.Fatalf("ID(%q).Valid() = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestString(t *testing.T) {
	if got := Union.String(); got != "union" {
		t.Fatalf("Union.String() = %q, want union", got)
	}
	if got := fmt.Sprint(Union); got != "union" {
		t.Fatalf("fmt.Sprint(Union) = %q, want union", got)
	}
	if got := ID("wake").String(); got != "wake" {
		t.Fatalf("ID(wake).String() = %q, want wake", got)
	}
}

func TestAllReturnsACopy(t *testing.T) {
	first := All()
	if len(first) != 1 || first[0] != Union {
		t.Fatalf("All() = %v, want [union]", first)
	}
	first[0] = ID("wake")
	appended := append(first, ID("extra"))
	if len(appended) != 2 || appended[0] != ID("wake") || appended[1] != ID("extra") {
		t.Fatalf("appended = %v, want [wake extra]", appended)
	}

	second := All()
	if len(second) != 1 || second[0] != Union {
		t.Fatalf("All() after append = %v, want [union]", second)
	}
}

func TestAllMatchesMigrationCheck(t *testing.T) {
	matches, err := filepath.Glob("../../db/migrations/*_parcels_county_check.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("county check migrations = %v, want one", matches)
	}

	body, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	up, down, ok := gooseSections(string(body))
	if !ok {
		t.Fatal("migration is missing goose Up and Down sections")
	}
	quoted, ok := constraintQuoted(up, "parcels_county_id_check")
	if !ok {
		t.Fatal("Up section has no parcels_county_id_check CHECK list")
	}
	if !strings.Contains(down, "DROP CONSTRAINT") || !strings.Contains(down, "parcels_county_id_check") {
		t.Fatalf("Down section = %q, want DROP CONSTRAINT parcels_county_id_check", down)
	}

	want := All()
	if len(quoted) != len(want) {
		t.Fatalf("check list %q, All() = %v", quoted, want)
	}
	for i := range want {
		if quoted[i] != want[i].String() {
			t.Fatalf("check[%d] = %q, All()[%d] = %q", i, quoted[i], i, want[i])
		}
	}
}

func TestImportsAreStandardLibrary(t *testing.T) {
	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	fset := token.NewFileSet()
	parsed := 0
	for _, name := range matches {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		parsed++
		for _, imp := range file.Imports {
			importPath, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", name, err)
			}
			if strings.Contains(importPath, ".") {
				t.Errorf("%s imports %q, want a standard library path", name, importPath)
			}
		}
	}
	if parsed == 0 {
		t.Fatal("parsed no non-test Go files")
	}
}

func BenchmarkParse(b *testing.B) {
	for i := 0; i < b.N; i++ {
		id, err := Parse("union")
		if err != nil || id != Union {
			b.Fatalf("Parse(%q) = %v, %v", "union", id, err)
		}
		id, err = Parse("wake")
		if id != "" || !errors.Is(err, ErrUnknown) {
			b.Fatalf("Parse(%q) = %v, %v", "wake", id, err)
		}
	}
}

func gooseSections(sql string) (up, down string, ok bool) {
	const upMark = "-- +goose Up"
	const downMark = "-- +goose Down"
	upAt := strings.Index(sql, upMark)
	downAt := strings.Index(sql, downMark)
	if upAt < 0 || downAt < 0 || downAt < upAt {
		return "", "", false
	}
	return sql[upAt+len(upMark) : downAt], sql[downAt+len(downMark):], true
}

func constraintQuoted(sql, name string) ([]string, bool) {
	idx := strings.Index(sql, name)
	if idx < 0 {
		return nil, false
	}
	rest := sql[idx:]
	checkAt := strings.Index(rest, "CHECK")
	if checkAt < 0 {
		return nil, false
	}
	rest = rest[checkAt:]
	open := strings.Index(rest, "(")
	if open < 0 {
		return nil, false
	}
	depth := 0
	end := -1
	for i := open; i < len(rest); i++ {
		switch rest[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				end = i
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		return nil, false
	}
	return singleQuoted(rest[open+1 : end]), true
}

func singleQuoted(sql string) []string {
	var out []string
	for i := 0; i < len(sql); i++ {
		if sql[i] != '\'' {
			continue
		}
		var b strings.Builder
		i++
		for i < len(sql) {
			if sql[i] != '\'' {
				b.WriteByte(sql[i])
				i++
				continue
			}
			if i+1 < len(sql) && sql[i+1] == '\'' {
				b.WriteByte('\'')
				i += 2
				continue
			}
			break
		}
		out = append(out, b.String())
	}
	return out
}
