package call

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

type fallbackFunctionShapeInput struct {
	Call *ast.FuncCallExpr

	SummarySignature     typ.Type
	Resolver             TypeResolver
	UseResolvedSignature bool
}

func fallbackFunctionShape(in fallbackFunctionShapeInput) *typ.Function {
	if in.Call == nil {
		return nil
	}
	if fn := functionShape(in.SummarySignature); fn != nil {
		return fn
	}
	var callee typ.Type
	if in.Call.Method != "" {
		receiver := in.Resolver.ResolveReceiver(in.Call.Receiver)
		if receiver == nil || typ.IsAbsentOrUnknown(receiver) {
			return nil
		}
		member, ok := in.Resolver.method(receiver, in.Call.Method)
		if !ok {
			return nil
		}
		callee = member
	} else if in.UseResolvedSignature {
		callee = in.Resolver.ResolveCallee(in.Call.Func)
		if functionShape(callee) == nil {
			callee = in.Resolver.ResolveStaticCallee(in.Call.Func)
		}
	} else {
		callee = in.Resolver.ResolveStaticCallee(in.Call.Func)
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
