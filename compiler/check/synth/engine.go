// Package synth provides type synthesis for Lua expressions during type checking.
//
// The synthesis engine operates in two distinct phases:
//
// 1. Declared Phase (pre-flow analysis): Synthesizes types using only declared
// type annotations and structural inference. Used during scope computation
// before dataflow analysis has been performed.
//
// 2. Narrowed Phase (post-flow analysis): Synthesizes types with full flow-sensitive
// narrowing, incorporating control flow predicates, type guards, and refined
// type information from assignments.
//
// The Engine struct is the primary entry point, configured via Config depending
// on the compilation phase. It delegates to the internal
// extract.Synthesizer which handles the actual synthesis logic.
//
// Key capabilities:
//   - Expression type synthesis (TypeOf, MultiTypeOf)
//   - Function signature inference (FunctionType, ResolveFunctionSignature)
//   - Value expansion for multi-return contexts (ExpandValues)
//   - Iterator variable inference (InferIterVars)
//   - Type definition resolution (ResolveTypeDef)
//   - Field and method lookup (Field, Method)
//
// The engine maintains separate caches for pre-flow (PreCache) and post-flow
// (NarrowCache) synthesis results to avoid redundant computation.
package synth

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/phase/extract"
	"github.com/wippyai/go-lua/compiler/check/synth/phase/resolve"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// Config configures a synthesis engine for a given pipeline phase.
//
// Phase controls whether flow-sensitive narrowing is enabled:
//   - api.PhaseNarrowing: flow-refined (post-flow)
//   - all earlier phases: declared-only (pre-flow)
type Config struct {
	Ctx            *db.QueryContext
	Types          core.TypeOps
	Scopes         api.ScopeMap
	Manifests      io.ManifestQuerier
	Env            api.BaseEnv
	Flow           api.FlowOps
	Paths          api.PathFromExprFunc
	PreCache       api.Cache
	NarrowCache    api.Cache
	Phase          api.Phase
	ModuleBindings *bind.BindingTable
	ModuleAliases  map[cfg.SymbolID]string
}

// Engine provides type synthesis configured by compilation phase.
//
// Engine is the main entry point for type synthesis during type checking.
// It wraps the internal extract.Synthesizer and provides a phase-aware API
// that automatically routes synthesis requests through the appropriate
// code paths for declared vs narrowed phases.
//
// The engine caches synthesis results to avoid redundant computation:
//   - PreCache: Results from declared phase (type annotations only)
//   - NarrowCache: Results from narrowed phase (flow-refined types)
//
// Thread safety: Engine instances are not thread-safe. Create separate
// instances for concurrent synthesis operations.
type Engine struct {
	*extract.Synthesizer
	deps  *extract.Deps
	phase api.Phase
}

// New creates a synthesis engine configured for the requested phase.
func New(cfg Config) *Engine {
	phase := cfg.Phase
	isNarrowing := phase == api.PhaseNarrowing
	if isNarrowing && cfg.Flow == nil {
		panic("synth: PhaseNarrowing requires Flow")
	}
	if cfg.Env != nil {
		if isNarrowing {
			if _, ok := cfg.Env.(api.NarrowEnv); !ok {
				panic("synth: PhaseNarrowing requires NarrowEnv")
			}
		} else {
			if _, ok := cfg.Env.(api.DeclaredEnv); !ok {
				panic("synth: pre-flow phases require DeclaredEnv")
			}
		}
	}

	preCache := cfg.PreCache
	if preCache == nil {
		preCache = make(api.Cache)
	}
	narrowCache := cfg.NarrowCache
	if narrowCache == nil && isNarrowing {
		narrowCache = make(api.Cache)
	}

	deps := &extract.Deps{
		Ctx:            cfg.Ctx,
		Types:          cfg.Types,
		Scopes:         cfg.Scopes,
		Manifests:      cfg.Manifests,
		CheckCtx:       cfg.Env,
		Flow:           cfg.Flow,
		Paths:          cfg.Paths,
		PreCache:       preCache,
		NarrowCache:    narrowCache,
		ModuleBindings: cfg.ModuleBindings,
		ModuleAliases:  cfg.ModuleAliases,
	}

	return &Engine{
		Synthesizer: extract.NewSynthesizer(deps, phase),
		deps:        deps,
		phase:       phase,
	}
}

// TypeOf returns the synthesized type of an expression at a CFG point.
//
// In narrowing phase, queries the narrow cache first, then synthesizes
// using flow information and caches the result. In declared phase,
// delegates to the extract synthesizer without flow context.
//
// Returns typ.Nil for nil expressions. Returns typ.Unknown if synthesis fails.
func (e *Engine) TypeOf(expr ast.Expr, p cfg.Point) typ.Type {
	if e.IsNarrowing() {
		if cached, ok := e.deps.NarrowCache.Get(expr, p); ok {
			return cached
		}
		t := e.SynthExpr(expr, p, e.deps.Flow)
		e.deps.NarrowCache.Put(expr, p, t)
		return t
	}
	return e.Synthesizer.TypeOf(expr, p)
}

// TypeOfWithExpected synthesizes an expression type with an expected type hint.
//
// The expected type guides inference for polymorphic expressions like empty
// tables, generic function calls, and function literals without annotations.
// Does not enforce that the result matches expected - only uses it as a hint.
func (e *Engine) TypeOfWithExpected(expr ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
	if e.IsNarrowing() {
		return e.SynthWithExpected(expr, p, expected)
	}
	return e.Synthesizer.TypeOfWithExpected(expr, p, expected)
}

// MultiTypeOf returns all types from a multi-valued expression.
//
// Used for expressions that may return multiple values:
//   - Function calls (returns all return values)
//   - Vararg expressions (returns vararg element type)
//   - Parenthesized expressions (single element slice)
//
// Returns nil for non-multi-valued expressions.
func (e *Engine) MultiTypeOf(expr ast.Expr, p cfg.Point) []typ.Type {
	if e.IsNarrowing() {
		return e.SynthMulti(expr, p, e.deps.Flow)
	}
	return e.Synthesizer.MultiTypeOf(expr, p)
}

// FunctionType synthesizes the function type from a function expression.
//
// Combines declared parameter types, return type annotations, and inferred
// information to build a complete function type. The scope state provides
// context for resolving type references in annotations.
func (e *Engine) FunctionType(fn *ast.FunctionExpr, sc *scope.State) *typ.Function {
	return e.Synthesizer.FunctionType(fn, sc)
}

// ExpandValues expands a list of expressions into the required number of types.
//
// Handles multi-return expansion: the last expression in the list may
// contribute multiple values (e.g., function call returns). Earlier expressions
// contribute exactly one value each.
//
// If fewer values are available than needed, pads with typ.Nil.
// If more values are available than needed, truncates to needed.
func (e *Engine) ExpandValues(exprs []ast.Expr, needed int, p cfg.Point) []typ.Type {
	if e.IsNarrowing() {
		return e.expandValuesNarrowed(exprs, needed, p)
	}
	return e.Synthesizer.ExpandValues(exprs, needed, p)
}

func (e *Engine) expandValuesNarrowed(exprs []ast.Expr, needed int, p cfg.Point) []typ.Type {
	if len(exprs) == 0 {
		return nil
	}
	result := make([]typ.Type, 0, needed)

	for i, expr := range exprs {
		if i == len(exprs)-1 {
			multi := e.MultiTypeOf(expr, p)
			result = append(result, multi...)
		} else {
			result = append(result, e.TypeOf(expr, p))
		}
	}

	for len(result) < needed {
		result = append(result, typ.Nil)
	}

	return result
}

// SynthWithExpected synthesizes with expected type, using flow information.
//
// Combines flow-sensitive synthesis with expected type hints. Returns typ.Nil
// for nil expressions. Falls back to TypeOf if no expected type provided.
func (e *Engine) SynthWithExpected(expr ast.Expr, p cfg.Point, expected typ.Type) typ.Type {
	if expr == nil {
		return typ.Nil
	}
	if expected == nil {
		return e.TypeOf(expr, p)
	}
	sc := e.deps.Scopes[p]
	recurse := func(ex ast.Expr) typ.Type { return e.SynthExpr(ex, p, e.deps.Flow) }
	return e.SynthExprWithExpectedCore(expr, sc, p, recurse, expected)
}

// Narrow returns a narrowing-capable synth if flow information is available
// and the engine is in PhaseNarrowing.
//
// Returns nil if the engine was created without flow information or the phase
// is pre-flow. Returns self if flow information is present and phase is narrowing.
func (e *Engine) Narrow() api.BaseSynth {
	if e.deps.Flow == nil || e.phase != api.PhaseNarrowing {
		return nil
	}
	return e
}

// IsNarrowing reports whether the engine operates in narrowing phase.
func (e *Engine) IsNarrowing() bool {
	return e.phase == api.PhaseNarrowing
}

// ResolveTypeDefAt resolves a type definition at a specific CFG point.
//
// This is used during scope computation to ensure typeof(expr) can see
// local annotated types that are in scope at the point of the typedef.
func (e *Engine) ResolveTypeDefAt(name string, typeExpr ast.TypeExpr, typeParams []ast.TypeParamExpr, sc *scope.State, p cfg.Point) typ.Type {
	resolver := resolve.New(resolve.Config{
		Manifests: e.deps.Manifests,
		ExprSynth: func(expr ast.Expr, _ cfg.Point) typ.Type {
			return e.SynthExprAt(expr, p, sc)
		},
	})
	return resolver.ResolveTypeDef(name, typeExpr, typeParams, sc)
}

// Context returns the query context for type database operations.
func (e *Engine) Context() *db.QueryContext {
	return e.deps.Ctx
}

// Scopes returns the CFG point to scope state mapping.
func (e *Engine) Scopes() api.ScopeMap {
	return e.deps.Scopes
}

// AllowReturnTransforms reports if return type transforms are allowed.
//
// Return transforms (like effect returns) are only applied during
// narrowing phase when full type information is available.
func (e *Engine) AllowReturnTransforms() bool {
	return e.IsNarrowing()
}

// Phase returns the current compilation phase.
func (e *Engine) Phase() api.Phase {
	return e.phase
}

// CallQuery returns the type operations interface for call resolution.
func (e *Engine) CallQuery() core.TypeOps {
	return e.Synthesizer.GetCallQuery()
}

// Entry returns the CFG entry point for the function being analyzed.
func (e *Engine) Entry() cfg.Point {
	return e.deps.Entry()
}

// ResolveFieldAccess resolves field or index access on a type.
//
// Handles both named field access (t.field) and computed index access (t[key]).
// Returns the result type and whether the access is valid.
func (e *Engine) ResolveFieldAccess(fullExpr *ast.AttrGetExpr, objType typ.Type, fieldName string, p cfg.Point) FieldAccessResult {
	return ResolveFieldAccess(e, fullExpr, objType, fieldName, p)
}
