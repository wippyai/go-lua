package projection

import (
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// projectProtectedCallTypestate preserves the states of resources that entered
// a callback across its two exit classes. A protected caller—not the callback
// summary—joins these snapshots, because pcall turns a raise into a normal
// caller continuation.
func projectProtectedCallTypestate(result ResultReader, normal state.State, hasNormal bool) callboundary.ProtectedCallTypestate {
	entryReader, ok := result.(entryStateReader)
	if !ok {
		return callboundary.ProtectedCallTypestate{}
	}
	entry, ok := entryReader.EntryState()
	if !ok {
		return callboundary.ProtectedCallTypestate{}
	}
	resources := entry.TypestateSnapshot().Resources()
	if len(resources) == 0 {
		return callboundary.ProtectedCallTypestate{}
	}
	out := callboundary.ProtectedCallTypestate{}
	if hasNormal {
		out.Normal = normal.TypestateSnapshot().Restrict(resources)
		out.HasNormal = true
	}

	stateReader, hasStates := result.(stateAtReader)
	noNormal, hasNoNormal := result.(noNormalReturnReader)
	if !hasStates || !hasNoNormal || result.Graph() == nil {
		return out
	}
	var exceptional typestate.Store
	foundExceptional := false
	for _, point := range result.Graph().RPO() {
		if !noNormal.NoNormalReturn(point) {
			continue
		}
		at, ok := stateReader.StateAt(point)
		if !ok {
			continue
		}
		snapshot := at.TypestateSnapshot().Restrict(resources)
		if !foundExceptional {
			exceptional = snapshot
			foundExceptional = true
			continue
		}
		exceptional = typestate.Join(exceptional, snapshot)
	}
	if foundExceptional {
		out.Exceptional = exceptional
		out.HasExceptional = true
		return out
	}

	// Lua calls can raise even where lowering cannot name a concrete error exit.
	// In that case, use every reachable state in the callback as the possible
	// caught outcome. This loses precision deliberately but never turns an
	// unlocated exceptional path into a false discharge of an obligation.
	for _, point := range result.Graph().RPO() {
		at, ok := stateReader.StateAt(point)
		if !ok {
			continue
		}
		snapshot := at.TypestateSnapshot().Restrict(resources)
		if !foundExceptional {
			exceptional = snapshot
			foundExceptional = true
			continue
		}
		exceptional = typestate.Join(exceptional, snapshot)
	}
	if foundExceptional {
		out.Exceptional = exceptional
		out.HasExceptional = true
	}
	return out
}
