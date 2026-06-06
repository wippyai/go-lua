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

// CallbackArgRefinementInput is the canonical call-boundary normalizer for
// higher-order arguments. It lets imported/generic call inference see the
// signature actually proven for a callback under the callee's expected entry
// context, without making call inference read program or driver state.
type CallbackArgRefinementInput struct {
	Call         *ast.FuncCallExpr
	ArgTypes     []typ.Type
	ExpectedArgs []typ.Type

	CallbackRefs       func(ast.Expr) ([]summary.FuncRef, bool)
	FunctionType       func(summary.FuncRef) typ.Type
	ContextualFunction func(summary.FuncRef, summary.EntryValues) typ.Type
}

// RefineCallbackArgTypes replaces only shallow/gradual function argument types
// with solved callback-value signatures. Concrete source annotations remain the
// authority for the public function type.
func RefineCallbackArgTypes(in CallbackArgRefinementInput) []typ.Type {
	if in.Call == nil || len(in.Call.Args) == 0 || in.CallbackRefs == nil {
		return in.ArgTypes
	}
	out := append([]typ.Type(nil), in.ArgTypes...)
	if len(out) < len(in.Call.Args) {
		out = append(out, make([]typ.Type, len(in.Call.Args)-len(out))...)
	}
	changed := false
	for i, arg := range in.Call.Args {
		argRefs, ok := in.CallbackRefs(arg)
		if !ok || len(argRefs) == 0 {
			continue
		}
		entryValues := expectedCallbackEntryValues(in.ExpectedArgs, i)
		acc := product.Domain.Bottom()
		for _, ref := range argRefs {
			t := typ.Type(nil)
			if in.FunctionType != nil {
				t = in.FunctionType(ref)
			}
			if len(entryValues) != 0 && in.ContextualFunction != nil {
				if contextual := in.ContextualFunction(ref, entryValues); !typ.IsAbsentOrUnknown(contextual) {
					t = contextual
				}
			}
			if typ.IsAbsentOrUnknown(t) {
				continue
			}
			if i < len(in.ExpectedArgs) {
				t = callboundary.ProjectContextualFunctionArg(in.ExpectedArgs[i], t)
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
		return in.ArgTypes
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

// ExpectedArgsInput computes the expected argument contract produced by the
// ordinary call matcher. Callback argument normalization consumes this as entry
// evidence for nested callback bodies.
type ExpectedArgsInput struct {
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

// ExpectedArgTypesForCall returns the callee-visible argument types inferred by
// the same generic matcher that will later synthesize call returns.
func ExpectedArgTypesForCall(in ExpectedArgsInput) []typ.Type {
	if in.Call == nil || in.Query == nil {
		return nil
	}
	def := ops.CallDef{
		Args:  callArgTypesForExpectedArgProjection(in),
		Query: in.Query,
	}
	if len(in.Call.TypeArgs) > 0 {
		def.TypeArgs = resolvedTypeArgs(in.Call.TypeArgs, in.ResolveTypeArg)
	}
	isMethod := in.IsMethod || in.Call.Method != ""
	methodName := in.MethodName
	if methodName == "" {
		methodName = in.Call.Method
	}
	if isMethod {
		def.IsMethod = true
		def.Receiver = in.MethodReceiverType
		if def.Receiver == nil || typ.IsAbsentOrUnknown(def.Receiver) {
			def.Receiver = in.Resolver.ResolveReceiver(in.Call.Receiver)
		}
		def.MethodName = methodName
		def.ForceMethodReceiver = in.ForceMethodReceiver
	} else if in.Callee != nil {
		def.Callee = in.Callee
	} else {
		def.Callee = in.Resolver.ResolveCallee(in.Call.Func)
	}
	inferred := ops.InferCall(in.Ctx, def)
	if len(inferred.ExpectedArgs) == 0 && inferred.ExpectedVariadic == nil {
		return nil
	}
	out := make([]typ.Type, len(in.Call.Args))
	for i := range in.Call.Args {
		out[i] = inferred.ExpectedArgType(i)
	}
	return out
}

func callArgTypesForExpectedArgProjection(in ExpectedArgsInput) []typ.Type {
	if in.Call == nil {
		return nil
	}
	n := len(in.Call.Args)
	if n <= 0 {
		return nil
	}
	out := make([]typ.Type, n)
	for i := 0; i < n; i++ {
		if in.CallbackArg != nil && in.CallbackArg(in.Call.Args[i]) {
			out[i] = typ.Unknown
		} else if i < len(in.ArgTypes) && in.ArgTypes[i] != nil {
			out[i] = in.ArgTypes[i]
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
