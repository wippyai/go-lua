package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/domain/resolve"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func (e *assignmentPointEmitter) emitVariantFieldOrigins(i int, targetPath constraint.Path) {
	if e == nil || e.state == nil || e.info == nil || targetPath.IsEmpty() {
		return
	}
	call, retIndex := e.info.CallForTarget(i)
	if call == nil || retIndex < 0 {
		return
	}
	fnType := resolve.CalleeType(
		call,
		e.p,
		e.state.wrappedSynth,
		e.state.symResolver,
		nil,
		e.state.fc.Graph,
		e.state.bindings,
		e.state.fc.ModuleBindings,
	)
	spec := contract.ExtractSpec(fnType)
	if spec == nil {
		return
	}
	ret := spec.Effects.GetReturn(retIndex)
	if ret == nil || ret.Transform == nil {
		return
	}
	transform, ok := ret.Transform.(effect.SelectResultOfCases)
	if !ok {
		return
	}
	casesIdx, ok := effect.ResolveParamIndex(transform.Cases, callsite.RuntimeArgCount(call))
	if !ok {
		return
	}
	casesExpr := callsite.RuntimeArgAt(call, casesIdx)
	caseExprs := selectCaseExpressions(casesExpr)
	if len(caseExprs) == 0 {
		return
	}
	targetPath = flowpath.WithVersion(targetPath, e.state.fc.Graph, e.p)
	for caseIdx, caseExpr := range caseExprs {
		sourcePath := e.selectCaseSourcePath(caseExpr)
		if sourcePath.IsEmpty() {
			continue
		}
		e.state.inputs.VariantFieldOrigins = append(e.state.inputs.VariantFieldOrigins, flow.VariantFieldOrigin{
			Target:             targetPath,
			Field:              effect.SelectResultChannelField,
			Source:             sourcePath,
			DiscriminatorField: effect.SelectResultCaseIDField,
			DiscriminatorValue: typ.LiteralInt(int64(caseIdx)),
		})
	}
}

func (e *assignmentPointEmitter) selectCaseSourcePath(expr ast.Expr) constraint.Path {
	if e == nil || e.state == nil || expr == nil {
		return constraint.Path{}
	}
	if call, ok := expr.(*ast.FuncCallExpr); ok && callsite.IsMethodLikeExpr(call) && call.Receiver != nil {
		return flowpath.FromExprWithBindingsAt(call.Receiver, e.constResolver, e.state.bindings, e.state.fc.Graph, e.p)
	}
	return flowpath.FromExprWithBindingsAt(expr, e.constResolver, e.state.bindings, e.state.fc.Graph, e.p)
}

func selectCaseExpressions(expr ast.Expr) []ast.Expr {
	tbl, ok := expr.(*ast.TableExpr)
	if !ok || tbl == nil || len(tbl.Fields) == 0 {
		return nil
	}
	out := make([]ast.Expr, 0, len(tbl.Fields))
	for _, field := range tbl.Fields {
		if field == nil || field.Value == nil || selectCaseFieldIsDefault(field) {
			continue
		}
		out = append(out, field.Value)
	}
	return out
}

func selectCaseFieldIsDefault(field *ast.Field) bool {
	if field == nil || field.Key == nil {
		return false
	}
	switch k := field.Key.(type) {
	case *ast.IdentExpr:
		return k.Value == "default"
	case *ast.StringExpr:
		return k.Value == "default"
	default:
		return false
	}
}
