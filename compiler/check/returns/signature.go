package returns

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/scope"
	phasecore "github.com/wippyai/go-lua/compiler/check/synth/phase/core"
	"github.com/wippyai/go-lua/types/typ"
)

type seedTypeResolver interface {
	ResolveType(ast.TypeExpr, *scope.State) typ.Type
	ResolveReturnTypes([]ast.TypeExpr, *scope.State) []typ.Type
}

// BuildPostflowSeedFunctionType builds the public source-declared signature
// seed used when committing the canonical post-flow FunctionFact product.
func BuildPostflowSeedFunctionType(result *api.FuncResult, fn *ast.FunctionExpr) *typ.Function {
	if result == nil || fn == nil {
		return nil
	}
	return result.PublicSeedSignature
}

// BuildSeedFunctionTypeWithBindings builds the source-declared seed signature
// used before a canonical FunctionFact projection is available.
//
// Optional binder metadata enables implicit-self detection in method definitions.
func BuildSeedFunctionTypeWithBindings(
	fn *ast.FunctionExpr,
	engine seedTypeResolver,
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

	implicitSelf := phasecore.HasImplicitSelfParam(fn, bindings)
	var implicitSelfType typ.Type
	if implicitSelf {
		implicitSelfType = typ.Unknown
		if st := resolveScope.SelfType(); st != nil {
			implicitSelfType = st
		}
	}
	phasecore.ApplyParamList(builder, fn, phasecore.ParamListConfig{
		ResolveType:      resolveType,
		ResolveScope:     resolveScope,
		Expected:         nil,
		ImplicitSelf:     implicitSelf,
		ImplicitSelfType: implicitSelfType,
		UntypedParamType: typ.Any,
	})

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
