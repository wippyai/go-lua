package static

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestStaticDeclaresNoSurfaceRow is the executable form of this domain's
// registration statement. A surface of the declaration table is reached by
// importing its package, so a domain that declares no row imports no surface,
// and a row declared here would be visible in this package's imports before any
// composition ran. The law is stated over the import set rather than over the
// sealed table because the table is composed elsewhere, and this domain sits
// below the composition that builds it.
//
// The law is scoped to the declaration surfaces alone. This domain does hold
// inter-domain edges - it reads the runtime family vocabulary and the type
// domain's graph, encoder, and contract representation - so an import of a peer
// domain is not the subject here; an import of a surface is.
func TestStaticDeclaresNoSurfaceRow(t *testing.T) {
	const surfaceRoot = "github.com/wippyai/go-lua/analysis/schema"
	for _, path := range productionSources(t) {
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imported := range parsed.Imports {
			value, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", path, err)
			}
			if strings.HasPrefix(value, surfaceRoot) {
				t.Errorf("static source %s imports declaration surface %s while its registration declares no row", filepath.Base(path), value)
			}
		}
	}
}

// TestStaticIsBelowTheComposition states the direction the zero-row statement
// rests on: the composition that seals the declaration table reads this domain,
// and this domain reads neither the composition nor the Link projection that
// mounts it. An edge in the other direction would make the registration above a
// statement about a cycle rather than about a domain.
func TestStaticIsBelowTheComposition(t *testing.T) {
	above := []string{
		"github.com/wippyai/go-lua/domain/composite",
		"github.com/wippyai/go-lua/domain/pack",
		"github.com/wippyai/go-lua/analysis/program/link",
	}
	for _, path := range productionSources(t) {
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imported := range parsed.Imports {
			value, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", path, err)
			}
			for _, consumer := range above {
				if value == consumer || strings.HasPrefix(value, consumer+"/") {
					t.Errorf("static source %s imports %s, which reads this domain", filepath.Base(path), value)
				}
			}
		}
	}
}

// productionSources returns every non-test Go source file of this package.
func productionSources(t *testing.T) []string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("static source location unavailable")
	}
	root := filepath.Dir(current)
	var sources []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		sources = append(sources, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk static sources: %v", err)
	}
	if len(sources) == 0 {
		t.Fatal("no static production sources found")
	}
	return sources
}
