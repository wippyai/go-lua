package call

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// DemandFunctionProjection supplies the call-boundary context for resolving the
// callable shape used by signature-owned argument obligations.
type DemandFunctionProjection struct {
	Call            *ast.FuncCallExpr
	SummaryFunction func(*ast.FuncCallExpr) *typ.Function
	Resolver        TypeResolver
}

// Function resolves the callable shape used for argument demand projection.
// Summary-known module targets win; otherwise the resolved caller-visible
// callee/receiver type is unwrapped to a function shape.
func (p DemandFunctionProjection) Function() *typ.Function {
	call := p.Call
	if call == nil {
		return nil
	}
	if p.SummaryFunction != nil {
		if fn := p.SummaryFunction(call); fn != nil {
			return fn
		}
	}
	var callee typ.Type
	if call.Method != "" {
		receiver := p.Resolver.ResolveReceiver(call.Receiver)
		if receiver == nil || typ.IsAbsentOrUnknown(receiver) {
			return nil
		}
		member, ok := p.Resolver.method(receiver, call.Method)
		if !ok {
			return nil
		}
		callee = member
	} else {
		callee = p.Resolver.ResolveCallee(call.Func)
		if functionShape(callee) == nil {
			callee = p.Resolver.ResolveStaticCallee(call.Func)
		}
	}
	return functionShape(callee)
}

func functionShape(t typ.Type) *typ.Function {
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
