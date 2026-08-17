package composite

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

// A query family is composed in one direction. The domain that owns the facts a
// family is answered from declares the family and the contributor that folds
// them; this package instantiates that declaration into the one query table;
// and the analyzer root reaches an answer through the sealed binding alone.
//
// The two laws below are the standing statement of that direction. Neither is a
// style rule: a contributor that reached the composition could be declared
// against the table it is a member of, and a root that reached a contributor
// could fold a family's answers a second way, beside the fold the table sealed.

const modulePath = "github.com/wippyai/go-lua/"

const compositePackage = modulePath + "domain/composite"

// queryContributorPackages is the closed set of packages that declare a query
// family's contributor, by their import paths and by their location under the
// domain tree.
var queryContributorPackages = map[string]string{
	"github.com/wippyai/go-lua/domain/value/owner":  "value/owner",
	"github.com/wippyai/go-lua/domain/effect/owner": "effect/owner",
}

// TestQueryContributorsDoNotImportTheComposition states the upward half of the
// direction: a domain declares its own query family and its contributor without
// naming the composition that instantiates them, so the declaration travels to
// the table and never the table to the declaration.
func TestQueryContributorsDoNotImportTheComposition(t *testing.T) {
	for path, name := range queryContributorPackages {
		for _, source := range packageSources(t, contributorDirectory(t, path)) {
			if imports(t, source, compositePackage) {
				t.Errorf("query contributor %s names the composition in %s", name, filepath.Base(source))
			}
		}
	}
}

// TestAnalyzerRootDoesNotImportAQueryContributor states the downward half: the
// analyzer root reaches a family's answers through the sealed binding the
// composition hands it, so no root file names the package that folds them and
// no second fold can be assembled beside the one the table sealed.
func TestAnalyzerRootDoesNotImportAQueryContributor(t *testing.T) {
	for _, source := range packageSources(t, analyzerRootDirectory(t)) {
		for path, name := range queryContributorPackages {
			if imports(t, source, path) {
				t.Errorf("analyzer root names query contributor %s in %s", name, filepath.Base(source))
			}
		}
	}
}

// imports reports whether one source file names a package.
func imports(t *testing.T, source, path string) bool {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), source, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", source, err)
	}
	for _, imported := range parsed.Imports {
		value, unquoteErr := strconv.Unquote(imported.Path.Value)
		if unquoteErr == nil && value == path {
			return true
		}
	}
	return false
}

// packageSources returns every non-test Go source directly inside one package
// directory.
func packageSources(t *testing.T, directory string) []string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read %s: %v", directory, err)
	}
	var sources []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		sources = append(sources, filepath.Join(directory, name))
	}
	if len(sources) == 0 {
		t.Fatalf("no sources found in %s", directory)
	}
	return sources
}

// contributorDirectory resolves one contributor package's directory from its
// import path.
func contributorDirectory(t *testing.T, path string) string {
	t.Helper()
	if !strings.HasPrefix(path, modulePath) {
		t.Fatalf("import path %q is outside this module", path)
	}
	return filepath.Join(compositionModuleRoot(t), filepath.FromSlash(strings.TrimPrefix(path, modulePath)))
}

// analyzerRootDirectory is the analyzer's own package directory.
func analyzerRootDirectory(t *testing.T) string {
	t.Helper()
	return filepath.Join(compositionModuleRoot(t), "analysis")
}

// compositionModuleRoot is the module root, found by walking up from this
// file's own directory to the go.mod that declares the module. It is derived
// rather than counted in directory levels, so moving this package does not
// silently point these laws at the wrong tree.
func compositionModuleRoot(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("composition source location unavailable")
	}
	directory := filepath.Dir(current)
	for {
		if _, err := os.Stat(filepath.Join(directory, "go.mod")); err == nil {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			t.Fatal("module root not found above the composition")
		}
		directory = parent
	}
}
