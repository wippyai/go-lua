package core

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

// ParamListConfig configures ApplyParamList.
// Expected is only used for unannotated params to provide contextual types.
type ParamListConfig struct {
	ResolveType  func(expr ast.TypeExpr, sc *scope.State) typ.Type
	ResolveScope *scope.State
	Expected     *typ.Function
}

// ApplyParamList applies parameter and vararg rules to the builder.
// Unannotated params are optional in Lua and unannotated functions accept extra args.
func ApplyParamList(builder *typ.FunctionBuilder, fn *ast.FunctionExpr, cfg ParamListConfig) {
	if builder == nil || fn == nil || fn.ParList == nil {
		return
	}

	hasUntyped := false
	for i, name := range fn.ParList.Names {
		paramType := typ.Any
		isOptional := false

		if fn.ParList.Types != nil && i < len(fn.ParList.Types) {
			if typeExpr := fn.ParList.Types[i]; typeExpr != nil {
				if _, ok := typeExpr.(*ast.OptionalTypeExpr); ok {
					isOptional = true
				}
				if cfg.ResolveType != nil && cfg.ResolveScope != nil {
					if t := cfg.ResolveType(typeExpr, cfg.ResolveScope); t != nil {
						paramType = t
						// Soft annotations allow expected types to refine the parameter.
						if cfg.Expected != nil && i < len(cfg.Expected.Params) && typ.IsSoft(t, typ.SoftAnnotationPolicy) {
							paramType = cfg.Expected.Params[i].Type
							isOptional = cfg.Expected.Params[i].Optional
						}
					}
				}
			}
		} else if cfg.Expected != nil && i < len(cfg.Expected.Params) {
			paramType = cfg.Expected.Params[i].Type
			isOptional = cfg.Expected.Params[i].Optional
		} else {
			// Unannotated params are optional in Lua (missing args become nil).
			isOptional = true
			hasUntyped = true
		}

		if isOptional {
			builder.OptParam(name, paramType)
		} else {
			builder.Param(name, paramType)
		}
	}

	if fn.ParList.HasVargs {
		varargType := typ.Any
		if fn.ParList.VarargType != nil && cfg.ResolveType != nil && cfg.ResolveScope != nil {
			if t := cfg.ResolveType(fn.ParList.VarargType, cfg.ResolveScope); t != nil {
				varargType = t
			}
		} else if cfg.Expected != nil && cfg.Expected.Variadic != nil {
			varargType = cfg.Expected.Variadic
		}
		builder.Variadic(varargType)
	} else if hasUntyped {
		// Unannotated functions accept extra args; treat as variadic any.
		builder.Variadic(typ.Any)
	}
}
