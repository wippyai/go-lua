package schema

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The declaration surfaces used to take their identity vocabularies from the
// compiled-artifact package: a rule's role, an axis's writer principal, and a
// diagnostic's observation population were all members of enums declared in
// analysis/program/artifact. That inverted the dependency - the projection owned
// the identities the declaration was written in - and it is the reason a domain
// could not declare a row without naming the artifact.
//
// The law below is the standing statement that the inversion stays undone. It is
// not "no surface names the artifact": two surfaces legitimately do, and both are
// named here with their reason. What it holds is that no surface takes an
// IDENTITY from it. A boundary that carries the compiled artifact as a value is
// pinned to exactly the identifiers it names, so re-adopting a catalog - a role,
// an output kind, an observation kind, a stage, an input kind - fails here.

const artifactPackage = "github.com/wippyai/go-lua/analysis/program/artifact"

// artifactBoundaries is the closed set of declaration-surface packages that name
// the compiled-artifact package at all, and what each is permitted to name.
//
//   - ingress is the artifact boundary itself: its whole job is to read a
//     compiled artifact into the declared vocabularies, so it names the artifact
//     freely and is pinned by its own laws instead.
//   - axis hands a mount hook the neutral view of one mounted artifact. The
//     compiled program is the payload that view carries, so the axis surface
//     names the artifact VALUE and nothing else: no catalog, no ordinal, no kind.
var artifactBoundaries = map[string][]string{
	"ingress": nil,
	"axis":    {"Artifact"},
}

// TestSchemaSurfacesDoNotImportProgramArtifact states the direction over every
// declaration surface: a surface outside the two named boundaries does not name
// the compiled-artifact package, and a boundary that does names only what it is
// pinned to.
func TestSchemaSurfacesDoNotImportProgramArtifact(t *testing.T) {
	fileset := token.NewFileSet()
	for _, path := range surfaceSources(t) {
		parsed, err := parser.ParseFile(fileset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		local, imported := artifactImport(parsed)
		if !imported {
			continue
		}
		surface := filepath.Base(filepath.Dir(path))
		pinned, boundary := artifactBoundaries[surface]
		if !boundary {
			t.Errorf("declaration surface %s names the compiled-artifact package in %s", surface, filepath.Base(path))
			continue
		}
		if pinned == nil {
			continue
		}
		for _, named := range artifactIdentifiers(parsed, local) {
			if !pinnedIdentifier(pinned, named) {
				t.Errorf("surface %s names artifact identifier %q in %s; it is pinned to %v", surface, named, filepath.Base(path), pinned)
			}
		}
	}
}

// artifactImport reports the local name the compiled-artifact package is bound
// to in one file, if it is imported at all.
func artifactImport(file *ast.File) (string, bool) {
	for _, imported := range file.Imports {
		value, err := strconv.Unquote(imported.Path.Value)
		if err != nil || value != artifactPackage {
			continue
		}
		if imported.Name != nil {
			return imported.Name.Name, true
		}
		return "artifact", true
	}
	return "", false
}

// artifactIdentifiers is every identifier one file names through the
// compiled-artifact package, in sorted order.
func artifactIdentifiers(file *ast.File, local string) []string {
	named := make(map[string]struct{})
	ast.Inspect(file, func(node ast.Node) bool {
		selector, isSelector := node.(*ast.SelectorExpr)
		if !isSelector {
			return true
		}
		qualifier, isIdent := selector.X.(*ast.Ident)
		if isIdent && qualifier.Name == local && qualifier.Obj == nil {
			named[selector.Sel.Name] = struct{}{}
		}
		return true
	})
	identifiers := make([]string, 0, len(named))
	for identifier := range named {
		identifiers = append(identifiers, identifier)
	}
	sort.Strings(identifiers)
	return identifiers
}

func pinnedIdentifier(pinned []string, named string) bool {
	for _, allowed := range pinned {
		if allowed == named {
			return true
		}
	}
	return false
}

// surfaceSources returns every non-test Go source under the declaration table's
// own tree.
func surfaceSources(t *testing.T) []string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("schema source location unavailable")
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
		t.Fatalf("walk declaration surface sources: %v", err)
	}
	if len(sources) == 0 {
		t.Fatal("no declaration surface sources found")
	}
	return sources
}
