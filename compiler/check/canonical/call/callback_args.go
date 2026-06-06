package call

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	phasecore "github.com/wippyai/go-lua/compiler/check/synth/core"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
	"github.com/wippyai/go-lua/types/callboundary"
	"github.com/wippyai/go-lua/types/db"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// CallbackArgRefinementProjection is the canonical call-boundary normalizer for
// higher-order arguments. It lets imported/generic call inference see the
// signature actually proven for a callback under the callee's expected entry
// context, without making call inference read program or driver state.
type CallbackArgRefinementProjection struct {
	Call         *ast.FuncCallExpr
	ArgTypes     []typ.Type
	ExpectedArgs []typ.Type

	CallbackRefs       func(ast.Expr) ([]summary.FuncRef, bool)
	FunctionType       func(summary.FuncRef) typ.Type
	ContextualFunction func(summary.FuncRef, summary.EntryValues) typ.Type
}

// RefinedTypes replaces only shallow/gradual function argument types
// with solved callback-value signatures. Concrete source annotations remain the
// authority for the public function type.
func (p CallbackArgRefinementProjection) RefinedTypes() []typ.Type {
	if p.Call == nil || len(p.Call.Args) == 0 || p.CallbackRefs == nil {
		return p.ArgTypes
	}
	out := append([]typ.Type(nil), p.ArgTypes...)
	if len(out) < len(p.Call.Args) {
		out = append(out, make([]typ.Type, len(p.Call.Args)-len(out))...)
	}
	changed := false
	for i, arg := range p.Call.Args {
		argRefs, ok := p.CallbackRefs(arg)
		if !ok || len(argRefs) == 0 {
			continue
		}
		entryValues := expectedCallbackEntryValues(p.ExpectedArgs, i)
		acc := product.Domain.Bottom()
		for _, ref := range argRefs {
			t := typ.Type(nil)
			if p.FunctionType != nil {
				t = p.FunctionType(ref)
			}
			if len(entryValues) != 0 && p.ContextualFunction != nil {
				if contextual := p.ContextualFunction(ref, entryValues); !typ.IsAbsentOrUnknown(contextual) {
					t = contextual
				}
			}
			if typ.IsAbsentOrUnknown(t) {
				continue
			}
			if i < len(p.ExpectedArgs) {
				t = callboundary.ProjectContextualFunctionArg(p.ExpectedArgs[i], t)
			}
			acc = product.Domain.Join(acc, product.FromType(t))
		}
		if product.Domain.Equal(acc, product.Domain.Bottom()) {
			continue
		}
		candidate := product.ProjectValueOrUnknown(acc)
		if !shouldUseRefinedFunctionArg(out[i], candidate) {
			continue
		}
		out[i] = candidate
		changed = true
	}
	if !changed {
		return p.ArgTypes
	}
	return out
}

// ShallowArgTypes builds the staged inference argument vector for a call. Direct
// callback literals are kept shallow so their body demand cannot solve callee
// generics before the callee provides the expected callback signature.
func ShallowArgTypes(args []ast.Expr, projected []typ.Type, exprType func(ast.Expr) typ.Type) []typ.Type {
	if len(args) == 0 {
		return nil
	}
	out := make([]typ.Type, len(args))
	for i, arg := range args {
		if fn, ok := arg.(*ast.FunctionExpr); ok {
			out[i] = phasecore.ShallowFunctionLiteralSignature(fn)
			continue
		}
		if i < len(projected) && projected[i] != nil && !typ.IsUnknown(projected[i]) {
			out[i] = projected[i]
			continue
		}
		if exprType != nil {
			out[i] = exprType(arg)
		}
		if out[i] == nil {
			out[i] = typ.Unknown
		}
	}
	return out
}

// ExpectedArgProjection computes the expected argument contract produced by the
// ordinary call matcher. Callback argument normalization consumes this as entry
// evidence for nested callback bodies.
type ExpectedArgProjection struct {
	Call                *ast.FuncCallExpr
	ArgTypes            []typ.Type
	CallbackArg         func(ast.Expr) bool
	Resolver            TypeResolver
	Ctx                 *db.QueryContext
	Query               core.TypeOps
	Callee              typ.Type
	IsMethod            bool
	MethodName          string
	MethodReceiverType  typ.Type
	ForceMethodReceiver bool
	ResolveTypeArg      func(ast.TypeExpr) typ.Type
}

// ExpectedTypes returns the callee-visible argument types inferred by
// the same generic matcher that will later synthesize call returns.
func (p ExpectedArgProjection) ExpectedTypes() []typ.Type {
	if p.Call == nil || p.Query == nil {
		return nil
	}
	def := ops.CallDef{
		Args:  callArgTypesForExpectedArgProjection(p),
		Query: p.Query,
	}
	if len(p.Call.TypeArgs) > 0 {
		def.TypeArgs = resolvedTypeArgs(p.Call.TypeArgs, p.ResolveTypeArg)
	}
	isMethod := p.IsMethod || p.Call.Method != ""
	methodName := p.MethodName
	if methodName == "" {
		methodName = p.Call.Method
	}
	if isMethod {
		def.IsMethod = true
		def.Receiver = p.MethodReceiverType
		if def.Receiver == nil || typ.IsAbsentOrUnknown(def.Receiver) {
			def.Receiver = p.Resolver.ResolveReceiver(p.Call.Receiver)
		}
		def.MethodName = methodName
		def.ForceMethodReceiver = p.ForceMethodReceiver
	} else if p.Callee != nil {
		def.Callee = p.Callee
	} else {
		def.Callee = p.Resolver.ResolveCallee(p.Call.Func)
	}
	inferred := ops.InferCall(p.Ctx, def)
	if len(inferred.ExpectedArgs) == 0 && inferred.ExpectedVariadic == nil {
		return nil
	}
	out := make([]typ.Type, len(p.Call.Args))
	for i := range p.Call.Args {
		out[i] = inferred.ExpectedArgType(i)
	}
	return out
}

func callArgTypesForExpectedArgProjection(p ExpectedArgProjection) []typ.Type {
	if p.Call == nil {
		return nil
	}
	n := len(p.Call.Args)
	if n <= 0 {
		return nil
	}
	out := make([]typ.Type, n)
	for i := 0; i < n; i++ {
		if p.CallbackArg != nil && p.CallbackArg(p.Call.Args[i]) {
			out[i] = typ.Unknown
		} else if i < len(p.ArgTypes) && p.ArgTypes[i] != nil {
			out[i] = p.ArgTypes[i]
		} else {
			out[i] = typ.Unknown
		}
	}
	return out
}

func expectedCallbackEntryValues(expected []typ.Type, idx int) summary.EntryValues {
	if idx < 0 || idx >= len(expected) {
		return nil
	}
	return summary.EntryValuesFromFunctionParams(unwrap.Function(expected[idx]))
}

func shouldUseRefinedFunctionArg(current, candidate typ.Type) bool {
	if candidate == nil || typ.IsAbsentOrUnknown(candidate) || typ.IsAny(candidate) {
		return false
	}
	if current == nil || typ.IsAbsentOrUnknown(current) || typ.IsAny(current) {
		return true
	}
	currentFn := unwrap.Function(current)
	candidateFn := unwrap.Function(candidate)
	if currentFn == nil || candidateFn == nil {
		return false
	}
	if len(currentFn.Params) != len(candidateFn.Params) || len(currentFn.Returns) != len(candidateFn.Returns) {
		return false
	}
	for _, param := range currentFn.Params {
		if typ.IsAbsentOrUnknown(param.Type) || typ.IsAny(param.Type) {
			return true
		}
	}
	for _, ret := range currentFn.Returns {
		if typ.IsAbsentOrUnknown(ret) || typ.IsAny(ret) {
			return true
		}
	}
	return false
}
