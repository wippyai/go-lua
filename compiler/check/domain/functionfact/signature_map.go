package functionfact

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

// ParameterEvidenceSignatures builds a function-expression keyed parameter
// evidence map for graph from canonical FunctionFacts.
func ParameterEvidenceSignatures(
	store api.StoreReader,
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
		functionFacts := store.GetInterprocFacts(graph, parent).FunctionFacts
		for _, sym := range cfg.SortedSymbolIDs(functionFacts) {
			add(store.FuncForSymbol(sym), ParameterEvidenceFromMap(functionFacts, sym))
		}
	}

	if meta, ok := store.NestedMetaFor(graph.ID()); ok {
		parentGraph := store.Graphs()[meta.ParentGraphID]
		if parentGraph != nil {
			defaultScope := (*scope.State)(nil)
			if _, isNestedParent := store.NestedMetaFor(parentGraph.ID()); !isNestedParent {
				defaultScope = stdlib
			}
			parentScope := api.ParentScopeForGraph(store, parentGraph.ID(), defaultScope)
			if parentScope != nil {
				parentFacts := store.GetInterprocFacts(parentGraph, parentScope).FunctionFacts
				if fn := graphFunction(store, graph); fn != nil {
					if sym, ok := store.SymbolForFunc(fn); ok {
						add(fn, ParameterEvidenceFromMap(parentFacts, sym))
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

func graphFunction(store api.StoreReader, graph *cfg.Graph) *ast.FunctionExpr {
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
