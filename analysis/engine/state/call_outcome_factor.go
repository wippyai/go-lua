package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

// ApplyCallOutcomeHeapObjectFactor writes one resolved external-call heap
// object directly into the registered HeapTableIdentity factor. Returned
// allocations join with the caller-owned object; all other identities replace
// it exactly. The caller chooses joinCurrent from the canonical identity law.
func (d ProductDomain) ApplyCallOutcomeHeapObjectFactor(
	factor LaneFactor,
	id identity.ID,
	object heapidentity.TableObject,
	joinCurrent bool,
) (LaneFactor, error) {
	runtime, err := d.validateFactor(factor)
	if err != nil || runtime.lane.id != LaneHeapTableIdentity || id == (identity.ID{}) {
		return LaneFactor{}, fmt.Errorf("%w: invalid call-outcome heap factor", ErrInvalidLaneFactor)
	}
	lane := typedLaneFactorValue[heapTableIdentityLane](factor.payload)
	if joinCurrent {
		object = heapidentity.ObjectDomain(d.reg).Join(lane.read(d.reg, id), object)
	}
	lane = lane.with(id, object)
	return LaneFactor{lane: runtime.lane, payload: typedLaneFactorPayload[heapTableIdentityLane]{value: lane}}, nil
}

// ApplyCallOutcomePlacementFactor joins one resolved placement into the
// registered pointwise placement factor. Bottom remains omission.
func (d ProductDomain) ApplyCallOutcomePlacementFactor(
	factor LaneFactor,
	id identity.ID,
	value placement.Value,
) (LaneFactor, error) {
	runtime, err := d.validateFactor(factor)
	if err != nil || runtime.lane.id != LanePlacement || id == (identity.ID{}) {
		return LaneFactor{}, fmt.Errorf("%w: invalid call-outcome placement factor", ErrInvalidLaneFactor)
	}
	lane := typedLaneFactorValue[placementLane](factor.payload)
	value = placement.Join(lane.read(id), value)
	if value == placement.Bottom {
		return factor, nil
	}
	lane = lane.with(id, value)
	return LaneFactor{lane: runtime.lane, payload: typedLaneFactorPayload[placementLane]{value: lane}}, nil
}

// ApplyProtectedCallTypestateFactor catches normal and exceptional callback
// snapshots directly in the registered Typestates factor. Snapshot overlay
// and exit-class join are identical to the concrete State law, but no State is
// reconstructed at this factor boundary.
func (d ProductDomain) ApplyProtectedCallTypestateFactor(
	factor LaneFactor,
	normal typestate.Store,
	hasNormal bool,
	exceptional typestate.Store,
	hasExceptional bool,
) (LaneFactor, error) {
	runtime, err := d.validateFactor(factor)
	if err != nil || runtime.lane.id != LaneTypestates {
		return LaneFactor{}, fmt.Errorf("%w: invalid protected-call typestate factor", ErrInvalidLaneFactor)
	}
	if !hasNormal && !hasExceptional {
		return factor, nil
	}
	current := typedLaneFactorValue[typestate.Store](factor.payload)
	var merged typestate.Store
	hasOutcome := false
	merge := func(snapshot typestate.Store) {
		candidate := current.Overlay(snapshot)
		if !hasOutcome {
			merged, hasOutcome = candidate, true
			return
		}
		merged = typestate.Join(merged, candidate)
	}
	if hasNormal {
		merge(normal)
	}
	if hasExceptional {
		merge(exceptional)
	}
	if !hasOutcome || typestate.Equal(current, merged) {
		return factor, nil
	}
	return LaneFactor{lane: runtime.lane, payload: typedLaneFactorPayload[typestate.Store]{value: merged.Clone()}}, nil
}
