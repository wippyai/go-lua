package call

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/typ"
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
