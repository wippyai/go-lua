package returns

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/check/synth"
	phasecore "github.com/wippyai/go-lua/compiler/check/synth/phase/core"
	"github.com/wippyai/go-lua/types/typ"
)

// BuildSeedFunctionTypeWithBindings builds a placeholder function type for an
// SCC sibling that has no return summary yet.
//
// Optional binder metadata enables implicit-self detection in method definitions.
func BuildSeedFunctionTypeWithBindings(
	fn *ast.FunctionExpr,
	engine *synth.Engine,
	parentScope *scope.State,
	bindings phasecore.ParamSymbolLookup,
) typ.Type {
	if fn == nil {
		return nil
	}

	resolveScope := parentScope
	if resolveScope == nil {
		resolveScope = scope.New()
	}

	resolveType := func(expr ast.TypeExpr, sc *scope.State) typ.Type {
		if engine == nil || expr == nil || sc == nil {
			return nil
		}
		return engine.ResolveType(expr, sc)
	}

	if len(fn.TypeParams) > 0 {
		tps := make(map[string]typ.Type, len(fn.TypeParams))
		for _, tp := range fn.TypeParams {
			var constr typ.Type
			if tp.Constraint != nil {
				constr = resolveType(tp.Constraint, resolveScope)
			}
			tps[tp.Name] = typ.NewTypeParam(tp.Name, constr)
		}
		resolveScope = resolveScope.WithTypeParams(tps)
	}

	builder := typ.Func()
	for _, tp := range fn.TypeParams {
		var constr typ.Type
		if tp.Constraint != nil {
			constr = resolveType(tp.Constraint, resolveScope)
		}
		builder = builder.TypeParam(tp.Name, constr)
	}

	if phasecore.HasImplicitSelfParam(fn, bindings) {
		selfType := typ.Unknown
		if st := resolveScope.SelfType(); st != nil {
			selfType = st
		}
		builder = builder.Param("self", selfType)
	}

	if fn.ParList != nil {
		for i, name := range fn.ParList.Names {
			paramType := typ.Unknown
			if i < len(fn.ParList.Types) && fn.ParList.Types[i] != nil {
				if t := resolveType(fn.ParList.Types[i], resolveScope); t != nil {
					paramType = t
				}
			}
			builder = builder.Param(name, paramType)
		}
		if fn.ParList.HasVargs {
			varargType := typ.Unknown
			if fn.ParList.VarargType != nil {
				if t := resolveType(fn.ParList.VarargType, resolveScope); t != nil {
					varargType = t
				}
			}
			builder = builder.Variadic(varargType)
		}
	}

	if len(fn.ReturnTypes) > 0 {
		rets := make([]typ.Type, len(fn.ReturnTypes))
		if engine != nil {
			rets = engine.ResolveReturnTypes(fn.ReturnTypes, resolveScope)
		}
		if len(rets) > 0 {
			for i, t := range rets {
				if t == nil {
					rets[i] = typ.Unknown
				}
			}
			builder = builder.Returns(rets...)
		} else {
			builder = builder.Returns(typ.Unknown)
		}
	} else {
		builder = builder.Returns(typ.Unknown)
	}

	return builder.Build()
}
