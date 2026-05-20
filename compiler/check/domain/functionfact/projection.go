package functionfact

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

// TypeCache memoizes canonical function-fact type projections for one check.
type TypeCache map[TypeCacheKey]typ.Type

// TypeCacheKey identifies a canonical function-fact type projection.
type TypeCacheKey struct {
	GraphID uint64
	Parent  *scope.State
	Sym     cfg.SymbolID
}

// ForSymbol returns the canonical stored function fact for sym.
func ForSymbol(store api.StoreReader, sym cfg.SymbolID, defaultParent *scope.State) (api.FunctionFact, bool) {
	if store == nil || sym == 0 {
		return api.FunctionFact{}, false
	}
	ref := store.FunctionRefBySym(sym)
	if ref == nil {
		return api.FunctionFact{}, false
	}
	parentGraph := graphForRef(store, ref)
	if parentGraph == nil {
		return api.FunctionFact{}, false
	}
	parent := api.ParentScopeForGraph(store, parentGraph.ID(), defaultParent)
	if parent == nil {
		return api.FunctionFact{}, false
	}
	return store.GetInterprocFacts(parentGraph, parent).FunctionFacts.Fact(sym)
}

// TypeForSymbol returns the canonical stored function type fact for sym.
func TypeForSymbol(store api.StoreReader, sym cfg.SymbolID, defaultParent *scope.State, cache TypeCache) typ.Type {
	if store == nil || sym == 0 {
		return nil
	}
	ref := store.FunctionRefBySym(sym)
	if ref == nil {
		return nil
	}
	return TypeForGraph(store, graphForRef(store, ref), sym, defaultParent, cache)
}

// TypeForGraph returns the canonical function type fact for sym from graph's
// function-fact product.
func TypeForGraph(store api.StoreReader, graph *cfg.Graph, sym cfg.SymbolID, defaultParent *scope.State, cache TypeCache) typ.Type {
	if store == nil || graph == nil || sym == 0 {
		return nil
	}
	parent := api.ParentScopeForGraph(store, graph.ID(), defaultParent)
	if parent == nil {
		return nil
	}
	key := TypeCacheKey{GraphID: graph.ID(), Parent: parent, Sym: sym}
	if cache != nil {
		if cached, ok := cache[key]; ok {
			return cached
		}
	}
	facts := functionFactsForGraph(store, graph, parent)
	factType := facts.FunctionType(sym)
	if cache != nil {
		cache[key] = factType
	}
	return factType
}

// RefinementsFromStore projects canonical function facts as refinement facts.
func RefinementsFromStore(store api.StoreReader, defaultParent *scope.State) api.RefinementFacts {
	if store == nil {
		return nil
	}
	return api.NewRefinementFacts(func(sym cfg.SymbolID) *constraint.FunctionRefinement {
		ff, ok := ForSymbol(store, sym, defaultParent)
		if !ok {
			return nil
		}
		return ff.Refinement
	})
}

func graphForRef(store api.StoreReader, ref *api.FunctionRef) *cfg.Graph {
	if store == nil || ref == nil {
		return nil
	}
	parentGraphID := ref.ParentGraphID
	if parentGraphID == 0 {
		parentGraphID = ref.GraphID
	}
	return store.Graphs()[parentGraphID]
}

func functionFactsForGraph(store api.StoreReader, graph *cfg.Graph, parent *scope.State) api.FunctionFacts {
	if store == nil || graph == nil || parent == nil {
		return nil
	}
	var facts api.FunctionFacts
	load := func() {
		facts = store.GetInterprocFacts(graph, parent).FunctionFacts
	}
	if phaser, ok := store.(interface{ WithPhase(api.Phase, func()) }); ok {
		phaser.WithPhase(api.PhaseScopeCompute, load)
	} else {
		load()
	}
	return facts
}
