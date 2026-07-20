package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// ApplyProtectedCallTypestate catches the callback's exceptional lifecycle
// state at pcall/xpcall. Each exit class is first materialized against the
// caller's state, then their typestate lanes are joined at the caught outcome.
// It is the sole authoritative protected-call State transfer used by both
// concrete call lowering and the reduced relation executor.
func ApplyProtectedCallTypestate(out state.State, protected callboundary.ProtectedCallTypestate) state.State {
	if protected.Empty() {
		return out
	}
	var merged typestate.Store
	hasOutcome := false
	merge := func(snapshot typestate.Store) {
		candidate := out.OverlayTypestateSnapshot(snapshot).TypestateSnapshot()
		if !hasOutcome {
			merged = candidate
			hasOutcome = true
			return
		}
		merged = typestate.Join(merged, candidate)
	}
	if protected.HasNormal {
		merge(protected.Normal)
	}
	if protected.HasExceptional {
		merge(protected.Exceptional)
	}
	if !hasOutcome {
		return out
	}
	return out.WithTypestateSnapshot(merged)
}
