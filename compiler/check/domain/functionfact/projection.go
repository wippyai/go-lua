package functionfact

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

// Cache memoizes canonical function-fact projections for one check.
type Cache struct {
	facts map[CacheKey]cachedFact
}

// CacheKey identifies a canonical function-fact projection.
type CacheKey struct {
	GraphID uint64
	Parent  *scope.State
	Sym     cfg.SymbolID
	Phase   api.Phase
}

type cachedFact struct {
	Fact  api.FunctionFact
	Found bool
}

// NewCache creates an empty function-fact projection cache.
func NewCache() *Cache {
	return &Cache{facts: make(map[CacheKey]cachedFact)}
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
	return FactForGraph(store, graphForRef(store, ref), sym, defaultParent, nil)
}

// TypeForSymbol returns the canonical stored function type fact for sym.
func TypeForSymbol(store api.StoreReader, sym cfg.SymbolID, defaultParent *scope.State, cache *Cache) typ.Type {
	if store == nil || sym == 0 {
		return nil
	}
	ref := store.FunctionRefBySym(sym)
	if ref == nil {
		return nil
	}
	return TypeForGraph(store, graphForRef(store, ref), sym, defaultParent, cache)
}

// ReturnSummaryForSymbol returns the canonical declared/pre-flow return summary
// for sym from its owning function-fact product.
func ReturnSummaryForSymbol(store api.StoreReader, sym cfg.SymbolID, defaultParent *scope.State, cache *Cache) []typ.Type {
	ff, ok := factForSymbolInPhase(store, sym, defaultParent, api.PhaseScopeCompute, cache)
	if !ok {
		return nil
	}
	return ff.Summary
}

// NarrowSummaryForSymbol returns the canonical post-flow return summary for sym
// from its owning function-fact product.
func NarrowSummaryForSymbol(store api.StoreReader, sym cfg.SymbolID, defaultParent *scope.State, cache *Cache) []typ.Type {
	ff, ok := factForSymbolInPhase(store, sym, defaultParent, api.PhaseNarrowing, cache)
	if !ok {
		return nil
	}
	return ff.Narrow
}

// GraphKeyForSymbol returns the canonical parent graph key that owns sym's
// function-fact product.
func GraphKeyForSymbol(store api.StoreReader, sym cfg.SymbolID, defaultParent *scope.State) (api.GraphKey, bool) {
	if store == nil || sym == 0 {
		return api.GraphKey{}, false
	}
	ref := store.FunctionRefBySym(sym)
	if ref == nil {
		return api.GraphKey{}, false
	}
	graph := graphForRef(store, ref)
	if graph == nil {
		return api.GraphKey{}, false
	}
	parent := api.ParentScopeForGraph(store, graph.ID(), defaultParent)
	if parent == nil {
		return api.GraphKey{}, false
	}
	return store.GraphKeyFor(graph, parent)
}

// TypeForGraph returns the canonical function type fact for sym from graph's
// function-fact product.
func TypeForGraph(store api.StoreReader, graph *cfg.Graph, sym cfg.SymbolID, defaultParent *scope.State, cache *Cache) typ.Type {
	ff, ok := FactForGraph(store, graph, sym, defaultParent, cache)
	if !ok {
		return nil
	}
	return ff.Type
}

// ReturnSummaryForGraph returns the canonical declared/pre-flow return summary
// for sym from graph's function-fact product.
func ReturnSummaryForGraph(store api.StoreReader, graph *cfg.Graph, sym cfg.SymbolID, defaultParent *scope.State, cache *Cache) []typ.Type {
	ff, ok := factForGraphInPhase(store, graph, sym, defaultParent, api.PhaseScopeCompute, cache)
	if !ok {
		return nil
	}
	return ff.Summary
}

// NarrowSummaryForGraph returns the canonical post-flow return summary for sym
// from graph's function-fact product.
func NarrowSummaryForGraph(store api.StoreReader, graph *cfg.Graph, sym cfg.SymbolID, defaultParent *scope.State, cache *Cache) []typ.Type {
	ff, ok := factForGraphInPhase(store, graph, sym, defaultParent, api.PhaseNarrowing, cache)
	if !ok {
		return nil
	}
	return ff.Narrow
}

// ReturnsForPhase returns the return projection visible in phase.
func ReturnsForPhase(facts api.FunctionFacts, sym cfg.SymbolID, phase api.Phase) []typ.Type {
	ff, ok := FactFromMap(facts, sym)
	if !ok {
		return nil
	}
	return returnsForPhase(ff, phase)
}

// TypeFromMap returns the canonical function type projection from a fact map.
func TypeFromMap(facts api.FunctionFacts, sym cfg.SymbolID) typ.Type {
	ff, ok := FactFromMap(facts, sym)
	if !ok {
		return nil
	}
	return ff.Type
}

// ParameterEvidenceFromMap returns the canonical parameter evidence projection
// from a fact map.
func ParameterEvidenceFromMap(facts api.FunctionFacts, sym cfg.SymbolID) []typ.Type {
	ff, ok := FactFromMap(facts, sym)
	if !ok {
		return nil
	}
	return ff.Params
}

// ReturnSummaryFromMap returns the canonical declared/pre-flow return summary
// projection from a fact map.
func ReturnSummaryFromMap(facts api.FunctionFacts, sym cfg.SymbolID) []typ.Type {
	ff, ok := FactFromMap(facts, sym)
	if !ok {
		return nil
	}
	return ff.Summary
}

// NarrowSummaryFromMap returns the canonical post-flow return summary
// projection from a fact map.
func NarrowSummaryFromMap(facts api.FunctionFacts, sym cfg.SymbolID) []typ.Type {
	ff, ok := FactFromMap(facts, sym)
	if !ok {
		return nil
	}
	return ff.Narrow
}

// RefinementFromMap returns the canonical refinement projection from a fact map.
func RefinementFromMap(facts api.FunctionFacts, sym cfg.SymbolID) *constraint.FunctionRefinement {
	ff, ok := FactFromMap(facts, sym)
	if !ok {
		return nil
	}
	return ff.Refinement
}

// FactFromMap returns the canonical stored function fact for sym from facts.
func FactFromMap(facts api.FunctionFacts, sym cfg.SymbolID) (api.FunctionFact, bool) {
	if len(facts) == 0 || sym == 0 {
		return api.FunctionFact{}, false
	}
	ff, ok := facts[sym]
	if !ok {
		return api.FunctionFact{}, false
	}
	ff = Normalize(ff)
	return ff, !Empty(ff)
}

// FactForGraph returns the canonical stored function fact for sym from graph's
// function-fact product.
func FactForGraph(store api.StoreReader, graph *cfg.Graph, sym cfg.SymbolID, defaultParent *scope.State, cache *Cache) (api.FunctionFact, bool) {
	return factForGraphInPhase(store, graph, sym, defaultParent, api.PhaseScopeCompute, cache)
}

func factForSymbolInPhase(store api.StoreReader, sym cfg.SymbolID, defaultParent *scope.State, phase api.Phase, cache *Cache) (api.FunctionFact, bool) {
	if store == nil || sym == 0 {
		return api.FunctionFact{}, false
	}
	ref := store.FunctionRefBySym(sym)
	if ref == nil {
		return api.FunctionFact{}, false
	}
	return factForGraphInPhase(store, graphForRef(store, ref), sym, defaultParent, phase, cache)
}

func factForGraphInPhase(store api.StoreReader, graph *cfg.Graph, sym cfg.SymbolID, defaultParent *scope.State, phase api.Phase, cache *Cache) (api.FunctionFact, bool) {
	if store == nil || graph == nil || sym == 0 {
		return api.FunctionFact{}, false
	}
	parent := api.ParentScopeForGraph(store, graph.ID(), defaultParent)
	if parent == nil {
		return api.FunctionFact{}, false
	}
	key := CacheKey{GraphID: graph.ID(), Parent: parent, Sym: sym, Phase: phase}
	if cache != nil {
		if cached, ok := cache.get(key); ok {
			return cached.Fact, cached.Found
		}
	}
	facts := functionFactsForGraph(store, graph, parent, phase)
	ff, found := FactFromMap(facts, sym)
	if cache != nil {
		cache.set(key, ff, found)
	}
	return ff, found
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

func returnsForPhase(ff api.FunctionFact, phase api.Phase) []typ.Type {
	if phase == api.PhaseNarrowing && len(ff.Narrow) > 0 {
		return ff.Narrow
	}
	return ff.Summary
}

func (c *Cache) get(key CacheKey) (cachedFact, bool) {
	if c == nil || c.facts == nil {
		return cachedFact{}, false
	}
	cached, ok := c.facts[key]
	return cached, ok
}

func (c *Cache) set(key CacheKey, fact api.FunctionFact, found bool) {
	if c == nil {
		return
	}
	if c.facts == nil {
		c.facts = make(map[CacheKey]cachedFact)
	}
	c.facts[key] = cachedFact{Fact: fact, Found: found}
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

func functionFactsForGraph(store api.StoreReader, graph *cfg.Graph, parent *scope.State, phase api.Phase) api.FunctionFacts {
	if store == nil || graph == nil || parent == nil {
		return nil
	}
	var facts api.FunctionFacts
	load := func() {
		facts = store.GetInterprocFacts(graph, parent).FunctionFacts
	}
	if phaser, ok := store.(interface{ WithPhase(api.Phase, func()) }); ok {
		phaser.WithPhase(phase, load)
	} else {
		load()
	}
	return facts
}
