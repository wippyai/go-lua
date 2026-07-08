package projectsummary

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
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
		cases := returnPointParamLiteralCases(reg, result, point, params)
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
	reg *axis.Registry,
	result ResultReader,
	point cfg.Point,
	params []pathdom.Path,
) []summary.ReturnParamLiteralCase {
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	sufficient, ok := result.(branchSufficientLiteralCaseReader)
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
		for _, succ := range cfg.SuccessorsReadOnly(graph, branch) {
			edge, ok := graph.EdgeCond(branch, succ)
			if !ok || !dom.PointDominates(succ, point) || !dominance.PostDominates(postdom, point, succ) {
				continue
			}
			branchCases := sufficient.BranchSufficientLiteralCases(branch)
			for _, literalCase := range branchCases {
				if literalCase.Edge() == edge {
					if candidate, ok := returnParamLiteralCaseFromFact(reg, literalCase, params); ok {
						out = append(out, candidate)
					}
					continue
				}
				candidates := complementaryReturnParamLiteralCasesFromFact(
					reg,
					result,
					branch,
					literalCase,
					branchCases,
					params,
				)
				out = append(out, candidates...)
			}
		}
	}
	return out
}

func complementaryReturnParamLiteralCasesFromFact(
	reg *axis.Registry,
	result ResultReader,
	branch cfg.Point,
	literalCase factflow.BranchSufficientLiteralCase,
	branchCases []factflow.BranchSufficientLiteralCase,
	params []pathdom.Path,
) []summary.ReturnParamLiteralCase {
	if !singleSufficientLiteralCaseForPath(branchCases, literalCase.TargetPathRef()) {
		return nil
	}
	valueReader, ok := result.(pathValueAtBoundaryReader)
	if !ok {
		return nil
	}
	value, ok := valueReader.PathValueAtBoundary(branch, literalCase.TargetPathRef())
	if !ok {
		return nil
	}
	domain, ok := typevalue.TypeOf(reg, value)
	if !ok {
		return nil
	}
	domainLiterals := finiteLiteralDomain(domain)
	if len(domainLiterals) == 0 {
		return nil
	}
	excluded, ok := typevalue.TypeOf(reg, literalCase.LiteralValue())
	if !ok {
		return nil
	}
	paramIndex, suffix, ok := paramPathSuffix(literalCase.TargetPathRef(), params)
	if !ok {
		return nil
	}
	var out []summary.ReturnParamLiteralCase
	for _, lit := range domainLiterals {
		if typ.TypeEquals(lit, excluded) {
			continue
		}
		out = append(out, summary.ReturnParamLiteralCase{
			ParamIndex:  paramIndex,
			ParamSuffix: append([]segment.Segment(nil), suffix...),
			When:        lit,
		})
	}
	return out
}

func singleSufficientLiteralCaseForPath(
	cases []factflow.BranchSufficientLiteralCase,
	target pathdom.Path,
) bool {
	if target.IsEmpty() {
		return false
	}
	count := 0
	for _, candidate := range cases {
		if candidate.TargetPathRef().Equal(target) {
			count++
			if count > 1 {
				return false
			}
		}
	}
	return count == 1
}

func finiteLiteralDomain(t typ.Type) []typ.Type {
	if t == nil {
		return nil
	}
	switch tt := typ.UnwrapTransparentWrappers(t).(type) {
	case *typ.Literal:
		return []typ.Type{tt}
	case *typ.Union:
		out := make([]typ.Type, 0, len(tt.Members))
		for _, member := range tt.Members {
			lit, ok := typ.UnwrapTransparentWrappers(member).(*typ.Literal)
			if !ok {
				return nil
			}
			out = append(out, lit)
		}
		return out
	default:
		return nil
	}
}

func returnParamLiteralCaseFromFact(
	reg *axis.Registry,
	literalCase factflow.BranchSufficientLiteralCase,
	params []pathdom.Path,
) (summary.ReturnParamLiteralCase, bool) {
	lit, ok := typevalue.TypeOf(reg, literalCase.LiteralValue())
	if !ok {
		return summary.ReturnParamLiteralCase{}, false
	}
	paramIndex, suffix, ok := paramPathSuffix(literalCase.TargetPathRef(), params)
	if !ok {
		return summary.ReturnParamLiteralCase{}, false
	}
	return summary.ReturnParamLiteralCase{
		ParamIndex:  paramIndex,
		ParamSuffix: suffix,
		When:        lit,
	}, true
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
