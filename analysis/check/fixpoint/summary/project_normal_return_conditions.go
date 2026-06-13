package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
)

func projectNormalReturnParamConditions(reg *axis.Registry, result ResultReader) []ParamCondition {
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
	var out []ParamCondition
	for _, point := range graph.RPO() {
		fact, ok := branchReader.BranchCondition(point)
		if !ok {
			continue
		}
		normalCond, ok := normalReturnBranchCondition(reg, result, graph, point)
		if !ok {
			continue
		}
		paramIndex, condition, ok := normalReturnParamCondition(fact.Check, normalCond, params)
		if !ok {
			continue
		}
		if out == nil {
			out = make([]ParamCondition, len(params))
			for i := range out {
				out[i] = ParamConditionTop
			}
		}
		out[paramIndex] = meetParamCondition(out[paramIndex], condition)
	}
	return out
}

func normalReturnBranchCondition(
	reg *axis.Registry,
	result ResultReader,
	graph cfg.Graph,
	point cfg.Point,
) (bool, bool) {
	if reg == nil || graph == nil || !graph.IsBranch(point) {
		return false, false
	}
	reachability, ok := newNormalReturnReachability(reg, result, graph)
	if !ok {
		return false, false
	}
	var sawTrue, sawFalse bool
	var trueCanComplete, falseCanComplete bool
	for _, succ := range graph.Successors(point) {
		cond, ok := graph.EdgeCond(point, succ)
		if !ok {
			continue
		}
		canComplete := reachability.canCompleteNormally(succ)
		if cond {
			sawTrue = true
			trueCanComplete = trueCanComplete || canComplete
		} else {
			sawFalse = true
			falseCanComplete = falseCanComplete || canComplete
		}
	}
	if !sawTrue || !sawFalse || trueCanComplete == falseCanComplete {
		return false, false
	}
	return trueCanComplete, true
}

func normalReturnParamCondition(
	check branchcond.Check,
	normalCond bool,
	params []path.Path,
) (int, ParamCondition, bool) {
	paramIndex, ok := normalReturnParamIndex(check.Path, params)
	if !ok {
		return 0, ParamConditionBottom, false
	}
	switch check.Kind {
	case branchcond.CheckTruthy:
		if normalCond {
			return paramIndex, ParamConditionTruthy, true
		}
		return paramIndex, ParamConditionFalsy, true
	case branchcond.CheckFalsy:
		if normalCond {
			return paramIndex, ParamConditionFalsy, true
		}
		return paramIndex, ParamConditionTruthy, true
	default:
		return 0, ParamConditionBottom, false
	}
}

func normalReturnParamIndex(target path.Path, params []path.Path) (int, bool) {
	if target.IsEmpty() || len(target.Segments) != 0 {
		return 0, false
	}
	for i, param := range params {
		if target.Equal(param) {
			return i, true
		}
	}
	return 0, false
}

func meetParamCondition(a, b ParamCondition) ParamCondition {
	if a == ParamConditionTop {
		return b
	}
	if b == ParamConditionTop || a == b {
		return a
	}
	if a == ParamConditionBottom || b == ParamConditionBottom {
		return ParamConditionBottom
	}
	return ParamConditionBottom
}
