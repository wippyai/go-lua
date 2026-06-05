package call

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/synth/ops"
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
		entryValues := ExpectedCallbackEntryValues(in.ExpectedArgs, i)
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
			acc = product.Domain.Join(acc, product.FromType(t))
		}
		if product.Domain.Equal(acc, product.Domain.Bottom()) {
			continue
		}
		candidate := product.ProjectValueOrUnknown(acc)
		if !ShouldUseRefinedFunctionArg(out[i], candidate) {
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

// ExpectedArgsInput computes the expected argument contract produced by the
// ordinary call matcher. Callback argument normalization consumes this as entry
// evidence for nested callback bodies.
type ExpectedArgsInput struct {
	Call               *ast.FuncCallExpr
	ArgTypes           []typ.Type
	Resolver           TypeResolver
	Ctx                *db.QueryContext
	Query              core.TypeOps
	MethodReceiverType typ.Type
	ResolveTypeArg     func(ast.TypeExpr) typ.Type
}

// ExpectedArgTypesForCall returns the callee-visible argument types inferred by
// the same generic matcher that will later synthesize call returns.
func ExpectedArgTypesForCall(in ExpectedArgsInput) []typ.Type {
	if in.Call == nil || in.Query == nil {
		return nil
	}
	def := ops.CallDef{
		Args:  normalizedCallArgTypesForExpectation(in.ArgTypes, len(in.Call.Args)),
		Query: in.Query,
	}
	if len(in.Call.TypeArgs) > 0 {
		def.TypeArgs = resolvedTypeArgs(in.Call.TypeArgs, in.ResolveTypeArg)
	}
	if in.Call.Method != "" {
		def.IsMethod = true
		def.Receiver = in.MethodReceiverType
		if def.Receiver == nil || typ.IsAbsentOrUnknown(def.Receiver) {
			def.Receiver = in.Resolver.ResolveReceiver(in.Call.Receiver)
		}
		def.MethodName = in.Call.Method
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

func normalizedCallArgTypesForExpectation(argTypes []typ.Type, n int) []typ.Type {
	if n <= 0 {
		return nil
	}
	out := make([]typ.Type, n)
	for i := 0; i < n; i++ {
		if i < len(argTypes) && argTypes[i] != nil {
			out[i] = argTypes[i]
		} else {
			out[i] = typ.Unknown
		}
	}
	return out
}

// ExpectedCallbackEntryValues projects a callback parameter contract into the
// summary entry-value carrier used to resummarize that callback at this call.
func ExpectedCallbackEntryValues(expected []typ.Type, idx int) summary.EntryValues {
	if idx < 0 || idx >= len(expected) {
		return nil
	}
	fn := unwrap.Function(expected[idx])
	if fn == nil || len(fn.Params) == 0 {
		return nil
	}
	var values summary.EntryValues
	for slot, param := range fn.Params {
		if param.Type == nil || typ.IsAbsentOrUnknown(param.Type) || typ.IsAny(param.Type) || typ.ContainsTypeParam(param.Type) {
			continue
		}
		values = summary.JoinEntryValue(values, slot, product.FromType(param.Type))
	}
	return values
}

// ShouldUseRefinedFunctionArg admits solved callback signatures only when they
// fill gradual holes in the argument type observed by the surrounding call.
func ShouldUseRefinedFunctionArg(current, candidate typ.Type) bool {
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
