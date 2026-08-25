package probe

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/internal/testfixture"
)

const module = "github.com/wippyai/go-lua"

// declarationSurfaces is the set of analyzer packages a domain may name to
// declare a row: the declaration surfaces themselves, and the neutral engine and
// identity vocabularies the surfaces' own hook signatures are typed in. A domain
// that declares its whole row set from its own package names nothing else.
var declarationSurfaces = []string{
	module + "/analysis/schema",
	module + "/analysis/engine",
	module + "/analysis/identity",
}

// TestProbeDeclaresEverySurfaceRowFromItsOwnPackage is the dual of the typestate
// domain's zero-row law, and the sharpest single statement of the identity-row
// revision. Typestate declares nothing and therefore imports nothing; this
// package declares a row on every surface a domain can reach - an axis, a rule,
// three semantic roles, an observation population, a publication family, and a
// published code - and still imports nothing but the declaration surfaces.
//
// The load-bearing half is the artifact. A row's identity used to come from a
// closed enum in analysis/program/artifact, so an axis or a rule could not be
// declared without naming the compiled-artifact package from the declaring
// domain. Nothing here names it, and the law says so.
func TestProbeDeclaresEverySurfaceRowFromItsOwnPackage(t *testing.T) {
	const artifact = module + "/analysis/program/artifact"
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
			switch {
			case strings.HasPrefix(value, artifact):
				t.Errorf("probe source %s imports the compiled-artifact package %s to declare a row", filepath.Base(path), value)
			case !strings.HasPrefix(value, module+"/"):
				if strings.Contains(strings.SplitN(value, "/", 2)[0], ".") {
					t.Errorf("probe source %s imports third-party package %s", filepath.Base(path), value)
				}
			case !declarationSurface(value):
				t.Errorf("probe source %s imports %s, which is not a declaration surface", filepath.Base(path), value)
			}
		}
	}
}

func declarationSurface(value string) bool {
	for _, surface := range declarationSurfaces {
		if value == surface || strings.HasPrefix(value, surface+"/") {
			return true
		}
	}
	return false
}

// productionSources returns every non-test Go source file of this package.
func productionSources(t *testing.T) []string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("probe source location unavailable")
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
		t.Fatalf("walk probe sources: %v", err)
	}
	if len(sources) == 0 {
		t.Fatal("no probe production sources found")
	}
	return sources
}

// TestProbeIsNamedByNoAnalyzerProduction states the other half of this package's
// standing: it is a conformance fixture, not an analyzer domain. Its rows are
// composed into a scratch table by a law and reach no sealed analyzer catalog,
// so a production source naming this package would put a fixture's coordinate
// space and published code into the analyzer's own inventory.
func TestProbeIsNamedByNoAnalyzerProduction(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("probe source location unavailable")
	}
	root, err := testfixture.RepositoryRoot(filepath.Dir(current))
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "analysis")); statErr != nil {
		t.Fatalf("probe production-import walk does not cover analysis/: %v", statErr)
	}
	self := module + "/domain/probe"
	visited := 0
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr == nil && strings.HasPrefix(filepath.ToSlash(rel), "analysis/") {
			visited++
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, imported := range parsed.Imports {
			value, unquoteErr := strconv.Unquote(imported.Path.Value)
			if unquoteErr != nil {
				return unquoteErr
			}
			if value == self {
				t.Errorf("production source %s imports the declaration-surface fixture", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk analyzer sources: %v", err)
	}
	if visited == 0 {
		t.Fatal("probe production-import walk visited no analysis/ sources")
	}
}
