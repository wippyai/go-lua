package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
)

type ResultReader interface {
	Registry() *axis.Registry
	Graph() cfg.Graph
	ExitState() (state.State, bool)
	ReturnPoints() []cfg.Point
	ReturnArity(cfg.Point) (int, bool)
}

type entryStateReader interface {
	EntryState() (state.State, bool)
}

type parameterValueSlotReader interface {
	ParameterValueSlots() []key.Value
}

type reassignedParameterValueSlotReader interface {
	ReassignedParameterValueSlots() map[key.Value]struct{}
}

type stateAtReader interface {
	StateAt(cfg.Point) (state.State, bool)
}

type branchConditionReader interface {
	BranchCondition(cfg.Point) (semantics.BranchConditionFact, bool)
}

type noNormalReturnReader interface {
	NoNormalReturn(cfg.Point) bool
}

// FromResult projects one completed check result into a fixed-point summary.
func FromResult(result ResultReader) Summary {
	if result == nil {
		return Summary{}
	}
	reg := result.Registry()
	graph := result.Graph()
	exit, ok := result.ExitState()
	if reg == nil || graph == nil || !ok {
		return Summary{}
	}

	summary := Summary{
		NormalReturnParams:          projectNormalReturnParams(reg, result, exit),
		NormalReturnParamConditions: projectNormalReturnParamConditions(reg, result),
		NormalReturnParamEqualities: projectNormalReturnParamEqualities(reg, result),
	}

	arity := 0
	for _, point := range result.ReturnPoints() {
		pointArity, ok := result.ReturnArity(point)
		if ok && pointArity > arity {
			arity = pointArity
		}
	}
	if arity > 0 {
		summary.Returns = make([]product.Value, arity)
		for i := range summary.Returns {
			summary.Returns[i] = exit.ReadValue(reg, key.ReturnSlot(i))
		}
	}
	return Normalize(reg, summary)
}

func projectNormalReturnParams(reg *axis.Registry, result ResultReader, exit state.State) []product.Value {
	entryReader, ok := result.(entryStateReader)
	if !ok {
		return nil
	}
	slotReader, ok := result.(parameterValueSlotReader)
	if !ok {
		return nil
	}
	entry, ok := entryReader.EntryState()
	if !ok {
		return nil
	}
	slots := slotReader.ParameterValueSlots()
	if len(slots) == 0 {
		return nil
	}
	var reassigned map[key.Value]struct{}
	if reassignedReader, ok := result.(reassignedParameterValueSlotReader); ok {
		reassigned = reassignedReader.ReassignedParameterValueSlots()
	}
	out := make([]product.Value, len(slots))
	for i := range out {
		out[i] = product.Top()
	}
	for i, slot := range slots {
		if slot == "" {
			continue
		}
		if _, ok := reassigned[slot]; ok {
			continue
		}
		value, ok := normalReturnParamConstraint(reg, entry.ReadValue(reg, slot), exit.ReadValue(reg, slot))
		if !ok {
			continue
		}
		out[i] = value
	}
	return out
}

func normalReturnParamConstraint(reg *axis.Registry, entry, exit product.Value) (product.Value, bool) {
	if product.Equal(reg, exit, product.Bottom(reg)) || product.Equal(reg, exit, product.Top()) {
		return product.Value{}, false
	}
	if product.Equal(reg, exit, entry) {
		return product.Value{}, false
	}
	if !product.LessOrEq(reg, exit, entry) {
		return product.Value{}, false
	}
	return exit, true
}

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

func normalReturnParamPaths(result ResultReader) []path.Path {
	slotReader, ok := result.(parameterValueSlotReader)
	if !ok {
		return nil
	}
	slots := slotReader.ParameterValueSlots()
	if len(slots) == 0 {
		return nil
	}
	out := make([]path.Path, len(slots))
	for i, slot := range slots {
		sym, ok := key.ParseSymbolValue(slot)
		if !ok {
			continue
		}
		out[i] = path.NewPath(sym, "")
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

type normalReturnReachability struct {
	reg      *axis.Registry
	graph    cfg.Graph
	states   stateAtReader
	noNormal noNormalReturnReader
	equal    func(state.State, state.State) bool
	memo     map[cfg.Point]bool
	visiting map[cfg.Point]struct{}
}

func newNormalReturnReachability(
	reg *axis.Registry,
	result ResultReader,
	graph cfg.Graph,
) (normalReturnReachability, bool) {
	states, ok := result.(stateAtReader)
	if !ok {
		return normalReturnReachability{}, false
	}
	noNormal, _ := result.(noNormalReturnReader)
	domain := state.Domain(reg)
	return normalReturnReachability{
		reg:      reg,
		graph:    graph,
		states:   states,
		noNormal: noNormal,
		equal:    domain.Equal,
		memo:     make(map[cfg.Point]bool),
		visiting: make(map[cfg.Point]struct{}),
	}, true
}

func (r normalReturnReachability) canCompleteNormally(point cfg.Point) bool {
	if got, ok := r.memo[point]; ok {
		return got
	}
	if _, ok := r.visiting[point]; ok {
		return true
	}
	st, ok := r.states.StateAt(point)
	if !ok || r.equal(st, state.State{}) {
		r.memo[point] = false
		return false
	}
	if point == r.graph.Exit() {
		r.memo[point] = true
		return true
	}
	if r.noNormal != nil && r.noNormal.NoNormalReturn(point) {
		r.memo[point] = false
		return false
	}
	r.visiting[point] = struct{}{}
	canComplete := false
	for _, succ := range r.graph.Successors(point) {
		if r.canCompleteNormally(succ) {
			canComplete = true
			break
		}
	}
	delete(r.visiting, point)
	r.memo[point] = canComplete
	return canComplete
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
