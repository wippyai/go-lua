package api

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	"github.com/wippyai/go-lua/compiler/check/domain/globalenv"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// FuncResult holds the complete analysis output for a single function.
// It is pure data with no back-references to Session or other FuncResults,
// enabling safe memoization and independent storage.
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

	// Facts provides effective type information for symbols at each CFG point.
	// This is the primary interface for querying narrowed types during synthesis.
	Facts flow.TypeFacts

	// FlowInputs contains the extracted flow constraints.
	// Includes assignments, conditions, type guards, and dead point markers.
	FlowInputs *flow.Inputs

	// FlowProjection is the producer-neutral solved-flow read surface. Product-state
	// producers set this to their projection.
	FlowProjection FlowOps

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

	// PublicSeedSignature is the source-declared public seed used when
	// canonicalizing post-flow FunctionFact signatures.
	PublicSeedSignature *typ.Function

	// NarrowSynth is the flow-refined synthesis engine for this function.
	// Use TypeOf to query expression types with flow-based narrowing applied.
	NarrowSynth Synth

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

// EffectiveTypeAt returns the narrowed type for a symbol at a specific CFG point.
// The effective type reflects flow-based narrowing (type guards, nil checks, etc.)
// applied to the symbol's declared type based on control flow leading to point p.
//
// Returns TypedValue with:
//   - Type: The narrowed type at point p
//   - State: StateResolved if type is known, StateUnknown otherwise
//
// Delegates to Facts.EffectiveTypeAt when available, otherwise returns unknown.
func (r *FuncResult) EffectiveTypeAt(p cfg.Point, sym cfg.SymbolID) flow.TypedValue {
	if r == nil || r.Facts == nil {
		return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
	}
	return r.Facts.EffectiveTypeAt(p, sym)
}

// SolvedFlow returns the normalized solved-flow projection for this result.
func (r *FuncResult) SolvedFlow() FlowOps {
	if r == nil {
		return nil
	}
	if r.FlowProjection != nil {
		return r.FlowProjection
	}
	return nil
}

// ConditionProofFacts returns the producer-neutral condition-proof surface for
// this result. Consumers use this instead of depending on concrete flow state.
func (r *FuncResult) ConditionProofFacts() flow.ConditionProofFacts {
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

// ConstFacts returns immutable constant facts for this result.
func (r *FuncResult) ConstFacts() flow.ConstFacts {
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
// result when the producer exposes one.
func (r *FuncResult) PathObservationFacts() flow.PathObservationFacts {
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
// result when the producer exposes one.
func (r *FuncResult) PathChildFacts() flow.PathChildFacts {
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
// this result when the producer exposes one.
func (r *FuncResult) TransferValueFacts() flow.TransferValueFacts {
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

// ViewFromResult constructs the nested-processing view from a full result.
func ViewFromResult(r *FuncResult) *FuncAnalysisView {
	if r == nil {
		return nil
	}
	return &FuncAnalysisView{
		Graph:                    r.Graph,
		AnalysisContext:          r.AnalysisContext,
		Scopes:                   r.Scopes,
		Facts:                    r.Facts,
		FlowInputs:               r.FlowInputs,
		FlowProjection:           r.FlowProjection,
		Evidence:                 r.Evidence,
		CallExpectedArgs:         r.CallExpectedArgs,
		CallContracts:            r.CallContracts,
		NarrowSynth:              r.NarrowSynth,
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
