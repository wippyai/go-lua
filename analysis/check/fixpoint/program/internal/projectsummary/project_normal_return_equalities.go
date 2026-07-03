package projectsummary

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
)

func projectNormalReturnParamEqualities(reg *axis.Registry, result ResultReader) []summary.ParamEquality {
	branchReader, ok := result.(branchConditionReader)
	if !ok {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	params := parameterValuePaths(result)
	if len(params) == 0 {
		return nil
	}
	reachability, ok := newNormalReturnReachability(reg, result, graph)
	if !ok {
		return nil
	}
	var out []summary.ParamEquality
	for _, point := range graph.RPO() {
		fact, ok := branchReader.BranchCondition(point)
		if !ok {
			continue
		}
		normalCond, ok := normalReturnBranchConditionWithReachability(graph, reachability, point)
		if !ok {
			continue
		}
		equality, ok := normalReturnParamEquality(fact.Check, normalCond, params)
		if !ok {
			continue
		}
		out = append(out, equality)
	}
	return out
}

func normalReturnParamEquality(
	check branchcond.Check,
	normalCond bool,
	params []path.Path,
) (summary.ParamEquality, bool) {
	switch check.Kind {
	case branchcond.CheckPathEqual:
		if !normalCond {
			return summary.ParamEquality{}, false
		}
	case branchcond.CheckPathNot:
		if normalCond {
			return summary.ParamEquality{}, false
		}
	default:
		return summary.ParamEquality{}, false
	}
	left, ok := normalReturnParamIndex(check.Path, params)
	if !ok {
		return summary.ParamEquality{}, false
	}
	right, ok := normalReturnParamIndex(check.OtherPath, params)
	if !ok {
		return summary.ParamEquality{}, false
	}
	if left == right {
		return summary.ParamEquality{}, false
	}
	return summary.ParamEquality{Left: left, Right: right}, true
}
