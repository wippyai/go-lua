package factorycatalog_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// The factory directory is a generic semantic seam.  Its production source
// may depend on the typed binding and signature contracts, but never on a
// domain implementation (or on another execution layer that could turn the
// directory into a dispatcher).
func TestFactoryCatalogProductionImportsStayAtTheSemanticSeam(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate import-law source")
	}
	packageDir := filepath.Dir(testFile)
	entries, err := filepath.Glob(filepath.Join(packageDir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]struct{}{
		"github.com/wippyai/go-lua/analysis/relation/semantic/binding":   {},
		"github.com/wippyai/go-lua/analysis/relation/semantic/signature": {},
	}
	for _, path := range entries {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}
		var source *ast.File = file
		if got := source.Name.Name; got != "factorycatalog" {
			t.Errorf("%s package = %q, want factorycatalog", path, got)
		}
		for _, imported := range source.Imports {
			value, unquoteErr := strconv.Unquote(imported.Path.Value)
			if unquoteErr != nil {
				t.Fatalf("unquote import in %s: %v", path, unquoteErr)
			}
			if _, ok := allowed[value]; !ok {
				t.Errorf("%s imports %s outside the generic semantic seam", path, value)
			}
			if strings.HasPrefix(value, "github.com/wippyai/go-lua/domain/") {
				t.Errorf("%s imports domain implementation %s", path, value)
			}
		}
	}
}
