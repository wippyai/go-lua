package functionfact

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

// SignatureProjectionReader is the minimal store surface needed to expose
// function-fact projections visible to graph analysis.
type SignatureProjectionReader interface {
	Graphs() map[uint64]*cfg.Graph
	FuncForGraph(graph *cfg.Graph) *ast.FunctionExpr
	FuncForSymbol(sym cfg.SymbolID) *ast.FunctionExpr
	SymbolForFunc(fn *ast.FunctionExpr) (cfg.SymbolID, bool)
	FunctionRefsByParentGraph(parentGraphID uint64) []api.FunctionRef
	NestedMetaFor(graphID uint64) (api.NestedMeta, bool)
	Parents() map[uint64]*scope.State
	GraphParentHashOf(graphID uint64) uint64
	CanonicalFunctionFactProjection(graph *cfg.Graph, parent *scope.State, sym cfg.SymbolID) (api.FunctionFact, bool)
}

// ParameterEvidenceSignatures builds a function-expression keyed parameter
// evidence map for graph from FunctionFacts projection.
func ParameterEvidenceSignatures(
	store SignatureProjectionReader,
	graph *cfg.Graph,
	parent *scope.State,
	stdlib *scope.State,
) map[*ast.FunctionExpr][]typ.Type {
	if store == nil || graph == nil {
		return nil
	}

	out := make(map[*ast.FunctionExpr][]typ.Type)
	add := func(fn *ast.FunctionExpr, evidence []typ.Type) {
		if fn == nil || !hasParameterEvidence(evidence) {
			return
		}
		if _, exists := out[fn]; !exists {
			out[fn] = evidence
		}
	}

	if parent != nil {
		if fn := graphFunction(store, graph); fn != nil {
			if sym, ok := store.SymbolForFunc(fn); ok {
				if ff, found := projectedFunctionFactForGraph(store, graph, parent, sym); found {
					add(fn, BodyEntryEvidence(ff))
				}
			}
		}
		for _, ref := range store.FunctionRefsByParentGraph(graph.ID()) {
			if ff, ok := projectedFunctionFactForGraph(store, graph, parent, ref.Sym); ok {
				add(store.FuncForSymbol(ref.Sym), BodyEntryEvidence(ff))
			}
		}
	}

	if meta, ok := store.NestedMetaFor(graph.ID()); ok {
		parentGraph := store.Graphs()[meta.ParentGraphID]
		if parentGraph != nil {
			defaultScope := (*scope.State)(nil)
			if _, isNestedParent := store.NestedMetaFor(parentGraph.ID()); !isNestedParent {
				defaultScope = stdlib
			}
			parentScope := parentScopeForGraph(store, parentGraph.ID(), defaultScope)
			if parentScope != nil {
				if fn := graphFunction(store, graph); fn != nil {
					if sym, ok := store.SymbolForFunc(fn); ok {
						if ff, ok := projectedFunctionFactForGraph(store, parentGraph, parentScope, sym); ok {
							add(fn, BodyEntryEvidence(ff))
						}
					}
				}
			}
		}
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

// VisibleFactsForGraph returns the FunctionFacts projection visible to graph
// analysis. For nested function graphs, same-scope function facts live in
// the lexical parent graph product; this projection makes those facts available
// to body analysis without duplicating storage.
func VisibleFactsForGraph(
	store SignatureProjectionReader,
	graph *cfg.Graph,
	parent *scope.State,
	stdlib *scope.State,
) api.FunctionFacts {
	if store == nil || graph == nil {
		return nil
	}

	var out api.FunctionFacts
	add := func(sym cfg.SymbolID, ff api.FunctionFact) {
		if sym == 0 || Empty(ff) {
			return
		}
		if out == nil {
			out = make(api.FunctionFacts)
		}
		out[sym] = ff
	}

	if fn := graphFunction(store, graph); fn != nil {
		if sym, ok := store.SymbolForFunc(fn); ok {
			if ff, found := projectedFunctionFactForGraph(store, graph, parent, sym); found {
				add(sym, ff)
			}
		}
	}
	for _, ref := range store.FunctionRefsByParentGraph(graph.ID()) {
		if ff, ok := projectedFunctionFactForGraph(store, graph, parent, ref.Sym); ok {
			add(ref.Sym, ff)
		}
	}

	if meta, ok := store.NestedMetaFor(graph.ID()); ok {
		parentGraph := store.Graphs()[meta.ParentGraphID]
		if parentGraph != nil {
			defaultScope := (*scope.State)(nil)
			if _, isNestedParent := store.NestedMetaFor(parentGraph.ID()); !isNestedParent {
				defaultScope = stdlib
			}
			parentScope := parentScopeForGraph(store, parentGraph.ID(), defaultScope)
			if parentScope != nil {
				for _, ref := range store.FunctionRefsByParentGraph(parentGraph.ID()) {
					if ff, ok := projectedFunctionFactForGraph(store, parentGraph, parentScope, ref.Sym); ok {
						add(ref.Sym, ff)
					}
				}
			}
		}
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func projectedFunctionFactForGraph(store canonicalFunctionFactProjector, graph *cfg.Graph, parent *scope.State, sym cfg.SymbolID) (api.FunctionFact, bool) {
	if store == nil || graph == nil || sym == 0 {
		return api.FunctionFact{}, false
	}
	return store.CanonicalFunctionFactProjection(graph, parent, sym)
}

func graphFunction(store interface {
	FuncForGraph(graph *cfg.Graph) *ast.FunctionExpr
}, graph *cfg.Graph) *ast.FunctionExpr {
	if store == nil || graph == nil {
		return nil
	}
	if fn := store.FuncForGraph(graph); fn != nil {
		return fn
	}
	return graph.Func()
}

func hasParameterEvidence(evidence []typ.Type) bool {
	for _, observed := range evidence {
		if observed != nil {
			return true
		}
	}
	return false
}
