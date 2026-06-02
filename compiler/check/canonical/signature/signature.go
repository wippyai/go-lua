// Package signature owns canonical callable signature construction.
//
// Driver code supplies source functions, scopes, type resolution, and inferred
// summary-return providers. This package owns how those inputs are lowered into
// typ.Function values: generic type-parameter scope, gradual optional source
// parameters, declared-return authority, inferred-return splicing, and method
// self insertion.
package signature

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
	phasecore "github.com/wippyai/go-lua/compiler/check/synth/phase/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ResolveType resolves a source type expression in a concrete type scope.
type ResolveType func(ast.TypeExpr, *scope.State) typ.Type

// ReturnMode selects the declared/inferred return policy for a callable
// signature. Different boundaries intentionally use different modes.
type ReturnMode uint8

const (
	// ReturnDeclaredOnly builds the source-declared callable shape only. It never
	// consults inferred summary returns.
	ReturnDeclaredOnly ReturnMode = iota
	// ReturnDeclaredThenInferred uses any source return annotation as
	// authoritative; when the function declares no returns, inferred summary
	// returns are spliced in.
	ReturnDeclaredThenInferred
	// ReturnResolvableDeclaredThenInferred preserves the historical ref-signature
	// rule: a declared return list is authoritative only if at least one declared
	// return annotation resolves to a non-nil type; otherwise inferred summary
	// returns are used.
	ReturnResolvableDeclaredThenInferred
)

// Input is the canonical signature-builder input.
type Input struct {
	Function *ast.FunctionExpr
	Method   *cfg.FuncDefInfo
	Base     *scope.State

	ResolveType     ResolveType
	InferredReturns func(*ast.FunctionExpr) []typ.Type
	ReturnMode      ReturnMode
}

// Build returns the canonical function signature for in.
func Build(in Input) *typ.Function {
	fn := in.Function
	if in.Method != nil {
		fn = in.Method.FuncExpr
	}
	if fn == nil {
		return nil
	}
	builder := typ.Func()
	sc := GenericScope(builder, ScopeInput{
		Function:    fn,
		Base:        in.Base,
		ResolveType: in.ResolveType,
	})
	if in.Method != nil {
		phasecore.ApplyParamList(builder, fn, phasecore.ParamListConfig{
			ResolveType:      in.ResolveType,
			ResolveScope:     sc,
			UntypedParamType: typ.Any,
			ImplicitSelf:     true,
			ImplicitSelfType: MethodSelfType(in.Method, sc),
		})
	} else {
		phasecore.ApplyParamList(builder, fn, phasecore.ParamListConfig{
			ResolveType:      in.ResolveType,
			ResolveScope:     sc,
			UntypedParamType: typ.Any,
		})
	}
	if returns := ReturnTypes(ReturnInput{
		Function:        fn,
		Scope:           sc,
		ResolveType:     in.ResolveType,
		InferredReturns: in.InferredReturns,
		Mode:            in.ReturnMode,
	}); len(returns) > 0 {
		builder.Returns(returns...)
	}
	return builder.Build()
}

// ScopeInput is the input for generic type-parameter scope construction.
type ScopeInput struct {
	Function    *ast.FunctionExpr
	Base        *scope.State
	ResolveType ResolveType
}

// GenericScope extends Base with Function's type parameters and records the same
// parameters on builder when builder is non-nil.
func GenericScope(builder *typ.FunctionBuilder, in ScopeInput) *scope.State {
	sc := in.Base
	fn := in.Function
	if fn == nil || len(fn.TypeParams) == 0 {
		return sc
	}
	typeParams := make(map[string]typ.Type, len(fn.TypeParams))
	for _, tp := range fn.TypeParams {
		var constr typ.Type
		if tp.Constraint != nil && in.ResolveType != nil {
			constr = in.ResolveType(tp.Constraint, sc)
		}
		param := typ.NewTypeParam(tp.Name, constr)
		typeParams[tp.Name] = param
		if builder != nil {
			builder.TypeParamRef(param)
		}
	}
	if sc == nil {
		return sc
	}
	return sc.WithTypeParams(typeParams)
}

// TypeParamScope returns the annotation scope for a function body/signature.
func TypeParamScope(in ScopeInput) *scope.State {
	return GenericScope(nil, in)
}

// ReturnInput is the return-vector lowering input.
type ReturnInput struct {
	Function *ast.FunctionExpr
	Scope    *scope.State

	ResolveType     ResolveType
	InferredReturns func(*ast.FunctionExpr) []typ.Type
	Mode            ReturnMode
}

// ReturnTypes lowers a function's source return annotations or inferred summary
// returns according to Mode.
func ReturnTypes(in ReturnInput) []typ.Type {
	fn := in.Function
	if fn == nil {
		return nil
	}
	switch in.Mode {
	case ReturnDeclaredOnly:
		return DeclaredReturnTypes(fn, in.Scope, in.ResolveType)
	case ReturnDeclaredThenInferred:
		if len(fn.ReturnTypes) > 0 {
			return DeclaredReturnTypes(fn, in.Scope, in.ResolveType)
		}
	case ReturnResolvableDeclaredThenInferred:
		if returns, ok := resolvableDeclaredReturnTypes(fn, in.Scope, in.ResolveType); ok {
			return returns
		}
	default:
		if len(fn.ReturnTypes) > 0 {
			return DeclaredReturnTypes(fn, in.Scope, in.ResolveType)
		}
	}
	if in.InferredReturns == nil {
		return nil
	}
	return in.InferredReturns(fn)
}

// DeclaredReturnTypes lowers only source-declared returns.
func DeclaredReturnTypes(fn *ast.FunctionExpr, sc *scope.State, resolve ResolveType) []typ.Type {
	if fn == nil || len(fn.ReturnTypes) == 0 {
		return nil
	}
	return resolveReturnList(fn.ReturnTypes, sc, resolve)
}

func resolvableDeclaredReturnTypes(fn *ast.FunctionExpr, sc *scope.State, resolve ResolveType) ([]typ.Type, bool) {
	if fn == nil || len(fn.ReturnTypes) == 0 {
		return nil, false
	}
	returns := make([]typ.Type, 0, len(fn.ReturnTypes))
	anyResolved := false
	for _, rt := range fn.ReturnTypes {
		if rt == nil {
			returns = append(returns, typ.Unknown)
			continue
		}
		var t typ.Type
		if resolve != nil {
			t = resolve(rt, sc)
		}
		if t == nil {
			t = typ.Unknown
		} else {
			anyResolved = true
		}
		returns = append(returns, t)
	}
	return returns, anyResolved
}

func resolveReturnList(types []ast.TypeExpr, sc *scope.State, resolve ResolveType) []typ.Type {
	if len(types) == 0 {
		return nil
	}
	returns := make([]typ.Type, 0, len(types))
	for _, rt := range types {
		if rt == nil {
			returns = append(returns, typ.Unknown)
			continue
		}
		var t typ.Type
		if resolve != nil {
			t = resolve(rt, sc)
		}
		if t == nil {
			t = typ.Unknown
		}
		returns = append(returns, t)
	}
	return returns
}

// MethodSelfType resolves the implicit self parameter for method definitions:
// a named receiver uses the type namespace binding; otherwise self is gradual.
func MethodSelfType(info *cfg.FuncDefInfo, sc *scope.State) typ.Type {
	if info == nil {
		return typ.Any
	}
	if info.ReceiverName != "" && sc != nil {
		if named, ok := sc.LookupType(info.ReceiverName); ok && named != nil {
			return unwrap.Alias(named)
		}
	}
	return typ.Any
}
