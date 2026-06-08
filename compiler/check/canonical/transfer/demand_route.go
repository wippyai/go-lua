package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/compiler/check/domain/provenance"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func (t *Transfer) demandExprCtx(out *flow.PointState, expr ast.Expr, ctx typ.Type, demand func(int, paramevidence.ParamContract)) {
	if demand == nil || ctx == nil || typ.IsAbsentOrUnknown(ctx) {
		return
	}
	valuePath, ok := t.demandPathForExpr(expr)
	if !ok || valuePath.Symbol == 0 {
		return
	}
	contract := paramevidence.DemandFromType(ctx)
	localContract := t.conditionedLeafContract(out, valuePath, contract)
	t.demandLocalPathContract(out, valuePath, localContract, demand)
}

func (t *Transfer) demandExprCapabilityCtx(
	out *flow.PointState,
	expr ast.Expr,
	cap paramevidence.Capability,
	demand func(int, paramevidence.ParamContract),
) {
	if demand == nil {
		return
	}
	valuePath, ok := t.demandPathForExpr(expr)
	if !ok || valuePath.Symbol == 0 {
		return
	}
	contract := paramevidence.DemandFromCapability(cap)
	localContract := t.conditionedLeafContract(out, valuePath, contract)
	t.demandLocalPathContract(out, valuePath, localContract, demand)
}

func (t *Transfer) demandExprContractCtx(
	out *flow.PointState,
	expr ast.Expr,
	contract paramevidence.ParamContract,
	demand func(int, paramevidence.ParamContract),
) {
	if demand == nil || paramevidence.ParamContractDomain.Equal(contract, paramevidence.ParamContractDomain.Bottom()) {
		return
	}
	valuePath, ok := t.demandPathForExpr(expr)
	if !ok || valuePath.Symbol == 0 {
		return
	}
	localContract := t.conditionedLeafContract(out, valuePath, contract)
	t.demandLocalPathContract(out, valuePath, localContract, demand)
}

func (t *Transfer) demandPathForExpr(expr ast.Expr) (constraint.Path, bool) {
	sym, segments, ok := t.pathSymbol(expr)
	if !ok || sym == 0 {
		return constraint.Path{}, false
	}
	return constraint.Path{
		Symbol:   sym,
		Segments: append([]constraint.Segment(nil), segments...),
	}, true
}

func (t *Transfer) demandParamPathContract(
	out *flow.PointState,
	idx int,
	path constraint.Path,
	contract paramevidence.ParamContract,
	demand func(int, paramevidence.ParamContract),
) {
	if demand == nil || idx < 0 ||
		paramevidence.ParamContractDomain.Equal(contract, paramevidence.ParamContractDomain.Bottom()) {
		return
	}
	if out != nil {
		conditioned, _ := paramevidence.ConditionedPathContract(path, contract, out.Cond)
		demand(idx, conditioned)
		return
	}
	demand(idx, paramevidence.DemandFromPathContract(path.Segments, contract))
}

func (t *Transfer) conditionedLeafContract(
	out *flow.PointState,
	path constraint.Path,
	contract paramevidence.ParamContract,
) paramevidence.ParamContract {
	if out == nil {
		return contract
	}
	conditioned, _ := paramevidence.ConditionedLeafContract(path, contract, out.Cond)
	return conditioned
}

func (t *Transfer) demandLocalPathContract(
	out *flow.PointState,
	path constraint.Path,
	contract paramevidence.ParamContract,
	demand func(int, paramevidence.ParamContract),
) {
	if demand == nil || path.Symbol == 0 ||
		paramevidence.ParamContractDomain.Equal(contract, paramevidence.ParamContractDomain.Bottom()) {
		return
	}
	var routes provenance.RouteResolver
	if out != nil {
		facts := flow.PointFactsOf(*out)
		routes = facts.ProvenanceRoutes
	}
	for _, target := range paramevidence.ProvenanceRouteContractClosure(path, contract, routes) {
		if param, isParam := t.params.Lookup(target.Path.Symbol); isParam {
			t.demandParamPathContract(out, param.Index, target.Path, target.Contract, demand)
		}
	}
}
