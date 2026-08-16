package bind

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func parsedWideSymbolAnnotations(tb testing.TB, width int) (*Result, []Symbol, []ast.TypeExpr) {
	tb.Helper()
	var source strings.Builder
	for i := 0; i < width; i++ {
		fmt.Fprintf(&source, "local value%d: number = %d\n", i, i)
	}
	source.WriteString("type Snapshot = typeof(function(input: string, ...: boolean): number return 0 end)\n")

	stmts, err := parse.ParseString(source.String(), "wide_annotations.lua")
	if err != nil {
		tb.Fatal(err)
	}
	result := BindChunk(stmts)
	ids := make([]Symbol, 0, width+2)
	types := make([]ast.TypeExpr, 0, width+2)
	for i := 0; i < width; i++ {
		local := stmts[i].(*ast.LocalAssignStmt)
		id, ok := result.LocalSymbolAt(local, 0)
		if !ok {
			tb.Fatalf("local %d symbol missing", i)
		}
		ids = append(ids, id)
		types = append(types, local.Types[0])
	}
	query := stmts[width].(*ast.TypeDefStmt).Type.(*ast.TypeOfExpr).Expr.(*ast.FunctionExpr)
	slots := result.ParamSlots(query)
	if len(slots) != 2 {
		tb.Fatalf("static query slots = %#v, want parameter and vararg", slots)
	}
	for _, slot := range slots {
		ids = append(ids, slot.Symbol)
		types = append(types, slot.Type)
	}
	return result, ids, types
}

func TestParsedWideSymbolAnnotationIndexIsExact(t *testing.T) {
	result, ids, types := parsedWideSymbolAnnotations(t, 512)
	if len(result.symbolAnnotations) != len(ids) {
		t.Fatalf("annotation index size = %d, want %d exact declarations", len(result.symbolAnnotations), len(ids))
	}
	for i, id := range ids {
		if got, ok := result.SymbolTypeAnnotation(id); !ok || got != types[i] {
			t.Fatalf("SymbolTypeAnnotation(%d) = %T/%v, want exact declaration %T", id, got, ok, types[i])
		}
	}
}

func BenchmarkSymbolTypeAnnotationWide(b *testing.B) {
	result, ids, types := parsedWideSymbolAnnotations(b, 4096)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for i, id := range ids {
			if got, ok := result.SymbolTypeAnnotation(id); !ok || got != types[i] {
				b.Fatal("annotation index changed")
			}
		}
	}
}
