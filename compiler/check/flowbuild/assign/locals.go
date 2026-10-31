package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	fbcore "github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// UniformScopes maps every point of graph to base. A function body inferred
// before its own scopes exist resolves names in its definition scope.
func UniformScopes(graph *cfg.Graph, base *scope.State) map[cfg.Point]*scope.State {
	if graph == nil {
		return nil
	}
	scopes := make(map[cfg.Point]*scope.State)
	graph.EachNode(func(p cfg.Point, _ cfg.NodeInfo) {
		scopes[p] = base
	})
	scopes[graph.Entry()] = base
	return scopes
}

// AnnotatedSymbols lists the symbols of graph whose type local inference
// keeps: parameters, symbols with a non-soft overlay type, and locals with a
// non-soft declared annotation.
func AnnotatedSymbols(graph *cfg.Graph, overlay api.SpecTypes, resolveAnnotation func(ast.TypeExpr) typ.Type) map[cfg.SymbolID]bool {
	annotated := make(map[cfg.SymbolID]bool, len(overlay))
	if graph == nil {
		return annotated
	}
	paramSet := make(map[cfg.SymbolID]bool)
	for _, sym := range graph.ParamSymbols() {
		if sym != 0 {
			paramSet[sym] = true
		}
	}
	for sym, tp := range overlay {
		if paramSet[sym] {
			annotated[sym] = true
			continue
		}
		if tp != nil && !typ.IsUnresolved(tp) && !typ.IsSoft(tp, typ.SoftAnnotationPolicy) {
			annotated[sym] = true
		}
	}
	graph.EachAssign(func(_ cfg.Point, info *cfg.AssignInfo) {
		if info == nil || len(info.TypeAnnotations) == 0 {
			return
		}
		for idx, target := range info.Targets {
			if target.Kind != cfg.TargetIdent || target.Symbol == 0 {
				continue
			}
			if idx >= len(info.TypeAnnotations) || info.TypeAnnotations[idx] == nil {
				continue
			}
			if tp, ok := overlay[target.Symbol]; ok && tp != nil {
				if !typ.IsSoft(tp, typ.SoftAnnotationPolicy) {
					annotated[target.Symbol] = true
				}
			} else if resolveAnnotation != nil {
				if resolved := resolveAnnotation(info.TypeAnnotations[idx]); resolved != nil && !typ.IsSoft(resolved, typ.SoftAnnotationPolicy) {
					annotated[target.Symbol] = true
				}
			}
		}
	})
	return annotated
}

// FunctionLocals infers the local variable types of a function body
// synthesized before its own flow. engine reads the function's declared
// overlay through env; symbols in annotated keep their overlay types.
func FunctionLocals(
	graph *cfg.Graph,
	scopes map[cfg.Point]*scope.State,
	engine api.SynthAPI,
	env api.BaseEnv,
	callCtx *db.QueryContext,
	typeOps core.TypeOps,
	overlay api.SpecTypes,
	annotated map[cfg.SymbolID]bool,
) api.SpecTypes {
	symResolver := func(p cfg.Point, sym cfg.SymbolID) (typ.Type, bool) {
		if env == nil || env.Types() == nil {
			return nil, false
		}
		tv := env.Types().EffectiveTypeAt(p, sym)
		if tv.State == flow.StateResolved && tv.Type != nil {
			return tv.Type, true
		}
		if t, ok := env.GlobalType(sym); ok && t != nil {
			return t, true
		}
		return nil, false
	}
	return CollectInferredTypes(&fbcore.FlowContext{
		Graph:   graph,
		Scopes:  scopes,
		API:     engine,
		CallCtx: callCtx,
		TypeOps: typeOps,
		Derived: &fbcore.Derived{
			SymResolver: symResolver,
		},
	}, overlay, annotated, nil)
}
