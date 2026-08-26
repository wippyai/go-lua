package delta

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

// The positive contribution classifier belongs to the publication door. The
// Later evaluator may carry an Apply result to that door, but it must not
// carry a second projection of the same Application or call a batch helper
// that rebuilds one. Keep this ABI rule structural so a future evaluator edit
// cannot silently reintroduce the duplicate observation.
func TestDeltaDoesNotCarryPositiveContributionProjection(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("law source path")
	}
	directory := filepath.Dir(current)
	files, err := filepath.Glob(filepath.Join(directory, "*.go"))
	if err != nil {
		t.Fatalf("delta source glob: %v", err)
	}
	set := token.NewFileSet()
	forbiddenBatchHelper := "Append" + "Transitions"
	productionFiles := 0
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		productionFiles++
		file, parseErr := parser.ParseFile(set, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.ImportSpec:
				importPath, unquoteErr := strconv.Unquote(value.Path.Value)
				if unquoteErr == nil && strings.HasSuffix(importPath, "/analysis/engine/relation/apply/contribution") {
					t.Errorf("%s imports the positive contribution classifier; only publish may consume it", path)
				}
			case *ast.FuncDecl:
				if value.Name != nil && value.Name.Name == "Contributions" && resultReceiver(value.Recv) {
					t.Errorf("%s exposes Result.Contributions; Applications are the only positive evaluator transport", path)
				}
			case *ast.TypeSpec:
				if value.Name == nil || value.Name.Name != "Result" {
					return true
				}
				structure, structureOK := value.Type.(*ast.StructType)
				if !structureOK || structure.Fields == nil {
					return true
				}
				for _, field := range structure.Fields.List {
					for _, name := range field.Names {
						if name != nil && name.Name == "contributions" {
							t.Errorf("%s carries a positive contribution vector in Result", path)
						}
					}
					if contributionTransitionSlice(field.Type) {
						t.Errorf("%s carries ContributionTransition in Result", path)
					}
				}
			case *ast.SelectorExpr:
				if value.Sel != nil && value.Sel.Name == forbiddenBatchHelper {
					t.Errorf("%s re-derives contribution transitions with the batch helper", path)
				}
			}
			return true
		})
	}
	if productionFiles == 0 {
		t.Fatal("delta production source inventory is empty")
	}
}

func resultReceiver(fields *ast.FieldList) bool {
	if fields == nil || len(fields.List) != 1 {
		return false
	}
	value := fields.List[0].Type
	if pointer, ok := value.(*ast.StarExpr); ok {
		value = pointer.X
	}
	identifier, ok := value.(*ast.Ident)
	return ok && identifier.Name == "Result"
}

func contributionTransitionSlice(expression ast.Expr) bool {
	array, ok := expression.(*ast.ArrayType)
	if !ok {
		return false
	}
	selector, ok := array.Elt.(*ast.SelectorExpr)
	return ok && selector.Sel != nil && selector.Sel.Name == "ContributionTransition"
}
