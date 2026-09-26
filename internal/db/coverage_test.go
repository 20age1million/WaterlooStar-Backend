package db_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/20age1million/WaterlooStar-Backend/internal/db/sqlcgen"
)

// Every query must be executed by a test.
//
// This is the guard that keeps the gap from reopening. Phase 4 added a query no
// test could reach, and it broke in production. A checklist would have caught it
// only if someone remembered the checklist; this notices on its own.
//
// "Executed" is read from the source: a query counts as covered when a test in
// this package, or a fixture in dbtest, names it. That is not proof the test
// asserts anything useful — nothing automatic can be — but it does mean the
// statement reached PostgreSQL, which is the failure this package exists for.

func TestEveryQueryIsExercised(t *testing.T) {
	queries := querierMethods(t)

	// Only test files here: this package's pool.go calls Ping itself, and
	// counting that would have reported a query as covered when no test ran it.
	called := namesCalledIn(t, source{dir: ".", testsOnly: true})
	for name := range namesCalledIn(t, source{dir: "dbtest"}) {
		called[name] = true
	}

	var missing []string
	for _, name := range queries {
		if !called[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)

	if len(missing) > 0 {
		subject, verb := "queries", "are"
		if len(missing) == 1 {
			subject, verb = "query", "is"
		}
		t.Fatalf(`%d %s in internal/db/queries/ %s never run by a test:

    %s

Add a test in this package that calls each one. A query no test executes is a
query PostgreSQL has never seen — which is how a malformed one reached
production in Phase 4.`,
			len(missing), subject, verb, strings.Join(missing, "\n    "))
	}
}

// querierMethods is the generated interface's method set — the full list of
// queries, straight from sqlc, so it cannot drift from what exists.
func querierMethods(t *testing.T) []string {
	t.Helper()

	iface := reflect.TypeOf((*sqlcgen.Querier)(nil)).Elem()
	names := make([]string, 0, iface.NumMethod())
	for i := range iface.NumMethod() {
		names = append(names, iface.Method(i).Name)
	}
	if len(names) == 0 {
		t.Fatal("the Querier interface has no methods — has sqlc been run?")
	}
	return names
}

// source is a directory to scan, and whether to count only its test files.
type source struct {
	dir       string
	testsOnly bool
}

// namesCalledIn collects every selector name used in the Go files under the
// given directories: `q.CreateUser(...)` contributes "CreateUser".
func namesCalledIn(t *testing.T, sources ...source) map[string]bool {
	t.Helper()

	called := map[string]bool{}
	fset := token.NewFileSet()

	for _, src := range sources {
		entries, err := os.ReadDir(src.dir)
		if err != nil {
			t.Fatalf("read %s: %v", src.dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				continue
			}
			if src.testsOnly && !strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(src.dir, entry.Name())
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			ast.Inspect(file, func(n ast.Node) bool {
				if sel, ok := n.(*ast.SelectorExpr); ok {
					called[sel.Sel.Name] = true
				}
				return true
			})
		}
	}
	return called
}
