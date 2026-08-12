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

// OwnsAdmittedContribution is the exported internal owner fence used by the
// runtime when a Contribution already crossed this exact Work. It is kept
// separate from OwnsContribution, whose historical signature proves a live
// ContributionBase and its callback inputs.
func (work *Work) OwnsAdmittedContribution(contribution Contribution) bool {
	return work.admittedContribution(contribution)
}

// admitContribution attaches the Work-local seal only after the existing
// state/coverage validator succeeds. Every carrier construction route goes
// through this helper so an unsealed or malformed internal value cannot enter
// the admitted fast path.
func (work *Work) admitContribution(state State, coverage contributionCoverage) (Contribution, bool) {
	result := Contribution{state: state, coverage: coverage}
	if work == nil || !work.live() || !work.OwnsState(state) || state.previewMarked() || state.contributionMarked() || !result.Valid() {
		return Contribution{}, false
	}
	result.seal = work.contributionSeal
	result.authority = state.authority
	return result, work.admittedContribution(result)
}

// admitConstructedContribution is the private hot cut for carrier-produced
// publications.  Its callers have already proved the complete State through
// the commit/transport/projection operation and have canonicalized every
// authored row through canonicalCoverage or an equivalent carrier-owned
// relation.  Rechecking those rows here would turn every fold back into an
// O(F) validation pass, so this helper checks only the immutable outer shape
// and reuses the existing Work seal and State authority.
//
// This is deliberately separate from admitContribution: callers that accept
// a State or coverage from an untrusted/public boundary must continue to pay
// the deep Valid check before receiving the seal.
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

// EmptyContribution pairs an ordinary immutable State with empty authorship.
// It is the unique base/Init construction route; the exact initial-root,
// false-support case additionally receives the Work-private neutral seal.
// No convenience path infers coverage from sparse roots.
func (work *Work) EmptyContribution(state State) (Contribution, bool) {
	if !work.live() || !work.OwnsState(state) || state.previewMarked() || state.contributionMarked() {
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
	for index := range left.targets {
		if !left.targets[index].target.Same(right.targets[index].target) || !left.targets[index].region.Equal(right.targets[index].region) {
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
			if !subRow.region.SameHandle(superRow.region) && !subRow.region.Entails(superRow.region) {
				return false
			}
			superIndex++
			subIndex++
		}
	}
	return subIndex == len(sub.targets)
}

func (work *Work) mergeCoverage(left, right contributionCoverage, within support.Mask) (contributionCoverage, bool) {
	if !work.live() || left.composition != work.composition || right.composition != work.composition || !within.Valid() {
		return contributionCoverage{}, false
	}
	count := work.composition.Count()
	result := contributionCoverage{composition: work.composition, slots: make([]slotCoverage, count)}
	nonempty := false
	for position := 0; position < count; position++ {
		var leftSlot, rightSlot slotCoverage
		if len(left.slots) != 0 {
			leftSlot = left.slots[position]
		}
		if len(right.slots) != 0 {
			rightSlot = right.slots[position]
		}
		merged, ok := work.mergeSlotCoverage(leftSlot, rightSlot, within)
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

func (work *Work) restrictCoverage(input contributionCoverage, within support.Mask) (contributionCoverage, bool) {
	empty := contributionCoverage{composition: work.composition}
	return work.mergeCoverage(input, empty, within)
}

// EqualContribution compares both semantic state and authored coverage. A
// coverage-only change is deliberately visible to the solver lifecycle even
// though it remains absent from the semantic ChangeSet.
func (work *Work) EqualContribution(left, right Contribution) bool {
	return work != nil && work.admittedContribution(left) && work.admittedContribution(right) && work.EqualUnder(left.state, right.state) && sameContributionCoverage(left.coverage, right.coverage)
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

// MergeSelectedContribution is the paired recurrence publication. Semantic
// roots follow the existing selected Widen/Narrow law. During Widen selected
// slots retain the union of current and selected authorship, while unselected
// slots install exact-right authorship; Narrow installs exact-right coverage
// everywhere so a restart/descent can delete stale authored regions.
func (work *Work) MergeSelectedContribution(kind MergeKind, current, selectedRight, exactRight Contribution, selected MergeScope) (Contribution, ChangeSet, bool) {
	if !work.admittedContribution(current) || !work.admittedContribution(selectedRight) || !work.admittedContribution(exactRight) || !selected.validFor(work.composition, kind) {
		return Contribution{}, ChangeSet{}, false
	}
	state, changes, ok := work.MergeSelectedUnder(kind, current.state, selectedRight.state, exactRight.state, selected)
	if !ok {
		return Contribution{}, ChangeSet{}, false
	}
	coverage := exactRight.coverage
	if kind == Widen {
		coverage = contributionCoverage{composition: work.composition, slots: make([]slotCoverage, work.composition.Count())}
		nonempty := false
		for position := 0; position < work.composition.Count(); position++ {
			physical := shape.Slot(position)
			var row slotCoverage
			if selected.members[position] {
				row, ok = work.mergeSlotCoverage(current.coverage.slot(physical), selectedRight.coverage.slot(physical), state.support)
			} else {
				row, ok = work.canonicalCoverage(exactRight.coverage.slot(physical).targets, state.support)
			}
			if !ok {
				return Contribution{}, ChangeSet{}, false
			}
			coverage.slots[position] = row
			nonempty = nonempty || len(row.targets) != 0
		}
		if !nonempty {
			coverage.slots = nil
		}
	}
	result, valid := work.admitConstructedContribution(state, coverage)
	return result, changes, valid
}

// TransportContribution applies the exact State boundary and the same
// pre/reindex/post relation to every authored Guard. Targets remain opaque
// Factor capabilities; only their coordinate region moves.
func (work *Work) TransportContribution(input Contribution, pre support.Mask, omega ReindexPlan, post support.Mask) (Contribution, bool) {
	if !work.admittedContribution(input) || !work.OwnsState(input.state) || !omega.validFor(work.composition) {
		return Contribution{}, false
	}
	state, ok := work.Transport(input.state, pre, omega, post)
	if !ok {
		return Contribution{}, false
	}
	if omega.identity() && pre.IsTrue() && post.IsTrue() {
		return work.admitDerivedContribution(input, state, input.coverage)
	}
	coverage, valid := work.transportContributionCoverage(input.coverage, pre, omega, post, state.support)
	if !valid {
		return Contribution{}, false
	}
	return work.admitDerivedContribution(input, state, coverage)
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
}

func (set CoverageChangeSet) Count() int { return len(set.rows) }
func (set CoverageChangeSet) At(index int) (CoverageRegion, bool) {
	if set.composition == nil || index < 0 || index >= len(set.rows) {
		return CoverageRegion{}, false
	}
	return set.rows[index], true
}
func (composition *Composition) OwnsCoverageChangeSet(set CoverageChangeSet) bool {
	return composition != nil && set.composition == composition
}

func (work *Work) CoverageChanges(left, right Contribution) (CoverageChangeSet, bool) {
	if !work.live() || !work.admittedContribution(left) || !work.admittedContribution(right) || left.coverage.composition != work.composition || right.coverage.composition != work.composition {
		return CoverageChangeSet{}, false
	}
	result := CoverageChangeSet{composition: work.composition}
	for position := 0; position < work.composition.Count(); position++ {
		leftSlot, rightSlot := left.coverage.slot(shape.Slot(position)), right.coverage.slot(shape.Slot(position))
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
