package api

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
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
// The result captures outputs from all analysis phases:
//   - Phase A (Resolve): Type annotations resolved into Scopes
//   - Phase B (Scope/Extract): BaseScope, Scopes, FlowInputs
//   - Phase C (Solve): FlowSolution
//   - Phase D (Narrow): Facts, FnRefinement, NarrowSynth
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

	// FlowInputs contains the extracted flow constraints from Phase B.
	// Includes assignments, conditions, type guards, and dead point markers.
	FlowInputs *flow.Inputs

	// FlowSolution contains the solved constraint system from Phase C.
	// Provides reachability conditions and exclusion facts for narrowing.
	FlowSolution *flow.Solution

	// Evidence records events discovered during abstract interpretation.
	Evidence FlowEvidence

	// FnRefinement captures the function's inferred refinement summary.
	// It includes propagated effect rows and branch-specific narrowing facts.
	FnRefinement *constraint.FunctionRefinement

	// SourceSignature is the annotation-only function signature resolved during
	// phase execution.
	SourceSignature *typ.Function

	// PublicSeedSignature is the source-declared public seed used when
	// canonicalizing post-flow FunctionFact signatures.
	PublicSeedSignature *typ.Function

	// NarrowSynth is the narrowed-phase synthesis engine for this function.
	// Use TypeOf to query expression types with flow-based narrowing applied.
	NarrowSynth Synth

	// QueryContext is the immutable query context used by pure type operations.
	QueryContext *db.QueryContext

	// TypeOps is the canonical type-operation surface for solved-state reads.
	TypeOps core.TypeOps

	// GlobalTypes is the immutable value namespace visible to this function,
	// including callback/global overlays from its analysis context.
	GlobalTypes map[string]typ.Type

	// LiteralSignatures holds synthesized signatures for function literals in this graph.
	LiteralSignatures map[*ast.FunctionExpr]*typ.Function

	// Extras stores results from registered ComputePass plugins.
	// Keyed by ComputePass.Name().
	Extras map[string]any

	// DepthLimitExceeded indicates scope depth limit was hit during scope phase.
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

// NarrowedTypeAt returns the precise path-sensitive narrowed type at a CFG point.
func (r *FuncResult) NarrowedTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	if r == nil || r.FlowSolution == nil {
		return nil
	}
	return r.FlowSolution.NarrowedTypeAt(p, path)
}

// PreStateTypeAt returns the path-sensitive type before point-local transfer effects.
func (r *FuncResult) PreStateTypeAt(p cfg.Point, path constraint.Path) typ.Type {
	if r == nil || r.FlowSolution == nil {
		return nil
	}
	return r.FlowSolution.PreStateTypeAt(p, path)
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
	if r == nil || r.FlowSolution == nil {
		return false
	}
	return r.FlowSolution.ExcludesTypeAt(p, path, declared)
}

// FuncAnalysisView is the stable slice of a function analysis result
// required by nested processing and interprocedural helpers.
type FuncAnalysisView struct {
	Graph               *cfg.Graph
	AnalysisContext     AnalysisContext
	Scopes              map[cfg.Point]*scope.State
	Facts               flow.TypeFacts
	FlowInputs          *flow.Inputs
	FlowSolution        *flow.Solution
	Evidence            FlowEvidence
	NarrowSynth         Synth
	SourceSignature     *typ.Function
	PublicSeedSignature *typ.Function
	LiteralSignatures   map[*ast.FunctionExpr]*typ.Function
	QueryContext        *db.QueryContext
	TypeOps             core.TypeOps
	GlobalTypes         map[string]typ.Type
	RecursiveFamilies   *typ.RecursiveFamilyInterner
	ClassFamilyJoin     func(existing, candidate typ.Type) typ.Type
}

// ViewFromResult constructs the nested-processing view from a full result.
func ViewFromResult(r *FuncResult) *FuncAnalysisView {
	if r == nil {
		return nil
	}
	return &FuncAnalysisView{
		Graph:               r.Graph,
		AnalysisContext:     r.AnalysisContext,
		Scopes:              r.Scopes,
		Facts:               r.Facts,
		FlowInputs:          r.FlowInputs,
		FlowSolution:        r.FlowSolution,
		Evidence:            r.Evidence,
		NarrowSynth:         r.NarrowSynth,
		SourceSignature:     r.SourceSignature,
		PublicSeedSignature: r.PublicSeedSignature,
		LiteralSignatures:   r.LiteralSignatures,
		QueryContext:        r.QueryContext,
		TypeOps:             r.TypeOps,
		GlobalTypes:         r.GlobalTypes,
		RecursiveFamilies:   r.RecursiveFamilies,
		ClassFamilyJoin:     r.ClassFamilyJoin,
	}
}
