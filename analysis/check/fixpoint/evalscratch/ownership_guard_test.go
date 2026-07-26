package evalscratch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

func TestEvaluatorScratchHasOneOwner(t *testing.T) {
	files := []string{
		filepath.Join("..", "equation", "compiled_evaluator.go"),
		filepath.Join("..", "interproc", "runtime_cache.go"),
	}
	declarations := 0
	for _, name := range files {
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			spec, ok := node.(*ast.TypeSpec)
			if ok && spec.Name.Name == "EvaluatorScratch" {
				declarations++
			}
			return true
		})
	}
	if declarations != 1 {
		t.Fatalf("EvaluatorScratch declarations = %d, want one equation owner", declarations)
	}
}
