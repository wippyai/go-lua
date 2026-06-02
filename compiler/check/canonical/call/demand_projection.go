package call

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/subst"
)

// CallArgDemandsInput is the canonical call-site policy input for argument
// demand projection. Summary targets are the product fixed-point proof path; a
// callable signature is only the external/type fallback.
type CallArgDemandsInput struct {
	Call *ast.FuncCallExpr

	SummaryDemands func(*ast.FuncCallExpr) ([]callobligation.Obligation, bool)
	FunctionShape  func(*ast.FuncCallExpr) *typ.Function
	SelfType       func(*ast.FuncCallExpr) typ.Type
}

// CallArgExpectedTypesInput supplies the call-site context for contextual
// argument typing. Unlike CallArgDemandsInput, this is not a diagnostic
// precondition projection: a callee body may place no demand on a callback
// parameter while the declared callable signature still gives that callback
// literal its parameter types.
type CallArgExpectedTypesInput struct {
	Call          *ast.FuncCallExpr
	FunctionShape func(*ast.FuncCallExpr) *typ.Function
	SelfType      func(*ast.FuncCallExpr) typ.Type
}

// CallArgDemandsForCall resolves caller-visible argument obligations in
// canonical precision order: selected summary targets first, then callable
// signature contracts. A summary hit with no demands is still authoritative and
// suppresses the signature fallback.
func CallArgDemandsForCall(in CallArgDemandsInput) []callobligation.Obligation {
	if in.Call == nil || len(in.Call.Args) == 0 {
		return nil
	}
	if in.SummaryDemands != nil {
		if demands, ok := in.SummaryDemands(in.Call); ok {
			return demands
		}
	}
	if in.FunctionShape == nil {
		return nil
	}
	var selfType typ.Type
	if in.SelfType != nil {
		selfType = in.SelfType(in.Call)
	}
	return paramevidence.FunctionCallArgObligationsWithSelf(in.Call, in.FunctionShape(in.Call), selfType)
}

// ExpectedArgTypeForCall resolves the declared/contextual type expected for one
// concrete call argument. This powers function-literal callback contexts and is
// intentionally independent of solved body obligations.
func ExpectedArgTypeForCall(in CallArgExpectedTypesInput, argIdx int) typ.Type {
	if in.Call == nil || argIdx < 0 || argIdx >= len(in.Call.Args) || in.FunctionShape == nil {
		return nil
	}
	fn := in.FunctionShape(in.Call)
	if fn == nil {
		return nil
	}
	offset := 0
	if in.Call.Method != "" {
		offset = 1
	}
	paramIdx := argIdx + offset
	var expected typ.Type
	switch {
	case paramIdx < len(fn.Params):
		expected = fn.Params[paramIdx].Type
	case fn.Variadic != nil:
		expected = fn.Variadic
	}
	if expected == nil {
		return nil
	}
	if in.SelfType != nil {
		expected = expectedArgSignatureType(expected, in.SelfType(in.Call))
	}
	if typ.IsAbsentOrUnknown(expected) || typ.IsAny(expected) {
		return nil
	}
	return expected
}

func expectedArgSignatureType(t typ.Type, selfType typ.Type) typ.Type {
	if t == nil || selfType == nil {
		return t
	}
	return subst.Self(t, selfType)
}

// DemandsForCallTargets projects selected callee demand targets onto a concrete
// call's source arguments. Program-specific target construction stays outside
// this package; the call-boundary projection shape is canonical here.
func DemandsForCallTargets(call *ast.FuncCallExpr, targets []paramevidence.CallArgDemandTarget) []callobligation.Obligation {
	if call == nil || len(targets) == 0 {
		return nil
	}
	return paramevidence.CallArgDemandProjection{
		Call:    call,
		Targets: targets,
	}.Obligations()
}
