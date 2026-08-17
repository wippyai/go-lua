package typedomain

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

// surfacePackages is the closed set of declaration-surface packages. A row on
// any surface of the analyzer declaration table is reached by importing the
// surface's package, so this list is what the zero-row statement is checked
// against.
//
// The schema's type-contract package is deliberately absent: it is the neutral
// portable envelope one authored type declaration travels in, not a surface, and
// this domain's adapter is its implementation rather than a declarer of rows.
var surfacePackages = []string{
	"github.com/wippyai/go-lua/analysis/schema/structure",
	"github.com/wippyai/go-lua/analysis/schema/axis",
	"github.com/wippyai/go-lua/analysis/schema/rule",
	"github.com/wippyai/go-lua/analysis/schema/diagnostic",
	"github.com/wippyai/go-lua/analysis/schema/composite",
	"github.com/wippyai/go-lua/analysis/schema/denominator",
	"github.com/wippyai/go-lua/analysis/schema/query",
}

// TestTypeDomainDeclaresNoSurfaceRow is the executable form of this domain's
// registration statement. A row declared anywhere beneath this directory would
// be visible in that package's imports before any composition ran, so the law is
// stated over the import set rather than over the sealed table.
//
// The statement is over the whole domain, not over this package: this directory
// carries no implementation of its own, and the packages that would declare a
// row are the ones below it.
func TestTypeDomainDeclaresNoSurfaceRow(t *testing.T) {
	for _, path := range domainSources(t) {
		for _, imported := range sourceImports(t, path) {
			for _, surface := range surfacePackages {
				if imported == surface || strings.HasPrefix(imported, surface+"/") {
					t.Errorf("type domain source %s imports declaration surface %s while the domain declares no row", relative(t, path), imported)
				}
			}
		}
	}
}

// TestTypeDomainImportsNoPeerDomain states the position the registration rests
// on: this domain is the base of the domain layer, so it reads no peer domain
// and every peer that reasons about types reads it. An edge in the other
// direction would make the registration a statement about a cycle rather than
// about a domain, and it would put the domain's declaration above a domain that
// already declares rows of its own.
func TestTypeDomainImportsNoPeerDomain(t *testing.T) {
	const domainRoot = "github.com/wippyai/go-lua/domain/"
	const self = domainRoot + "type"
	for _, path := range domainSources(t) {
		for _, imported := range sourceImports(t, path) {
			if !strings.HasPrefix(imported, domainRoot) {
				continue
			}
			if imported == self || strings.HasPrefix(imported, self+"/") {
				continue
			}
			t.Errorf("type domain source %s imports peer domain %s", relative(t, path), imported)
		}
	}
}

// domainSources returns every non-test Go source file of this domain, this
// directory and every package beneath it.
func domainSources(t *testing.T) []string {
	t.Helper()
	root := domainRootDir(t)
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
		t.Fatalf("walk type domain sources: %v", err)
	}
	if len(sources) == 0 {
		t.Fatal("no type domain production sources found")
	}
	return sources
}

func sourceImports(t *testing.T, path string) []string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	imports := make([]string, 0, len(parsed.Imports))
	for _, imported := range parsed.Imports {
		value, unquoteErr := strconv.Unquote(imported.Path.Value)
		if unquoteErr != nil {
			t.Fatalf("unquote import in %s: %v", path, unquoteErr)
		}
		imports = append(imports, value)
	}
	return imports
}

func domainRootDir(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("type domain source location unavailable")
	}
	return filepath.Dir(current)
}

func relative(t *testing.T, path string) string {
	t.Helper()
	rel, err := filepath.Rel(domainRootDir(t), path)
	if err != nil {
		return path
	}
	return rel
}
