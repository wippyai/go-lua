package store

import (
	"sort"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/domain/interproc"
	"github.com/wippyai/go-lua/compiler/check/domain/postflow"
	"github.com/wippyai/go-lua/compiler/check/domain/trace"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

type SessionStore struct {
	// Module contains module-wide immutable state.
	// Created once at the start of checking and shared by all CFG builds.
	Module *ModuleStore

	// postflowPrev holds stable postflow projection lanes from completed iterations.
	postflowPrev *postflowProjectionState
	// postflowNext accumulates postflow projection lanes during the current iteration.
	postflowNext *postflowProjectionState

	// GraphParentHash records the parent scope hash for each graph ID.
	GraphParentHash map[uint64]uint64

	// lastSwapDiffs records lanes changed by the most recent AdvancePostflowProjections.
	// Stored per-session to avoid cross-session contamination.
	lastSwapDiffs []string

	factInputs *factInputs
	factCtx    *db.QueryContext

	analysisContexts map[api.GraphKey]api.AnalysisContext

	synthMode api.SynthMode
}

// postflowProjectionState holds one side of the noncanonical postflow projection
// loop. Each lane is stored independently so unrelated facts do not share a
// mixed convergence product.
type postflowProjectionState struct {
	functionFacts     map[api.GraphKey]api.FunctionFacts
	capturedTypes     map[api.GraphKey]postflow.CapturedTypes
	capturedFields    map[api.GraphKey]postflow.CapturedFieldAssigns
	constructorFields map[api.GraphKey]postflow.ConstructorFields
}

func newPostflowProjectionState() *postflowProjectionState {
	return &postflowProjectionState{
		functionFacts:     make(map[api.GraphKey]api.FunctionFacts),
		capturedTypes:     make(map[api.GraphKey]postflow.CapturedTypes),
		capturedFields:    make(map[api.GraphKey]postflow.CapturedFieldAssigns),
		constructorFields: make(map[api.GraphKey]postflow.ConstructorFields),
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

// NewSessionStoreWithDB creates a store whose postflow projection lanes are
// tracked as query inputs. The checker uses this form so function-result queries
// are revalidated from the exact facts they read instead of from a coarse
// iteration revision key.
func NewSessionStoreWithDB(database *db.DB) *SessionStore {
	return &SessionStore{
		Module:           NewModuleStore(),
		postflowPrev:     newPostflowProjectionState(),
		postflowNext:     newPostflowProjectionState(),
		GraphParentHash:  make(map[uint64]uint64),
		analysisContexts: make(map[api.GraphKey]api.AnalysisContext),
		factInputs:       newFactInputs(database),
		synthMode:        api.SynthModeDeclared,
	}
}

// SetSynthMode sets the current synthesis view for fact-product access checks.
func (s *SessionStore) SetSynthMode(mode api.SynthMode) {
	if s == nil {
		return
	}
	s.synthMode = mode
}

// SynthMode returns the current synthesis view.
func (s *SessionStore) SynthMode() api.SynthMode {
	if s == nil {
		return api.SynthModeDeclared
	}
	return s.synthMode
}

// WithSynthMode runs fn with a temporary synthesis view and restores the prior
// view afterward.
func (s *SessionStore) WithSynthMode(mode api.SynthMode, fn func()) {
	if fn == nil {
		return
	}
	if s == nil {
		fn()
		return
	}
	prev := s.synthMode
	s.synthMode = mode
	defer func() {
		s.synthMode = prev
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

func (s *SessionStore) ensurePostflowProjectionStates() {
	if s == nil {
		return
	}
	if s.postflowPrev == nil {
		s.postflowPrev = newPostflowProjectionState()
	}
	if s.postflowNext == nil {
		s.postflowNext = newPostflowProjectionState()
	}
	if s.postflowPrev.functionFacts == nil {
		s.postflowPrev.functionFacts = make(map[api.GraphKey]api.FunctionFacts)
	}
	if s.postflowPrev.capturedTypes == nil {
		s.postflowPrev.capturedTypes = make(map[api.GraphKey]postflow.CapturedTypes)
	}
	if s.postflowPrev.capturedFields == nil {
		s.postflowPrev.capturedFields = make(map[api.GraphKey]postflow.CapturedFieldAssigns)
	}
	if s.postflowPrev.constructorFields == nil {
		s.postflowPrev.constructorFields = make(map[api.GraphKey]postflow.ConstructorFields)
	}
	if s.postflowNext.functionFacts == nil {
		s.postflowNext.functionFacts = make(map[api.GraphKey]api.FunctionFacts)
	}
	if s.postflowNext.capturedTypes == nil {
		s.postflowNext.capturedTypes = make(map[api.GraphKey]postflow.CapturedTypes)
	}
	if s.postflowNext.capturedFields == nil {
		s.postflowNext.capturedFields = make(map[api.GraphKey]postflow.CapturedFieldAssigns)
	}
	if s.postflowNext.constructorFields == nil {
		s.postflowNext.constructorFields = make(map[api.GraphKey]postflow.ConstructorFields)
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

func (s *SessionStore) swapPostflowProjections() []string {
	s.ensurePostflowProjectionStates()

	products := []struct {
		name string
		swap func() bool
	}{
		{
			name: "FunctionFactProjection",
			swap: func() bool {
				return swapProductMap(
					&s.postflowPrev.functionFacts,
					&s.postflowNext.functionFacts,
					interproc.WidenFunctionFactMaps,
					interproc.FunctionFactMapsEqual,
					func() map[api.GraphKey]api.FunctionFacts {
						return make(map[api.GraphKey]api.FunctionFacts)
					},
				)
			},
		},
		{
			name: "CapturedTypeProjection",
			swap: func() bool {
				return swapProductMap(
					&s.postflowPrev.capturedTypes,
					&s.postflowNext.capturedTypes,
					interproc.WidenCapturedTypeMaps,
					interproc.CapturedTypeMapsEqual,
					func() map[api.GraphKey]postflow.CapturedTypes {
						return make(map[api.GraphKey]postflow.CapturedTypes)
					},
				)
			},
		},
		{
			name: "CapturedFieldProjection",
			swap: func() bool {
				return swapProductMap(
					&s.postflowPrev.capturedFields,
					&s.postflowNext.capturedFields,
					interproc.WidenCapturedFieldAssignMaps,
					interproc.CapturedFieldAssignMapsEqual,
					func() map[api.GraphKey]postflow.CapturedFieldAssigns {
						return make(map[api.GraphKey]postflow.CapturedFieldAssigns)
					},
				)
			},
		},
		{
			name: "ConstructorFieldProjection",
			swap: func() bool {
				return swapProductMap(
					&s.postflowPrev.constructorFields,
					&s.postflowNext.constructorFields,
					interproc.WidenConstructorFieldMaps,
					interproc.ConstructorFieldMapsEqual,
					func() map[api.GraphKey]postflow.ConstructorFields {
						return make(map[api.GraphKey]postflow.ConstructorFields)
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

// AdvancePostflowProjections advances postflow projection lanes at an iteration boundary.
//
// OPERATIONS PERFORMED:
//  1. Compare each stable lane with its accumulated lane
//  2. Move next into prev (current results become baseline for next iteration)
//  3. Allocate fresh next maps (empty for accumulating new results)
//  4. Record which lanes changed for diagnostic reporting
//
// CHANGE DETECTION: Each lane uses domain-owned structural equality.
//
// RETURN VALUE: Returns true if the product changed, signaling another iteration
// is needed. Returns false when the product stabilizes.
func (s *SessionStore) AdvancePostflowProjections() bool {
	diffs := s.swapPostflowProjections()

	s.syncFactInputs()

	s.lastSwapDiffs = diffs

	return len(diffs) > 0
}

// PostflowProjectionDiffs returns lanes changed by the most recent swap.
func (s *SessionStore) PostflowProjectionDiffs() []string {
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

// ClearPostflowProjectionState clears all postflow projection lane state for a fresh run.
func (s *SessionStore) ClearPostflowProjectionState() {
	if s == nil {
		return
	}
	s.postflowPrev = newPostflowProjectionState()
	s.postflowNext = newPostflowProjectionState()
	s.lastSwapDiffs = nil
	if s.factInputs != nil {
		s.factInputs.reset()
	}
	clear(s.analysisContexts)
}

// PostflowProjectionStateInitialized reports whether the postflow projection owner is
// initialized. It exposes only store ownership health, not the lane maps.
func (s *SessionStore) PostflowProjectionStateInitialized() bool {
	return s != nil &&
		postflowProjectionStateInitialized(s.postflowPrev) &&
		postflowProjectionStateInitialized(s.postflowNext)
}

func postflowProjectionStateInitialized(state *postflowProjectionState) bool {
	return state != nil &&
		state.functionFacts != nil &&
		state.capturedTypes != nil &&
		state.capturedFields != nil &&
		state.constructorFields != nil
}

// PostflowProjectionCounts reports postflow projection-lane occupancy for tests and
// compatibility assertions without exposing the lane maps.
func (s *SessionStore) PostflowProjectionCounts() (prev int, next int) {
	if s == nil {
		return 0, 0
	}
	if s.postflowPrev != nil {
		prev = postflowProjectionStateCount(s.postflowPrev)
	}
	if s.postflowNext != nil {
		next = postflowProjectionStateCount(s.postflowNext)
	}
	return prev, next
}

func postflowProjectionStateCount(state *postflowProjectionState) int {
	if state == nil {
		return 0
	}
	return len(state.functionFacts) +
		len(state.capturedTypes) +
		len(state.capturedFields) +
		len(state.constructorFields)
}

// ProjectionFunctionRefinementsForExport returns the final refinement projection
// from the converged function-fact lane. Export code reads this projection instead of
// peeking into projection state.
func (s *SessionStore) ProjectionFunctionRefinementsForExport() map[cfg.SymbolID]*constraint.FunctionRefinement {
	if s == nil || s.postflowPrev == nil || len(s.postflowPrev.functionFacts) == 0 {
		return nil
	}
	refinements := make(map[cfg.SymbolID]*constraint.FunctionRefinement)
	for _, key := range api.SortedGraphKeys(s.postflowPrev.functionFacts) {
		facts := s.postflowPrev.functionFacts[key]
		for _, sym := range cfg.SortedSymbolIDs(facts) {
			if refinement := functionfact.FactsProjection(facts).Refinement(sym); refinement != nil {
				refinements[sym] = refinement
			}
		}
	}
	if len(refinements) == 0 {
		return nil
	}
	return refinements
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

// MergeFunctionFactProjection merges one function-fact projection row.
func (s *SessionStore) MergeFunctionFactProjection(key api.GraphKey, sym cfg.SymbolID, fact api.FunctionFact) {
	if sym == 0 || functionfact.Empty(fact) {
		return
	}
	s.ensurePostflowProjectionStates()
	existing := s.postflowNext.functionFacts[key]
	facts := interproc.JoinFunctionFacts(existing, api.FunctionFacts{sym: fact})
	if len(facts) == 0 {
		if len(existing) == 0 {
			return
		}
		delete(s.postflowNext.functionFacts, key)
		return
	}
	if interproc.FunctionFactsEqual(existing, facts) {
		return
	}
	s.postflowNext.functionFacts[key] = facts
}

// MergeCapturedTypeProjection merges one captured-symbol type row.
func (s *SessionStore) MergeCapturedTypeProjection(key api.GraphKey, sym cfg.SymbolID, value product.AbstractValue) {
	if sym == 0 || value.IsZero() {
		return
	}
	s.ensurePostflowProjectionStates()
	existing := s.postflowNext.capturedTypes[key]
	types := interproc.JoinCapturedTypes(existing, postflow.CapturedTypes{sym: value})
	if len(types) == 0 {
		if len(existing) == 0 {
			return
		}
		delete(s.postflowNext.capturedTypes, key)
		return
	}
	if interproc.CapturedTypesEqual(existing, types) {
		return
	}
	s.postflowNext.capturedTypes[key] = types
}

// MergeCapturedFieldProjection merges field writes performed by one nested
// function against one captured parent symbol.
func (s *SessionStore) MergeCapturedFieldProjection(key api.GraphKey, nestedSym cfg.SymbolID, capturedSym cfg.SymbolID, fields postflow.FieldValues) {
	if nestedSym == 0 || capturedSym == 0 || len(fields) == 0 {
		return
	}
	s.ensurePostflowProjectionStates()
	existing := s.postflowNext.capturedFields[key]
	next := postflow.CapturedFieldAssigns{nestedSym: {capturedSym: fields}}
	assigns := interproc.JoinCapturedFieldAssigns(existing, next)
	if len(assigns) == 0 {
		if len(existing) == 0 {
			return
		}
		delete(s.postflowNext.capturedFields, key)
		return
	}
	if interproc.CapturedFieldAssignsEqual(existing, assigns) {
		return
	}
	s.postflowNext.capturedFields[key] = assigns
}

// MergeConstructorFieldProjection merges constructor field evidence into the
// module-wide projection lane.
func (s *SessionStore) MergeConstructorFieldProjection(classSym cfg.SymbolID, fields postflow.FieldValues) {
	if classSym == 0 || len(fields) == 0 {
		return
	}
	key := api.ModuleFactsKey()
	s.ensurePostflowProjectionStates()
	existing := s.postflowNext.constructorFields[key]
	next := postflow.ConstructorFields{classSym: fields}
	constructors := interproc.JoinConstructorFields(existing, next)
	if len(constructors) == 0 {
		if len(existing) == 0 {
			return
		}
		delete(s.postflowNext.constructorFields, key)
		return
	}
	if interproc.ConstructorFieldsEqual(existing, constructors) {
		return
	}
	s.postflowNext.constructorFields[key] = constructors
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

// CapturedTypeProjection returns the visible captured-symbol type projection for
// a graph key.
func (s *SessionStore) CapturedTypeProjection(graph *cfg.Graph, parent *scope.State, sym cfg.SymbolID) (typ.Type, bool) {
	if s == nil || graph == nil || sym == 0 {
		return nil, false
	}
	key, ok := s.GraphKeyFor(graph, parent)
	if !ok {
		return nil, false
	}
	return s.capturedTypeByKey(api.CapturedTypeKey{GraphKey: key, Symbol: sym})
}

// CapturedFieldAssignsProjection returns the visible captured-field projection
// for a graph key.
func (s *SessionStore) CapturedFieldAssignsProjection(graph *cfg.Graph, parent *scope.State) postflow.CapturedFieldAssigns {
	if s == nil || graph == nil {
		return nil
	}
	key, ok := s.GraphKeyFor(graph, parent)
	if !ok {
		return nil
	}
	return s.capturedFieldAssignsByKey(key)
}

// ConstructorFieldsProjection returns visible constructor field evidence for a
// class symbol from the module-wide projection lane.
func (s *SessionStore) ConstructorFieldsProjection(classSym cfg.SymbolID) (postflow.FieldValues, bool) {
	if s == nil || classSym == 0 {
		return nil, false
	}
	return s.constructorFieldsByKey(api.ConstructorFieldKey{GraphKey: api.ModuleFactsKey(), Symbol: classSym})
}

// FunctionFactProjection returns the final/public FunctionFact projection for
// one symbol. This uses the keyed projection cell so normal checker paths do
// not clone or scan the graph's whole FunctionFacts map.
func (s *SessionStore) FunctionFactProjection(graph *cfg.Graph, parent *scope.State, sym cfg.SymbolID) (api.FunctionFact, bool) {
	if s == nil || graph == nil || sym == 0 {
		return api.FunctionFact{}, false
	}
	key, ok := s.GraphKeyFor(graph, parent)
	if !ok {
		return api.FunctionFact{}, false
	}
	return s.functionFactByKey(api.FunctionFactKey{GraphKey: key, Symbol: sym})
}

// FunctionFactsProjectionForExport returns final/public FunctionFacts for a
// graph as a whole map. This bulk projection is for export/debug surfaces; hot
// symbol lookups should use FunctionFactProjection.
func (s *SessionStore) FunctionFactsProjectionForExport(graph *cfg.Graph, parent *scope.State) api.FunctionFacts {
	if s == nil || graph == nil {
		return nil
	}
	key, ok := s.GraphKeyFor(graph, parent)
	if !ok {
		return nil
	}
	return s.functionFactsByKey(key)
}
