// Package synth provides type synthesis for Lua expressions during type checking.
//
// The synthesis engine operates in two distinct modes:
//
// 1. Declared/static mode: Synthesizes types using only declared type annotations
// and structural inference.
//
// 2. Flow mode: Synthesizes types with flow-sensitive narrowing, incorporating
// control flow predicates, type guards, and refined type information from
// assignments.
//
// The Engine struct is the primary entry point, configured via Config. It
// delegates to the internal extract.Synthesizer which handles the actual
// synthesis logic.
//
// Key capabilities:
//   - Expression type synthesis (TypeOf, MultiTypeOf)
//   - Function signature inference (FunctionType, ResolveFunctionSignature)
//   - Value expansion for multi-return contexts (ExpandValues)
//   - Iterator variable inference (InferIterVars)
//   - Type definition resolution (ResolveTypeDef)
//   - Field and method lookup (Field, Method)
//
// Repeated pure expression synthesis runs through db.Query/QueryContext so cache
// hits are observationally equivalent to recomputation.
package synth

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth/extract"
	"github.com/wippyai/go-lua/compiler/check/synth/resolve"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// Config configures a synthesis engine for a requested mode.
//
// SynthMode controls whether flow-sensitive narrowing is enabled:
//   - api.SynthModeFlow: flow-refined (post-flow)
//   - all earlier modes: declared-only (pre-flow)
type Config struct {
	Ctx            *db.QueryContext
	Types          core.TypeOps
	Scopes         api.ScopeMap
	Manifests      io.ManifestQuerier
	Env            api.BaseEnv
	FunctionFacts  api.FunctionFacts
	Flow           api.FlowOps
	Inputs         *flow.Inputs
	Paths          api.PathFromExprFunc
	Evidence       api.FlowEvidence
	Graphs         api.GraphProvider
	SynthMode      api.SynthMode
	ModuleBindings *bind.BindingTable
	ModuleAliases  map[cfg.SymbolID]string

	// RecursiveFamilies is the compilation-scoped recursive-family interner used
	// to seal class metatables into shared interned families during synthesis.
	RecursiveFamilies *typ.RecursiveFamilyInterner
}

// Engine provides type synthesis configured by mode.
//
// Engine is the main entry point for type synthesis during type checking.
// It wraps the internal extract.Synthesizer and provides a mode-aware API
// that automatically routes synthesis requests through the appropriate
// code paths for declared/static vs flow-refined modes.
//
// The engine delegates pure memoization to db.Query/QueryContext.
//
// Thread safety: Engine instances are not thread-safe. Create separate
// instances for concurrent synthesis operations.
type Engine struct {
	*extract.Synthesizer
	deps            *extract.Deps
	narrowTypeQuery *db.Query[exprTypeQueryKey, typ.Type]
}

type exprTypeQueryKey struct {
	Expr  ast.Expr
	Point cfg.Point
}

// New creates a synthesis engine configured for the requested mode.
func New(cfg Config) *Engine {
	mode := cfg.SynthMode
	usesFlow := mode == api.SynthModeFlow
	if usesFlow && cfg.Flow == nil {
		panic("synth: SynthModeFlow requires Flow")
	}
	if cfg.Env != nil {
		if usesFlow {
			if _, ok := cfg.Env.(api.NarrowEnv); !ok {
				panic("synth: SynthModeFlow requires NarrowEnv")
			}
		} else {
			if _, ok := cfg.Env.(api.DeclaredEnv); !ok {
				panic("synth: declared/static modes require DeclaredEnv")
			}
		}
	}

	graphs := cfg.Graphs
	if graphs == nil {
		graphs = api.GraphsFrom(cfg.Ctx)
	}

	deps := &extract.Deps{
		Ctx:               cfg.Ctx,
		Types:             cfg.Types,
		Scopes:            cfg.Scopes,
		Manifests:         cfg.Manifests,
		CheckCtx:          cfg.Env,
		FunctionFacts:     cfg.FunctionFacts,
		Graphs:            graphs,
		Flow:              cfg.Flow,
		Inputs:            cfg.Inputs,
		Paths:             cfg.Paths,
		Evidence:          cfg.Evidence,
		ModuleBindings:    cfg.ModuleBindings,
		ModuleAliases:     cfg.ModuleAliases,
		RecursiveFamilies: cfg.RecursiveFamilies,
	}

	engine := &Engine{
		Synthesizer: extract.NewSynthesizer(deps, mode),
		deps:        deps,
	}
	engine.narrowTypeQuery = db.NewQuery("check.synth.narrow-type-of", func(ctx *db.QueryContext, key exprTypeQueryKey) typ.Type {
		queryEngine := engine.withQueryContext(ctx)
		return queryEngine.SynthExpr(key.Expr, key.Point, queryEngine.deps.Flow)
	}, typ.TypeEquals)
	return engine
}

func (e *Engine) withQueryContext(ctx *db.QueryContext) *Engine {
	if e == nil || e.deps == nil || e.Synthesizer == nil || e.deps.Ctx == ctx {
		return e
	}
	next := *e
	next.Synthesizer = e.Synthesizer.WithQueryContext(ctx)
	next.deps = next.Synthesizer.Deps()
	return &next
}

// TypeOf returns the synthesized type of an expression at a CFG point.
//
// In flow mode, queries the narrow cache first, then synthesizes
// using flow information and caches the result. In declared/static mode,
// delegates to the extract synthesizer without flow context.
//
// Returns typ.Nil for nil expressions. Returns typ.Unknown if synthesis fails.
func (e *Engine) TypeOf(expr ast.Expr, p cfg.Point) typ.Type {
	if e.IsNarrowing() {
		if expr == nil {
			return typ.Nil
		}
		if e.narrowTypeQuery == nil {
			return e.SynthExpr(expr, p, e.deps.Flow)
		}
		return e.narrowTypeQuery.Get(e.deps.Ctx, exprTypeQueryKey{Expr: expr, Point: p})
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

// Narrow returns a flow-capable synth if flow information is available and the
// engine is in SynthModeFlow.
//
// Returns nil if the engine was created without flow information or the mode is
// declared/static. Returns self if flow information is present and mode is flow.
func (e *Engine) Narrow() api.BaseSynth {
	if e.deps.Flow == nil || !e.IsNarrowing() {
		return nil
	}
	return e
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
		ModuleBindings: e.deps.ModuleBindings,
		ModuleAliases:  e.deps.ModuleAliases,
	})
	return resolver.ResolveTypeDef(name, typeExpr, typeParams, sc)
}

// ResolveFieldAccess resolves field or index access on a type.
//
// Handles both named field access (t.field) and computed index access (t[key]).
// Returns the result type and whether the access is valid.
func (e *Engine) ResolveFieldAccess(fullExpr *ast.AttrGetExpr, objType typ.Type, fieldName string, p cfg.Point) FieldAccessResult {
	return ResolveFieldAccess(e, fullExpr, objType, fieldName, p)
}
