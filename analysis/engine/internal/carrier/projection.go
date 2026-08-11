package carrier

import "github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"

// ProjectContribution keeps exactly one Factor plane from an already
// transported contribution.  The carrier owns the projection because a
// Point edge must not reconstruct roots or coverage outside the one shared
// State/Contribution representation.
//
// The returned State keeps the transported support and scope.  Its root
// vector is the composition's initial vector except for slot, whose root is
// taken from input.  Authored coverage follows the same projection; every
// other slot is uncovered.  No typed key, lattice value, or callback crosses
// this cut.
func (work *Work) ProjectContribution(input Contribution, slot shape.Slot) (Contribution, bool) {
	if work == nil || !work.live() || !work.admittedContribution(input) || !work.OwnsState(input.state) || !work.composition.shape.ValidSlot(slot) {
		return Contribution{}, false
	}
	position := int(slot)
	if position < 0 || position >= len(work.composition.initial) || position >= len(input.state.roots) {
		return Contribution{}, false
	}
	// State roots are immutable handles.  When the selected plane already has
	// its initial root, the immutable Composition vector is the projection and
	// the hot path allocates nothing here.  A fresh vector is needed only when
	// the selected transported root differs from that initial root.
	roots := work.composition.initial
	if input.state.roots[position] != roots[position] {
		roots = append([]RootHandle(nil), roots...)
		roots[position] = input.state.roots[position]
	}
	state := State{authority: work.authority, scope: input.state.scope, support: input.state.support, roots: roots}
	if !state.live() || !state.acceptsRoot(slot, roots[position]) {
		return Contribution{}, false
	}

	// Keep only the selected slot's authored coverage.  The target rows are
	// immutable capabilities and are therefore safe to share; the enclosing
	// slot vector remains private to this Contribution.
	selected := input.coverage.slot(slot)
	coverage := contributionCoverage{composition: work.composition}
	if len(selected.targets) != 0 {
		coverage.slots = make([]slotCoverage, work.composition.Count())
		coverage.slots[position] = selected
	}
	return work.admitDerivedContribution(input, state, coverage)
}
