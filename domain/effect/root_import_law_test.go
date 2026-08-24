package effect

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestEffectRootDoesNotImportFactor pins the reference-direction law for the
// effect axis: the root names the Fact type (Value, Atom) and the factor
// algebra imports the root, never the reverse. internal/testfixture reaches
// this package directly for Label/ParamRef/Row, and domain/static's own
// tests import testfixture, so an edge from this package into factor (which
// imports domain/static for its Program/Target/Link authorities) closes the
// cycle testfixture -> effect -> effect/factor -> domain/static again. This
// is the third time an axis-root edge into factor has reopened it; this law
// makes the direction load-bearing instead of incidental.
func TestEffectRootDoesNotImportFactor(t *testing.T) {
	const forbidden = "github.com/wippyai/go-lua/domain/effect/factor"
	for _, path := range rootSources(t) {
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imported := range parsed.Imports {
			value, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", path, err)
			}
			if value == forbidden {
				t.Errorf("%s imports %s: the axis root must not import the factor algebra", filepath.Base(path), forbidden)
			}
		}
	}
}

// rootSources returns this package's own production sources, one directory
// level only. Descendant packages under domain/effect/... (factor, owner,
// callsite, ...) legitimately import factor and are out of scope.
func rootSources(t *testing.T) []string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("effect root source location unavailable")
	}
	dir := filepath.Dir(current)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var sources []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		sources = append(sources, filepath.Join(dir, name))
	}
	if len(sources) == 0 {
		t.Fatal("no effect root production sources found")
	}
	return sources
}
