package body

import (
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callproducer"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func (r *Result) boundaryStateAt(point cfg.Point) (state.State, bool) {
	if r == nil {
		return state.State{}, false
	}
	if r.hasNodeLocalBoundaryEffects(point) {
		if out, ok := r.nodeOutputAt(point); ok {
			return out, true
		}
	}
	return r.solvedStateAt(point)
}

func (r *Result) boundaryRead(point cfg.Point) state.State {
	if out, ok := r.nodeOutputAt(point); ok {
		return out
	}
	if st, ok := r.solvedStateAt(point); ok {
		return st
	}
	return state.State{}
}

func (r *Result) nodeOutputAt(point cfg.Point) (state.State, bool) {
	if r == nil {
		return state.State{}, false
	}
	if out, ok := r.boundary[point]; ok {
		return out, true
	}
	graph := r.Graph()
	if r.registry == nil || graph == nil || r.boundaryXfer == nil {
		return state.State{}, false
	}
	in, ok := r.solvedStateAt(point)
	if !ok {
		return state.State{}, false
	}
	out := r.boundaryXfer(transfer.NodeContext{
		Graph:    graph,
		Registry: r.registry,
		Point:    point,
		Node:     graph.Node(point),
		Read:     r.stateRead,
	}, in)
	if r.boundary == nil {
		r.boundary = make(map[cfg.Point]state.State)
	}
	r.boundary[point] = out
	return out, true
}

func (r *Result) stateRead(point cfg.Point) state.State {
	if st, ok := r.solvedStateAt(point); ok {
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
	if callproducer.Has(r.facts, point) {
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

func (r *Result) sourceValueAtPoint(point cfg.Point, source factflow.ValueSource, st state.State, read func(cfg.Point) state.State) (product.Value, bool) {
	sources := r.boundarySources()
	if sources == nil {
		return product.Value{}, false
	}
	value, ok := sources.ValueOfSource(point, source, st, read)
	if !ok {
		return product.Value{}, false
	}
	return value, true
}
