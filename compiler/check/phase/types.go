// Package phase implements the type-checker analysis pipeline phases.
// Each phase is a pure function with explicit inputs and outputs, following
// the design principle that phases consume immutable inputs and produce
// immutable outputs without side effects.
//
// # PHASE PIPELINE
//
// The analysis pipeline consists of four main phases:
//
//	Phase A (Resolve): Resolves type annotation expressions from AST into
//	concrete typ.Type values. Handles @type, @param, @return annotations
//	and user-defined type aliases.
//
//	Phase B (Scope): Builds lexical scope states for each CFG point.
//	Extracts declared types from annotations and synthesizes function
//	literal signatures. Also collects flow constraints from assignments
//	and control flow.
//
//	Phase C (Solve): Solves the extracted flow constraint system to compute
//	reachability conditions and type narrowing facts. Uses the narrow.Resolver
//	for type guard and assertion processing.
//
//	Phase D (Narrow): Applies flow solution to narrow declared types.
//	Computes effective types for all expressions and generates function
//	effects (side effects, termination).
//
// INPUT/OUTPUT STRUCTS
//
// Each phase has corresponding Input and Output structs that explicitly
// declare all data dependencies. This enables:
//   - Clear understanding of phase data flow
//   - Memoization based on input equality
//   - Future parallelization of independent phases
//
// # PHASE ENV
//
// PhaseEnv bundles shared environment fields used across all phases.
// It is embedded in each phase input to reduce boilerplate while
// maintaining explicit dependency declaration.
package phase

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// LiteralSigsProvider provides lookup for pre-computed function literal signatures.
// Implemented by both map[*ast.FunctionExpr]*typ.Function and lazy view types.
type LiteralSigsProvider interface {
	Lookup(fn *ast.FunctionExpr) *typ.Function
}

// LiteralSigsMap wraps a map to implement LiteralSigsProvider.
type LiteralSigsMap map[*ast.FunctionExpr]*typ.Function

// Lookup returns the signature for a function from the map.
func (m LiteralSigsMap) Lookup(fn *ast.FunctionExpr) *typ.Function {
	return m[fn]
}

// PhaseEnv holds shared environment fields used across all analysis phases.
// It is built once by the checker and embedded into each phase input struct,
// reducing boilerplate while maintaining explicit dependency declaration.
//
// All fields are read-only during phase execution. Phases must not mutate
// PhaseEnv fields; any derived data should be returned in the phase output.
type PhaseEnv struct {
	// Ctx provides query infrastructure for memoization and type caching.
	Ctx *db.QueryContext

	// Graph is the function's control flow graph.
	Graph *cfg.Graph

	// Fn is the function AST node being analyzed.
	Fn *ast.FunctionExpr

	// Types provides type construction and manipulation operations.
	Types core.TypeOps

	// Manifests provides imported module type information.
	Manifests io.ManifestQuerier

	// GlobalTypes contains built-in global function types (print, pairs, etc.).
	GlobalTypes map[string]typ.Type

	// ModuleAliases maps symbols to their require() module paths.
	ModuleAliases map[cfg.SymbolID]string

	// ModuleBindings is the binding table for the entire module.
	ModuleBindings *bind.BindingTable

	// Refinements provides canonical function refinement lookups for callee analysis.
	Refinements api.RefinementFacts

	// Evidence is the transfer-owned graph event trace for this function.
	Evidence api.FlowEvidence

	// Scopes maps CFG points to scope states (populated after scope phase).
	Scopes map[cfg.Point]*scope.State

	// RecursiveFamilies is the compilation-scoped recursive-family interner used
	// to seal class metatables into shared interned families during synthesis.
	RecursiveFamilies *typ.RecursiveFamilyInterner
}

// TypeResolver resolves type expressions to types.
type TypeResolver interface {
	ResolveType(expr ast.TypeExpr, sc *scope.State) typ.Type
}

// TypeResolverFunc adapts a function to TypeResolver.
type TypeResolverFunc func(expr ast.TypeExpr, sc *scope.State) typ.Type

func (f TypeResolverFunc) ResolveType(expr ast.TypeExpr, sc *scope.State) typ.Type {
	if f == nil {
		return nil
	}
	return f(expr, sc)
}

// FunctionSignatureResolver resolves function signatures.
type FunctionSignatureResolver interface {
	ResolveFunctionSignature(fn *ast.FunctionExpr, sc *scope.State) *typ.Function
}

// FunctionSignatureResolverFunc adapts a function to FunctionSignatureResolver.
type FunctionSignatureResolverFunc func(fn *ast.FunctionExpr, sc *scope.State) *typ.Function

func (f FunctionSignatureResolverFunc) ResolveFunctionSignature(fn *ast.FunctionExpr, sc *scope.State) *typ.Function {
	if f == nil {
		return nil
	}
	return f(fn, sc)
}

// ResolveInput contains inputs for Phase A (type resolution).
// This phase resolves type annotation expressions into concrete types.
type ResolveInput struct {
	PhaseEnv
	// Bindings provides AST-to-symbol mapping for identifier resolution.
	Bindings *bind.BindingTable
	// BaseScope provides the parent scope for type name lookups.
	BaseScope *scope.State
}

// ResolveOutput contains outputs from Phase A (type resolution).
// The primary output is a resolver for subsequent phases.
type ResolveOutput struct {
	// TypeResolver converts type expressions to concrete types.
	// Used by scope and extract phases for annotation processing.
	TypeResolver TypeResolver
}

// ScopeInput contains inputs for Phase B (scope computation).
// This phase builds lexical scope states and extracts declared types.
// All inputs are explicit read-only values - no mutable store references.
type ScopeInput struct {
	PhaseEnv
	// Parent is the parent scope state (from enclosing function or stdlib).
	Parent *scope.State
	// MaxScopeDepth limits scope nesting depth (0 = disabled).
	MaxScopeDepth int
	// Resolve contains the type expression resolver from Phase A.
	Resolve ResolveOutput
	// SynthesizedFunctionSig is the pre-computed signature for this function.
	// May come from literal signature cache or explicit annotation.
	SynthesizedFunctionSig *typ.Function
	// FunctionLiteralSignatures contains pre-computed signatures for nested literals.
	// Read-only - populated from LiteralSigs channel during iteration.
	// Can be a map or LiteralSigsProvider interface for lazy lookup.
	FunctionLiteralSignatures LiteralSigsProvider
	// ParameterEvidenceSignatures contains function-expression keyed parameter evidence.
	// Read-only - projected from canonical FunctionFacts during iteration.
	ParameterEvidenceSignatures map[*ast.FunctionExpr][]typ.Type
	// FunctionFacts contains canonical facts for functions in this graph
	// context. Explicit input - not looked up from store during phase execution.
	FunctionFacts api.FunctionFacts
}

// ScopeOutput contains outputs from Phase B (scope computation).
// Provides scope states and declared types for subsequent phases.
type ScopeOutput struct {
	// BaseScope is the function's entry scope with parameters and type params.
	BaseScope *scope.State
	// Scopes maps each CFG point to its effective scope state.
	Scopes map[cfg.Point]*scope.State
	// DeclaredTypes maps symbols to their declared (pre-flow) types.
	DeclaredTypes flow.DeclaredTypes
	// AnnotatedVars tracks which symbols have explicit type annotations.
	AnnotatedVars map[cfg.SymbolID]bool
	// ParamTypes contains parameter symbol to type mappings.
	ParamTypes map[cfg.SymbolID]typ.Type
	// FunctionSignatureResolver resolves function signatures from AST.
	FunctionSignatureResolver FunctionSignatureResolver
	// FunctionFacts contains canonical function facts (passed through from input).
	FunctionFacts api.FunctionFacts
	// DepthLimitExceeded indicates scope depth exceeded MaxScopeDepth.
	DepthLimitExceeded bool
}

// LiteralInput contains inputs for the function literal synthesis phase.
// Phase B (continued): synthesizes function literal types.
type LiteralInput struct {
	PhaseEnv
	Scope         ScopeOutput
	FunctionFacts api.FunctionFacts
}

// LiteralOutput contains outputs from the function literal synthesis phase.
// Phase B (continued) outputs: literal types and signatures.
type LiteralOutput struct {
	LiteralTypes flow.DeclaredTypes
	Signatures   map[*ast.FunctionExpr]*typ.Function
}

// FlowExtractInput contains inputs for the flow extraction phase.
// Phase B: extracts flow constraints and assignments.
type FlowExtractInput struct {
	PhaseEnv
	Resolve       ResolveOutput
	Scope         ScopeOutput
	FunctionFacts api.FunctionFacts
	LiteralTypes  flow.DeclaredTypes
}

// FlowExtractOutput contains outputs from the flow extraction phase.
// Phase B outputs: flow inputs for the solver.
type FlowExtractOutput struct {
	Inputs     *flow.Inputs
	Params     []flow.ParamInfo
	ReturnType typ.Type
	Evidence   api.FlowEvidence
}

// FlowSolveInput contains inputs for the flow solve phase.
// Phase C: solves the flow system.
type FlowSolveInput struct {
	PhaseEnv
	Extract  FlowExtractOutput
	Resolver narrow.Resolver
}

// FlowSolveOutput contains outputs from the flow solve phase.
// Phase C outputs: the flow solution.
type FlowSolveOutput struct {
	Solution *flow.Solution
}

// NarrowInput contains inputs for the narrowing phase.
// Phase D: builds TypeFacts and infers effects.
type NarrowInput struct {
	PhaseEnv
	Scope         ScopeOutput
	Extract       FlowExtractOutput
	Solve         FlowSolveOutput
	FunctionFacts api.FunctionFacts
	LiteralTypes  flow.DeclaredTypes
}

// NarrowOutput contains outputs from the narrowing phase.
// Phase D outputs: TypeFacts and the inferred function refinement.
type NarrowOutput struct {
	Facts        flow.TypeFacts
	Refinement   *constraint.FunctionRefinement
	Synth        synth.Synth
	QueryContext *db.QueryContext
	TypeOps      core.TypeOps
}

// ContextBuilder constructs Env instances from phase outputs.
// Centralizes the wiring that was previously duplicated across phase run files.
type ContextBuilder struct {
	env           PhaseEnv
	bindings      *bind.BindingTable
	baseScope     *scope.State
	declaredTypes flow.DeclaredTypes
	annotatedVars map[cfg.SymbolID]bool
	functionFacts api.FunctionFacts
	literalTypes  flow.DeclaredTypes
	solution      *flow.Solution
}

// NewContextBuilder creates a builder pre-populated from the shared phase environment.
func NewContextBuilder(env PhaseEnv) *ContextBuilder {
	var bindings *bind.BindingTable
	if env.Graph != nil {
		bindings = env.Graph.Bindings()
	}
	return &ContextBuilder{
		env:      env,
		bindings: bindings,
	}
}

// WithScope populates scope-derived fields (base scope, declared types,
// annotated vars, sibling types).
func (b *ContextBuilder) WithScope(out ScopeOutput) *ContextBuilder {
	b.baseScope = out.BaseScope
	b.declaredTypes = out.DeclaredTypes
	b.annotatedVars = out.AnnotatedVars
	b.functionFacts = out.FunctionFacts
	return b
}

// WithLiterals populates literal type overlay from literal phase output.
func (b *ContextBuilder) WithLiterals(out LiteralOutput) *ContextBuilder {
	b.literalTypes = out.LiteralTypes
	return b
}

// WithSolution sets the flow solution for narrowing-phase contexts.
func (b *ContextBuilder) WithSolution(sol *flow.Solution) *ContextBuilder {
	b.solution = sol
	return b
}

// WithBindings overrides the binding table.
func (b *ContextBuilder) WithBindings(bt *bind.BindingTable) *ContextBuilder {
	b.bindings = bt
	return b
}

// WithBaseScope overrides the base scope state.
func (b *ContextBuilder) WithBaseScope(sc *scope.State) *ContextBuilder {
	b.baseScope = sc
	return b
}

// WithDeclaredTypes overrides declared types.
func (b *ContextBuilder) WithDeclaredTypes(dt flow.DeclaredTypes) *ContextBuilder {
	b.declaredTypes = dt
	return b
}

// WithAnnotatedVars overrides annotated variable flags.
func (b *ContextBuilder) WithAnnotatedVars(av map[cfg.SymbolID]bool) *ContextBuilder {
	b.annotatedVars = av
	return b
}

// WithFunctionFacts overrides canonical function facts.
func (b *ContextBuilder) WithFunctionFacts(ff api.FunctionFacts) *ContextBuilder {
	b.functionFacts = ff
	return b
}

// WithLiteralTypes overrides the literal type overlay directly.
func (b *ContextBuilder) WithLiteralTypes(lt flow.DeclaredTypes) *ContextBuilder {
	b.literalTypes = lt
	return b
}

// BuildDeclared constructs a declared-phase Env from accumulated fields.
func (b *ContextBuilder) BuildDeclared() *api.DeclaredEnvImpl {
	return b.buildDeclaredWithProjection(functionfact.ProjectionSibling)
}

// BuildFlowInput constructs the declared-phase Env used by abstract
// interpreter input extraction. It excludes body/entry parameter evidence from
// function-value projections; extraction must not consume solved body state.
func (b *ContextBuilder) BuildFlowInput() *api.DeclaredEnvImpl {
	return b.buildDeclaredWithProjection(functionfact.ProjectionFlowInput)
}

func (b *ContextBuilder) buildDeclaredWithProjection(projection functionfact.Projection) *api.DeclaredEnvImpl {
	return api.NewDeclaredEnv(api.DeclaredEnvConfig{
		Graph:         b.env.Graph,
		Bindings:      b.bindings,
		DeclaredTypes: b.declaredTypes,
		LiteralTypes:  b.literalTypes,
		AnnotatedVars: b.annotatedVars,
		BaseScope:     b.baseScope,
		Refinements:   b.env.Refinements,
		ModuleAliases: b.env.ModuleAliases,
		GlobalTypes:   b.env.GlobalTypes,
		FunctionType:  functionfact.ProjectionLookup(b.functionFacts, projection, api.PhaseScopeCompute),
	})
}

// BuildNarrow constructs a narrowing-phase Env from accumulated fields.
func (b *ContextBuilder) BuildNarrow() *api.NarrowEnvImpl {
	return api.NewNarrowEnv(api.NarrowEnvConfig{
		Graph:         b.env.Graph,
		Bindings:      b.bindings,
		DeclaredTypes: b.declaredTypes,
		LiteralTypes:  b.literalTypes,
		AnnotatedVars: b.annotatedVars,
		Solution:      b.solution,
		BaseScope:     b.baseScope,
		Refinements:   b.env.Refinements,
		ModuleAliases: b.env.ModuleAliases,
		GlobalTypes:   b.env.GlobalTypes,
		FunctionType:  functionfact.ProjectionLookup(b.functionFacts, functionfact.ProjectionSibling, api.PhaseNarrowing),
	})
}
