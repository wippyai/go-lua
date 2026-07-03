package projectsummary

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func projectReturnParamLiteralCases(
	reg *axis.Registry,
	result ResultReader,
) []summary.ReturnParamLiteralCase {
	slots := newReturnSlotProjection(reg, result)
	if !slots.OK() || slots.arity == 0 {
		return nil
	}
	params := parameterValuePaths(result)
	if len(params) == 0 {
		return nil
	}
	var out []summary.ReturnParamLiteralCase
	for _, point := range slots.reachable {
		cases := returnPointParamLiteralCases(result, point, params)
		if len(cases) == 0 {
			continue
		}
		sources, ok := slots.Sources(point)
		if !ok {
			continue
		}
		for returnIndex := range slots.arity {
			value, ok := slots.Value(point, sources, returnIndex)
			if !ok {
				continue
			}
			value = slots.ValueWithDeclaredContract(value, returnIndex)
			for _, candidate := range cases {
				candidate.ReturnIndex = returnIndex
				candidate.Value = value
				out = append(out, candidate)
			}
		}
	}
	return out
}

func returnPointParamLiteralCases(
	result ResultReader,
	point cfg.Point,
	params []pathdom.Path,
) []summary.ReturnParamLiteralCase {
	graph := result.Graph()
	branches, ok := result.(branchConditionReader)
	if !ok || graph == nil {
		return nil
	}
	sufficient, ok := result.(branchSufficientCheckReader)
	if !ok {
		return nil
	}
	dom, ok := result.(pointDominatorReader)
	if !ok {
		return nil
	}
	postdom := dominance.ComputeImmediatePostDominators(graph)
	var out []summary.ReturnParamLiteralCase
	for _, branch := range graph.RPO() {
		if branch == point {
			continue
		}
		fact, ok := branches.BranchCondition(branch)
		if !ok {
			continue
		}
		for _, succ := range cfg.SuccessorsReadOnly(graph, branch) {
			edge, ok := graph.EdgeCond(branch, succ)
			if !ok || !dom.PointDominates(succ, point) || !dominance.PostDominates(postdom, point, succ) {
				continue
			}
			for _, check := range sufficient.BranchConditionSufficientChecksOnEdge(fact, edge) {
				if candidate, ok := returnParamLiteralCaseFromCheck(check, params); ok {
					out = append(out, candidate)
				}
			}
		}
	}
	return out
}

func returnParamLiteralCaseFromCheck(
	check branchcond.ImpliedCheck,
	params []pathdom.Path,
) (summary.ReturnParamLiteralCase, bool) {
	lit, ok := literalValueProvenBySufficientCheck(check)
	if !ok {
		return summary.ReturnParamLiteralCase{}, false
	}
	paramIndex, suffix, ok := paramPathSuffix(check.Check.Path, params)
	if !ok {
		return summary.ReturnParamLiteralCase{}, false
	}
	return summary.ReturnParamLiteralCase{
		ParamIndex:  paramIndex,
		ParamSuffix: suffix,
		When:        lit,
	}, true
}

func literalValueProvenBySufficientCheck(check branchcond.ImpliedCheck) (typ.Type, bool) {
	switch check.Check.Kind {
	case branchcond.CheckLiteralEqual:
		if check.Polarity {
			return check.Check.LiteralValue()
		}
	case branchcond.CheckLiteralNot:
		if !check.Polarity {
			return check.Check.LiteralValue()
		}
	}
	return nil, false
}

func paramPathSuffix(target pathdom.Path, params []pathdom.Path) (int, []segment.Segment, bool) {
	if target.IsEmpty() {
		return 0, nil, false
	}
	for i, param := range params {
		if param.Symbol == 0 || target.Symbol != param.Symbol {
			continue
		}
		return i, append([]segment.Segment(nil), target.Segments...), true
	}
	return 0, nil, false
}
