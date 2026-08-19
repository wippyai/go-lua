package analysis

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"
)

// remainingCompiledStateObservationFields is shrink-only. The cut lands when
// it is empty: compiledState retains no diagnostic observation reconstruction.
var remainingCompiledStateObservationFields = []string{}

// TestCompiledStateOwnsNoDiagnosticObservationArrays is the Snapshot
// consumption floor: compiledState retains no compile-time observation
// reconstruction. Observations are read from the committed Snapshot at detach.
func TestCompiledStateOwnsNoDiagnosticObservationArrays(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller file")
	}
	path := filepath.Join(filepath.Dir(thisFile), "analyze.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	observed := map[string]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		spec, ok := node.(*ast.TypeSpec)
		if !ok || spec.Name == nil || spec.Name.Name != "compiledState" {
			return true
		}
		structType, ok := spec.Type.(*ast.StructType)
		if !ok || structType.Fields == nil {
			return false
		}
		for _, field := range structType.Fields.List {
			for _, name := range field.Names {
				switch name.Name {
				case "observations", "ordinaryObservations":
					observed[name.Name] = true
				}
			}
		}
		return false
	})
	allowed := map[string]bool{}
	for _, name := range remainingCompiledStateObservationFields {
		if allowed[name] {
			t.Fatalf("remainingCompiledStateObservationFields must stay unique: %s", name)
		}
		allowed[name] = true
		if !observed[name] {
			t.Fatalf("stale compiledState observation pin %s; the list is shrink-only, so remove it", name)
		}
	}
	for name := range observed {
		if !allowed[name] {
			t.Fatalf("compiledState retains diagnostic observation array %s", name)
		}
	}
}
