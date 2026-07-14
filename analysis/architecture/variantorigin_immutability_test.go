package architecture_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"testing"
)

// TestVariantOriginCasesHaveNoRawSliceEscape is the repository architecture
// tripwire equivalent of `rg CasesRef analysis`: the removed borrowed-slice API
// must not be reintroduced or consumed anywhere in production analysis code.
func TestVariantOriginCasesHaveNoRawSliceEscape(t *testing.T) {
	fileSet := token.NewFileSet()
	err := filepath.WalkDir("..", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" {
			return nil
		}
		file, err := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok && identifier.Name == "CasesRef" {
				t.Errorf("%s:%d reintroduces forbidden mutable variant-origin slice escape CasesRef", path, fileSet.Position(identifier.Pos()).Line)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
