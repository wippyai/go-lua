package carrier

import "github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"

type pointFoldCoverageCursor struct {
	rows  []TargetRegion
	index int
}

type pointFoldTransaction struct {
	active       bool
	reference    PointState
	base         PointRHS
	terms        []PointState
	slotTerms    []PointFoldTerm
	patches      []Patch
	cursors      []pointFoldCoverageCursor
	heap         []int
	coverageRows []TargetRegion
	resultSlots  []slotCoverage
}

// BeginPointRHSFold opens the one linear canonical point-fold transaction.
// Terms must subsequently be appended in the executor's sealed order. The
// Work retains only borrowed headers and clears them on Finish or Abort.
func (work *Work) BeginPointRHSFold(reference PointState, base PointRHS) bool {
	if work == nil || !work.live() || !work.admittedPointState(reference) || !work.admittedPointRHS(base) || !reference.Scope().Same(base.Scope()) {
		return false
	}
	if work.pointFold == nil {
		work.pointFold = &pointFoldTransaction{}
	}
	if work.pointFold.active {
		return false
	}
	work.pointFold.clear()
	work.pointFold.active = true
	work.pointFold.reference = reference
	work.pointFold.base = base
	return true
}

func (work *Work) AddPointFoldEnvironment(point PointState) bool {
	return work.appendPointFoldState(point)
}

func (work *Work) AddPointFoldRule(rule RuleContribution) bool {
	if !work.admittedRuleContribution(rule) {
		return false
	}
	point, ok := work.PointStateFromRuleContribution(rule)
	return ok && work.appendPointFoldState(point)
}

func (work *Work) appendPointFoldState(point PointState) bool {
	transaction := work.activePointFold()
	if transaction == nil || !work.admittedPointState(point) || !point.Scope().Same(transaction.base.Scope()) {
		return false
	}
	transaction.terms = append(transaction.terms, point)
	return true
}

func (work *Work) activePointFold() *pointFoldTransaction {
	if work == nil || !work.live() || work.pointFold == nil || !work.pointFold.active {
		return nil
	}
	return work.pointFold
}

// AbortPointRHSFold drops every borrowed operand header without publishing a
// root. It is idempotent only for an active transaction; a caller cannot use
// it to reset unrelated Work state.
func (work *Work) AbortPointRHSFold() bool {
	transaction := work.activePointFold()
	if transaction == nil {
		return false
	}
	transaction.clear()
	return true
}

// FinishPointRHSFold computes one final support/coverage surface, invokes one
// synchronized typed fold per physical slot, and performs one carrier commit.
// It exactly models the old left comb, including its sole raw-header adoption
// case, but publishes no intermediate RHS root vector or terminal prefix. The
// returned ChangeSet is the exact result of that same reference-to-folded
// commit. It is operation-local evidence: callers must consume it immediately
// for that returned RHS and must not retain or replay it against another state.
func (work *Work) FinishPointRHSFold() (PointRHS, ChangeSet, bool) {
	transaction := work.activePointFold()
	if transaction == nil {
		return PointRHS{}, ChangeSet{}, false
	}
	defer transaction.clear()
	if len(transaction.terms) == 0 {
		return transaction.base, ChangeSet{}, true
	}
	for _, slot := range work.slots {
		if _, ok := slot.(PointFoldSlotWork); !ok {
			return PointRHS{}, ChangeSet{}, false
		}
	}

	physical := transaction.base.point
	start := 0
	currentSupport := physical.state.support
	pristine := work.closedInitialPoint(physical)
	for index, point := range transaction.terms {
		if !work.admittedPointState(point) || !point.Scope().Same(transaction.base.Scope()) {
			return PointRHS{}, ChangeSet{}, false
		}
		// This is the exact old zero-copy adoption law. Any earlier term while
		// pristine was a contained C-empty identity, so changing the physical
		// base discards no semantic operand.
		if pristine && work.entailsSupport(currentSupport, point.state.support) {
			physical = point
			start = index + 1
			currentSupport = point.state.support
			pristine = work.closedInitialPoint(point)
			continue
		}
		if !work.entailsSupport(point.state.support, currentSupport) {
			var ok bool
			currentSupport, ok = work.unionSupport(currentSupport, point.state.support)
			if !ok {
				return PointRHS{}, ChangeSet{}, false
			}
		}
		if !emptyContributionCoverage(point.coverage) {
			pristine = false
		}
	}
	finalSupport := currentSupport
	carryBaseOutside := finalSupport.Equal(physical.state.support)
	nextCoverage, ok := work.foldCoverageUnion(transaction, physical.coverage, transaction.terms[start:])
	if !ok {
		return PointRHS{}, ChangeSet{}, false
	}
	split, ok := work.threeSupport(transaction.reference.state.support, finalSupport)
	if !ok {
		return PointRHS{}, ChangeSet{}, false
	}
	delta := work.newSupportWork()
	if delta == nil {
		return PointRHS{}, ChangeSet{}, false
	}
	clear(transaction.patches)
	transaction.patches = transaction.patches[:0]
	for position, rawSlot := range work.slots {
		slot, typed := rawSlot.(PointFoldSlotWork)
		if !typed || !work.live() {
			delta.Discard()
			dropPatches(transaction.patches)
			return PointRHS{}, ChangeSet{}, false
		}
		physicalSlot := shape.Slot(position)
		clear(transaction.slotTerms)
		transaction.slotTerms = transaction.slotTerms[:0]
		for _, term := range transaction.terms[start:] {
			coverage := term.coverage.slot(physicalSlot)
			if len(coverage.targets) == 0 {
				continue
			}
			transaction.slotTerms = append(transaction.slotTerms, PointFoldTerm{root: term.state.roots[position], support: term.state.support, coverage: coverage})
		}
		change, valid := slot.FoldPointRHSUnder(
			transaction.reference.state.roots[position],
			physical.state.roots[position],
			physical.state.support,
			finalSupport,
			coverageRows(physical.coverage.slot(physicalSlot)),
			physical.closed,
			carryBaseOutside,
			transaction.slotTerms,
			delta,
		)
		if !valid {
			delta.Discard()
			dropPatches(transaction.patches)
			return PointRHS{}, ChangeSet{}, false
		}
		if !work.acceptInto(&transaction.patches, transaction.reference.state, change, delta) {
			delta.Discard()
			return PointRHS{}, ChangeSet{}, false
		}
	}
	next, changes, ok := work.commit(transaction.reference.state, transaction.patches, finalSupport, split.RightOnly(), split.LeftOnly(), delta)
	if !ok {
		return PointRHS{}, ChangeSet{}, false
	}
	point := PointState{
		state:     next,
		coverage:  nextCoverage,
		roleSeal:  work.contributionSeal,
		authority: next.authority,
		closed:    !carryBaseOutside || physical.closed,
	}
	if !work.admittedPointState(point) {
		return PointRHS{}, ChangeSet{}, false
	}
	result := PointRHS{point: point, roleSeal: work.contributionSeal}
	return result, changes, work.admittedPointRHS(result) && work.composition.OwnsChangeSet(changes)
}

func emptyContributionCoverage(coverage contributionCoverage) bool {
	if len(coverage.slots) == 0 {
		return true
	}
	for _, slot := range coverage.slots {
		if len(slot.targets) != 0 {
			return false
		}
	}
	return true
}

func (transaction *pointFoldTransaction) clear() {
	if transaction == nil {
		return
	}
	transaction.active = false
	transaction.reference = PointState{}
	transaction.base = PointRHS{}
	clear(transaction.terms)
	transaction.terms = transaction.terms[:0]
	clear(transaction.slotTerms)
	transaction.slotTerms = transaction.slotTerms[:0]
	clear(transaction.patches)
	transaction.patches = transaction.patches[:0]
	for index := range transaction.cursors {
		transaction.cursors[index] = pointFoldCoverageCursor{}
	}
	transaction.cursors = transaction.cursors[:0]
	clear(transaction.heap)
	transaction.heap = transaction.heap[:0]
	clear(transaction.coverageRows)
	transaction.coverageRows = transaction.coverageRows[:0]
	clear(transaction.resultSlots)
	transaction.resultSlots = transaction.resultSlots[:0]
}

func (work *Work) foldCoverageUnion(transaction *pointFoldTransaction, base contributionCoverage, terms []PointState) (contributionCoverage, bool) {
	if work == nil || transaction == nil || base.composition != work.composition {
		return contributionCoverage{}, false
	}
	count := work.composition.Count()
	if cap(transaction.resultSlots) < count {
		transaction.resultSlots = make([]slotCoverage, count)
	} else {
		transaction.resultSlots = transaction.resultSlots[:count]
		clear(transaction.resultSlots)
	}
	nonempty := false
	wholeDominant := -1
	for position := 0; position < count; position++ {
		if !work.live() {
			return contributionCoverage{}, false
		}
		slot := shape.Slot(position)
		sources := 0
		var sole slotCoverage
		total := 0
		dominantSource := -1
		var dominant slotCoverage
		consider := func(source int, coverage slotCoverage) {
			if len(coverage.targets) == 0 {
				return
			}
			sources++
			sole = coverage
			total += len(coverage.targets)
			if dominantSource == -1 {
				dominantSource = source
				dominant = coverage
				return
			}
			if work.slotCoverageContains(dominant, coverage) {
				return
			}
			if work.slotCoverageContains(coverage, dominant) {
				dominantSource = source
				dominant = coverage
				return
			}
			// Keep the later candidate. A still-later source may dominate both
			// this candidate and the earlier incomparable source; the final
			// all-source proof below decides whether sharing is lawful.
			dominantSource = source
			dominant = coverage
		}
		consider(0, base.slot(slot))
		for index, term := range terms {
			if term.coverage.composition != work.composition {
				return contributionCoverage{}, false
			}
			consider(index+1, term.coverage.slot(slot))
		}
		switch sources {
		case 0:
			continue
		case 1:
			transaction.resultSlots[position] = sole
			if dominantSource >= 0 {
				if wholeDominant == -1 {
					wholeDominant = dominantSource
				} else if wholeDominant >= 0 && wholeDominant != dominantSource {
					wholeDominant = -2
				}
			}
			nonempty = true
			continue
		}
		{
			// A later source may dominate an earlier candidate that was
			// incomparable with an intermediate source. Re-prove the final
			// candidate against every source before sharing its header.
			candidateValid := true
			for source := 0; source < len(terms)+1; source++ {
				if !work.slotCoverageContains(dominant, pointFoldSlotCoverageAt(base, terms, slot, source)) {
					candidateValid = false
					break
				}
			}
			if candidateValid {
				transaction.resultSlots[position] = dominant
				if wholeDominant == -1 {
					wholeDominant = dominantSource
				} else if wholeDominant >= 0 && wholeDominant != dominantSource {
					wholeDominant = -2
				}
				nonempty = true
				continue
			}
			wholeDominant = -2
		}
		transaction.cursors = transaction.cursors[:0]
		transaction.heap = transaction.heap[:0]
		appendCursor := func(coverage slotCoverage) {
			if len(coverage.targets) == 0 {
				return
			}
			transaction.cursors = append(transaction.cursors, pointFoldCoverageCursor{rows: coverage.targets})
			transaction.coverageHeapPush(len(transaction.cursors) - 1)
		}
		appendCursor(base.slot(slot))
		for _, term := range terms {
			appendCursor(term.coverage.slot(slot))
		}
		if cap(transaction.coverageRows) < total {
			transaction.coverageRows = make([]TargetRegion, 0, total)
		} else {
			clear(transaction.coverageRows)
			transaction.coverageRows = transaction.coverageRows[:0]
		}
		for len(transaction.heap) != 0 {
			if !work.live() {
				return contributionCoverage{}, false
			}
			cursorIndex := transaction.heap[0]
			row := transaction.cursors[cursorIndex].rows[transaction.cursors[cursorIndex].index]
			target, region := row.target, row.region
			for len(transaction.heap) != 0 {
				cursorIndex = transaction.heap[0]
				cursor := &transaction.cursors[cursorIndex]
				candidate := cursor.rows[cursor.index]
				if !candidate.target.Same(target) {
					break
				}
				_, _ = transaction.coverageHeapPop()
				if !candidate.region.SameHandle(region) {
					var valid bool
					region, valid = work.unionSupport(region, candidate.region)
					if !valid {
						return contributionCoverage{}, false
					}
				}
				cursor.index++
				if cursor.index < len(cursor.rows) {
					transaction.coverageHeapPush(cursorIndex)
				}
			}
			transaction.coverageRows = append(transaction.coverageRows, TargetRegion{target: target, region: region})
		}
		rows := append([]TargetRegion(nil), transaction.coverageRows...)
		transaction.resultSlots[position] = slotCoverage{targets: rows}
		nonempty = true
	}
	if wholeDominant >= 0 {
		return contributionCoverageAt(base, terms, wholeDominant), true
	}
	result := contributionCoverage{composition: work.composition}
	if nonempty {
		result.slots = make([]slotCoverage, count)
		copy(result.slots, transaction.resultSlots)
	}
	return result, true
}

func pointFoldSlotCoverageAt(base contributionCoverage, terms []PointState, slot shape.Slot, source int) slotCoverage {
	if source == 0 {
		return base.slot(slot)
	}
	if source > 0 && source <= len(terms) {
		return terms[source-1].coverage.slot(slot)
	}
	return slotCoverage{}
}

func contributionCoverageAt(base contributionCoverage, terms []PointState, source int) contributionCoverage {
	if source == 0 {
		return base
	}
	if source > 0 && source <= len(terms) {
		return terms[source-1].coverage
	}
	return contributionCoverage{}
}

func (transaction *pointFoldTransaction) coverageHeapLess(left, right int) bool {
	leftCursor := transaction.cursors[transaction.heap[left]]
	rightCursor := transaction.cursors[transaction.heap[right]]
	leftTarget := leftCursor.rows[leftCursor.index].target
	rightTarget := rightCursor.rows[rightCursor.index].target
	return leftTarget.Less(rightTarget)
}

func (transaction *pointFoldTransaction) coverageHeapPush(cursor int) {
	transaction.heap = append(transaction.heap, cursor)
	for index := len(transaction.heap) - 1; index > 0; {
		parent := (index - 1) / 2
		if !transaction.coverageHeapLess(index, parent) {
			break
		}
		transaction.heap[index], transaction.heap[parent] = transaction.heap[parent], transaction.heap[index]
		index = parent
	}
}

func (transaction *pointFoldTransaction) coverageHeapPop() (int, bool) {
	if len(transaction.heap) == 0 {
		return 0, false
	}
	result := transaction.heap[0]
	last := len(transaction.heap) - 1
	transaction.heap[0] = transaction.heap[last]
	transaction.heap[last] = 0
	transaction.heap = transaction.heap[:last]
	for index := 0; ; {
		left := index*2 + 1
		if left >= len(transaction.heap) {
			break
		}
		right, smallest := left+1, left
		if right < len(transaction.heap) && transaction.coverageHeapLess(right, left) {
			smallest = right
		}
		if !transaction.coverageHeapLess(smallest, index) {
			break
		}
		transaction.heap[index], transaction.heap[smallest] = transaction.heap[smallest], transaction.heap[index]
		index = smallest
	}
	return result, true
}
