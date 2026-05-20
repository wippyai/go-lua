package core

import (
	"reflect"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/scope"
	typecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

// ParamListConfig configures ApplyParamList.
// Expected is only used for unannotated params to provide contextual types.
type ParamListConfig struct {
	ResolveType  func(expr ast.TypeExpr, sc *scope.State) typ.Type
	ResolveScope *scope.State
	Expected     *typ.Function
	// ImplicitSelf prepends a required `self` parameter before source params.
	// Use for method definitions that bind an implicit receiver.
	ImplicitSelf bool
	// ImplicitSelfType is used for the prepended `self` parameter.
	// When nil, `unknown` is used.
	ImplicitSelfType typ.Type
	// UntypedParamType is used for unannotated source parameters without an
	// expected type. When nil, `unknown` is used.
	UntypedParamType typ.Type
}

// ParamSymbolLookup exposes parameter symbol layout for a function expression.
type ParamSymbolLookup interface {
	ParamSymbols(fn *ast.FunctionExpr) []typecfg.SymbolID
	Name(sym typecfg.SymbolID) string
}

// HasExplicitSelfParam reports whether source declares an explicit leading self parameter.
func HasExplicitSelfParam(fn *ast.FunctionExpr) bool {
	if fn == nil || fn.ParList == nil || len(fn.ParList.Names) == 0 {
		return false
	}
	return fn.ParList.Names[0] == "self"
}

// HasUnannotatedExplicitSelfParam reports whether source declares explicit self
// with no type annotation on the first parameter.
func HasUnannotatedExplicitSelfParam(fn *ast.FunctionExpr) bool {
	if !HasExplicitSelfParam(fn) {
		return false
	}
	return len(fn.ParList.Types) == 0 || fn.ParList.Types[0] == nil
}

// HasImplicitSelfParam reports whether binder introduced an implicit leading
// `self` parameter for this function (method definition sugar).
func HasImplicitSelfParam(fn *ast.FunctionExpr, bindings ParamSymbolLookup) bool {
	if fn == nil || fn.ParList == nil || bindings == nil {
		return false
	}
	v := reflect.ValueOf(bindings)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if v.IsNil() {
			return false
		}
	}
	paramSyms := bindings.ParamSymbols(fn)
	if len(paramSyms) != len(fn.ParList.Names)+1 || len(paramSyms) == 0 {
		return false
	}
	first := paramSyms[0]
	if first == 0 || bindings.Name(first) != "self" {
		return false
	}
	return len(fn.ParList.Names) == 0 || fn.ParList.Names[0] != "self"
}

// HasSelfParam reports whether function's effective parameter list includes self,
// either explicitly in source or implicitly via method-definition sugar.
func HasSelfParam(fn *ast.FunctionExpr, bindings ParamSymbolLookup) bool {
	return HasExplicitSelfParam(fn) || HasImplicitSelfParam(fn, bindings)
}

// HasUnannotatedSelfParam reports whether function has a leading self parameter
// with no source annotation, including implicit self introduced by binder.
func HasUnannotatedSelfParam(fn *ast.FunctionExpr, bindings ParamSymbolLookup) bool {
	if HasUnannotatedExplicitSelfParam(fn) {
		return true
	}
	return HasImplicitSelfParam(fn, bindings)
}

// ApplyParamList applies parameter and source-vararg rules to the builder.
// Unannotated params are optional in Lua; discarded extra-argument acceptance is
// a call-site rule, not a fake variadic slot in the function type.
func ApplyParamList(builder *typ.FunctionBuilder, fn *ast.FunctionExpr, cfg ParamListConfig) {
	if builder == nil || fn == nil || fn.ParList == nil {
		return
	}
	builder.ReserveParams(paramListCapacity(fn, cfg.ImplicitSelf))

	shiftExpected := false
	if cfg.ImplicitSelf {
		selfType := cfg.ImplicitSelfType
		if selfType == nil {
			selfType = typ.Unknown
		}
		builder.Param("self", selfType)
		if cfg.Expected != nil && len(cfg.Expected.Params) > 0 && cfg.Expected.Params[0].Name == "self" {
			shiftExpected = true
		}
	}

	for i, name := range fn.ParList.Names {
		paramType := typ.Unknown
		isOptional := false
		expectedIdx := i
		if shiftExpected {
			expectedIdx = i + 1
		}

		var typeExpr ast.TypeExpr
		if fn.ParList.Types != nil && i < len(fn.ParList.Types) {
			typeExpr = fn.ParList.Types[i]
		}
		if typeExpr != nil {
			if _, ok := typeExpr.(*ast.OptionalTypeExpr); ok {
				isOptional = true
			}
			if cfg.ResolveType != nil && cfg.ResolveScope != nil {
				if t := cfg.ResolveType(typeExpr, cfg.ResolveScope); t != nil {
					paramType = t
					// Soft annotations allow expected types to refine the parameter.
					if typ.IsRefinableAnnotation(t) {
						if expectedType, expectedOptional, ok := expectedParamTypeAt(cfg.Expected, expectedIdx); ok {
							paramType = expectedType
							isOptional = expectedOptional
						}
					}
				}
			}
		} else if expectedType, expectedOptional, ok := expectedParamTypeAt(cfg.Expected, expectedIdx); ok {
			paramType = expectedType
			isOptional = expectedOptional
		} else {
			// Unannotated params are optional in Lua (missing args become nil).
			if cfg.UntypedParamType != nil {
				paramType = cfg.UntypedParamType
			}
			isOptional = true
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
	}
}

func expectedParamTypeAt(expected *typ.Function, idx int) (typ.Type, bool, bool) {
	if expected == nil || idx < 0 {
		return nil, false, false
	}
	if idx < len(expected.Params) {
		p := expected.Params[idx]
		return p.Type, p.Optional, true
	}
	if expected.Variadic != nil {
		return expected.Variadic, true, true
	}
	return nil, false, false
}

func paramListCapacity(fn *ast.FunctionExpr, implicitSelf bool) int {
	if fn == nil || fn.ParList == nil {
		return 0
	}
	n := len(fn.ParList.Names)
	if implicitSelf {
		n++
	}
	return n
}
