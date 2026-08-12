package carrier

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// Transport performs one complete directed input boundary:
//
//	post ∧ omega(pre ∧ state)
//
// The precondition is applied in the source scope before reindexing; the
// postcondition is applied only after the result reaches the target scope.
// This is the sole carrier entry point for a complete equation input, so a
// caller cannot reuse a State through a rename, forget, false filter, or
// changed target interface.
func (work *Work) Transport(state State, pre support.Mask, omega ReindexPlan, post support.Mask) (State, bool) {
	if !work.live() || !work.OwnsState(state) || !omega.validFor(work.composition) || !state.scope.same(omega.source()) ||
		!validBoundaryMask(pre, state.scope) || !validBoundaryMask(post, omega.target()) {
		return State{}, false
	}
	// This fast path is intentionally exact. A semantically equivalent but
	// separately issued scope cannot reach it because ReindexPlan.Identity
	// already proves same issued source/target scope.
	if omega.identity() && pre.IsTrue() && post.IsTrue() {
		return state, true
	}
	filtered, ok := work.filter(state, pre)
	if !ok {
		return State{}, false
	}
	reindexed, ok := work.Reindex(filtered, omega)
	if !ok {
		return State{}, false
	}
	return work.filter(reindexed, post)
}

// MergeTransportedContribution fuses one directed environment transport with
// the subsequent contribution fold.  The carrier computes the complete
// support/coverage relation first, then asks a typed SlotWork to reindex and
// join only slots whose transformed authored coverage is nonempty.  An
// uncovered right slot is therefore the fold identity even when transport
// enlarges the enclosing support; the accumulator root is retained exactly.
//
// The typed boundary receives no intermediate Contribution or published
// reindexed root.  Its final ChangeHandle enters the same ordinary commit cut
// as MergeContribution, preserving cancellation, pending-root drop, and
// canonical physical-slot order.
func (work *Work) MergeTransportedContribution(left, right Contribution, pre support.Mask, omega ReindexPlan, post support.Mask) (Contribution, ChangeSet, bool) {
	if !work.live() || work.reindexing || !work.admittedContribution(left) || !work.admittedContribution(right) || !omega.validFor(work.composition) ||
		!left.state.scope.same(omega.target()) || !right.state.scope.same(omega.source()) ||
		!validBoundaryMask(pre, right.state.scope) || !validBoundaryMask(post, left.state.scope) {
		return Contribution{}, ChangeSet{}, false
	}
	// A neutral accumulator is the complete carrier identity, not merely an
	// uncovered sparse slot.  The existing two-step route must own this case so
	// TransportContribution can retain every hidden/off-coverage transported
	// root before MergeContribution returns the right operand unchanged.
	if work.neutralContribution(left) {
		if omega.identity() && pre.IsTrue() && post.IsTrue() {
			return work.MergeContribution(left, right)
		}
		transported, ok := work.TransportContribution(right, pre, omega, post)
		if !ok {
			return Contribution{}, ChangeSet{}, false
		}
		return work.MergeContribution(left, transported)
	}
	// An exact identity boundary has no transport work at all.  Keeping this
	// route on the existing contribution API also preserves its same-state
	// proof without introducing a second merge authority.
	if omega.identity() && pre.IsTrue() && post.IsTrue() {
		return work.MergeContribution(left, right)
	}
	work.reindexing = true
	defer func() { work.reindexing = false }()

	// Support is transported before any typed operation.  This is precisely
	// post ∧ Ω(pre ∧ right.support); typed transport receives the source
	// support after pre and the final target support after post.
	sourceSupport, ok := work.intersectSupport(right.state.support, pre)
	if !ok {
		return Contribution{}, ChangeSet{}, false
	}
	reindexedSupport, ok := work.reindexSupport(sourceSupport, omega.relation)
	if !ok {
		return Contribution{}, ChangeSet{}, false
	}
	rightSupport, ok := work.intersectSupport(reindexedSupport, post)
	if !ok {
		return Contribution{}, ChangeSet{}, false
	}
	transportedCoverage, ok := work.transportContributionCoverage(right.coverage, pre, omega, post, rightSupport)
	if !ok {
		return Contribution{}, ChangeSet{}, false
	}
	split, ok := work.threeSupport(left.state.support, rightSupport)
	if !ok {
		return Contribution{}, ChangeSet{}, false
	}
	nextCoverage, ok := work.mergeCoverage(left.coverage, transportedCoverage, split.Union())
	if !ok {
		return Contribution{}, ChangeSet{}, false
	}

	// No transformed authored surface means no typed transport/join.  Support
	// and coverage still publish through the normal carrier cut, so coverage-
	// only wake/version evidence remains visible to the runtime.
	hasAuthored := false
	for position := range work.slots {
		if len(transportedCoverage.slot(shape.Slot(position)).targets) != 0 {
			hasAuthored = true
			break
		}
	}
	if !hasAuthored {
		next, changes, committed := work.commit(left.state, nil, split.Union(), split.RightOnly(), emptyMask(work.composition.guards), nil)
		if !committed {
			return Contribution{}, ChangeSet{}, false
		}
		result, valid := work.admitContribution(next, nextCoverage)
		return result, changes, valid
	}

	delta := work.newSupportWork()
	if delta == nil {
		return Contribution{}, ChangeSet{}, false
	}
	patches := make([]Patch, 0, len(work.slots))
	for position, slot := range work.slots {
		if !work.live() || slot == nil {
			delta.Discard()
			dropPatches(patches)
			return Contribution{}, ChangeSet{}, false
		}
		physical := shape.Slot(position)
		rightSlot := transportedCoverage.slot(physical)
		if len(rightSlot.targets) == 0 {
			// The right contribution is absent at this slot after transport;
			// preserving the accumulator root is the sparse fold law.
			continue
		}
		change, valid := slot.MergeTransportedUnder(left.state.roots[position], right.state.roots[position], left.state.support, sourceSupport, reindexedSupport, rightSupport, omega.relation, coverageRows(left.coverage.slot(physical)), coverageRows(rightSlot), delta)
		if !valid {
			delta.Discard()
			dropPatches(patches)
			return Contribution{}, ChangeSet{}, false
		}
		if !work.acceptInto(&patches, left.state, change, delta) {
			delta.Discard()
			return Contribution{}, ChangeSet{}, false
		}
	}
	next, changes, committed := work.commit(left.state, patches, split.Union(), split.RightOnly(), emptyMask(work.composition.guards), delta)
	if !committed {
		return Contribution{}, ChangeSet{}, false
	}
	result, valid := work.admitContribution(next, nextCoverage)
	return result, changes, valid
}

// transportContributionCoverage applies the same source-pre, reindex, and
// target-post relation to authored rows that TransportContribution uses.  It
// remains carrier-owned: target capabilities stay opaque and only their
// support regions cross the transport boundary.
func (work *Work) transportContributionCoverage(input contributionCoverage, pre support.Mask, omega ReindexPlan, post, targetSupport support.Mask) (contributionCoverage, bool) {
	if !work.live() || input.composition != work.composition || !omega.validFor(work.composition) || !pre.Valid() || !post.Valid() || !targetSupport.Valid() || pre.Manager() != work.composition.guards || post.Manager() != work.composition.guards || targetSupport.Manager() != work.composition.guards {
		return contributionCoverage{}, false
	}
	coverage := contributionCoverage{composition: work.composition, slots: make([]slotCoverage, work.composition.Count())}
	nonempty := false
	for position := 0; position < work.composition.Count(); position++ {
		source := input.slot(shape.Slot(position))
		if len(source.targets) == 0 {
			continue
		}
		rows := make([]TargetRegion, 0, len(source.targets))
		for _, row := range source.targets {
			region, valid := work.intersectSupport(row.region, pre)
			if !valid {
				return contributionCoverage{}, false
			}
			if support.Empty(region) {
				continue
			}
			region, valid = work.reindexSupport(region, omega.relation)
			if !valid {
				return contributionCoverage{}, false
			}
			region, valid = work.intersectSupport(region, post)
			if !valid || support.Empty(region) {
				if !valid {
					return contributionCoverage{}, false
				}
				continue
			}
			rows = append(rows, TargetRegion{target: row.target, region: region})
		}
		canonical, valid := work.canonicalCoverage(rows, targetSupport)
		if !valid {
			return contributionCoverage{}, false
		}
		coverage.slots[position] = canonical
		nonempty = nonempty || len(canonical.targets) != 0
	}
	if !nonempty {
		coverage.slots = nil
	}
	return coverage, true
}

func validBoundaryMask(mask support.Mask, scope Scope) bool {
	if !mask.Valid() || !scope.Valid() || scope.composition == nil || mask.Manager() != scope.composition.guards {
		return false
	}
	root, ok := mask.Guard()
	return ok && scope.guard.Contains(root)
}

// filter retains the exact state roots under a support-only boundary filter.
// No typed root is rebuilt: Transfer with no patches carries the immutable
// vector and changes only outer support. This is the filter-only sharing law.
func (work *Work) filter(state State, within support.Mask) (State, bool) {
	if !work.live() || !work.OwnsState(state) || !validBoundaryMask(within, state.scope) {
		return State{}, false
	}
	if within.SameHandle(state.support) {
		return state, true
	}
	if !withinScope(state, within) {
		return State{}, false
	}
	restricted, ok := work.intersectSupport(state.support, within)
	if !ok {
		return State{}, false
	}
	view := View{state: state, support: restricted}
	next, _, ok := work.Transfer(state, view, nil)
	return next, ok
}
