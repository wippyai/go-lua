package store

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/interproc"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/db"
)

type SessionStore struct {
	// Module contains module-wide immutable state.
	// Created once at the start of checking and shared by all CFG builds.
	Module *ModuleStore

	// InterprocPrev holds the stable interproc product from completed iterations.
	InterprocPrev *InterprocState
	// InterprocNext accumulates facts/effects produced during the current iteration.
	InterprocNext *InterprocState

	// GraphParentHash records the parent scope hash for each graph ID.
	GraphParentHash map[uint64]uint64

	// lastSwapDiffs records product components changed by the most recent FixpointSwap.
	// Stored per-session to avoid cross-session contamination.
	lastSwapDiffs []string

	factInputs *factInputs
	factCtx    *db.QueryContext

	phase api.Phase
}

// InterprocState holds the graph-keyed interprocedural fact product for one iteration side.
type InterprocState struct {
	Facts map[api.GraphKey]api.Facts
}

// NewInterprocState creates an initialized interproc product side.
func NewInterprocState() *InterprocState {
	return &InterprocState{
		Facts: make(map[api.GraphKey]api.Facts),
	}
}

// ModuleStore holds module-wide immutable state.
// Created once at the start of checking and never modified during iteration.
type ModuleStore struct {
	// ModuleBindings is the binding table for the entire module.
	ModuleBindings *bind.BindingTable

	// Graphs maps graph IDs to CFG graphs.
	Graphs map[uint64]*cfg.Graph

	// Funcs maps graph IDs to FunctionExpr nodes.
	Funcs map[uint64]*ast.FunctionExpr

	// GraphToFunc maps CFG graphs to their FunctionExpr nodes for O(1) lookup.
	GraphToFunc map[*cfg.Graph]*ast.FunctionExpr
	// FuncToGraph maps FunctionExpr nodes to their CFG graphs for O(1) lookup.
	FuncToGraph map[*ast.FunctionExpr]*cfg.Graph
	// Functions stores canonical mappings between symbols and function graphs.
	Functions *FunctionRegistry

	// Parents maps scope hashes to parent scope states.
	Parents map[uint64]*scope.State

	// ModuleAliases maps symbol IDs to module paths from require() assignments.
	// Computed once from the chunk graph and shared with all nested functions.
	ModuleAliases map[cfg.SymbolID]string

	// NestedMeta maps child graph IDs to their parent graph and definition point.
	NestedMeta map[uint64]api.NestedMeta
}

// FunctionRegistry provides symbol <-> function graph lookup.
type FunctionRegistry struct {
	BySym     map[cfg.SymbolID]*api.FunctionRef
	ByFunc    map[*ast.FunctionExpr]*api.FunctionRef
	ByGraphID map[uint64]*api.FunctionRef
}

// NewSessionStore creates an initialized store with all sub-structs.
func NewSessionStore() *SessionStore {
	return NewSessionStoreWithDB(nil)
}

// NewSessionStoreWithDB creates a store whose interproc fact products are
// tracked as query inputs. The checker uses this form so function-result queries
// are revalidated from the exact facts they read instead of from a coarse
// iteration revision key.
func NewSessionStoreWithDB(database *db.DB) *SessionStore {
	return &SessionStore{
		Module:          NewModuleStore(),
		InterprocPrev:   NewInterprocState(),
		InterprocNext:   NewInterprocState(),
		GraphParentHash: make(map[uint64]uint64),
		factInputs:      newFactInputs(database),
		phase:           api.PhaseScopeCompute,
	}
}

// SetPhase sets the current check phase for fact-product access checks.
func (s *SessionStore) SetPhase(phase api.Phase) {
	if s == nil {
		return
	}
	s.phase = phase
}

// Phase returns the current check phase.
func (s *SessionStore) Phase() api.Phase {
	if s == nil {
		return api.PhaseScopeCompute
	}
	return s.phase
}

func (s *SessionStore) requirePhase(allowed ...api.Phase) {
	if s == nil {
		return
	}
	for _, phase := range allowed {
		if s.phase == phase {
			return
		}
	}
	panic("store: interproc facts accessed in wrong phase")
}

// WithPhase runs fn with a temporary phase, restoring the prior phase afterward.
func (s *SessionStore) WithPhase(phase api.Phase, fn func()) {
	if fn == nil {
		return
	}
	if s == nil {
		fn()
		return
	}
	prev := s.phase
	s.phase = phase
	defer func() {
		s.phase = prev
	}()
	fn()
}

// NewModuleStore creates an initialized module store.
func NewModuleStore() *ModuleStore {
	return &ModuleStore{
		Graphs:        make(map[uint64]*cfg.Graph),
		Funcs:         make(map[uint64]*ast.FunctionExpr),
		GraphToFunc:   make(map[*cfg.Graph]*ast.FunctionExpr),
		FuncToGraph:   make(map[*ast.FunctionExpr]*cfg.Graph),
		Functions:     newFunctionRegistry(),
		Parents:       make(map[uint64]*scope.State),
		ModuleAliases: make(map[cfg.SymbolID]string),
		NestedMeta:    make(map[uint64]api.NestedMeta),
	}
}

func (s *SessionStore) ensureInterprocStates() {
	if s == nil {
		return
	}
	if s.InterprocPrev == nil {
		s.InterprocPrev = NewInterprocState()
	}
	if s.InterprocNext == nil {
		s.InterprocNext = NewInterprocState()
	}
}

func swapProductMap[T any](
	prev *T,
	next *T,
	merge func(prev, next T) T,
	equal func(a, b T) bool,
	reset func() T,
) bool {
	if prev == nil || next == nil || merge == nil || equal == nil || reset == nil {
		return false
	}
	merged := merge(*prev, *next)
	changed := !equal(*prev, merged)
	*prev = merged
	*next = reset()
	return changed
}

func (s *SessionStore) swapInterprocFacts() []string {
	s.ensureInterprocStates()

	products := []struct {
		name string
		swap func() bool
	}{
		{
			name: "InterprocFacts",
			swap: func() bool {
				return swapProductMap(
					&s.InterprocPrev.Facts,
					&s.InterprocNext.Facts,
					interproc.WidenFactMap,
					interproc.FactMapEqual,
					func() map[api.GraphKey]api.Facts {
						return make(map[api.GraphKey]api.Facts)
					},
				)
			},
		},
	}

	diffs := make([]string, 0, len(products))
	for _, product := range products {
		if product.swap() {
			diffs = append(diffs, product.name)
		}
	}
	return diffs
}

// FixpointSwap advances the interproc product at an iteration boundary.
//
// OPERATIONS PERFORMED:
//  1. Compare the stable product with the accumulated product
//  2. Move next → prev (current results become baseline for next iteration)
//  3. Allocate fresh next maps (empty for accumulating new results)
//  4. Record which product components changed for diagnostic reporting
//
// CHANGE DETECTION: The product uses domain-owned structural equality.
//
// RETURN VALUE: Returns true if the product changed, signaling another iteration
// is needed. Returns false when the product stabilizes.
func (s *SessionStore) FixpointSwap() bool {
	diffs := s.swapInterprocFacts()

	s.syncFactInputs()

	s.lastSwapDiffs = diffs

	return len(diffs) > 0
}

// FixpointDiffs returns product components changed by the most recent swap.
func (s *SessionStore) FixpointDiffs() []string {
	if s == nil {
		return nil
	}
	if len(s.lastSwapDiffs) == 0 {
		return nil
	}
	out := make([]string, len(s.lastSwapDiffs))
	copy(out, s.lastSwapDiffs)
	return out
}

// ClearInterprocState clears all interproc product state for a fresh run.
func (s *SessionStore) ClearInterprocState() {
	if s == nil {
		return
	}
	s.InterprocPrev = NewInterprocState()
	s.InterprocNext = NewInterprocState()
	s.lastSwapDiffs = nil
	if s.factInputs != nil {
		s.factInputs.reset()
	}
}

// ModuleBindings returns the module binding table.
func (s *SessionStore) ModuleBindings() *bind.BindingTable {
	return s.Module.ModuleBindings
}

// SetModuleBindings installs the module binding table.
func (s *SessionStore) SetModuleBindings(bindings *bind.BindingTable) {
	if s == nil || s.Module == nil {
		return
	}
	s.Module.ModuleBindings = bindings
}

// Graphs returns the graph map.
func (s *SessionStore) Graphs() map[uint64]*cfg.Graph {
	return s.Module.Graphs
}

// GraphKeyFor returns the interproc graph key for a graph and parent scope.
func (s *SessionStore) GraphKeyFor(graph *cfg.Graph, parent *scope.State) (api.GraphKey, bool) {
	if s == nil || graph == nil {
		return api.GraphKey{}, false
	}
	parentHash := api.ParentHashForGraph(s, graph.ID(), parent)
	if parentHash == 0 {
		return api.GraphKey{}, false
	}
	return api.KeyForGraph(graph, parentHash), true
}

// ParentGraphKeyForSymbol returns the graph key for the parent graph that owns sym.
func (s *SessionStore) ParentGraphKeyForSymbol(sym cfg.SymbolID) (api.GraphKey, bool) {
	if s == nil || sym == 0 {
		return api.GraphKey{}, false
	}
	ref := s.FunctionRefBySym(sym)
	if ref == nil {
		return api.GraphKey{}, false
	}
	parentGraphID := ref.ParentGraphID
	if parentGraphID == 0 {
		parentGraphID = ref.GraphID
	}
	graph := s.Graphs()[parentGraphID]
	if graph == nil {
		return api.GraphKey{}, false
	}
	parentHash := s.GraphParentHashOf(parentGraphID)
	if parentHash == 0 {
		return api.GraphKey{}, false
	}
	return api.KeyForGraph(graph, parentHash), true
}

// MergeInterprocFactsNext merges a canonical fact delta into the next
// interprocedural product for the current iteration.
func (s *SessionStore) MergeInterprocFactsNext(key api.GraphKey, delta api.Facts) {
	if s == nil {
		return
	}
	s.ensureInterprocStates()
	existing := s.InterprocNext.Facts[key]
	facts := interproc.JoinFacts(existing, delta)
	if interproc.Empty(facts) {
		if interproc.Empty(existing) {
			return
		}
		delete(s.InterprocNext.Facts, key)
		s.syncFactsInput(key)
		return
	}
	if interproc.FactsEqual(existing, facts) {
		return
	}
	s.InterprocNext.Facts[key] = facts
	s.syncFactsInput(key)
}

// Funcs returns the function map.
func (s *SessionStore) Funcs() map[uint64]*ast.FunctionExpr {
	return s.Module.Funcs
}

// RegisterGraph registers a graph and its associated function for lookup.
func (s *SessionStore) RegisterGraph(graph *cfg.Graph, fn *ast.FunctionExpr) {
	if graph == nil || fn == nil {
		return
	}
	graphID := graph.ID()
	s.Module.Graphs[graphID] = graph
	s.Module.Funcs[graphID] = fn
	s.Module.GraphToFunc[graph] = fn
	s.Module.FuncToGraph[fn] = graph
}

func newFunctionRegistry() *FunctionRegistry {
	return &FunctionRegistry{
		BySym:     make(map[cfg.SymbolID]*api.FunctionRef),
		ByFunc:    make(map[*ast.FunctionExpr]*api.FunctionRef),
		ByGraphID: make(map[uint64]*api.FunctionRef),
	}
}

// RegisterFunctionRef records a canonical mapping for a function symbol.
func (s *SessionStore) RegisterFunctionRef(
	sym cfg.SymbolID,
	fn *ast.FunctionExpr,
	graph *cfg.Graph,
	parentGraphID uint64,
	defPoint cfg.Point,
) {
	if s == nil || s.Module == nil || sym == 0 || fn == nil || graph == nil {
		return
	}
	reg := s.Module.Functions
	if reg == nil {
		reg = newFunctionRegistry()
		s.Module.Functions = reg
	}
	if existing := reg.BySym[sym]; existing != nil {
		// Preserve parent graph metadata when it was already recorded.
		if existing.ParentGraphID != 0 && parentGraphID == 0 {
			parentGraphID = existing.ParentGraphID
		}
		if existing.DefPoint != 0 && defPoint == 0 {
			defPoint = existing.DefPoint
		}
	}
	ref := &api.FunctionRef{
		Sym:           sym,
		GraphID:       graph.ID(),
		ParentGraphID: parentGraphID,
		DefPoint:      defPoint,
		Func:          fn,
	}
	reg.BySym[sym] = ref
	reg.ByFunc[fn] = ref
	reg.ByGraphID[graph.ID()] = ref
}

// FunctionRefBySym returns the function ref for a symbol.
func (s *SessionStore) FunctionRefBySym(sym cfg.SymbolID) *api.FunctionRef {
	if s == nil || s.Module == nil || s.Module.Functions == nil || sym == 0 {
		return nil
	}
	return s.Module.Functions.BySym[sym]
}

// FunctionRefByFunc returns the function ref for a function literal.
func (s *SessionStore) FunctionRefByFunc(fn *ast.FunctionExpr) *api.FunctionRef {
	if s == nil || s.Module == nil || s.Module.Functions == nil || fn == nil {
		return nil
	}
	return s.Module.Functions.ByFunc[fn]
}

// FunctionRefByGraphID returns the function ref for a graph ID.
func (s *SessionStore) FunctionRefByGraphID(graphID uint64) *api.FunctionRef {
	if s == nil || s.Module == nil || s.Module.Functions == nil || graphID == 0 {
		return nil
	}
	return s.Module.Functions.ByGraphID[graphID]
}

// FuncForSymbol returns the function literal for a symbol.
func (s *SessionStore) FuncForSymbol(sym cfg.SymbolID) *ast.FunctionExpr {
	if ref := s.FunctionRefBySym(sym); ref != nil {
		return ref.Func
	}
	return nil
}

// GraphForSymbol returns the graph for a symbol.
func (s *SessionStore) GraphForSymbol(sym cfg.SymbolID) *cfg.Graph {
	if ref := s.FunctionRefBySym(sym); ref != nil {
		return s.Module.Graphs[ref.GraphID]
	}
	return nil
}

// SymbolForFunc returns the symbol for a function literal.
func (s *SessionStore) SymbolForFunc(fn *ast.FunctionExpr) (cfg.SymbolID, bool) {
	if ref := s.FunctionRefByFunc(fn); ref != nil && ref.Sym != 0 {
		return ref.Sym, true
	}
	return 0, false
}

// RegisterNestedMeta records the parent graph and definition point for a nested graph.
func (s *SessionStore) RegisterNestedMeta(childGraphID, parentGraphID uint64, defPoint cfg.Point) {
	if s == nil || s.Module == nil || childGraphID == 0 || parentGraphID == 0 {
		return
	}
	s.Module.NestedMeta[childGraphID] = api.NestedMeta{
		ParentGraphID: parentGraphID,
		DefPoint:      defPoint,
	}
}

// NestedMetaFor returns nested metadata for a graph ID.
func (s *SessionStore) NestedMetaFor(graphID uint64) (api.NestedMeta, bool) {
	if s == nil || s.Module == nil || graphID == 0 {
		return api.NestedMeta{}, false
	}
	meta, ok := s.Module.NestedMeta[graphID]
	return meta, ok
}

// SetGraphParentHash records the parent scope hash for a graph ID.
func (s *SessionStore) SetGraphParentHash(graphID, parentHash uint64) {
	if s == nil || graphID == 0 {
		return
	}
	s.GraphParentHash[graphID] = parentHash
}

// GraphParentHashOf returns the parent scope hash for a graph ID.
func (s *SessionStore) GraphParentHashOf(graphID uint64) uint64 {
	if s == nil || graphID == 0 {
		return 0
	}
	return s.GraphParentHash[graphID]
}

// FuncForGraph returns the function associated with a graph.
func (s *SessionStore) FuncForGraph(graph *cfg.Graph) *ast.FunctionExpr {
	if graph == nil || s.Module == nil {
		return nil
	}
	return s.Module.GraphToFunc[graph]
}

// GraphForFunc returns the graph associated with a function.
func (s *SessionStore) GraphForFunc(fn *ast.FunctionExpr) *cfg.Graph {
	if fn == nil || s.Module == nil {
		return nil
	}
	if graph := s.Module.FuncToGraph[fn]; graph != nil {
		return graph
	}
	ids := make([]uint64, 0, len(s.Module.Funcs))
	for id := range s.Module.Funcs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})
	for _, id := range ids {
		f := s.Module.Funcs[id]
		if f == fn {
			graph := s.Module.Graphs[id]
			if graph != nil {
				s.Module.FuncToGraph[fn] = graph
			}
			return graph
		}
	}
	return nil
}

// Parents returns the parent scope map.
func (s *SessionStore) Parents() map[uint64]*scope.State {
	return s.Module.Parents
}

// SetParentScope records a parent scope for a given hash.
func (s *SessionStore) SetParentScope(parentHash uint64, parent *scope.State) {
	if s == nil || s.Module == nil {
		return
	}
	s.Module.Parents[parentHash] = parent
}

// ModuleAliases returns the module aliases map.
func (s *SessionStore) ModuleAliases() map[cfg.SymbolID]string {
	return s.Module.ModuleAliases
}

// SetModuleAliases installs the module alias map.
func (s *SessionStore) SetModuleAliases(aliases map[cfg.SymbolID]string) {
	if s == nil || s.Module == nil {
		return
	}
	s.Module.ModuleAliases = aliases
}

// GetInterprocFacts returns the visible interproc fact product for a graph.
// Visibility is the stable product overlaid with facts already produced in the
// current iteration, giving deterministic Gauss-Seidel propagation instead of
// forcing every local refinement through a full outer iteration.
func (s *SessionStore) GetInterprocFacts(
	graph *cfg.Graph,
	parent *scope.State,
) api.Facts {
	if s == nil || graph == nil {
		return api.Facts{}
	}
	key, ok := s.GraphKeyFor(graph, parent)
	if !ok {
		return api.Facts{}
	}
	return s.interprocFactsByKey(key)
}

// GetModuleFacts returns module-wide interprocedural facts.
func (s *SessionStore) GetModuleFacts() api.Facts {
	return s.interprocFactsByKey(api.ModuleFactsKey())
}
