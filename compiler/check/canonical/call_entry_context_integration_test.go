package canonical

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/topology"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestProductCallEntryContextProjectsDirectCallerAxes(t *testing.T) {
	const captured cfg.SymbolID = 101
	const other cfg.SymbolID = 202
	ref := summary.FuncRef{GraphID: 10, ParentHash: 20}

	calleeGraph := graphWithCapturedSymbol(t, captured, "captured")
	prog := &program{
		funcTopology: topology.NewFunctionTopology([]topology.FunctionEntry{
			{Ref: ref, Graph: calleeGraph},
		}),
	}
	ct := callTyper{d: &Driver{activeProgram: prog}}

	capturedPath := constraint.NewPath(captured, "captured")
	otherPath := constraint.NewPath(other, "other")
	cells := flow.CaptureCellsOf([]flow.CaptureCell{
		{Symbol: captured, Value: product.FromType(typ.String)},
		{Symbol: other, Value: product.FromType(typ.Number)},
	})
	refs := flow.WithFunctionRefPath(nil, capturedPath, flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 11}))
	refs = flow.WithFunctionRefPath(refs, otherPath, flow.FunctionRefSetOf(flow.FunctionRef{GraphID: 22}))
	closures := flow.WithClosureRefPath(nil, capturedPath, flow.ClosureRefSetOf(
		flow.ClosureRefOf(flow.FunctionRef{GraphID: 33}, flow.CaptureCellsDomain.Bottom(), nil),
	))
	closures = flow.WithClosureRefPath(closures, otherPath, flow.ClosureRefSetOf(
		flow.ClosureRefOf(flow.FunctionRef{GraphID: 44}, flow.CaptureCellsDomain.Bottom(), nil),
	))

	entry, ok := ct.productCallEntryContext(ref, &ast.FuncCallExpr{}, transfer.ProductCallContext{
		References: flow.ReferenceContextOf(cells, refs, closures),
	})
	if !ok {
		t.Fatal("productCallEntryContext returned false")
	}
	if entry.Ref() != ref {
		t.Fatalf("Ref() = %+v, want %+v", entry.Ref(), ref)
	}
	if av, ok := entry.CaptureCells().Value(captured); !ok || !typ.TypeEquals(av.ProjectValue(), typ.String) {
		t.Fatalf("captured cell = %v/%v, want string", av.ProjectValue(), ok)
	}
	if _, ok := entry.CaptureCells().Value(other); ok {
		t.Fatal("non-captured cell leaked into entry context")
	}
	if set, ok := flow.FunctionRefAtPath(entry.FunctionRefs(), capturedPath); !ok || set.IsBottom() {
		t.Fatalf("captured function refs missing: %v/%v", set, ok)
	}
	if _, ok := flow.FunctionRefAtPath(entry.FunctionRefs(), otherPath); ok {
		t.Fatal("non-captured function ref leaked into entry context")
	}
	if set, ok := flow.ClosureRefAtPath(entry.ClosureRefs(), capturedPath); !ok || set.IsBottom() {
		t.Fatalf("captured closure refs missing: %v/%v", set, ok)
	}
	if _, ok := flow.ClosureRefAtPath(entry.ClosureRefs(), otherPath); ok {
		t.Fatal("non-captured closure ref leaked into entry context")
	}
}

func graphWithCapturedSymbol(t *testing.T, sym cfg.SymbolID, name string) *cfg.Graph {
	t.Helper()
	ident := &ast.IdentExpr{Value: name}
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.ReturnStmt{Exprs: []ast.Expr{ident}},
		},
	}
	b := bind.NewBindingTable()
	b.Bind(ident, sym)
	b.SetName(sym, name)
	b.SetKind(sym, cfg.SymbolLocal)
	g := cfg.BuildWithBindings(fn, b)
	if g == nil {
		t.Fatal("BuildWithBindings returned nil")
	}
	return g
}
