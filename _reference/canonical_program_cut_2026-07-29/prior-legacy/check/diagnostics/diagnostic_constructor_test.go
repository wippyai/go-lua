package diagnostics

import (
	goast "go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDiagnosticProducersUseConstructor(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var violations []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		fset := token.NewFileSet()
		fileAST, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		goast.Inspect(fileAST, func(node goast.Node) bool {
			switch n := node.(type) {
			case *goast.CompositeLit:
				if selectorName(n.Type) == "diagnostic.Diagnostic" && len(n.Elts) != 0 {
					violations = append(violations, fset.Position(n.Pos()).String()+": use diagnostic.New")
				}
			case *goast.SelectorExpr:
				name := selectorName(n)
				if name == "diagnostic.PositionFromSpan" || name == "diagnostic.PositionFromSpanInFile" {
					violations = append(violations, fset.Position(n.Pos()).String()+": let diagnostic.New derive Position from Span")
				}
			}
			return true
		})
	}
	if len(violations) != 0 {
		t.Fatalf("diagnostic constructors bypass canonical assembly:\n%s", strings.Join(violations, "\n"))
	}
}

func selectorName(expr goast.Expr) string {
	sel, ok := expr.(*goast.SelectorExpr)
	if !ok {
		return ""
	}
	ident, ok := sel.X.(*goast.Ident)
	if !ok {
		return ""
	}
	return ident.Name + "." + sel.Sel.Name
}
