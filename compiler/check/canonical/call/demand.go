package call

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// DemandFunctionInput supplies the call boundary context for projecting
// signature-owned argument obligations.
type DemandFunctionInput struct {
	Call            *ast.FuncCallExpr
	SummaryFunction func(*ast.FuncCallExpr) *typ.Function
	Resolver        TypeResolver
}

// FunctionForDemand resolves the callable function shape used for argument
// demand projection. Summary-known module targets win; otherwise the resolved
// caller-visible callee/receiver type is unwrapped to a function shape.
func FunctionForDemand(in DemandFunctionInput) *typ.Function {
	call := in.Call
	if call == nil {
		return nil
	}
	if in.SummaryFunction != nil {
		if fn := in.SummaryFunction(call); fn != nil {
			return fn
		}
	}
	var callee typ.Type
	if call.Method != "" {
		receiver := in.Resolver.ResolveReceiver(call.Receiver)
		if receiver == nil || typ.IsAbsentOrUnknown(receiver) {
			return nil
		}
		member, ok := in.Resolver.method(receiver, call.Method)
		if !ok {
			return nil
		}
		callee = member
	} else {
		callee = in.Resolver.ResolveCallee(call.Func)
		if FunctionShape(callee) == nil {
			callee = in.Resolver.ResolveStaticCallee(call.Func)
		}
	}
	return FunctionShape(callee)
}

// FunctionShape unwraps a caller-visible callable type into a function shape.
func FunctionShape(t typ.Type) *typ.Function {
	if t == nil || typ.IsAbsentOrUnknown(t) || typ.IsAny(t) {
		return nil
	}
	if fn := unwrap.Function(t); fn != nil {
		return fn
	}
	if expanded := subst.ExpandInstantiated(t); expanded != t {
		return unwrap.Function(expanded)
	}
	return nil
}
