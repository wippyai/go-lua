package body

import (
	"github.com/wippyai/go-lua/analysis/check/body/internal/readexpr"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// SourceValueAtBoundary resolves a lowered value source at the diagnostic read
// boundary for point. Node-local solved effects, such as call-result facts,
// postconditions, and assignments, are visible at that boundary.
func (r *Result) SourceValueAtBoundary(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	if r == nil || r.registry == nil {
		return product.Value{}, false
	}
	if source.Kind == factflow.ValueSourceCall {
		return r.callResultValueAtBoundary(source)
	}
	in, ok := r.boundaryStateAt(point)
	if !ok {
		return product.Value{}, false
	}
	sources := r.boundarySources()
	if sources == nil {
		return product.Value{}, false
	}
	value, ok := sources.ValueOfSource(point, source, in, r.boundaryRead)
	if !ok || product.Equal(r.registry, value, product.Bottom(r.registry)) {
		return product.Value{}, false
	}
	return value, true
}

// ExpressionValueAtBoundary projects a Lua expression's product value at the
// diagnostic read boundary for point.
func (r *Result) ExpressionValueAtBoundary(point cfg.Point, expr ast.Expr) (product.Value, bool) {
	p, ok := r.ExpressionPath(expr)
	if !ok {
		return product.Value{}, false
	}
	return r.PathValueAtBoundary(point, p)
}

// PathValueAtBoundary projects a path's product value at the diagnostic read
// boundary for point.
func (r *Result) PathValueAtBoundary(point cfg.Point, p pathdom.Path) (product.Value, bool) {
	if r == nil || r.registry == nil || p.IsEmpty() {
		return product.Value{}, false
	}
	in, ok := r.boundaryStateAt(point)
	if !ok {
		return product.Value{}, false
	}
	value, ok := readexpr.Project(readexpr.Config{
		Registry:   r.registry,
		Facts:      r.facts,
		Visibility: r.visibility,
	}, point, p, in)
	if !ok || product.Equal(r.registry, value, product.Bottom(r.registry)) {
		return product.Value{}, false
	}
	return value, true
}

// SymbolValueAtBoundary reads a root symbol value at the diagnostic read
// boundary for point.
func (r *Result) SymbolValueAtBoundary(point cfg.Point, id symbol.ID) (product.Value, bool) {
	if id == 0 {
		return product.Value{}, false
	}
	return r.PathValueAtBoundary(point, pathdom.NewPath(id, r.SymbolName(id)))
}

// CallOutcomeAt resolves the configured call-boundary evidence for point.
func (r *Result) CallOutcomeAt(point cfg.Point) (factapply.CallOutcome, bool) {
	if r == nil || r.registry == nil || r.callOutcome == nil {
		return factapply.CallOutcome{}, false
	}
	site, ok := r.facts.CallSite(point)
	if !ok {
		return factapply.CallOutcome{}, false
	}
	in, ok := r.StateAt(point)
	if !ok {
		return factapply.CallOutcome{}, false
	}
	graph := r.Graph()
	ctx := transfer.NodeContext{
		Graph:    graph,
		Point:    point,
		Registry: r.registry,
		Read:     r.boundaryRead,
	}
	if graph != nil {
		ctx.Node = graph.Node(point)
	}
	return r.callOutcome(ctx, site, in, r.boundaryRead), true
}

func (r *Result) callResultValueAtBoundary(source factflow.ValueSource) (product.Value, bool) {
	if !source.HasCallPoint || source.ResultIndex < 0 {
		return product.Value{}, false
	}
	out, ok := r.nodeOutputAt(source.CallPoint)
	if !ok {
		out, ok = r.StateAt(source.CallPoint)
	}
	if !ok {
		return product.Value{}, false
	}
	value := out.ReadReturnSlot(r.registry, source.ResultIndex)
	if product.Equal(r.registry, value, product.Bottom(r.registry)) {
		return product.Value{}, false
	}
	return value, true
}

func (r *Result) boundaryStateAt(point cfg.Point) (state.State, bool) {
	if r == nil {
		return state.State{}, false
	}
	if r.hasNodeLocalBoundaryEffects(point) {
		if out, ok := r.nodeOutputAt(point); ok {
			return out, true
		}
	}
	return r.StateAt(point)
}

func (r *Result) boundaryRead(point cfg.Point) state.State {
	if out, ok := r.nodeOutputAt(point); ok {
		return out
	}
	if st, ok := r.StateAt(point); ok {
		return st
	}
	return state.State{}
}

func (r *Result) nodeOutputAt(point cfg.Point) (state.State, bool) {
	if r == nil || r.registry == nil {
		return state.State{}, false
	}
	graph := r.Graph()
	if graph == nil {
		return state.State{}, false
	}
	in, ok := r.StateAt(point)
	if !ok {
		return state.State{}, false
	}
	transferFn := factapply.NewFactsNodeTransfer(factapply.FactsNodeTransferConfig{
		Facts:       r.facts,
		Sources:     r.sources,
		CallOutcome: r.callOutcome,
		Visibility:  r.visibility,
	})
	return transferFn(transfer.NodeContext{
		Graph:    graph,
		Registry: r.registry,
		Point:    point,
		Node:     graph.Node(point),
		Read:     r.stateRead,
	}, in), true
}

func (r *Result) stateRead(point cfg.Point) state.State {
	if st, ok := r.StateAt(point); ok {
		return st
	}
	return state.State{}
}

func (r *Result) hasNodeLocalBoundaryEffects(point cfg.Point) bool {
	if _, ok := r.facts.RootAssignment(point); ok {
		return true
	}
	if _, ok := r.facts.PathAssignment(point); ok {
		return true
	}
	if _, ok := r.facts.PathDescendantInvalidation(point); ok {
		return true
	}
	if _, ok := r.facts.Call(point); ok {
		return true
	}
	if r.callOutcome != nil {
		if _, ok := r.facts.CallSite(point); ok {
			return true
		}
	}
	if r.facts.NoNormalReturn(point) {
		return true
	}
	return len(r.facts.PostconditionRefinements(point)) != 0 ||
		len(r.facts.PostconditionPathRelations(point)) != 0
}

func (r *Result) boundarySources() sourcevalue.SourceValues {
	if r == nil || r.sources == nil {
		return nil
	}
	return sourcevalue.WithExpressionRefinements(r.registry, r.sources, r.facts.ExpressionRefinements())
}
