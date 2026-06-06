package call

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/typ"
)

// CallArgDemandProjection is the canonical call-site policy for argument
// demand projection. Summary targets are the product fixed-point proof path; a
// callable signature is only the external/type fallback.
type CallArgDemandProjection struct {
	Call *ast.FuncCallExpr

	SummaryDemands func(*ast.FuncCallExpr) ([]callobligation.Obligation, bool)
	FunctionShape  func(*ast.FuncCallExpr) *typ.Function
	SelfType       func(*ast.FuncCallExpr) typ.Type
}

// Demands resolves caller-visible argument obligations in
// canonical precision order: selected summary targets first, then callable
// signature contracts. A summary hit with no demands is still authoritative and
// suppresses the signature fallback.
func (p CallArgDemandProjection) Demands() []callobligation.Obligation {
	if p.Call == nil || len(p.Call.Args) == 0 {
		return nil
	}
	if p.SummaryDemands != nil {
		if demands, ok := p.SummaryDemands(p.Call); ok {
			return demands
		}
	}
	if p.FunctionShape == nil {
		return nil
	}
	var selfType typ.Type
	if p.SelfType != nil {
		selfType = p.SelfType(p.Call)
	}
	return paramevidence.FunctionCallArgObligationsWithSelf(p.Call, p.FunctionShape(p.Call), selfType)
}
