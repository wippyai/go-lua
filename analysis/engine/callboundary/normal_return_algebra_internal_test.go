package callboundary

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"
)

func TestNormalReturnAlgebraRegistryCoversStorageLanes(t *testing.T) {
	storage := NormalReturnFactLanes()
	if len(normalReturnSummaryLanes) != len(storage) {
		t.Fatalf("normal-return algebra lanes = %d, want storage lane count %d", len(normalReturnSummaryLanes), len(storage))
	}
	for _, lane := range normalReturnSummaryLanes {
		if !normalReturnSummaryLaneValid(lane.Value) {
			t.Fatal("normal-return algebra lane has incomplete behavior")
		}
	}
}

func TestNormalReturnFactsOperationsUseLaneRegistry(t *testing.T) {
	file := parseNormalReturnAlgebraSource(t)
	fields := normalReturnAlgebraStorageFields(t)

	for _, name := range []string{"normalizeNormalReturnFactsWith", "CloneNormalReturnFacts"} {
		fn := requireNormalReturnFuncDecl(t, file, name)
		if !funcUsesNormalReturnLaneRegistry(fn) {
			t.Fatalf("%s must iterate normalReturnSummaryLanes", name)
		}
		if field := firstSelectedNormalReturnStorageField(fn, fields); field != "" {
			t.Fatalf("%s selects storage field %s directly; use normalReturnSummaryLanes", name, field)
		}
	}
}

func parseNormalReturnAlgebraSource(t *testing.T) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "normal_return_algebra.go", nil, 0)
	if err != nil {
		t.Fatalf("parse normal_return_algebra.go: %v", err)
	}
	return file
}

func requireNormalReturnFuncDecl(t *testing.T, file *ast.File, name string) *ast.FuncDecl {
	t.Helper()
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == name {
			return fn
		}
	}
	t.Fatalf("function %s not found", name)
	return nil
}

func funcUsesNormalReturnLaneRegistry(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		ident, ok := node.(*ast.Ident)
		if ok && ident.Name == "normalReturnSummaryLanes" {
			found = true
			return false
		}
		return true
	})
	return found
}

func normalReturnAlgebraStorageFields(t *testing.T) map[string]struct{} {
	t.Helper()
	out := make(map[string]struct{})
	typ := reflect.TypeOf(NormalReturnFacts{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.Type.Kind() == reflect.Slice {
			out[field.Name] = struct{}{}
		}
	}
	return out
}

func firstSelectedNormalReturnStorageField(fn *ast.FuncDecl, fields map[string]struct{}) string {
	var found string
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		sel, ok := node.(*ast.SelectorExpr)
		if ok {
			if _, isStorageField := fields[sel.Sel.Name]; isStorageField {
				found = sel.Sel.Name
				return false
			}
		}
		return true
	})
	return found
}
