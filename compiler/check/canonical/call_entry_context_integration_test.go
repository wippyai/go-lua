package canonical

import (
	"slices"
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

	projector := callEntryProjector{program: prog, typer: ct}
	entry := projector.productEntryContext(ref, &ast.FuncCallExpr{}, transfer.ProductCallContext{
		References: flow.ReferenceContextOf(cells, refs, closures),
	})
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

func TestProductCallEntryContextCarriesProjectedBoundaryFacts(t *testing.T) {
	ref := summary.FuncRef{GraphID: 12}
	param := &ast.IdentExpr{Value: "graph"}
	call := &ast.FuncCallExpr{Args: []ast.Expr{param}}
	graphSym := cfg.SymbolID(303)
	graphPath := constraint.NewPath(graphSym, "graph")
	nodeOrder := graphPath.Field("node_order")
	edges := graphPath.Field("edges")
	edgeValue := product.FromType(typ.NewRecord().
		Field("targets", typ.NewArray(typ.String)).
		Field("error_targets", typ.NewArray(typ.String)).
		Build())

	callerBindings := bind.NewBindingTable()
	callerBindings.Bind(param, graphSym)
	callerBindings.SetName(graphSym, "graph")
	callerGraph := cfg.BuildWithBindings(&ast.FunctionExpr{}, callerBindings)
	calleeFn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"graph"}}}
	calleeGraph := cfg.Build(calleeFn)
	prog := &program{
		funcTopology: topology.NewFunctionTopology([]topology.FunctionEntry{
			{Ref: ref, Graph: calleeGraph, Function: calleeFn},
		}),
	}
	ct := callTyper{d: &Driver{activeProgram: prog}, g: callerGraph}
	projector := callEntryProjector{program: prog, graph: callerGraph, typer: ct}
	entry := projector.productEntryContext(ref, call, transfer.ProductCallContext{
		RuntimeArgValues: []product.AbstractValue{product.FromType(typ.NewRecord().Build())},
		BoundaryFacts: flow.BoundaryFactProjectionInput{
			KeyPresence: flow.KeyPresenceFacts{}.
				WithKeyArrayValueAddresses(
					testCallEntryStableAddress(t, nodeOrder),
					testCallEntryStableAddress(t, edges),
					edgeValue,
				),
		},
	})

	facts := entry.EntryFacts().KeyArrayValues()
	if len(facts) != 1 {
		t.Fatalf("entry facts = %#v, want one key-array-value proof", entry.EntryFacts())
	}
	wantArray := flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 0, Segments: nodeOrder.Segments}
	wantTable := flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 0, Segments: edges.Segments}
	if facts[0].Array.Kind != wantArray.Kind ||
		facts[0].Array.Index != wantArray.Index ||
		!slices.Equal(facts[0].Array.Segments, wantArray.Segments) ||
		facts[0].Table.Kind != wantTable.Kind ||
		facts[0].Table.Index != wantTable.Index ||
		!slices.Equal(facts[0].Table.Segments, wantTable.Segments) ||
		!product.Domain.Equal(facts[0].Value, edgeValue) {
		t.Fatalf("entry fact = %#v, want array %#v table %#v value %s", facts[0], wantArray, wantTable, edgeValue.ProjectValue())
	}
}

func TestPointCallEntryFactsCarryProjectedBoundaryFacts(t *testing.T) {
	ref := summary.FuncRef{GraphID: 13}
	param := &ast.IdentExpr{Value: "graph"}
	call := &ast.FuncCallExpr{Args: []ast.Expr{param}}
	graphSym := cfg.SymbolID(313)
	graphPath := constraint.NewPath(graphSym, "graph")
	refsPath := graphPath.Field("references")
	keyPath := graphPath.Field("last_node_id")
	value := product.FromType(typ.String)

	callerBindings := bind.NewBindingTable()
	callerBindings.Bind(param, graphSym)
	callerBindings.SetName(graphSym, "graph")
	callerGraph := cfg.BuildWithBindings(&ast.FunctionExpr{}, callerBindings)
	calleeFn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"graph"}}}
	calleeGraph := cfg.Build(calleeFn)
	prog := &program{
		funcTopology: topology.NewFunctionTopology([]topology.FunctionEntry{
			{Ref: ref, Graph: calleeGraph, Function: calleeFn},
		}),
	}
	ct := callTyper{d: &Driver{activeProgram: prog}, g: callerGraph}
	projector := callEntryProjector{program: prog, graph: callerGraph, typer: ct}
	state := flow.PointState{
		IndexWrites: flow.IndexWriteAdmissionFacts{}.WithAddress(flow.IndexWriteAdmissionAddressFact{
			Target:     testCallEntryStableAddress(t, refsPath),
			KeyPath:    testCallEntryStableAddress(t, keyPath),
			HasKeyPath: true,
			Key:        product.FromType(typ.String),
			Value:      value,
		}),
	}

	facts := projector.pointFacts(ref, call, &state)
	writes := facts.IndexWrites()
	wantTable := flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 0, Segments: refsPath.Segments}
	wantKey := flow.BoundaryPath{Kind: flow.BoundaryPathParam, Index: 0, Segments: keyPath.Segments}
	if len(writes) != 1 ||
		!boundaryPathEqual(writes[0].Table, wantTable) ||
		!writes[0].HasKeyPath ||
		!boundaryPathEqual(writes[0].KeyPath, wantKey) ||
		!product.Domain.Equal(writes[0].Value, value) {
		t.Fatalf("point entry index writes = %#v, want table %#v key %#v", writes, wantTable, wantKey)
	}
}

func testCallEntryStableAddress(t *testing.T, path constraint.Path) flow.StableAddress {
	t.Helper()
	addr, ok := flow.StableAddressOfPath(path)
	if !ok {
		t.Fatalf("stable address for path %s", path.Key())
	}
	return addr
}

func boundaryPathEqual(a, b flow.BoundaryPath) bool {
	return a.Kind == b.Kind && a.Index == b.Index && slices.Equal(a.Segments, b.Segments)
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
