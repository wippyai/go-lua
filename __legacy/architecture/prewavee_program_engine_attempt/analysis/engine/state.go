package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/coordinate"
	"github.com/wippyai/go-lua/program"
)

// State is one completed immutable Solver result. It retains exactly one
// compact materialized observation group for each demanded coordinate. Facts
// owns symbolic support and typed planes only while a transaction is live;
// it is never retained here.
//
// Scheduler work, dependency indexes, rule frames, and relation discovery
// are transaction-local operational state. They are discarded before this
// value becomes observable and therefore cannot become a continuation path.
type State struct {
	owner   *solverOwner
	content program.ContentID
	roots   []stateRoot
}

func (state *State) root(slot int, coordinate coordinate.Coordinate) (stateRoot, bool) {
	if state == nil || slot < 0 || !coordinate.Valid() || slot >= len(state.roots) {
		return stateRoot{}, false
	}
	root := state.roots[slot]
	if root.coordinate != coordinate {
		return stateRoot{}, false
	}
	return root, true
}

// stateRoot is private because Coordinate is an operational address, not a
// semantic identity. A public Query owns a compact State position and one
// private result position within it. The erased payload is immutable and is
// type-asserted only by that exact Query capability.
type stateRoot struct {
	coordinate coordinate.Coordinate
	results    []stateResult
}

// stateResult is the terminal observation projection for one demanded
// (coordinate, Factor, key) triple. It deliberately retains no Fact root,
// BDD, terminal arena, guard, or dependency index. present distinguishes a
// pruned/unavailable observation from an admitted Factor Default.
type stateResult struct {
	value   any
	present bool
}

func (state *State) result(rootSlot, resultSlot int, coordinate coordinate.Coordinate) (stateResult, bool) {
	root, ok := state.root(rootSlot, coordinate)
	if !ok || resultSlot < 0 || resultSlot >= len(root.results) {
		return stateResult{}, false
	}
	return root.results[resultSlot], true
}
