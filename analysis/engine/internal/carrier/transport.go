package carrier

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// MergeTransportedPointContribution fuses one total PointState transport with
// the subsequent closed RHS fold.  The carrier computes output C first; the
// typed path totalizes reachable PointState absences to Default through omega,
// then joins only at target-present cells and physically closes the output.
//
// The typed boundary receives no intermediate Contribution or published
// reindexed root.  Its final ChangeHandle enters the same ordinary commit cut
// as MergeContribution, preserving cancellation, pending-root drop, and
// canonical physical-slot order.
func (work *Work) MergeTransportedPointContribution(left, right Contribution, pre support.Mask, omega ReindexPlan, post support.Mask) (Contribution, ChangeSet, bool) {
	if !work.live() || work.reindexing || !work.admittedContribution(left) || !work.admittedContribution(right) || !omega.validFor(work.composition) ||
		!left.state.scope.same(omega.target()) || !right.state.scope.same(omega.source()) ||
		!validBoundaryMask(pre, right.state.scope) || !validBoundaryMask(post, left.state.scope) {
		return Contribution{}, ChangeSet{}, false
	}
	// A neutral accumulator is the complete carrier identity. Point transport
	// still uses total State semantics before the final C close.
	if work.neutralContribution(left) {
		if omega.identity() && pre.IsTrue() && post.IsTrue() {
			return work.MergeContribution(left, right)
		}
		transported, ok := work.TransportPointContribution(right, pre, omega, post)
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
	if omega.coordinateIdentity() && pre.IsTrue() && post.IsTrue() {
		transported, ok := work.rescopeContribution(right, omega.target())
		if !ok {
			return Contribution{}, ChangeSet{}, false
		}
		return work.MergeContribution(left, transported)
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
	nextCoverage, ok := work.unionCoverage(left.coverage, transportedCoverage)
	if !ok {
		return Contribution{}, ChangeSet{}, false
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
		change, valid := slot.MergeTransportedPointUnder(left.state.roots[position], right.state.roots[position], left.state.support, sourceSupport, reindexedSupport, rightSupport, omega.relation, coverageRows(left.coverage.slot(physical)), coverageRows(rightSlot), delta)
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
	result, valid := work.admitConstructedContribution(next, nextCoverage)
	return result, changes, valid
}

// transportContributionCoverage applies the same source-pre, reindex, and
// target-post relation to authored rows that Point/Rule transport use.  It
// remains carrier-owned: target capabilities stay opaque and only their
// support regions cross the transport boundary.
func (work *Work) transportContributionCoverage(input contributionCoverage, pre support.Mask, omega ReindexPlan, post, targetSupport support.Mask) (contributionCoverage, bool) {
	if !work.live() || input.composition != work.composition || !omega.validFor(work.composition) || !pre.Valid() || !post.Valid() || !targetSupport.Valid() || pre.Manager() != work.composition.guards || post.Manager() != work.composition.guards || targetSupport.Manager() != work.composition.guards {
		return contributionCoverage{}, false
	}
	// Input coverage is already an immutable authored surface. Validate its
	// target ownership and strict order before opening the candidate so the
	// batch below can retain the same sorted, unique Target sequence without
	// invoking the sealed-only canonicalizer on candidate masks.
	if !work.validTransportSourceCoverage(input) {
		return contributionCoverage{}, false
	}
	// All rows belong to one authored coverage relation. Keep their support
	// intersections and reindexing in one candidate transaction so the
	// correlated masks cross one publication cut. A failed row discards the
	// entire batch; no partially transported Target region can escape.
	batch := work.newSupportWork()
	if batch == nil {
		return contributionCoverage{}, false
	}
	committed := false
	defer func() {
		if !committed {
			batch.Discard()
		}
	}()
	coverage := contributionCoverage{composition: work.composition, slots: make([]slotCoverage, work.composition.Count())}
	nonempty := false
	for position := 0; position < work.composition.Count(); position++ {
		source := input.slot(shape.Slot(position))
		if len(source.targets) == 0 {
			continue
		}
		rows := make([]TargetRegion, 0, len(source.targets))
		for _, row := range source.targets {
			region, valid := batch.And(row.region, pre)
			if !valid {
				return contributionCoverage{}, false
			}
			empty, valid := transportCandidateEmpty(batch, region)
			if !valid {
				return contributionCoverage{}, false
			}
			if empty {
				continue
			}
			region, valid = batch.Reindex(region, omega.relation)
			if !valid {
				return contributionCoverage{}, false
			}
			empty, valid = transportCandidateEmpty(batch, region)
			if !valid {
				return contributionCoverage{}, false
			}
			if empty {
				continue
			}
			region, valid = batch.And(region, post)
			if !valid {
				return contributionCoverage{}, false
			}
			empty, valid = transportCandidateEmpty(batch, region)
			if !valid {
				return contributionCoverage{}, false
			}
			if empty {
				continue
			}
			rows = append(rows, TargetRegion{target: row.target, region: region})
		}
		// Each retained row is constructed by exact And operations from an
		// admitted source row and the target boundary. The algebra therefore
		// proves row ⊆ targetSupport; no second entailment or canonicalization
		// transaction is needed while these masks remain candidates.
		coverage.slots[position] = slotCoverage{targets: rows}
		nonempty = nonempty || len(rows) != 0
	}
	if !batch.Seal() {
		return contributionCoverage{}, false
	}
	committed = true
	if !validTransportCoverageShape(coverage, work.composition) {
		return contributionCoverage{}, false
	}
	if !nonempty {
		coverage.slots = nil
	}
	return coverage, true
}

func (work *Work) validTransportSourceCoverage(input contributionCoverage) bool {
	if work == nil || input.composition != work.composition {
		return false
	}
	for position := 0; position < work.composition.Count(); position++ {
		rows := input.slot(shape.Slot(position)).targets
		for index, row := range rows {
			slot, ok := row.target.Slot()
			if !ok || int(slot) != position || !work.composition.OwnsTarget(slot, row.target) || !row.region.Valid() || row.region.Manager() != work.composition.guards || support.Empty(row.region) {
				return false
			}
			if index != 0 && !rows[index-1].target.Less(row.target) {
				return false
			}
		}
	}
	return true
}

func transportCandidateEmpty(batch *support.Work, region support.Mask) (bool, bool) {
	if batch == nil {
		return false, false
	}
	view, ok := batch.Decompose(region)
	if !ok {
		return false, false
	}
	return view.Terminal && !view.Value, true
}

func validTransportCoverageShape(coverage contributionCoverage, composition *Composition) bool {
	if composition == nil || coverage.composition != composition || len(coverage.slots) != 0 && len(coverage.slots) != composition.Count() {
		return false
	}
	for position := range coverage.slots {
		rows := coverage.slots[position].targets
		for index, row := range rows {
			if !row.region.Valid() || row.region.Manager() != composition.guards || support.Empty(row.region) || index != 0 && !rows[index-1].target.Less(row.target) {
				return false
			}
		}
	}
	return true
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
