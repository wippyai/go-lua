package hooks

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

func TestReturnTypeCompatibleRejectsNilableActualAgainstNonNilDeclared(t *testing.T) {
	action := typ.NewUnion(
		typ.NewRecord().
			Field("kind", typ.LiteralString("a")).
			Field("x", typ.String).
			Build(),
		typ.NewRecord().
			Field("kind", typ.LiteralString("b")).
			Field("y", typ.String).
			Build(),
	)

	if returnTypeCompatible(typ.NewOptional(action), action) {
		t.Fatal("return boundary accepted nilable actual for non-nil declared return")
	}
	if returnTypeCompatible(typ.Nil, action) {
		t.Fatal("return boundary accepted nil for non-nil declared return")
	}
}

func TestDeclaredReturnScopeUsesGraphEntryScope(t *testing.T) {
	graph := cfg.Build(&ast.FunctionExpr{ParList: &ast.ParList{}})
	if graph == nil {
		t.Fatal("cfg.Build returned nil")
	}

	baseParam := typ.NewTypeParam("T", typ.Number)
	entryParam := typ.NewTypeParam("T", typ.String)
	base := scope.New().WithTypeParams(map[string]typ.Type{"T": baseParam})
	entry := scope.New().WithTypeParams(map[string]typ.Type{"T": entryParam})

	got := declaredReturnScope(graph, map[cfg.Point]*scope.State{
		graph.Entry(): entry,
	}, base)
	if got == nil {
		t.Fatal("declaredReturnScope returned nil")
	}
	resolved, ok := got.LookupTypeParam("T")
	if !ok || resolved != entryParam {
		t.Fatalf("declaredReturnScope resolved T = %#v/%v, want entry param", resolved, ok)
	}
}

func TestDeclaredReturnScopeFallsBackToBaseScope(t *testing.T) {
	base := scope.New()
	if got := declaredReturnScope(nil, nil, base); got != base {
		t.Fatalf("fallback scope = %#v, want base", got)
	}
}
