package projectsummary

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

func projectNormalReturnParamEqualities(reg *axis.Registry, result ResultReader) []summary.ParamEquality {
	relationReader, ok := result.(branchPathRelationReader)
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
		normalCond, ok := normalReturnBranchConditionWithReachability(graph, reachability, point)
		if !ok {
			continue
		}
		for _, relation := range relationReader.BranchPathRelations(point) {
			equality, ok := normalReturnParamEquality(relation, normalCond, params)
			if !ok {
				continue
			}
			out = append(out, equality)
		}
	}
	return out
}

func normalReturnParamEquality(
	relation factflow.BranchPathRelation,
	normalCond bool,
	params []path.Path,
) (summary.ParamEquality, bool) {
	if relation.Kind() != factflow.BranchPathRelationEqual || !relation.ActiveOnEdge(normalCond) {
		return summary.ParamEquality{}, false
	}
	left, ok := normalReturnParamIndex(relation.LeftPath(), params)
	if !ok {
		return summary.ParamEquality{}, false
	}
	right, ok := normalReturnParamIndex(relation.RightPath(), params)
	if !ok {
		return summary.ParamEquality{}, false
	}
	if left == right {
		return summary.ParamEquality{}, false
	}
	return summary.ParamEquality{Left: left, Right: right}, true
}
