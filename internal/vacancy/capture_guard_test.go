package vacancy

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// The refusal path must not register a destructive cleanup callback first.
// This checks ordering without connecting to, or mutating, a denied database.
func TestCaptureJourneyRejectsConfiguredTargetBeforeConnection(t *testing.T) {
	source, err := os.ReadFile("capture_journey_postgres_test.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	parse := strings.Index(text, "pgxpool.ParseConfig(dsn)")
	guard := strings.Index(text, "strings.HasPrefix(config.ConnConfig.Database")
	connect := strings.Index(text, "pgxpool.NewWithConfig(ctx, config)")
	if parse < 0 || guard < parse || connect < guard {
		t.Fatal("configured database target must be checked before opening any connection")
	}
}
func TestCaptureJourneyGuardsDatabaseBeforeDestructiveCleanup(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "capture_journey_postgres_test.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range file.Decls {
		fn, ok := declaration.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "TestCapturedReceiptsAdmissionPublicationAndAPI" {
			continue
		}
		guarded := false
		for _, statement := range fn.Body.List {
			if branch, ok := statement.(*ast.IfStmt); ok {
				hasDatabaseCheck, hasRefusal := false, false
				ast.Inspect(branch, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					if method, ok := call.Fun.(*ast.SelectorExpr); ok {
						if method.Sel.Name == "QueryRow" {
							hasDatabaseCheck = true
						}
						if method.Sel.Name == "Fatal" {
							hasRefusal = true
						}
					}
					return true
				})
				if hasDatabaseCheck && hasRefusal {
					guarded = true
				}
			}
			if deferred, ok := statement.(*ast.DeferStmt); ok {
				if _, cleanup := deferred.Call.Fun.(*ast.FuncLit); cleanup {
					if !guarded {
						t.Fatal("destructive cleanup registered before database refusal")
					}
					return
				}
			}
		}
		t.Fatal("destructive cleanup guard not found")
	}
	t.Fatal("capture journey test not found")
}
