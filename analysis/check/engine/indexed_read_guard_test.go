package engine

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPartitionFullScansStayDisplaced makes the indexed read policy structural:
// every partition Values call is rejected unless it is the single audited
// whole-module serialization projection and states why it needs every family.
func TestPartitionFullScansStayDisplaced(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	fullScans := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, name, nil, parser.ParseComments)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", name, parseErr)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || selector.Sel.Name != "Values" || len(call.Args) != 0 {
					return true
				}
				receiver, ok := selector.X.(*ast.Ident)
				if !ok || receiver.Name != "partition" {
					return true
				}
				fullScans++
				if function.Name.Name != "joinVisibleFacts" {
					t.Errorf("%s: partition.Values full scan must use an indexed read", fset.Position(call.Pos()))
				}
				return true
			})
		}
		if name == "engine.go" && !strings.Contains(sourceText(t, name), "Full scan required: module publication serialization projects every visible fact family.") {
			t.Error("joinVisibleFacts full scan lacks its audit justification")
		}
	}
	if fullScans != 1 {
		t.Fatalf("production partition.Values full scans = %d, want the single audited serialization projection", fullScans)
	}
}

func sourceText(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
