package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
)

func projectNormalReturnParamEqualities(reg *axis.Registry, result ResultReader) []ParamEquality {
	branchReader, ok := result.(branchConditionReader)
	if !ok {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	params := normalReturnParamPaths(result)
	if len(params) == 0 {
		return nil
	}
	var out []ParamEquality
	for _, point := range graph.RPO() {
		fact, ok := branchReader.BranchCondition(point)
		if !ok {
			continue
		}
		normalCond, ok := normalReturnBranchCondition(reg, result, graph, point)
		if !ok {
			continue
		}
		equality, ok := normalReturnParamEquality(fact.Check, normalCond, params)
		if !ok {
			continue
		}
		out = append(out, equality)
	}
	return normalizeParamEqualities(out)
}

func normalReturnParamEquality(
	check branchcond.Check,
	normalCond bool,
	params []path.Path,
) (ParamEquality, bool) {
	switch check.Kind {
	case branchcond.CheckPathEqual:
		if !normalCond {
			return ParamEquality{}, false
		}
	case branchcond.CheckPathNot:
		if normalCond {
			return ParamEquality{}, false
		}
	default:
		return ParamEquality{}, false
	}
	left, ok := normalReturnParamIndex(check.Path, params)
	if !ok {
		return ParamEquality{}, false
	}
	right, ok := normalReturnParamIndex(check.OtherPath, params)
	if !ok {
		return ParamEquality{}, false
	}
	if left == right {
		return ParamEquality{}, false
	}
	return ParamEquality{Left: left, Right: right}, true
}
