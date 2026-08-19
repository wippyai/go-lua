package rule

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestRuleProductionNamesNoProgramConstruction is the construction-hook floor:
// schema/rule production stores no ProgramConstruction, ReceiptAssembly, or
// ReceiptGraph. Member attach lives on the bound cell, not on a schema hook.
func TestRuleProductionNamesNoProgramConstruction(t *testing.T) {
	forbidden := map[string]struct{}{
		"ProgramConstruction": {},
		"ReceiptAssembly":     {},
		"ReceiptGraph":        {},
		"LinkMember":          {},
		"RegisterMountedSlot": {},
		"RegisterLinkSlot":    {},
		"RuleSlot":            {},
		"SchemaBuilder":       {},
		"SchemaBinding":       {},
		"RuleSlotCapability":  {},
	}
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller file")
	}
	root := filepath.Dir(thisFile)
	visited := 0
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		visited++
		file, parseErr := parser.ParseFile(token.NewFileSet(), filepath.Join(root, name), nil, 0)
		if parseErr != nil {
			t.Fatalf("%s: %v", name, parseErr)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			ident, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			if _, banned := forbidden[ident.Name]; banned {
				t.Errorf("%s names construction identifier %s", name, ident.Name)
			}
			return true
		})
	}
	if visited == 0 {
		t.Fatal("schema/rule production walk visited no files")
	}
}
