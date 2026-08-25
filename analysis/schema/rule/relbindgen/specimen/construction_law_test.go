package specimen_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// forbiddenBindingImports are the surfaces a thin typed binding must not be
// able to name. The type of an owner operation already admits no engine value;
// this law holds the package that hosts those types to the same fence, so the
// reach cannot be reintroduced by an import either.
var forbiddenBindingImports = []string{
	"github.com/wippyai/go-lua/analysis/engine",
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra",
	"github.com/wippyai/go-lua/analysis/relation/schema/plan",
	"github.com/wippyai/go-lua/analysis/relation/check",
	"github.com/wippyai/go-lua/analysis/relation/mount",
	"github.com/wippyai/go-lua/analysis/snapshot",
}

func TestBindingsCannotNameTheRelationalMachine(t *testing.T) {
	for _, directory := range []string{".", ".."} {
		set := token.NewFileSet()
		packages, err := parser.ParseDir(set, directory, func(entry fs.FileInfo) bool {
			return !strings.HasSuffix(entry.Name(), "_test.go")
		}, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", directory, err)
		}
		for _, parsed := range packages {
			for name, file := range parsed.Files {
				for _, imported := range file.Imports {
					path, quoteErr := strconv.Unquote(imported.Path.Value)
					if quoteErr != nil {
						t.Fatalf("import path in %s", name)
					}
					for _, forbidden := range forbiddenBindingImports {
						if path == forbidden || strings.HasPrefix(path, forbidden+"/") {
							t.Fatalf("%s imports %s", filepath.Base(name), path)
						}
					}
				}
			}
		}
	}
}
