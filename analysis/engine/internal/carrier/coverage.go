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

// EmptyContribution pairs an ordinary immutable State with empty authorship.
// It is the unique base/Init construction route; no convenience path infers
// coverage from sparse roots.
func (work *Work) EmptyContribution(state State) (Contribution, bool) {
	if !work.live() || !work.OwnsState(state) || state.previewMarked() || state.contributionMarked() {
		return Contribution{}, false
	}
	return Contribution{state: state, coverage: contributionCoverage{composition: work.composition}}, true
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
			region, ok = support.IntersectWithCheckpoint(work.checkpointFunc(), region, within)
			if !ok {
				return slotCoverage{}, false
			}
		}
		if support.Empty(region) {
			continue
		}
		if len(result) != 0 && result[len(result)-1].target.Same(row.target) {
			region, ok = support.UnionWithCheckpoint(work.checkpointFunc(), result[len(result)-1].region, region)
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
			region, ok = support.IntersectWithCheckpoint(work.checkpointFunc(), region, within)
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
			region, valid := support.UnionWithCheckpoint(work.checkpointFunc(), left.targets[leftIndex].region, right.targets[rightIndex].region)
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
	return work != nil && left.Valid() && right.Valid() && work.EqualUnder(left.state, right.state) && sameContributionCoverage(left.coverage, right.coverage)
}

func coverageRows(slot slotCoverage) SlotCoverage { return SlotCoverage{value: &slot} }

func (work *Work) ReplaceContribution(old, recomputed Contribution) (Contribution, ChangeSet, bool) {
	if !old.Valid() || !recomputed.Valid() {
		return Contribution{}, ChangeSet{}, false
	}
	state, changes, ok := work.Replace(old.state, recomputed.state)
	if !ok {
		return Contribution{}, ChangeSet{}, false
	}
	result := Contribution{state: state, coverage: recomputed.coverage}
	return result, changes, result.Valid()
}

// MergeSelectedContribution is the paired recurrence publication. Semantic
// roots follow the existing selected Widen/Narrow law. During Widen selected
// slots retain the union of current and selected authorship, while unselected
// slots install exact-right authorship; Narrow installs exact-right coverage
// everywhere so a restart/descent can delete stale authored regions.
func (work *Work) MergeSelectedContribution(kind MergeKind, current, selectedRight, exactRight Contribution, selected MergeScope) (Contribution, ChangeSet, bool) {
	if !current.Valid() || !selectedRight.Valid() || !exactRight.Valid() || !selected.validFor(work.composition, kind) {
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
	result := Contribution{state: state, coverage: coverage}
	return result, changes, result.Valid()
}

// TransportContribution applies the exact State boundary and the same
// pre/reindex/post relation to every authored Guard. Targets remain opaque
// Factor capabilities; only their coordinate region moves.
func (work *Work) TransportContribution(input Contribution, pre support.Mask, omega ReindexPlan, post support.Mask) (Contribution, bool) {
	if !input.Valid() || !work.OwnsState(input.state) || !omega.validFor(work.composition) {
		return Contribution{}, false
	}
	state, ok := work.Transport(input.state, pre, omega, post)
	if !ok {
		return Contribution{}, false
	}
	if omega.identity() && pre.IsTrue() && post.IsTrue() {
		return Contribution{state: state, coverage: input.coverage}, true
	}
	coverage := contributionCoverage{composition: work.composition, slots: make([]slotCoverage, work.composition.Count())}
	nonempty := false
	for position := 0; position < work.composition.Count(); position++ {
		source := input.coverage.slot(shape.Slot(position))
		rows := make([]TargetRegion, 0, len(source.targets))
		for _, row := range source.targets {
			region, valid := support.IntersectWithCheckpoint(work.checkpointFunc(), row.region, pre)
			if !valid {
				return Contribution{}, false
			}
			if support.Empty(region) {
				continue
			}
			region, valid = support.Reindex(region, omega.relation)
			if !valid {
				return Contribution{}, false
			}
			region, valid = support.IntersectWithCheckpoint(work.checkpointFunc(), region, post)
			if !valid {
				return Contribution{}, false
			}
			if !support.Empty(region) {
				rows = append(rows, TargetRegion{target: row.target, region: region})
			}
		}
		canonical, valid := work.canonicalCoverage(rows, state.support)
		if !valid {
			return Contribution{}, false
		}
		coverage.slots[position] = canonical
		nonempty = nonempty || len(canonical.targets) != 0
	}
	if !nonempty {
		coverage.slots = nil
	}
	result := Contribution{state: state, coverage: coverage}
	return result, result.Valid()
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
	if !work.live() || !left.Valid() || !right.Valid() || left.coverage.composition != work.composition || right.coverage.composition != work.composition {
		return CoverageChangeSet{}, false
	}
	result := CoverageChangeSet{composition: work.composition}
	for position := 0; position < work.composition.Count(); position++ {
		leftSlot, rightSlot := left.coverage.slot(shape.Slot(position)), right.coverage.slot(shape.Slot(position))
		if sameSlotCoverage(leftSlot, rightSlot) {
			continue
		}
		regions := make([]support.Mask, 0, len(leftSlot.targets)+len(rightSlot.targets))
		leftIndex, rightIndex := 0, 0
		for leftIndex < len(leftSlot.targets) || rightIndex < len(rightSlot.targets) {
			switch {
			case rightIndex == len(rightSlot.targets) || leftIndex < len(leftSlot.targets) && leftSlot.targets[leftIndex].target.Less(rightSlot.targets[rightIndex].target):
				regions = append(regions, leftSlot.targets[leftIndex].region)
				leftIndex++
			case leftIndex == len(leftSlot.targets) || rightSlot.targets[rightIndex].target.Less(leftSlot.targets[leftIndex].target):
				regions = append(regions, rightSlot.targets[rightIndex].region)
				rightIndex++
			default:
				split, ok := support.ThreeWithCheckpoint(work.checkpointFunc(), leftSlot.targets[leftIndex].region, rightSlot.targets[rightIndex].region)
				if !ok {
					return CoverageChangeSet{}, false
				}
				changed, ok := support.UnionWithCheckpoint(work.checkpointFunc(), split.LeftOnly(), split.RightOnly())
				if !ok {
					return CoverageChangeSet{}, false
				}
				if !support.Empty(changed) {
					regions = append(regions, changed)
				}
				leftIndex++
				rightIndex++
			}
		}
		if len(regions) == 0 {
			continue
		}
		region := regions[0]
		for _, next := range regions[1:] {
			var ok bool
			region, ok = support.UnionWithCheckpoint(work.checkpointFunc(), region, next)
			if !ok {
				return CoverageChangeSet{}, false
			}
		}
		if !support.Empty(region) {
			result.rows = append(result.rows, CoverageRegion{slot: shape.Slot(position), region: region})
		}
	}
	return result, true
}
