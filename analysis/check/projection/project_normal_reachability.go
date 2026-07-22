package projection

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

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
	for _, succ := range cfg.SuccessorsReadOnly(r.graph, point) {
		if r.canCompleteNormally(succ) {
			canComplete = true
			break
		}
	}
	delete(r.visiting, point)
	r.memo[point] = canComplete
	return canComplete
}
