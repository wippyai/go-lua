package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// returnSynth is the api.Synth the WithReturn / WithExhaustiveness passes read. It
// is a facade over the two real components of the observation surface: the
// driver's annotation resolver (declared type/return resolution) and the canonical
// observation Projector (expression typing). It introduces no independent type
// logic; every method delegates to one of those two.
type returnSynth struct {
	driver *Driver
	obs    api.ExprSynth
	ctx    *db.QueryContext
}

// compile-time assertion: returnSynth satisfies api.Synth.
var _ api.Synth = (*returnSynth)(nil)

func (s *returnSynth) TypeOf(expr ast.Expr, p cfg.Point) typ.Type {
	if s.obs == nil {
		return typ.Unknown
	}
	return s.obs(expr, p)
}

func (s *returnSynth) TypeOfWithExpected(expr ast.Expr, p cfg.Point, _ typ.Type) typ.Type {
	return s.TypeOf(expr, p)
}

func (s *returnSynth) MultiTypeOf(expr ast.Expr, p cfg.Point) []typ.Type {
	return []typ.Type{s.TypeOf(expr, p)}
}

func (s *returnSynth) FunctionType(*ast.FunctionExpr, *scope.State) *typ.Function { return nil }

func (s *returnSynth) ExpandValues(exprs []ast.Expr, needed int, p cfg.Point) []typ.Type {
	out := make([]typ.Type, 0, needed)
	for i := 0; i < needed; i++ {
		if i < len(exprs) {
			out = append(out, s.TypeOf(exprs[i], p))
		} else {
			out = append(out, typ.Nil)
		}
	}
	return out
}

func (s *returnSynth) InferIterVars(_ []ast.Expr, count int, _ cfg.Point) []typ.Type {
	out := make([]typ.Type, count)
	for i := range out {
		out[i] = typ.Unknown
	}
	return out
}

func (s *returnSynth) ResolveType(expr ast.TypeExpr, sc *scope.State) typ.Type {
	if s.driver == nil || s.driver.resolver == nil {
		return typ.Unknown
	}
	if sc == nil {
		sc = s.driver.baseScope()
	}
	return s.driver.resolver.ResolveType(expr, sc)
}

func (s *returnSynth) ResolveReturnTypes(types []ast.TypeExpr, sc *scope.State) []typ.Type {
	out := make([]typ.Type, 0, len(types))
	for _, t := range types {
		if t == nil {
			out = append(out, nil)
			continue
		}
		out = append(out, s.ResolveType(t, sc))
	}
	return out
}

func (s *returnSynth) ResolveFunctionSignature(*ast.FunctionExpr, *scope.State) *typ.Function {
	return nil
}

func (s *returnSynth) ResolveTypeDef(name string, typeExpr ast.TypeExpr, typeParams []ast.TypeParamExpr, sc *scope.State) typ.Type {
	if s.driver == nil || s.driver.resolver == nil {
		return typ.Unknown
	}
	if sc == nil {
		sc = s.driver.baseScope()
	}
	return s.driver.resolver.ResolveTypeDef(name, typeExpr, typeParams, sc)
}

// Narrow returns the same facade: the canonical observation Projector is already
// the flow-refined view (it reads the converged flow-refined per-point types).
func (s *returnSynth) Narrow() api.BaseSynth { return s }

func (s *returnSynth) WithFlow(api.FlowOps) api.BaseSynth { return s }

func (s *returnSynth) Method(t typ.Type, name string) (typ.Type, bool) {
	if s != nil && s.driver != nil && s.driver.cfg.Types != nil && s.ctx != nil {
		return s.driver.cfg.Types.Method(s.ctx, t, name)
	}
	return querycore.Method(t, name)
}

func (s *returnSynth) Field(t typ.Type, name string) (typ.Type, bool) {
	if s != nil && s.driver != nil && s.driver.cfg.Types != nil && s.ctx != nil {
		return s.driver.cfg.Types.Field(s.ctx, t, name)
	}
	return querycore.Field(t, name)
}

func (s *returnSynth) SynthWithExpected(expr ast.Expr, p cfg.Point, _ typ.Type) typ.Type {
	return s.TypeOf(expr, p)
}

func (s *returnSynth) CallQuery() querycore.TypeOps { return s.driver.cfg.Types }

func (s *returnSynth) AllowReturnTransforms() bool { return false }

func (s *returnSynth) Context() *db.QueryContext { return s.ctx }

// buildObservationInputs assembles the per-function flow.Inputs the diagnostic
// passes read directly (DeclaredTypes / AnnotatedVars), backed by the resolved
// declared-type context.
func buildObservationInputs(g *cfg.Graph, obsCtx functionObservationContext) *flow.Inputs {
	in := &flow.Inputs{
		DeclaredTypes: make(map[cfg.SymbolID]typ.Type, len(obsCtx.declared)),
		BindingTypes:  make(map[cfg.SymbolID]typ.Type, len(obsCtx.bindings)),
	}
	if g != nil {
		in.Graph = g
	}
	for sym, t := range obsCtx.declared {
		in.DeclaredTypes[sym] = t
	}
	for _, sym := range obsCtx.annotated.Symbols() {
		in.AnnotatedVars.Add(sym)
	}
	for sym, t := range obsCtx.bindings {
		in.BindingTypes[sym] = t
	}
	return in
}
