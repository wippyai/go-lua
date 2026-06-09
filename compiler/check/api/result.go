package api

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	"github.com/wippyai/go-lua/compiler/check/domain/globalenv"
	"github.com/wippyai/go-lua/compiler/check/scope"
	flowcfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// FuncResult is the public read model for one analyzed function.
// It is pure data with no back-references to Session or other FuncResults,
// enabling safe memoization and independent storage. Canonical checking treats
// it as output projection, not as a semantic input to the solve.
//
// The result captures the checker flow's analysis outputs: graph and scope
// facts, immutable extraction evidence, product-state facts, and the solved-flow
// projection diagnostics consume.
type FuncResult struct {
	// Graph is the function's control flow graph containing CFG nodes,
	// binding information, and iteration metadata.
	Graph *cfg.Graph

	// ModuleBindings is the module-level binding table used when graph-local
	// bindings are insufficient for canonical symbol resolution.
	ModuleBindings *bind.BindingTable

	// AnalysisContext is the analysis-sensitive dynamic context for this
	// function key, including callback-scoped globals and expected signatures.
	AnalysisContext AnalysisContext

	// BaseScope is the function's entry scope containing parameters,
	// type definitions, and inherited context from the parent scope.
	BaseScope *scope.State

	// Scopes maps each CFG point to its effective scope state.
	// Includes declared types, narrowed types, and lexical bindings.
	Scopes map[cfg.Point]*scope.State

	// analysis holds construction-only solver artifacts. Consumers must use the
	// named read methods below instead of treating FuncResult as an engine handle.
	analysis FuncAnalysisArtifacts

	// Evidence records events discovered during abstract interpretation.
	Evidence FlowEvidence

	// CallExpectedArgs stores solved/contextual expected argument types,
	// index-aligned with Evidence.Calls. These are used to seed contextual
	// analysis of callback literals and are deliberately separate from diagnostic
	// call-edge obligations.
	CallExpectedArgs []CallExpectedArgEvidence

	// CallContracts stores solved call-edge argument obligations, index-aligned with
	// Evidence.Calls. These are fixed-point projections (for example callee
	// Summary.Params), not extraction evidence; keeping them separate preserves the
	// immutable FlowEvidence carrier.
	CallContracts []CallContractEvidence

	// FnRefinement captures the function's inferred refinement summary.
	// It includes propagated effect rows and branch-specific narrowing facts.
	FnRefinement *constraint.FunctionRefinement

	// ReturnRelations captures caller-visible return-slot relations proven by the
	// canonical product summary, such as Lua's `(value, err)` inverse convention.
	ReturnRelations flow.ReturnRelations

	// SourceSignature is the annotation-only function signature resolved during
	// analysis.
	SourceSignature *typ.Function

	// ResolvedTypeDefs are module-local type definitions resolved against this
	// function's canonical observation surface. Module export reads these instead
	// of rebuilding typedefs from early scopes, so typeof(...) aliases see the same
	// declared and solved facts diagnostics use.
	ResolvedTypeDefs map[string]typ.Type

	// PublicSeedSignature is the source-declared public seed used when
	// canonicalizing post-flow FunctionFact signatures.
	PublicSeedSignature *typ.Function

	// QueryContext is the immutable query context used by pure type operations.
	QueryContext *db.QueryContext

	// TypeOps is the canonical type-operation surface for solved-state reads.
	TypeOps core.TypeOps

	// GlobalTypes is the immutable value namespace visible to this function,
	// including callback/global overlays from its analysis context.
	GlobalTypes map[string]typ.Type

	// GlobalTypeBindings is the normalized source-global type overlay visible to
	// this function. In-process consumers should use GlobalTypeOverlay(); the map
	// field above remains the public external projection.
	GlobalTypeBindings globalenv.TypeOverlay

	// LiteralSignatures holds synthesized signatures for function literals in this graph.
	LiteralSignatures map[*ast.FunctionExpr]*typ.Function

	// LiteralSignatureProvider is the normalized lookup surface for function
	// literal signatures. In-process consumers should use LiteralSignatureLookup();
	// the map above remains the public external projection.
	LiteralSignatureProvider LiteralSignatureLookup

	// Extras stores results from registered ComputePass plugins.
	// Keyed by ComputePass.Name().
	Extras map[string]any

	// DepthLimitExceeded indicates scope depth limit was hit while building scopes.
	DepthLimitExceeded bool

	// RecursiveFamilies is the compilation-scoped recursive-family interner used
	// to seal class metatables into shared interned families. Solved-state
	// observation reuses it so a constructor's observed metatable resolves to the
	// same converging family the synthesis engine seals.
	RecursiveFamilies *typ.RecursiveFamilyInterner

	// ClassFamilyJoin is the function-aware body lattice join the class-family
	// seal widens with. It is supplied by the pipeline so observation can seal a
	// metatable into the shared family without importing the producing package.
	ClassFamilyJoin func(existing, candidate typ.Type) typ.Type
}

// CallExpectedArgEvidence is the solved contextual expected-type vector for one
// FlowEvidence.Calls entry. It is not a diagnostic contract; it gives function
// literal arguments their parameter context.
type CallExpectedArgEvidence struct {
	Args []typ.Type
}

// NewCallExpectedArgEvidence stores a cloned call expected-argument vector.
func NewCallExpectedArgEvidence(args []typ.Type) CallExpectedArgEvidence {
	if len(args) == 0 {
		return CallExpectedArgEvidence{}
	}
	out := make([]typ.Type, len(args))
	copy(out, args)
	return CallExpectedArgEvidence{Args: out}
}

// ArgType returns the contextual expected type for argument idx.
func (e CallExpectedArgEvidence) ArgType(idx int) typ.Type {
	if idx < 0 || idx >= len(e.Args) {
		return nil
	}
	return e.Args[idx]
}

// CallContractEvidence is the solved call-edge contract vector for one
// FlowEvidence.Calls entry. ExpectedArgs remain call-signature contextual
// synthesis input; Args are post-fixpoint obligations consumed at diagnostics.
type CallContractEvidence struct {
	Args []callobligation.Obligation
}

// NewCallContractEvidence stores a cloned call-edge contract vector.
func NewCallContractEvidence(args []callobligation.Obligation) CallContractEvidence {
	if len(args) == 0 {
		return CallContractEvidence{}
	}
	out := make([]callobligation.Obligation, len(args))
	copy(out, args)
	return CallContractEvidence{Args: out}
}

// ArgObligation returns the solved call-edge obligation for argument idx.
func (e CallContractEvidence) ArgObligation(idx int) callobligation.Obligation {
	if idx < 0 || idx >= len(e.Args) {
		return callobligation.Obligation{}
	}
	return e.Args[idx]
}

// ArgType returns the solved call-edge contract for argument idx.
func (e CallContractEvidence) ArgType(idx int) typ.Type {
	return e.ArgObligation(idx).Type
}

// FuncAnalysisArtifacts is the construction-only solved-analysis payload for a
// FuncResult. It is installed by result projectors and exposed to consumers only
// through named read/view methods on FuncResult.
type FuncAnalysisArtifacts struct {
	TypeFacts      flow.TypeFacts
	FlowInputs     *flow.Inputs
	FlowProjection FlowOps
	SolvedSynth    Synth
}

// InstallAnalysisArtifacts installs solved-analysis artifacts while constructing
// a FuncResult. Normal consumers should use TypeFacts, SolvedFlow,
// FlowInputView, SolvedSynth, or ObservationState.
func (r *FuncResult) InstallAnalysisArtifacts(artifacts FuncAnalysisArtifacts) {
	if r == nil {
		return
	}
	r.analysis = artifacts
}

// InstallSolvedSynth installs the solved synthesis view during result
// construction after observation context initialization.
func (r *FuncResult) InstallSolvedSynth(synth Synth) {
	if r == nil {
		return
	}
	r.analysis.SolvedSynth = synth
}

// EffectiveTypeAt returns the narrowed type for a symbol at a specific CFG point.
// The effective type reflects flow-based narrowing (type guards, nil checks, etc.)
// applied to the symbol's declared type based on control flow leading to point p.
//
// Returns TypedValue with:
//   - Type: The narrowed type at point p
//   - State: StateResolved if type is known, StateUnknown otherwise
//
// Delegates to TypeFacts().EffectiveTypeAt when available, otherwise returns unknown.
func (r *FuncResult) EffectiveTypeAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	if r == nil || r.analysis.TypeFacts == nil {
		return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
	}
	return r.analysis.TypeFacts.EffectiveTypeAt(p, sym)
}

// DeclaredTypeAt returns source/static declaration facts for a symbol.
func (r *FuncResult) DeclaredTypeAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	if r == nil || r.analysis.TypeFacts == nil {
		return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
	}
	return r.analysis.TypeFacts.DeclaredAt(p, sym)
}

// TypeFacts returns the producer-neutral solved type facts for this result.
func (r *FuncResult) TypeFacts() flow.TypeFacts {
	if r == nil {
		return nil
	}
	return r.analysis.TypeFacts
}

// SolvedSynth returns the flow-refined synthesis view for this result.
func (r *FuncResult) SolvedSynth() Synth {
	if r == nil {
		return nil
	}
	return r.analysis.SolvedSynth
}

// SolvedTypeOf observes an expression through the solved synthesis view.
func (r *FuncResult) SolvedTypeOf(expr ast.Expr, p cfg.Point) typ.Type {
	synth := r.SolvedSynth()
	if synth == nil {
		return nil
	}
	return synth.TypeOf(expr, p)
}

// SolvedFlow returns the normalized solved-flow projection for this result.
func (r *FuncResult) SolvedFlow() FlowOps {
	if r == nil {
		return nil
	}
	if r.analysis.FlowProjection != nil {
		return r.analysis.FlowProjection
	}
	return nil
}

// HasSolvedFlowProjection reports whether the result carries a concrete
// producer-neutral flow projection.
func (r *FuncResult) HasSolvedFlowProjection() bool {
	return r != nil && r.analysis.FlowProjection != nil
}

// FlowInputView is a read-only facade over extracted flow inputs. It keeps
// consumers from treating FuncResult as an internal engine handle.
type FlowInputView struct {
	inputs *flow.Inputs
}

// Present reports whether extracted flow inputs are available.
func (v FlowInputView) Present() bool {
	return v.inputs != nil
}

// DeclaredTypes returns source declaration facts extracted for flow.
func (v FlowInputView) DeclaredTypes() flow.DeclaredTypes {
	if v.inputs == nil {
		return nil
	}
	return v.inputs.DeclaredTypes
}

// BindingTypes returns immutable value-binding type facts extracted for flow.
func (v FlowInputView) BindingTypes() map[cfg.SymbolID]typ.Type {
	if v.inputs == nil {
		return nil
	}
	return v.inputs.BindingTypes
}

// Assignments returns normalized assignment evidence extracted for flow.
func (v FlowInputView) Assignments() []flow.UnifiedAssignment {
	if v.inputs == nil {
		return nil
	}
	return v.inputs.Assignments
}

// LoopInsertLengths returns flow-extracted length evidence from counted loops.
func (v FlowInputView) LoopInsertLengths() []flow.LoopInsertLength {
	if v.inputs == nil {
		return nil
	}
	return v.inputs.LoopInsertLengths
}

// EdgeConditions returns flow-extracted edge condition evidence.
func (v FlowInputView) EdgeConditions() []flow.EdgeCondition {
	if v.inputs == nil {
		return nil
	}
	return v.inputs.EdgeConditions
}

// Graph returns the versioned graph carried by extracted flow inputs.
func (v FlowInputView) Graph() flowcfg.VersionedGraph {
	if v.inputs == nil {
		return nil
	}
	return v.inputs.Graph
}

// IsPointDead reports whether flow extraction marked point unreachable.
func (v FlowInputView) IsPointDead(p cfg.Point) bool {
	return v.inputs != nil && v.inputs.DeadPoints[p]
}

// FlowInputView returns the read-only extracted-input view for this result.
func (r *FuncResult) FlowInputView() FlowInputView {
	if r == nil {
		return FlowInputView{}
	}
	return FlowInputView{inputs: r.analysis.FlowInputs}
}

// ConditionProofFacts returns the producer-neutral condition-proof surface for
// this result. Consumers use this instead of depending on concrete flow state.
func (r *FuncResult) ConditionProofFacts() flow.ConditionProofFacts {
	if r == nil {
		return nil
	}
	if facts, ok := r.analysis.TypeFacts.(flow.ConditionProofFacts); ok {
		return facts
	}
	if facts, ok := r.SolvedFlow().(flow.ConditionProofFacts); ok {
		return facts
	}
	return nil
}

// ConstFacts returns immutable constant facts for this result.
func (r *FuncResult) ConstFacts() flow.ConstFacts {
	if r == nil {
		return nil
	}
	if facts, ok := r.analysis.TypeFacts.(flow.ConstFacts); ok {
		return facts
	}
	if facts, ok := r.SolvedFlow().(flow.ConstFacts); ok {
		return facts
	}
	if r.analysis.FlowInputs != nil {
		return r.analysis.FlowInputs
	}
	return nil
}

// PathObservationFacts returns the high-level path-observation surface for this
// result when the producer exposes one.
func (r *FuncResult) PathObservationFacts() flow.PathObservationFacts {
	if r == nil {
		return nil
	}
	if facts, ok := r.analysis.TypeFacts.(flow.PathObservationFacts); ok {
		return facts
	}
	if facts, ok := r.SolvedFlow().(flow.PathObservationFacts); ok {
		return facts
	}
	return nil
}

// PathChildFacts returns the producer-neutral finite child path surface for this
// result when the producer exposes one.
func (r *FuncResult) PathChildFacts() flow.PathChildFacts {
	if r == nil {
		return nil
	}
	if facts, ok := r.analysis.TypeFacts.(flow.PathChildFacts); ok {
		return facts
	}
	if facts, ok := r.SolvedFlow().(flow.PathChildFacts); ok {
		return facts
	}
	return nil
}

// TransferValueFacts returns the producer-neutral transfer-value surface for
// this result when the producer exposes one.
func (r *FuncResult) TransferValueFacts() flow.TransferValueFacts {
	if r == nil {
		return nil
	}
	if facts, ok := r.analysis.TypeFacts.(flow.TransferValueFacts); ok {
		return facts
	}
	if facts, ok := r.SolvedFlow().(flow.TransferValueFacts); ok {
		return facts
	}
	return nil
}

// NarrowedTypeAt returns the precise path-sensitive narrowed type at a CFG point.
func (r *FuncResult) NarrowedTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	flowOps := r.SolvedFlow()
	if flowOps == nil {
		return nil
	}
	return flowOps.NarrowedTypeAt(p, path)
}

// PreStateTypeAt returns the path-sensitive type before point-local transfer effects.
func (r *FuncResult) PreStateTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	flowOps := r.SolvedFlow()
	if flowOps == nil {
		return nil
	}
	return flowOps.PreStateTypeAt(p, path)
}

// ExcludesTypeAt checks if the flow solution proves a type is excluded at a CFG point.
// Used for type narrowing when control flow eliminates certain type possibilities.
//
// For example, after "if x ~= nil then", the nil type is excluded from x's type
// in the then-branch. Similarly, "if type(x) == 'string'" excludes non-string types.
//
// Parameters:
//   - p: CFG point where exclusion is checked
//   - path: Symbol path (may include field accesses)
//   - declared: The declared type being queried
//
// Returns true if flow analysis proves the declared type cannot occur at point p.
func (r *FuncResult) ExcludesTypeAt(p cfg.Point, path constraint.Path, declared typ.Type) bool {
	flowOps := r.SolvedFlow()
	if flowOps == nil {
		return false
	}
	return flowOps.ExcludesTypeAt(p, path, declared)
}

// GlobalTypeOverlay returns the normalized source-global type overlay for this
// result. If only the external map projection is populated, it is normalized here.
func (r *FuncResult) GlobalTypeOverlay() globalenv.TypeOverlay {
	if r == nil {
		return nil
	}
	if len(r.GlobalTypeBindings) != 0 {
		return r.GlobalTypeBindings.Clone()
	}
	return globalenv.TypeOverlayFromMap(r.GlobalTypes)
}

// LiteralSignatureLookup returns the normalized function-literal signature
// lookup surface for this result. If only the external map projection is populated,
// it is normalized here.
func (r *FuncResult) LiteralSignatureLookup() LiteralSignatureLookup {
	if r == nil {
		return nil
	}
	if r.LiteralSignatureProvider != nil {
		return r.LiteralSignatureProvider
	}
	if len(r.LiteralSignatures) == 0 {
		return nil
	}
	return LiteralSignatureLookupFromMap(r.LiteralSignatures)
}

// FuncAnalysisView is the stable slice of a function analysis result
// required by nested processing and interprocedural helpers.
type FuncAnalysisView struct {
	Graph                    *cfg.Graph
	AnalysisContext          AnalysisContext
	Scopes                   map[cfg.Point]*scope.State
	Facts                    flow.TypeFacts
	FlowInputs               *flow.Inputs
	FlowProjection           FlowOps
	Evidence                 FlowEvidence
	CallExpectedArgs         []CallExpectedArgEvidence
	CallContracts            []CallContractEvidence
	NarrowSynth              Synth
	SourceSignature          *typ.Function
	PublicSeedSignature      *typ.Function
	LiteralSignatures        map[*ast.FunctionExpr]*typ.Function
	LiteralSignatureProvider LiteralSignatureLookup
	QueryContext             *db.QueryContext
	TypeOps                  core.TypeOps
	GlobalTypes              map[string]typ.Type
	GlobalTypeBindings       globalenv.TypeOverlay
	RecursiveFamilies        *typ.RecursiveFamilyInterner
	ClassFamilyJoin          func(existing, candidate typ.Type) typ.Type
}

// SolvedObservationState is the producer-neutral solved-state bundle consumed
// by expression observation. FuncResult and FuncAnalysisView normalize into this
// shape so observation does not duplicate carrier-specific field plumbing.
type SolvedObservationState struct {
	Graph                    *cfg.Graph
	Bindings                 *bind.BindingTable
	Scopes                   map[cfg.Point]*scope.State
	DefaultScope             *scope.State
	Facts                    flow.TypeFacts
	Inputs                   *flow.Inputs
	Flow                     FlowOps
	Ctx                      *db.QueryContext
	TypeOps                  core.TypeOps
	LiteralSignatureProvider LiteralSignatureLookup
	GlobalTypeOverlay        globalenv.TypeOverlay
	RecursiveFamilies        *typ.RecursiveFamilyInterner
	ClassFamilyJoin          func(existing, candidate typ.Type) typ.Type
	ResolveType              func(ast.TypeExpr, *scope.State) typ.Type
}

// ObservationState returns the normalized solved-state bundle for expression
// observation from a complete function result.
func (r *FuncResult) ObservationState() SolvedObservationState {
	if r == nil {
		return SolvedObservationState{}
	}
	bindings := r.ModuleBindings
	if r.Graph != nil && r.Graph.Bindings() != nil {
		bindings = r.Graph.Bindings()
	}
	state := SolvedObservationState{
		Graph:                    r.Graph,
		Bindings:                 bindings,
		Scopes:                   r.Scopes,
		DefaultScope:             r.BaseScope,
		Facts:                    r.analysis.TypeFacts,
		Inputs:                   r.analysis.FlowInputs,
		Flow:                     r.SolvedFlow(),
		Ctx:                      r.QueryContext,
		TypeOps:                  r.TypeOps,
		LiteralSignatureProvider: r.LiteralSignatureLookup(),
		GlobalTypeOverlay:        r.GlobalTypeOverlay(),
		RecursiveFamilies:        r.RecursiveFamilies,
		ClassFamilyJoin:          r.ClassFamilyJoin,
	}
	if r.analysis.SolvedSynth != nil {
		state.ResolveType = r.analysis.SolvedSynth.ResolveType
	}
	return state
}

// ObservationState returns the normalized solved-state bundle for expression
// observation from a stable function-analysis view.
func (r *FuncAnalysisView) ObservationState() SolvedObservationState {
	if r == nil {
		return SolvedObservationState{}
	}
	state := SolvedObservationState{
		Graph:                    r.Graph,
		Scopes:                   r.Scopes,
		Facts:                    r.Facts,
		Inputs:                   r.FlowInputs,
		Flow:                     r.SolvedFlow(),
		Ctx:                      r.QueryContext,
		TypeOps:                  r.TypeOps,
		LiteralSignatureProvider: r.LiteralSignatureLookup(),
		GlobalTypeOverlay:        r.GlobalTypeOverlay(),
		RecursiveFamilies:        r.RecursiveFamilies,
		ClassFamilyJoin:          r.ClassFamilyJoin,
	}
	if r.NarrowSynth != nil {
		state.ResolveType = r.NarrowSynth.ResolveType
	}
	return state
}

// ViewFromResult constructs the nested-processing view from a full result.
func ViewFromResult(r *FuncResult) *FuncAnalysisView {
	if r == nil {
		return nil
	}
	return &FuncAnalysisView{
		Graph:                    r.Graph,
		AnalysisContext:          r.AnalysisContext,
		Scopes:                   r.Scopes,
		Facts:                    r.analysis.TypeFacts,
		FlowInputs:               r.analysis.FlowInputs,
		FlowProjection:           r.analysis.FlowProjection,
		Evidence:                 r.Evidence,
		CallExpectedArgs:         r.CallExpectedArgs,
		CallContracts:            r.CallContracts,
		NarrowSynth:              r.analysis.SolvedSynth,
		SourceSignature:          r.SourceSignature,
		PublicSeedSignature:      r.PublicSeedSignature,
		LiteralSignatures:        r.LiteralSignatures,
		LiteralSignatureProvider: r.LiteralSignatureLookup(),
		QueryContext:             r.QueryContext,
		TypeOps:                  r.TypeOps,
		GlobalTypes:              r.GlobalTypes,
		GlobalTypeBindings:       r.GlobalTypeOverlay(),
		RecursiveFamilies:        r.RecursiveFamilies,
		ClassFamilyJoin:          r.ClassFamilyJoin,
	}
}

// ReleaseAnalysisArtifacts drops heavyweight internal analysis machinery while
// preserving the public result object identity for session cleanup.
func (r *FuncResult) ReleaseAnalysisArtifacts() {
	if r == nil {
		return
	}
	r.analysis.TypeFacts = nil
	r.analysis.FlowInputs = nil
	r.analysis.FlowProjection = nil
	r.analysis.SolvedSynth = nil
}

// EffectiveTypeAt returns the narrowed type for a symbol at a specific CFG point.
func (r *FuncAnalysisView) EffectiveTypeAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	if r == nil || r.Facts == nil {
		return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
	}
	return r.Facts.EffectiveTypeAt(p, sym)
}

// DeclaredTypeAt returns source/static declaration facts for a symbol.
func (r *FuncAnalysisView) DeclaredTypeAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	if r == nil || r.Facts == nil {
		return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
	}
	return r.Facts.DeclaredAt(p, sym)
}

// RefinedTypeAt returns flow-refined type facts for a symbol.
func (r *FuncAnalysisView) RefinedTypeAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	if r == nil || r.Facts == nil {
		return flow.TypedValue{Type: nil, State: flow.StateUnknown}
	}
	return r.Facts.RefinedAt(p, sym)
}

// TypeFacts returns the mode-safe solved type-fact view for this analysis view.
func (r *FuncAnalysisView) TypeFacts() flow.TypeFacts {
	if r == nil {
		return nil
	}
	return r.Facts
}

// SolvedSynth returns the flow-refined synthesis view for this analysis view.
func (r *FuncAnalysisView) SolvedSynth() Synth {
	if r == nil {
		return nil
	}
	return r.NarrowSynth
}

// FlowInputView returns the read-only extracted-input view for this analysis view.
func (r *FuncAnalysisView) FlowInputView() FlowInputView {
	if r == nil {
		return FlowInputView{}
	}
	return FlowInputView{inputs: r.FlowInputs}
}

// SolvedFlow returns the normalized solved-flow projection for this view.
func (r *FuncAnalysisView) SolvedFlow() FlowOps {
	if r == nil {
		return nil
	}
	if r.FlowProjection != nil {
		return r.FlowProjection
	}
	return nil
}

// ConditionProofFacts returns the producer-neutral condition-proof surface for
// this view.
func (r *FuncAnalysisView) ConditionProofFacts() flow.ConditionProofFacts {
	if r == nil {
		return nil
	}
	if facts, ok := r.Facts.(flow.ConditionProofFacts); ok {
		return facts
	}
	if facts, ok := r.SolvedFlow().(flow.ConditionProofFacts); ok {
		return facts
	}
	return nil
}

// ConstFacts returns immutable constant facts for this view.
func (r *FuncAnalysisView) ConstFacts() flow.ConstFacts {
	if r == nil {
		return nil
	}
	if facts, ok := r.Facts.(flow.ConstFacts); ok {
		return facts
	}
	if facts, ok := r.SolvedFlow().(flow.ConstFacts); ok {
		return facts
	}
	if r.FlowInputs != nil {
		return r.FlowInputs
	}
	return nil
}

// PathObservationFacts returns the high-level path-observation surface for this
// view when the producer exposes one.
func (r *FuncAnalysisView) PathObservationFacts() flow.PathObservationFacts {
	if r == nil {
		return nil
	}
	if facts, ok := r.Facts.(flow.PathObservationFacts); ok {
		return facts
	}
	if facts, ok := r.SolvedFlow().(flow.PathObservationFacts); ok {
		return facts
	}
	return nil
}

// PathChildFacts returns the producer-neutral finite child path surface for this
// view when the producer exposes one.
func (r *FuncAnalysisView) PathChildFacts() flow.PathChildFacts {
	if r == nil {
		return nil
	}
	if facts, ok := r.Facts.(flow.PathChildFacts); ok {
		return facts
	}
	if facts, ok := r.SolvedFlow().(flow.PathChildFacts); ok {
		return facts
	}
	return nil
}

// TransferValueFacts returns the producer-neutral transfer-value surface for
// this view when the producer exposes one.
func (r *FuncAnalysisView) TransferValueFacts() flow.TransferValueFacts {
	if r == nil {
		return nil
	}
	if facts, ok := r.Facts.(flow.TransferValueFacts); ok {
		return facts
	}
	if facts, ok := r.SolvedFlow().(flow.TransferValueFacts); ok {
		return facts
	}
	return nil
}

// GlobalTypeOverlay returns the normalized source-global type overlay for this
// view. If only the external map projection is populated, it is normalized here.
func (r *FuncAnalysisView) GlobalTypeOverlay() globalenv.TypeOverlay {
	if r == nil {
		return nil
	}
	if len(r.GlobalTypeBindings) != 0 {
		return r.GlobalTypeBindings.Clone()
	}
	return globalenv.TypeOverlayFromMap(r.GlobalTypes)
}

// LiteralSignatureLookup returns the normalized function-literal signature
// lookup surface for this view. If only the external map projection is populated,
// it is normalized here.
func (r *FuncAnalysisView) LiteralSignatureLookup() LiteralSignatureLookup {
	if r == nil {
		return nil
	}
	if r.LiteralSignatureProvider != nil {
		return r.LiteralSignatureProvider
	}
	if len(r.LiteralSignatures) == 0 {
		return nil
	}
	return LiteralSignatureLookupFromMap(r.LiteralSignatures)
}
