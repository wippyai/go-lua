// Package facts projects stable interprocedural facts from the product state.
package facts

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

// FunctionFactForSymbol returns the canonical stable function fact for sym.
func FunctionFactForSymbol(store api.StoreReader, sym cfg.SymbolID, defaultParent *scope.State) (api.FunctionFact, bool) {
	if store == nil || sym == 0 {
		return api.FunctionFact{}, false
	}
	ref := store.FunctionRefBySym(sym)
	if ref == nil {
		return api.FunctionFact{}, false
	}
	parentGraphID := ref.ParentGraphID
	if parentGraphID == 0 {
		parentGraphID = ref.GraphID
	}
	parentGraph := store.Graphs()[parentGraphID]
	if parentGraph == nil {
		return api.FunctionFact{}, false
	}
	parent := api.ParentScopeForGraph(store, parentGraph.ID(), defaultParent)
	if parent == nil {
		return api.FunctionFact{}, false
	}
	return store.GetInterprocFacts(parentGraph, parent).FunctionFacts.Fact(sym)
}

// FunctionTypeForSymbol returns the canonical stable function type fact for sym.
func FunctionTypeForSymbol(store api.StoreReader, sym cfg.SymbolID, defaultParent *scope.State) typ.Type {
	ff, ok := FunctionFactForSymbol(store, sym, defaultParent)
	if !ok {
		return nil
	}
	return ff.Type
}

// RefinementsFromFunctionFacts projects canonical function facts as refinement facts.
func RefinementsFromFunctionFacts(store api.StoreReader, defaultParent *scope.State) api.RefinementFacts {
	if store == nil {
		return nil
	}
	return api.NewRefinementFacts(func(sym cfg.SymbolID) *constraint.FunctionRefinement {
		ff, ok := FunctionFactForSymbol(store, sym, defaultParent)
		if !ok {
			return nil
		}
		return ff.Refinement
	})
}
