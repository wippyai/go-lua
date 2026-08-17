package carrier

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// Contribution is the sole publishable Rule/Point value. State remains the
// semantic carrier; coverage is the exact authored Target x Guard relation
// needed only while independent producer contributions are folded. Keeping
// the pair opaque prevents sparse Default from being confused with an
// uncovered Factor without exposing typed keys or creating another fact
// plane.
type Contribution struct {
	state    State
	coverage contributionCoverage
	// seal is issued only by an owning Work after the complete contribution
	// (including every coverage row) has passed the ordinary deep validator.
	// It is deliberately private: code outside carrier cannot manufacture the
	// admission proof or retarget a publication to another Work.
	seal      *contributionSeal
	authority *stateAuthority
}

// contributionSeal is one evaluator-local admission capability. Contributions
// retain the same immutable token as they move through a Work's fold; the
// token is not a second state or coverage representation. A Work also owns a
// distinct neutral seal. That seal is issued only after the exact initial-root
// and false-support proof, so the fast merge identity cannot be forged by a
// caller with an ordinary empty-coverage Contribution.
type contributionSeal struct {
	work        *Work
	composition *Composition
}

type contributionCoverage struct {
	composition *Composition
	slots       []slotCoverage
}

type slotCoverage struct {
	targets []TargetRegion
}

// TargetRegion is one immutable authored write surface. Target keeps the
// concrete key set beside its typed Binding; carrier owns only its exact
// nonempty Guard region.
type TargetRegion struct {
	target Target
	region support.Mask
}

func (row TargetRegion) Target() Target       { return row.target }
func (row TargetRegion) Region() support.Mask { return row.region }

// SlotCoverage is the read-only slot-local projection supplied to SlotWork.
// Its backing rows remain carrier-owned and contain no typed key vocabulary.
type SlotCoverage struct{ value *slotCoverage }

func (coverage SlotCoverage) Count() int {
	if coverage.value == nil {
		return 0
	}
	return len(coverage.value.targets)
}

func (coverage SlotCoverage) At(index int) (TargetRegion, bool) {
	if coverage.value == nil || index < 0 || index >= len(coverage.value.targets) {
		return TargetRegion{}, false
	}
	return coverage.value.targets[index], true
}

// State returns the semantic half of this exact paired publication.
func (contribution Contribution) State() State { return contribution.state }
func (contribution Contribution) Support() support.Mask {
	return contribution.state.Support()
}
func (contribution Contribution) Scope() Scope { return contribution.state.Scope() }
func (contribution Contribution) HandleAt(slot shape.Slot) (RootHandle, bool) {
	return contribution.state.HandleAt(slot)
}

func (contribution Contribution) Valid() bool {
	return contribution.state.live() && contribution.coverage.validFor(contribution.state)
}

// admittedContribution is the hot internal boundary for an already published
// Contribution. Public Valid remains deliberately deep; this check is only
// used after carrier construction has issued the private seal and therefore
// proves ownership and the immutable outer shape in O(1), without walking
// authored rows.
func (work *Work) admittedContribution(contribution Contribution) bool {
	if work == nil || !work.live() || work.contributionSeal == nil || contribution.seal == nil ||
		(contribution.seal != work.contributionSeal && contribution.seal != work.neutralSeal) ||
		contribution.seal.work != work || contribution.seal.composition != work.composition {
		return false
	}
	state := contribution.state
	if contribution.authority == nil || state.authority != contribution.authority || state.authority.composition != work.composition || !state.live() || state.previewMarked() || state.contributionMarked() {
		return false
	}
	coverage := contribution.coverage
	return coverage.composition == work.composition && (len(coverage.slots) == 0 || len(coverage.slots) == work.composition.Count())
}

// admitContribution attaches the Work-local seal only after the existing
// state/coverage validator succeeds. Every carrier construction route goes
// through this helper so an unsealed or malformed internal value cannot enter
// the admitted fast path.
func (work *Work) admitContribution(state State, coverage contributionCoverage) (Contribution, bool) {
	result := Contribution{state: state, coverage: coverage}
	if work == nil || !work.live() || !work.OwnsState(state) || state.previewMarked() || state.contributionMarked() || !result.Valid() || !work.closedContributionRoots(state, coverage) {
		return Contribution{}, false
	}
	result.seal = work.contributionSeal
	result.authority = state.authority
	return result, work.admittedContribution(result)
}

// closedContributionRoots is the cold issuance proof for a State accepted
// through the untrusted/raw boundary.  Canonical producer paths close their
// roots while constructing their pending patch and use
// admitConstructedContribution below, so this traversal is never a hot
// admitted-read check or a duplicate per-fold validation.
func (work *Work) closedContributionRoots(state State, coverage contributionCoverage) bool {
	if work == nil || !work.live() || !work.OwnsState(state) || coverage.composition != work.composition {
		return false
	}
	for position, slot := range work.slots {
		if slot == nil || position >= len(state.roots) || !slot.ContributionClosedUnder(state.roots[position], state.support, coverageRows(coverage.slot(shape.Slot(position)))) {
			return false
		}
	}
	return true
}

// admitConstructedContribution is the private hot cut for carrier-produced
// publications.  Its callers have already proved the complete State through
// the commit/transport/projection operation and have canonicalized every
// authored row through canonicalCoverage or an equivalent carrier-owned
// relation.  Rechecking those rows here would turn every fold back into an
// O(F) validation pass, so this helper checks only the immutable outer shape
// and reuses the existing Work seal and State authority.
//
// This is deliberately separate from admitContribution: every caller is a
// canonical carrier producer which has already physically closed its roots at
// the typed construction boundary.  Raw State/coverage admission continues
// through admitContribution and pays the whole-root closure proof.
func (work *Work) admitConstructedContribution(state State, coverage contributionCoverage) (Contribution, bool) {
	if work == nil || !work.live() || !work.OwnsState(state) || state.previewMarked() || state.contributionMarked() || coverage.composition != work.composition || len(coverage.slots) != 0 && len(coverage.slots) != work.composition.Count() {
		return Contribution{}, false
	}
	result := Contribution{state: state, coverage: coverage, seal: work.contributionSeal, authority: state.authority}
	return result, work.admittedContribution(result)
}

// exactNeutralState is the carrier's issuance-time semantic identity proof.
// It checks that roots remain the immutable Composition initial vector and
// support is the exact empty Boolean region. This deep proof is deliberately
// paid only while EmptyContribution issues the private seal; the hot
// neutralContribution check uses that seal (plus ordinary ownership and empty
// coverage admission) without revisiting the root vector. Scope is deliberately
// not folded into this proof: MergeContribution's ordinary Work/scope fence
// checks both operands before the identity is used.
func (work *Work) exactNeutralState(state State) bool {
	return work != nil && work.live() && work.OwnsState(state) &&
		support.Empty(state.support) && sameRootVector(state.roots, work.composition.initial)
}

func (work *Work) admitNeutralContribution(state State, coverage contributionCoverage) (Contribution, bool) {
	result := Contribution{state: state, coverage: coverage}
	if work == nil || work.neutralSeal == nil || !work.exactNeutralState(state) || len(coverage.slots) != 0 || coverage.composition != work.composition || !result.Valid() {
		return Contribution{}, false
	}
	result.seal = work.neutralSeal
	result.authority = state.authority
	return result, work.admittedContribution(result)
}

// neutralContribution reports the private proof carried by a Contribution.
// The deep initial-root/false-support proof runs only when EmptyContribution
// issues the seal. The hot merge check is only token/ownership admission plus
// the empty authored-coverage shape; it never walks the slot vector.
func (work *Work) neutralContribution(contribution Contribution) bool {
	return work != nil && contribution.seal == work.neutralSeal && work.admittedContribution(contribution) && len(contribution.coverage.slots) == 0
}

// EmptyContribution is the sole Point-base construction route.  A raw State
// cannot be relabelled as an empty RHS: only the composition's immutable
// initial root vector is valid, under any already-issued outer support.  The
// false-support case additionally receives the merge-neutral seal; a
// nonempty initial support remains a normal (non-neutral) contribution so a
// later fold cannot silently discard its structural support.
func (work *Work) EmptyContribution(state State) (Contribution, bool) {
	if !work.live() || !work.OwnsState(state) || state.previewMarked() || state.contributionMarked() || !sameRootVector(state.roots, work.composition.initial) {
		return Contribution{}, false
	}
	coverage := contributionCoverage{composition: work.composition}
	if work.exactNeutralState(state) {
		return work.admitNeutralContribution(state, coverage)
	}
	return work.admitContribution(state, coverage)
}

// admitDerivedContribution preserves the neutral seal only when an operation
// started from an already-proved neutral input and the derived State remains
// the exact initial-root/false-support, empty-coverage identity. Ordinary
// derived Contributions use the private constructed cut because Transport
// and ProjectContribution prove their State roots and canonical coverage
// before reaching this boundary. Ordinary empty Contributions never gain the
// neutral proof merely by passing through a transport or projection.
func (work *Work) admitDerivedContribution(input Contribution, state State, coverage contributionCoverage) (Contribution, bool) {
	if work.neutralContribution(input) {
		if result, ok := work.admitNeutralContribution(state, coverage); ok {
			return result, true
		}
	}
	return work.admitConstructedContribution(state, coverage)
}

// rescopeContribution publishes one coordinate-identical PointState in the
// relation's exact target Scope without touching its immutable support,
// roots, or authored coverage. This is a new carrier wrapper, not a second
// semantic root or transport cache.
func (work *Work) rescopeContribution(input Contribution, target Scope) (Contribution, bool) {
	if work == nil || !work.live() || !work.admittedContribution(input) || !target.validFor(work.composition) {
		return Contribution{}, false
	}
	state := input.state
	state.scope = target
	return work.admitConstructedContribution(state, input.coverage)
}

func (coverage contributionCoverage) validFor(state State) bool {
	if !state.live() || coverage.composition == nil || state.authority.composition != coverage.composition || len(coverage.slots) != 0 && len(coverage.slots) != coverage.composition.Count() {
		return false
	}
	for position := range coverage.slots {
		for index, row := range coverage.slots[position].targets {
			slot, ok := row.target.Slot()
			if !ok || int(slot) != position || !coverage.composition.OwnsTarget(slot, row.target) || !row.region.Valid() || row.region.Manager() != coverage.composition.guards || support.Empty(row.region) || index > 0 && !coverage.slots[position].targets[index-1].target.Less(row.target) {
				return false
			}
		}
	}
	return true
}

func (coverage contributionCoverage) slot(slot shape.Slot) slotCoverage {
	if coverage.composition == nil || !coverage.composition.shape.ValidSlot(slot) || len(coverage.slots) == 0 {
		return slotCoverage{}
	}
	return coverage.slots[int(slot)]
}

func sameSlotCoverage(left, right slotCoverage) bool {
	if len(left.targets) != len(right.targets) {
		return false
	}
	if len(left.targets) != 0 && &left.targets[0] == &right.targets[0] {
		return true
	}
	for index := range left.targets {
		if !left.targets[index].target.Same(right.targets[index].target) || !left.targets[index].region.SameHandle(right.targets[index].region) && !left.targets[index].region.Equal(right.targets[index].region) {
			return false
		}
	}
	return true
}

func sameContributionCoverage(left, right contributionCoverage) bool {
	if left.composition == nil || left.composition != right.composition {
		return false
	}
	count := left.composition.Count()
	for position := 0; position < count; position++ {
		var leftSlot, rightSlot slotCoverage
		if len(left.slots) != 0 {
			leftSlot = left.slots[position]
		}
		if len(right.slots) != 0 {
			rightSlot = right.slots[position]
		}
		if !sameSlotCoverage(leftSlot, rightSlot) {
			return false
		}
	}
	return true
}

// unionCoverage joins two already-admitted closed contribution surfaces.
// The caller has proved that output support is the union of the operands, so
// every row is already within the result and needs neither clipping nor a
// repeated Target ownership scan. Cold/raw coverage still enters through the
// validating canonicalCoverage/restrictCoverage paths.
func (work *Work) unionCoverage(left, right contributionCoverage) (contributionCoverage, bool) {
	if !work.live() || left.composition != work.composition || right.composition != work.composition {
		return contributionCoverage{}, false
	}
	result := contributionCoverage{composition: work.composition, slots: make([]slotCoverage, work.composition.Count())}
	nonempty := false
	for position := range result.slots {
		merged, ok := work.unionSlotCoverage(left.slot(shape.Slot(position)), right.slot(shape.Slot(position)))
		if !ok {
			return contributionCoverage{}, false
		}
		result.slots[position] = merged
		nonempty = nonempty || len(merged.targets) != 0
	}
	if !nonempty {
		result.slots = nil
	}
	return result, true
}

func (work *Work) unionSlotCoverage(left, right slotCoverage) (slotCoverage, bool) {
	switch {
	case len(left.targets) == 0:
		return right, true
	case len(right.targets) == 0:
		return left, true
	case sameSlotCoverage(left, right) || slotCoverageContains(left, right):
		return left, true
	case slotCoverageContains(right, left):
		return right, true
	}
	rows := make([]TargetRegion, 0, len(left.targets)+len(right.targets))
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left.targets) || rightIndex < len(right.targets) {
		switch {
		case rightIndex == len(right.targets) || leftIndex < len(left.targets) && left.targets[leftIndex].target.Less(right.targets[rightIndex].target):
			rows = append(rows, left.targets[leftIndex])
			leftIndex++
		case leftIndex == len(left.targets) || right.targets[rightIndex].target.Less(left.targets[leftIndex].target):
			rows = append(rows, right.targets[rightIndex])
			rightIndex++
		default:
			region, ok := work.unionSupport(left.targets[leftIndex].region, right.targets[rightIndex].region)
			if !ok {
				return slotCoverage{}, false
			}
			rows = append(rows, TargetRegion{target: left.targets[leftIndex].target, region: region})
			leftIndex++
			rightIndex++
		}
	}
	return slotCoverage{targets: rows}, true
}

func (work *Work) canonicalCoverage(rows []TargetRegion, within support.Mask) (slotCoverage, bool) {
	if !work.live() || !within.Valid() || within.Manager() != work.composition.guards {
		return slotCoverage{}, false
	}
	if len(rows) == 0 || support.Empty(within) {
		return slotCoverage{}, true
	}
	// Most producer rows are already sorted, unique, and clipped by the
	// preceding exact carrier boundary.  Validate that shape before copying;
	// rows are immutable after this function returns and every caller either
	// supplies an immutable admitted slice or a fresh finish-local slice.  The
	// fallback below remains the sole canonicalizer for arbitrary row order,
	// duplicate targets, or rows that cross the retained support boundary.
	orderedRows := true
	for index, row := range rows {
		slot, ok := row.target.Slot()
		if !ok || !work.composition.OwnsTarget(slot, row.target) || !row.region.Valid() || row.region.Manager() != work.composition.guards || support.Empty(row.region) {
			return slotCoverage{}, false
		}
		if index != 0 && !rows[index-1].target.Less(row.target) {
			orderedRows = false
		}
		if !row.region.Entails(within) {
			orderedRows = false
		}
	}
	if orderedRows {
		return slotCoverage{targets: rows}, true
	}
	ordered := append([]TargetRegion(nil), rows...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].target.Less(ordered[right].target) })
	result := make([]TargetRegion, 0, len(ordered))
	for _, row := range ordered {
		slot, ok := row.target.Slot()
		if !ok || !work.composition.OwnsTarget(slot, row.target) || !row.region.Valid() || row.region.Manager() != work.composition.guards || support.Empty(row.region) {
			return slotCoverage{}, false
		}
		region := row.region
		if !region.Entails(within) {
			region, ok = work.intersectSupport(region, within)
			if !ok {
				return slotCoverage{}, false
			}
		}
		if support.Empty(region) {
			continue
		}
		if len(result) != 0 && result[len(result)-1].target.Same(row.target) {
			region, ok = work.unionSupport(result[len(result)-1].region, region)
			if !ok {
				return slotCoverage{}, false
			}
			result[len(result)-1].region = region
			continue
		}
		result = append(result, TargetRegion{target: row.target, region: region})
	}
	return slotCoverage{targets: result}, true
}

// restrictSlotCoverage clips one already-canonical immutable slot relation.
// The common fold case reuses its backing rows when the destination support
// contains every authored region; it allocates only after the first actual
// clipping or deletion.
func (work *Work) restrictSlotCoverage(input slotCoverage, within support.Mask) (slotCoverage, bool) {
	if !work.live() || !within.Valid() || within.Manager() != work.composition.guards {
		return slotCoverage{}, false
	}
	if len(input.targets) == 0 || support.Empty(within) {
		return slotCoverage{}, true
	}
	var result []TargetRegion
	for index, row := range input.targets {
		slot, ok := row.target.Slot()
		if !ok || !work.composition.OwnsTarget(slot, row.target) || !row.region.Valid() || row.region.Manager() != work.composition.guards || support.Empty(row.region) || index > 0 && !input.targets[index-1].target.Less(row.target) {
			return slotCoverage{}, false
		}
		region := row.region
		if !region.Entails(within) {
			region, ok = work.intersectSupport(region, within)
			if !ok {
				return slotCoverage{}, false
			}
		}
		if region.Equal(row.region) && result == nil {
			continue
		}
		if result == nil {
			result = make([]TargetRegion, 0, len(input.targets))
			result = append(result, input.targets[:index]...)
		}
		if !support.Empty(region) {
			result = append(result, TargetRegion{target: row.target, region: region})
		}
	}
	if result == nil {
		return input, true
	}
	return slotCoverage{targets: result}, true
}

func (work *Work) mergeSlotCoverage(left, right slotCoverage, within support.Mask) (slotCoverage, bool) {
	if len(left.targets) == 0 {
		return work.restrictSlotCoverage(right, within)
	}
	if len(right.targets) == 0 {
		return work.restrictSlotCoverage(left, within)
	}
	left, ok := work.restrictSlotCoverage(left, within)
	if !ok {
		return slotCoverage{}, false
	}
	right, ok = work.restrictSlotCoverage(right, within)
	if !ok {
		return slotCoverage{}, false
	}
	if len(left.targets) == 0 {
		return right, true
	}
	if len(right.targets) == 0 {
		return left, true
	}
	// The union of two canonical relations is one operand whenever the first
	// relation covers every target/region of the second.  Prove that before
	// allocating the merged row vector; this is exact authored coverage
	// inclusion, not a semantic-root shortcut, so coverage-only wake/version
	// evidence remains unchanged by the caller's outer publication.
	if sameSlotCoverage(left, right) || slotCoverageContains(left, right) {
		return left, true
	}
	if slotCoverageContains(right, left) {
		return right, true
	}
	rows := make([]TargetRegion, 0, len(left.targets)+len(right.targets))
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left.targets) || rightIndex < len(right.targets) {
		switch {
		case rightIndex == len(right.targets) || leftIndex < len(left.targets) && left.targets[leftIndex].target.Less(right.targets[rightIndex].target):
			rows = append(rows, left.targets[leftIndex])
			leftIndex++
		case leftIndex == len(left.targets) || right.targets[rightIndex].target.Less(left.targets[leftIndex].target):
			rows = append(rows, right.targets[rightIndex])
			rightIndex++
		default:
			region, valid := work.unionSupport(left.targets[leftIndex].region, right.targets[rightIndex].region)
			if !valid {
				return slotCoverage{}, false
			}
			rows = append(rows, TargetRegion{target: left.targets[leftIndex].target, region: region})
			leftIndex++
			rightIndex++
		}
	}
	result := slotCoverage{targets: rows}
	if sameSlotCoverage(result, left) {
		return left, true
	}
	if sameSlotCoverage(result, right) {
		return right, true
	}
	return result, true
}

// slotCoverageContains proves relation inclusion for canonical, sorted slot
// rows.  `super` contains `sub` exactly when every target in sub is present in
// super and its authored region is included by super's region.  The merge
// union is therefore super.  This helper is intentionally local to the one
// carrier fold and retains no index/cache authority.
func slotCoverageContains(super, sub slotCoverage) bool {
	if len(sub.targets) == 0 {
		return true
	}
	if len(super.targets) < len(sub.targets) {
		return false
	}
	superIndex, subIndex := 0, 0
	for superIndex < len(super.targets) && subIndex < len(sub.targets) {
		superRow, subRow := super.targets[superIndex], sub.targets[subIndex]
		switch {
		case superRow.target.Less(subRow.target):
			superIndex++
		case subRow.target.Less(superRow.target):
			return false
		default:
			if !superRow.target.Same(subRow.target) || !subRow.region.SameHandle(superRow.region) && !subRow.region.Entails(superRow.region) {
				return false
			}
			superIndex++
			subIndex++
		}
	}
	return subIndex == len(sub.targets)
}

// EqualContribution compares closed contributions in the lifted authored
// algebra.  Target rows are deliberately compared by their typed extensional
// presence/value semantics rather than by their private generator spelling:
// two aliasing Target rows can therefore be equal without manufacturing a
// key-level carrier surface. A coverage-only change remains visible whenever
// it changes that extensional surface.
func (work *Work) EqualContribution(left, right Contribution) bool {
	return work != nil && work.admittedContribution(left) && work.admittedContribution(right) && work.equalContributionSurface(left.state, left.coverage, right.state, right.coverage)
}

// validContributionSurface is the common private role fence for all lifted
// comparisons.  A PointState and PointRHS use the same State+C storage as a
// RuleContribution, but they may retain physical root fibers outside their
// support.  Those fibers are intentionally not part of this surface.
func (work *Work) validContributionSurface(state State, coverage contributionCoverage) bool {
	return work != nil && work.live() && work.OwnsState(state) && !state.previewMarked() && !state.contributionMarked() && coverage.composition == work.composition && (len(coverage.slots) == 0 || len(coverage.slots) == work.composition.Count())
}

// lessOrEqContributionSurface proves the lifted partial order over one
// private State+C pair.  Support inclusion is the outer feasibility proof;
// each typed Binding expands opaque Target rows only for the keys it owns.
// It deliberately never calls raw State order, which would inspect latent
// physical point branches or totalize an absent authored cell to Default.
func (work *Work) lessOrEqContributionSurface(leftState State, leftCoverage contributionCoverage, rightState State, rightCoverage contributionCoverage) bool {
	if !work.validContributionSurface(leftState, leftCoverage) || !work.validContributionSurface(rightState, rightCoverage) || !work.liveFor(leftState, rightState) || !leftState.support.Entails(rightState.support) {
		return false
	}
	if sameState(leftState, rightState) && sameContributionCoverage(leftCoverage, rightCoverage) {
		return true
	}
	for position, slot := range work.slots {
		if !work.live() || slot == nil {
			return false
		}
		physical := shape.Slot(position)
		ordered, valid := slot.LessOrEqContributionUnder(leftState.roots[position], rightState.roots[position], leftState.support, rightState.support, coverageRows(leftCoverage.slot(physical)), coverageRows(rightCoverage.slot(physical)))
		if !valid || !ordered {
			return false
		}
	}
	return true
}

func (work *Work) equalContributionSurface(leftState State, leftCoverage contributionCoverage, rightState State, rightCoverage contributionCoverage) bool {
	if work.validContributionSurface(leftState, leftCoverage) && work.validContributionSurface(rightState, rightCoverage) && work.liveFor(leftState, rightState) && sameState(leftState, rightState) && sameContributionCoverage(leftCoverage, rightCoverage) {
		return true
	}
	return work.lessOrEqContributionSurface(leftState, leftCoverage, rightState, rightCoverage) && work.lessOrEqContributionSurface(rightState, rightCoverage, leftState, leftCoverage)
}

func coverageRows(slot slotCoverage) SlotCoverage { return SlotCoverage{value: &slot} }

func (work *Work) ReplaceContribution(old, recomputed Contribution) (Contribution, ChangeSet, bool) {
	if !work.admittedContribution(old) || !work.admittedContribution(recomputed) {
		return Contribution{}, ChangeSet{}, false
	}
	state, changes, ok := work.Replace(old.state, recomputed.state)
	if !ok {
		return Contribution{}, ChangeSet{}, false
	}
	result, valid := work.admitConstructedContribution(state, recomputed.coverage)
	return result, changes, valid
}

// MergeSelectedContribution is the paired closed recurrence publication.
// Its output presence is always the exact RHS coverage.  The phase fences at
// the executor establish that a selected Widen result remains within that
// exact surface; retaining historical current coverage here would make
// Present(Default) survive an exact RHS descent.
func (work *Work) MergeSelectedContribution(kind MergeKind, current, selectedRight, exactRight Contribution, selected MergeScope) (Contribution, ChangeSet, bool) {
	if !work.admittedContribution(current) || !work.admittedContribution(selectedRight) || !work.admittedContribution(exactRight) {
		return Contribution{}, ChangeSet{}, false
	}
	state, changes, ok := work.mergeSelectedContributionSurface(kind, current.state, current.coverage, selectedRight.state, selectedRight.coverage, exactRight.state, exactRight.coverage, selected)
	if !ok {
		return Contribution{}, ChangeSet{}, false
	}
	result, valid := work.admitConstructedContribution(state, exactRight.coverage)
	return result, changes, valid
}

// mergeSelectedContributionSurface is the sole physical recurrence
// transaction for both legacy closed Contributions and nominal PointState /
// PointRHS values.  The inputs are private paired State+C headers, never a
// raw State relabelled as an RHS.  Its output is physically closed to exact C
// even when the current PointState retains a latent root outside support.
//
// Keeping this transaction role-neutral is important: recurrence has one
// carrier authority and one exact-C law, regardless of whether the executor
// is still using compatibility Contribution storage or nominal point roles.
func (work *Work) mergeSelectedContributionSurface(kind MergeKind, currentState State, currentCoverage contributionCoverage, selectedState State, selectedCoverage contributionCoverage, exactState State, exactCoverage contributionCoverage, selected MergeScope) (State, ChangeSet, bool) {
	if !work.validContributionSurface(currentState, currentCoverage) || !work.validContributionSurface(selectedState, selectedCoverage) || !work.validContributionSurface(exactState, exactCoverage) || !selected.validFor(work.composition, kind) {
		return State{}, ChangeSet{}, false
	}
	if kind != Widen && kind != Narrow || !work.liveFor(currentState, selectedState) || !work.liveFor(currentState, exactState) {
		return State{}, ChangeSet{}, false
	}
	hasSelected := false
	for _, member := range selected.members {
		hasSelected = hasSelected || member
	}
	if kind == Widen && !selectedState.support.Equal(exactState.support) || hasSelected && kind == Widen && !currentState.support.Entails(selectedState.support) || hasSelected && kind == Narrow && (!selectedState.support.Entails(currentState.support) || !selectedState.support.Equal(exactState.support)) {
		return State{}, ChangeSet{}, false
	}
	// Narrow has one exact desired RHS. Accepting an independently supplied
	// selected operand would let a foreign authored surface or value influence
	// selected keys before the final exact-C close.
	if kind == Narrow && !work.equalContributionSurface(selectedState, selectedCoverage, exactState, exactCoverage) {
		return State{}, ChangeSet{}, false
	}
	// This proof applies even to an empty selection, whose implementation is
	// an all-slot exact close rather than a typed Widen. The operation still
	// carries Widen phase meaning and therefore may not erase authored
	// presence before that publication cut.
	if kind == Widen {
		if !work.lessOrEqContributionSurface(exactState, exactCoverage, selectedState, selectedCoverage) {
			return State{}, ChangeSet{}, false
		}
		for position, slot := range work.slots {
			if !work.live() || slot == nil {
				return State{}, ChangeSet{}, false
			}
			physical := shape.Slot(position)
			if !slot.ContributionPresenceIncludedUnder(currentState.support, exactState.support, coverageRows(currentCoverage.slot(physical)), coverageRows(exactCoverage.slot(physical))) ||
				!slot.ContributionPresenceIncludedUnder(selectedState.support, exactState.support, coverageRows(selectedCoverage.slot(physical)), coverageRows(exactCoverage.slot(physical))) {
				return State{}, ChangeSet{}, false
			}
		}
	}
	selectedSplit, ok := work.threeSupport(currentState.support, selectedState.support)
	if !ok {
		return State{}, ChangeSet{}, false
	}
	exactSplit, ok := work.threeSupport(currentState.support, exactState.support)
	if !ok {
		return State{}, ChangeSet{}, false
	}
	delta := work.newSupportWork()
	if delta == nil {
		return State{}, ChangeSet{}, false
	}
	patches := make([]Patch, 0, len(work.slots))
	for position, slot := range work.slots {
		if !work.live() || slot == nil {
			delta.Discard()
			dropPatches(patches)
			return State{}, ChangeSet{}, false
		}
		physical := shape.Slot(position)
		var change ChangeHandle
		var valid bool
		if hasSelected && selected.members[position] {
			change, valid = slot.MergeSelectedContributionUnder(kind, selected.scopes[position], currentState.roots[position], selectedState.roots[position], exactState.roots[position], selectedSplit, exactSplit, coverageRows(currentCoverage.slot(physical)), coverageRows(selectedCoverage.slot(physical)), coverageRows(exactCoverage.slot(physical)), delta)
		} else {
			change, valid = slot.CloseContributionUnder(currentState.roots[position], exactState.roots[position], exactSplit, coverageRows(exactCoverage.slot(physical)), delta)
		}
		if !valid || !work.acceptInto(&patches, currentState, change, delta) {
			delta.Discard()
			if valid {
				dropPatches(patches)
			}
			return State{}, ChangeSet{}, false
		}
	}
	added, removed := emptyMask(work.composition.guards), emptyMask(work.composition.guards)
	if !added.Valid() || !removed.Valid() {
		delta.Discard()
		dropPatches(patches)
		return State{}, ChangeSet{}, false
	}
	if !hasSelected {
		// Factor-free recurrence is an exact publication. Unlike a selected
		// Narrow it may replace support in either direction, so preserve both
		// structural sides exactly as Replace would.
		added, removed = exactSplit.RightOnly(), exactSplit.LeftOnly()
	} else if kind == Widen {
		added = exactSplit.RightOnly()
	} else {
		removed = exactSplit.LeftOnly()
	}
	state, changes, ok := work.commit(currentState, patches, exactState.support, added, removed, delta)
	if !ok {
		return State{}, ChangeSet{}, false
	}
	return state, changes, true
}

// TransportPointContribution transports a semantic PointState through the
// total-Default State relation, then closes the result to transported RHS
// coverage.  The point source is not a lifted partial RuleContribution:
// absent reachable source cells are the Factor Default during omega.  C
// decides only which target cells become RHS-present afterwards.
func (work *Work) TransportPointContribution(input Contribution, pre support.Mask, omega ReindexPlan, post support.Mask) (Contribution, bool) {
	if !work.live() || work.reindexing || !work.admittedContribution(input) || !omega.validFor(work.composition) || !input.state.scope.same(omega.source()) || !validBoundaryMask(pre, input.state.scope) || !validBoundaryMask(post, omega.target()) {
		return Contribution{}, false
	}
	if omega.identity() && pre.IsTrue() && post.IsTrue() {
		return input, true
	}
	if omega.coordinateIdentity() && pre.IsTrue() && post.IsTrue() {
		return work.rescopeContribution(input, omega.target())
	}
	work.reindexing = true
	defer func() { work.reindexing = false }()
	sourceSupport, ok := work.intersectSupport(input.state.support, pre)
	if !ok {
		return Contribution{}, false
	}
	reindexedSupport, ok := work.reindexSupport(sourceSupport, omega.relation)
	if !ok {
		return Contribution{}, false
	}
	targetSupport, ok := work.intersectSupport(reindexedSupport, post)
	if !ok {
		return Contribution{}, false
	}
	coverage, ok := work.transportContributionCoverage(input.coverage, pre, omega, post, targetSupport)
	if !ok {
		return Contribution{}, false
	}
	split, ok := work.threeSupport(input.state.support, targetSupport)
	if !ok {
		return Contribution{}, false
	}
	delta := work.newSupportWork()
	if delta == nil {
		return Contribution{}, false
	}
	patches := make([]Patch, 0, len(work.slots))
	for position, slot := range work.slots {
		if !work.live() || slot == nil {
			delta.Discard()
			dropPatches(patches)
			return Contribution{}, false
		}
		physical := shape.Slot(position)
		change, valid := slot.ReindexPointContributionUnder(input.state.roots[position], sourceSupport, targetSupport, omega.relation, coverageRows(coverage.slot(physical)), delta)
		if !valid {
			delta.Discard()
			dropPatches(patches)
			return Contribution{}, false
		}
		if !work.acceptInto(&patches, input.state, change, delta) {
			delta.Discard()
			return Contribution{}, false
		}
	}
	next, _, committed := work.commit(input.state, patches, targetSupport, split.RightOnly(), split.LeftOnly(), delta)
	if !committed {
		return Contribution{}, false
	}
	next.scope = omega.target()
	result, valid := work.admitDerivedContribution(input, next, coverage)
	return result, valid
}

// TransportRuleContribution is the lifted-partial transport for an already
// authored RuleContribution.  Only source-present C fibers participate;
// Present(Default) remains distinct from Absent through noninjective reindex.
// No runtime PointState path calls this operation unless it has a genuine
// RuleContribution source role.
func (work *Work) TransportRuleContribution(input Contribution, pre support.Mask, omega ReindexPlan, post support.Mask) (Contribution, bool) {
	if !work.live() || work.reindexing || !work.admittedContribution(input) || !omega.validFor(work.composition) || !input.state.scope.same(omega.source()) || !validBoundaryMask(pre, input.state.scope) || !validBoundaryMask(post, omega.target()) {
		return Contribution{}, false
	}
	if omega.identity() && pre.IsTrue() && post.IsTrue() {
		return input, true
	}
	work.reindexing = true
	defer func() { work.reindexing = false }()
	sourceSupport, ok := work.intersectSupport(input.state.support, pre)
	if !ok {
		return Contribution{}, false
	}
	reindexedSupport, ok := work.reindexSupport(sourceSupport, omega.relation)
	if !ok {
		return Contribution{}, false
	}
	targetSupport, ok := work.intersectSupport(reindexedSupport, post)
	if !ok {
		return Contribution{}, false
	}
	coverage, ok := work.transportContributionCoverage(input.coverage, pre, omega, post, targetSupport)
	if !ok {
		return Contribution{}, false
	}
	split, ok := work.threeSupport(input.state.support, targetSupport)
	if !ok {
		return Contribution{}, false
	}
	delta := work.newSupportWork()
	if delta == nil {
		return Contribution{}, false
	}
	patches := make([]Patch, 0, len(work.slots))
	for position, slot := range work.slots {
		if !work.live() || slot == nil {
			delta.Discard()
			dropPatches(patches)
			return Contribution{}, false
		}
		physical := shape.Slot(position)
		change, valid := slot.ReindexContributionUnder(input.state.roots[position], sourceSupport, targetSupport, omega.relation, coverageRows(input.coverage.slot(physical)), coverageRows(coverage.slot(physical)), delta)
		if !valid {
			delta.Discard()
			dropPatches(patches)
			return Contribution{}, false
		}
		if !work.acceptInto(&patches, input.state, change, delta) {
			delta.Discard()
			return Contribution{}, false
		}
	}
	next, _, committed := work.commit(input.state, patches, targetSupport, split.RightOnly(), split.LeftOnly(), delta)
	if !committed {
		return Contribution{}, false
	}
	next.scope = omega.target()
	result, valid := work.admitDerivedContribution(input, next, coverage)
	return result, valid
}

// CoverageRegion is one slot-local authorship change. It is structural wake
// evidence only and must never be converted into a semantic FactorRegion.
type CoverageRegion struct {
	slot   shape.Slot
	region support.Mask
}

func (row CoverageRegion) Slot() shape.Slot     { return row.slot }
func (row CoverageRegion) Region() support.Mask { return row.region }

type CoverageChangeSet struct {
	composition *Composition
	rows        []CoverageRegion
	targets     []CoverageTargetRegion
}

// CoverageTargetRegion is the exact Target-local authorship delta retained
// for incremental structural transport. Demand routing deliberately consumes
// only the coarser CoverageRegion projection; typed Binding is the only layer
// allowed to resolve this Target to concrete keys.
type CoverageTargetRegion struct {
	slot   shape.Slot
	target Target
	region support.Mask
}

func (row CoverageTargetRegion) Slot() shape.Slot     { return row.slot }
func (row CoverageTargetRegion) Target() Target       { return row.target }
func (row CoverageTargetRegion) Region() support.Mask { return row.region }

func (set CoverageChangeSet) Count() int { return len(set.rows) }
func (set CoverageChangeSet) TargetCount() int {
	return len(set.targets)
}
func (set CoverageChangeSet) At(index int) (CoverageRegion, bool) {
	if set.composition == nil || index < 0 || index >= len(set.rows) {
		return CoverageRegion{}, false
	}
	return set.rows[index], true
}
func (set CoverageChangeSet) TargetAt(index int) (CoverageTargetRegion, bool) {
	if set.composition == nil || index < 0 || index >= len(set.targets) {
		return CoverageTargetRegion{}, false
	}
	return set.targets[index], true
}
func (composition *Composition) OwnsCoverageChangeSet(set CoverageChangeSet) bool {
	return composition != nil && set.composition == composition
}

// CoverageChanges derives both the slot/guard wake projection and the exact
// Target-local delta used by sparse structural transport.
func (work *Work) CoverageChanges(left, right Contribution) (CoverageChangeSet, bool) {
	if !work.live() || !work.admittedContribution(left) || !work.admittedContribution(right) || left.coverage.composition != work.composition || right.coverage.composition != work.composition {
		return CoverageChangeSet{}, false
	}
	return work.coverageChangesSurface(left.coverage, right.coverage, true)
}

// coverageChangesSurface derives one Target-local delta from two private
// compact coverage headers. It is shared by closed RuleContributions and
// semantic PointStates; only the latter can retain latent payload outside
// support, which never enters this coverage calculation.
func (work *Work) coverageChangesSurface(left, right contributionCoverage, retainTargets bool) (CoverageChangeSet, bool) {
	if !work.live() || left.composition != work.composition || right.composition != work.composition {
		return CoverageChangeSet{}, false
	}
	result := CoverageChangeSet{composition: work.composition}
	for position := 0; position < work.composition.Count(); position++ {
		leftSlot, rightSlot := left.slot(shape.Slot(position)), right.slot(shape.Slot(position))
		if sameSlotCoverage(leftSlot, rightSlot) {
			continue
		}
		var region support.Mask
		hasRegion := false
		leftIndex, rightIndex := 0, 0
		for leftIndex < len(leftSlot.targets) || rightIndex < len(rightSlot.targets) {
			switch {
			case rightIndex == len(rightSlot.targets) || leftIndex < len(leftSlot.targets) && leftSlot.targets[leftIndex].target.Less(rightSlot.targets[rightIndex].target):
				next := leftSlot.targets[leftIndex].region
				if retainTargets {
					result.targets = append(result.targets, CoverageTargetRegion{slot: shape.Slot(position), target: leftSlot.targets[leftIndex].target, region: next})
				}
				if !support.Empty(next) && !hasRegion {
					region, hasRegion = next, true
				} else if !support.Empty(next) {
					var ok bool
					region, ok = work.unionSupport(region, next)
					if !ok {
						return CoverageChangeSet{}, false
					}
				}
				leftIndex++
			case leftIndex == len(leftSlot.targets) || rightSlot.targets[rightIndex].target.Less(leftSlot.targets[leftIndex].target):
				next := rightSlot.targets[rightIndex].region
				if retainTargets {
					result.targets = append(result.targets, CoverageTargetRegion{slot: shape.Slot(position), target: rightSlot.targets[rightIndex].target, region: next})
				}
				if !support.Empty(next) && !hasRegion {
					region, hasRegion = next, true
				} else if !support.Empty(next) {
					var ok bool
					region, ok = work.unionSupport(region, next)
					if !ok {
						return CoverageChangeSet{}, false
					}
				}
				rightIndex++
			default:
				split, ok := work.threeSupport(leftSlot.targets[leftIndex].region, rightSlot.targets[rightIndex].region)
				if !ok {
					return CoverageChangeSet{}, false
				}
				changed, ok := work.unionSupport(split.LeftOnly(), split.RightOnly())
				if !ok {
					return CoverageChangeSet{}, false
				}
				if !support.Empty(changed) {
					if retainTargets {
						result.targets = append(result.targets, CoverageTargetRegion{slot: shape.Slot(position), target: leftSlot.targets[leftIndex].target, region: changed})
					}
					if !hasRegion {
						region, hasRegion = changed, true
					} else {
						region, ok = work.unionSupport(region, changed)
						if !ok {
							return CoverageChangeSet{}, false
						}
					}
				}
				leftIndex++
				rightIndex++
			}
		}
		if !hasRegion {
			continue
		}
		if !support.Empty(region) {
			result.rows = append(result.rows, CoverageRegion{slot: shape.Slot(position), region: region})
		}
	}
	return result, true
}
