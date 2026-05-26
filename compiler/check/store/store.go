package store

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/trace"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/interproc"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/typ"
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

	analysisContexts map[api.GraphKey]api.AnalysisContext

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
	// Evidence maps graph IDs to canonical abstract-interpreter event evidence.
	Evidence map[uint64]api.FlowEvidence
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
		Module:           NewModuleStore(),
		InterprocPrev:    NewInterprocState(),
		InterprocNext:    NewInterprocState(),
		GraphParentHash:  make(map[uint64]uint64),
		analysisContexts: make(map[api.GraphKey]api.AnalysisContext),
		factInputs:       newFactInputs(database),
		phase:            api.PhaseScopeCompute,
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
		Evidence:      make(map[uint64]api.FlowEvidence),
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
	clear(s.analysisContexts)
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

// EvidenceForGraph returns the canonical abstract-interpreter evidence for graph.
func (s *SessionStore) EvidenceForGraph(graph *cfg.Graph) api.FlowEvidence {
	if graph == nil {
		return api.FlowEvidence{}
	}
	if s == nil || s.Module == nil {
		return trace.GraphEvidence(graph, graph.Bindings())
	}
	if s.Module.Evidence == nil {
		s.Module.Evidence = make(map[uint64]api.FlowEvidence)
	}
	graphID := graph.ID()
	if evidence, ok := s.Module.Evidence[graphID]; ok {
		return evidence
	}
	bindings := graph.Bindings()
	if bindings == nil {
		bindings = s.Module.ModuleBindings
	}
	evidence := trace.GraphEvidence(graph, bindings)
	s.Module.Evidence[graphID] = evidence
	return evidence
}

// SetEvidenceForGraph records canonical abstract-interpreter evidence for graph.
func (s *SessionStore) SetEvidenceForGraph(graph *cfg.Graph, evidence api.FlowEvidence) {
	if s == nil || s.Module == nil || graph == nil {
		return
	}
	if s.Module.Evidence == nil {
		s.Module.Evidence = make(map[uint64]api.FlowEvidence)
	}
	s.Module.Evidence[graph.ID()] = evidence
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
		return
	}
	if interproc.FactsEqual(existing, facts) {
		return
	}
	s.InterprocNext.Facts[key] = facts
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

// FunctionRefsByParentGraph returns function refs declared directly in a parent graph.
func (s *SessionStore) FunctionRefsByParentGraph(parentGraphID uint64) []api.FunctionRef {
	if s == nil || s.Module == nil || s.Module.Functions == nil || parentGraphID == 0 {
		return nil
	}
	refs := make([]api.FunctionRef, 0)
	for _, ref := range s.Module.Functions.BySym {
		if ref == nil || ref.ParentGraphID != parentGraphID || ref.Sym == 0 {
			continue
		}
		refs = append(refs, *ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].DefPoint != refs[j].DefPoint {
			return refs[i].DefPoint < refs[j].DefPoint
		}
		return refs[i].Sym < refs[j].Sym
	})
	return refs
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

// SetGraphAnalysisContext records execution context for a graph analysis key.
func (s *SessionStore) SetGraphAnalysisContext(key api.GraphKey, ctx api.AnalysisContext) {
	if s == nil || key.GraphID == 0 || ctx.Empty() {
		return
	}
	if s.analysisContexts == nil {
		s.analysisContexts = make(map[api.GraphKey]api.AnalysisContext)
	}
	s.analysisContexts[key] = api.MergeAnalysisContext(s.analysisContexts[key], ctx)
}

// GraphAnalysisContext returns the execution context for a graph analysis key.
func (s *SessionStore) GraphAnalysisContext(key api.GraphKey) api.AnalysisContext {
	if s == nil || len(s.analysisContexts) == 0 {
		return api.AnalysisContext{}
	}
	return api.MergeAnalysisContext(api.AnalysisContext{}, s.analysisContexts[key])
}

type interprocProductView struct {
	store *SessionStore
	key   api.GraphKey
	ok    bool
}

func (v interprocProductView) FunctionFacts() api.FunctionFacts {
	if v.store == nil || !v.ok {
		return nil
	}
	return v.store.functionFactsByKey(v.key)
}

func (v interprocProductView) FunctionFact(sym cfg.SymbolID) (api.FunctionFact, bool) {
	if v.store == nil || !v.ok || sym == 0 {
		return api.FunctionFact{}, false
	}
	return v.store.functionFactByKey(api.FunctionFactKey{GraphKey: v.key, Symbol: sym})
}

func (v interprocProductView) LiteralSig(fn *ast.FunctionExpr) (*typ.Function, bool) {
	if v.store == nil || !v.ok || fn == nil {
		return nil, false
	}
	return v.store.literalSigByKey(api.LiteralSigKey{GraphKey: v.key, Func: fn})
}

func (v interprocProductView) CapturedType(sym cfg.SymbolID) (typ.Type, bool) {
	if v.store == nil || !v.ok || sym == 0 {
		return nil, false
	}
	return v.store.capturedTypeByKey(api.CapturedTypeKey{GraphKey: v.key, Symbol: sym})
}

func (v interprocProductView) CapturedFieldAssigns() api.CapturedFieldAssigns {
	if v.store == nil || !v.ok {
		return nil
	}
	return v.store.capturedFieldAssignsByKey(v.key)
}

func (v interprocProductView) CapturedContainerMutations() api.CapturedContainerMutations {
	if v.store == nil || !v.ok {
		return nil
	}
	return v.store.capturedContainerMutationsByKey(v.key)
}

func (v interprocProductView) ConstructorFields(classSym cfg.SymbolID) (map[string]typ.Type, bool) {
	if v.store == nil || !v.ok || classSym == 0 {
		return nil, false
	}
	return v.store.constructorFieldsByKey(api.ConstructorFieldKey{GraphKey: v.key, Symbol: classSym})
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

// ModuleFacts returns the module-wide visible interproc product view.
func (s *SessionStore) ModuleFacts() api.InterprocFactProduct {
	if s == nil {
		return interprocProductView{}
	}
	return interprocProductView{store: s, key: api.ModuleFactsKey(), ok: true}
}

// InterprocFacts returns the visible interproc product view for a graph.
func (s *SessionStore) InterprocFacts(graph *cfg.Graph, parent *scope.State) api.InterprocFactProduct {
	if s == nil || graph == nil {
		return interprocProductView{}
	}
	key, ok := s.GraphKeyFor(graph, parent)
	return interprocProductView{store: s, key: key, ok: ok}
}
