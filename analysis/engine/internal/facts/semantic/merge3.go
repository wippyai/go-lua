package semantic

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/diagram"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
)

// ContributionChange is one owner-derived concrete key region whose right
// contribution may have advanced. It is transaction-local traversal input,
// not a retained fact or dependency relation.
type ContributionChange[K scalar.Key] struct {
	Key    K
	Region support.Mask
}

// JoinUnder applies typed Join over the one already-computed carrier split
// using caller-owned typed traversal storage.
func (domain *Domain[F, K, V]) JoinUnder(left, right Plane[F, K, V], split support.Split, scratch *diagram.SoleScratch[K, V], regions *support.Work, report diagram.SoleChange[K]) (Plane[F, K, V], bool) {
	return domain.mergeUnder(left, right, split.Left(), split.Right(), split.Left(), split, scratch, regions, domain.ops.Join, binaryJoin, report)
}

// JoinContributions applies Join only where both independently authored
// target regions cover a concrete key. The supplied regions also carry the
// accumulated left value outside the right coverage, making an uncovered
// sparse zero fold identity while a covered sparse zero denotes Default.
func (domain *Domain[F, K, V]) JoinContributions(left, right Plane[F, K, V], scratch *diagram.SoleScratch[K, V], regions *support.Work, report diagram.SoleChange[K], covers diagram.SoleRegions[K]) (Plane[F, K, V], bool) {
	if !domain.validPlane(left) || !domain.validPlane(right) || scratch == nil || regions == nil || !regions.Open() || report == nil || covers == nil {
		return Plane[F, K, V]{}, false
	}
	values := domain.terminals.Begin()
	builder := domain.diagram.BeginWithTerminals(values)
	if values == nil || builder == nil {
		return Plane[F, K, V]{}, false
	}
	root, ok := builder.MergeSoleFactorRegions(left.root, right.root, scratch, regions, func(key K, first, second terminal.ID[V]) (terminal.ID[V], bool) {
		return domain.terminalsBinary(values, domain.ops.Join, binaryJoin, key)(first, second)
	}, func(first, second terminal.ID[V]) bool {
		return domain.equalTerminal(values, first, second)
	}, report, covers)
	if !ok {
		builder.Discard()
		return Plane[F, K, V]{}, false
	}
	root, ok = builder.Seal(root)
	if !ok {
		return Plane[F, K, V]{}, false
	}
	return Plane[F, K, V]{root: root}, true
}

// JoinContributionsMany is the fixed-order lifted fold used by one point RHS
// transaction. Diagram synchronizes all operand FDDs and authored regions;
// this layer evaluates each completed terminal cell as a local V left-comb
// and admits only its final value. No intermediate Join prefix receives a
// terminal identity or survives in the sealed terminal page. previous is
// passed only for sparse-root reconstruction; it never contributes a value or
// presence lane to the join.
func (domain *Domain[F, K, V]) JoinContributionsMany(previous Plane[F, K, V], inputs []Plane[F, K, V], scratch *diagram.SoleScratch[K, V], regions *support.Work, covers diagram.SoleManyRegions[K]) (Plane[F, K, V], bool) {
	if domain == nil || !domain.validPlane(previous) || len(inputs) == 0 || scratch == nil || regions == nil || !regions.Open() || covers == nil {
		return Plane[F, K, V]{}, false
	}
	for _, input := range inputs {
		if !domain.validPlane(input) {
			return Plane[F, K, V]{}, false
		}
	}
	values := domain.terminals.Begin()
	builder := domain.diagram.BeginWithTerminals(values)
	if values == nil || builder == nil {
		return Plane[F, K, V]{}, false
	}
	// Root values are operation-local inputs, not semantic state. Borrow the
	// caller's scratch vector so a repeated point fold does not allocate a
	// fresh wrapper slice for every operand set.
	scratch.Clear()
	roots := diagram.BorrowRootVector[F, K, V](scratch, len(inputs))
	if len(roots) != len(inputs) {
		builder.Discard()
		return Plane[F, K, V]{}, false
	}
	for index, input := range inputs {
		roots[index] = input.root
	}
	combine := domain.terminalsMany(values)
	dbgSemantic.MergeMany++
	root, ok := builder.MergeSoleFactorMany(previous.root, roots, scratch, regions, combine, covers)
	if !ok {
		builder.Discard()
		return Plane[F, K, V]{}, false
	}
	root, ok = builder.Seal(root)
	if !ok {
		return Plane[F, K, V]{}, false
	}
	return Plane[F, K, V]{root: root}, true
}

// terminalsMany supplies the one typed callback for every arity. The diagram
// hands over the distinct contributions present at a cell, so a covered
// sparse zero arrives as the zero terminal ID and still means Default, while
// an uncovered operand simply does not appear.
func (domain *Domain[F, K, V]) terminalsMany(values *terminal.Work[V]) diagram.SoleManyCombine[K, V] {
	zero := terminal.ID[V]{}
	combine := func(_ K, ids []terminal.ID[V]) (terminal.ID[V], bool) {
		if values == nil || len(ids) == 0 {
			return zero, false
		}
		dbgSemantic.Cells++
		dbgSemantic.CellWidth += uint64(len(ids))
		if uint64(len(ids)) > dbgSemantic.MaxOperand {
			dbgSemantic.MaxOperand = uint64(len(ids))
		}
		var accumulator V
		var accumulatorID terminal.ID[V]
		have := false
		for index := range ids {
			value := domain.ops.Default
			valueID := domain.defaultID
			if ids[index] != zero {
				var valid bool
				value, valid = values.Value(ids[index])
				if !valid {
					return zero, false
				}
				valueID = ids[index]
			}
			if !have {
				accumulator, have = value, true
				accumulatorID = valueID
				continue
			}
			var joined bool
			dbgSemantic.CellPairs++
			if zzProbeCellPairHook != nil {
				zzProbeCellPair(zzProbeDomainLabel(accumulator), accumulatorID != zero, accumulatorID, domain.ops.Fingerprint(accumulator), valueID)
			}
			accumulatorID = terminal.ID[V]{}
			accumulator, joined = domain.joinPair(accumulator, value)
			if !joined {
				return zero, false
			}
		}
		if domain.ops.Equal(accumulator, domain.ops.Default) {
			return zero, true
		}
		if !domain.joinStable(accumulator) {
			return zero, false
		}
		return values.Admit(accumulator)
	}
	return combine
}

// JoinContributionChanges applies a right operand at exact authored/changed
// key regions supplied by the Binding. The final sorted sparse mutations are
// published in one persistent AVL batch; unchanged subtrees and FDDs are
// retained. Supplying the whole right authored surface implements an ordinary
// closed contribution join. Supplying only an ascending publication delta is
// lawful when the caller proves left already contains the prior right version.
func (domain *Domain[F, K, V]) JoinContributionChanges(left, right Plane[F, K, V], changes []ContributionChange[K], scratch *diagram.SoleScratch[K, V], regions *support.Work, report diagram.SoleChange[K], covers diagram.SoleRegions[K]) (Plane[F, K, V], bool) {
	if !domain.validPlane(left) || !domain.validPlane(right) || len(changes) == 0 || scratch == nil || regions == nil || !regions.Open() || report == nil || covers == nil {
		return Plane[F, K, V]{}, false
	}
	values := domain.terminals.Begin()
	builder := domain.diagram.BeginWithTerminals(values)
	if values == nil || builder == nil {
		return Plane[F, K, V]{}, false
	}
	empty, ok := support.FromGuard(domain.guards(), domain.guards().False())
	if !ok {
		builder.Discard()
		return Plane[F, K, V]{}, false
	}
	root, ok := builder.MergeSoleFactorChanges(left.root, right.root, len(changes), scratch, regions, func(key K, first, second terminal.ID[V]) (terminal.ID[V], bool) {
		return domain.terminalsBinary(values, domain.ops.Join, binaryJoin, key)(first, second)
	}, func(first, second terminal.ID[V]) bool {
		return domain.equalTerminal(values, first, second)
	}, report, func(index int) (K, support.Mask, support.Mask, support.Mask, bool) {
		change := changes[index]
		changeView, changeValid := regions.Decompose(change.Region)
		if !changeValid || changeView.Terminal && !changeView.Value || index > 0 && changes[index-1].Key >= change.Key {
			return 0, support.Mask{}, support.Mask{}, support.Mask{}, false
		}
		leftRegion, rightRegion, referenceRegion, covered := covers(change.Key)
		if !covered || !regions.Valid(leftRegion) || !regions.Valid(rightRegion) || !regions.Valid(referenceRegion) {
			return 0, support.Mask{}, support.Mask{}, support.Mask{}, false
		}
		rightRegion, intersected := regions.And(rightRegion, change.Region)
		if !intersected {
			return 0, support.Mask{}, support.Mask{}, support.Mask{}, false
		}
		leftView, leftValid := regions.Decompose(leftRegion)
		if !leftValid {
			return 0, support.Mask{}, support.Mask{}, support.Mask{}, false
		}
		if leftView.Terminal && !leftView.Value {
			leftRegion = empty
		}
		return change.Key, leftRegion, rightRegion, referenceRegion, true
	})
	if !ok {
		builder.Discard()
		return Plane[F, K, V]{}, false
	}
	root, ok = builder.Seal(root)
	if !ok {
		return Plane[F, K, V]{}, false
	}
	return Plane[F, K, V]{root: root}, true
}

// WidenUnderKeys applies Widen only at a presealed Factor-owned concrete key
// scope. Every other key installs the exact right terminal. This is the
// key-level analogue of carrier's mixed-coordinate Widen: no old root may
// survive outside the recurrence scope, and the operation remains one fused
// FDD traversal with the normal exact change report.
func (domain *Domain[F, K, V]) WidenUnderKeys(left, right Plane[F, K, V], split support.Split, scratch *diagram.SoleScratch[K, V], regions *support.Work, report diagram.SoleChange[K], selected func(K) bool) (Plane[F, K, V], bool) {
	if !domain.validPlane(left) || !domain.validPlane(right) || !domain.validSplit(split) || !domain.validSupport(split.Left()) || !domain.validSupport(split.Right()) || scratch == nil || selected == nil || report != nil && (regions == nil || !regions.Open()) {
		return Plane[F, K, V]{}, false
	}
	work := domain.terminals.Begin()
	builder := domain.diagram.BeginWithTerminals(work)
	if work == nil || builder == nil {
		return Plane[F, K, V]{}, false
	}
	root, ok := builder.MergeSoleFactor(left.root, right.root, split.Left(), split.Right(), split.Left(), scratch, regions,
		func(key K, first, second terminal.ID[V]) (terminal.ID[V], bool) {
			if !selected(key) {
				return second, true
			}
			return domain.terminalsBinary(work, domain.ops.Widen, binaryWiden, key)(first, second)
		}, func(first, second terminal.ID[V]) bool {
			return domain.equalTerminal(work, first, second)
		}, report)
	if !ok {
		builder.Discard()
		return Plane[F, K, V]{}, false
	}
	root, ok = builder.Seal(root)
	if !ok {
		return Plane[F, K, V]{}, false
	}
	return Plane[F, K, V]{root: root}, true
}

// NarrowUnderKeys applies Narrow only at a presealed Factor-owned concrete
// key scope. Every other key installs the exact right terminal, so a selected
// Factor cannot silently narrow keys outside its declared recurrence surface.
func (domain *Domain[F, K, V]) NarrowUnderKeys(left, right Plane[F, K, V], split support.Split, scratch *diagram.SoleScratch[K, V], regions *support.Work, report diagram.SoleChange[K], selected func(K) bool) (Plane[F, K, V], bool) {
	if !domain.validPlane(left) || !domain.validPlane(right) || !domain.validSplit(split) || !split.Right().Entails(split.Left()) || scratch == nil || selected == nil || report != nil && (regions == nil || !regions.Open()) {
		return Plane[F, K, V]{}, false
	}
	work := domain.terminals.Begin()
	builder := domain.diagram.BeginWithTerminals(work)
	if work == nil || builder == nil {
		return Plane[F, K, V]{}, false
	}
	root, ok := builder.MergeSoleFactor(left.root, right.root, split.Right(), split.Right(), split.Right(), scratch, regions,
		func(key K, first, second terminal.ID[V]) (terminal.ID[V], bool) {
			if !selected(key) {
				return second, true
			}
			return domain.terminalsBinary(work, domain.ops.Narrow, binaryNarrow, key)(first, second)
		}, func(first, second terminal.ID[V]) bool {
			return domain.equalTerminal(work, first, second)
		}, report)
	if !ok {
		builder.Discard()
		return Plane[F, K, V]{}, false
	}
	root, ok = builder.Seal(root)
	if !ok {
		return Plane[F, K, V]{}, false
	}
	return Plane[F, K, V]{root: root}, true
}

// SelectUnderKeys installs selected from its first input only at selected
// keys, retaining exact everywhere else from its second input. The first
// support must be contained by the second, so exact right-only regions remain
// exact without materializing a separate support partition.
func (domain *Domain[F, K, V]) SelectUnderKeys(selected, exact Plane[F, K, V], split support.Split, scratch *diagram.SoleScratch[K, V], regions *support.Work, report diagram.SoleChange[K], choose func(K) bool) (Plane[F, K, V], bool) {
	if !domain.validPlane(selected) || !domain.validPlane(exact) || !domain.validSplit(split) || !split.Left().Entails(split.Right()) || scratch == nil || choose == nil || report != nil && (regions == nil || !regions.Open()) {
		return Plane[F, K, V]{}, false
	}
	work := domain.terminals.Begin()
	builder := domain.diagram.BeginWithTerminals(work)
	if work == nil || builder == nil {
		return Plane[F, K, V]{}, false
	}
	root, ok := builder.MergeSoleFactor(selected.root, exact.root, split.Left(), split.Right(), split.Right(), scratch, regions,
		func(key K, first, second terminal.ID[V]) (terminal.ID[V], bool) {
			if choose(key) {
				return first, true
			}
			return second, true
		}, func(first, second terminal.ID[V]) bool {
			return domain.equalTerminal(work, first, second)
		}, report)
	if !ok {
		builder.Discard()
		return Plane[F, K, V]{}, false
	}
	root, ok = builder.Seal(root)
	if !ok {
		return Plane[F, K, V]{}, false
	}
	return Plane[F, K, V]{root: root}, true
}

// PreserveUnder transports unselected Factors through the same one-pass
// zipper. Its overlap callback rejects a mismatch instead of quietly applying
// a recurrence operator to a Factor that was not selected for recurrence.
func (domain *Domain[F, K, V]) PreserveUnder(left, right Plane[F, K, V], split support.Split, scratch *diagram.SoleScratch[K, V], regions *support.Work, report diagram.SoleChange[K]) (Plane[F, K, V], bool) {
	return domain.mergeUnder(left, right, split.Left(), split.Right(), split.Left(), split, scratch, regions, func(first, second V) (V, bool) {
		if !domain.ops.Equal(first, second) {
			var zero V
			return zero, false
		}
		return first, true
	}, binaryPreserve, report)
}

// NarrowUnder is a subset transition. Only split.Right is reachable in the
// output, so it is supplied as both input supports to the fused zipper; no
// left-only carrier can survive merely because it existed before narrowing.
func (domain *Domain[F, K, V]) NarrowUnder(left, right Plane[F, K, V], split support.Split, scratch *diagram.SoleScratch[K, V], regions *support.Work, report diagram.SoleChange[K]) (Plane[F, K, V], bool) {
	if !domain.validSplit(split) || !split.Right().Entails(split.Left()) {
		return Plane[F, K, V]{}, false
	}
	// Right is Narrow's desired/lower operand and exact output support.  The
	// published-state delta still compares predecessor left to output under
	// that right support; removed left-only support is carrier-level evidence.
	return domain.mergeUnder(left, right, split.Right(), split.Right(), split.Right(), split, scratch, regions, domain.ops.Narrow, binaryNarrow, report)
}

// ReplaceUnder derives old-to-right differences solely over the overlap of a
// carrier structural replacement. It intentionally does not construct or
// publish a replacement Plane: the caller retains right's already-published
// root exactly. The existing fused sparse/FDD zipper chooses the right
// terminal on every overlap cell and emits the corresponding key deltas.
// Support-only additions and removals are not plane deltas.
func (domain *Domain[F, K, V]) ReplaceUnder(left, right Plane[F, K, V], split support.Split, scratch *diagram.SoleScratch[K, V], regions *support.Work, report diagram.SoleChange[K]) bool {
	if !domain.validPlane(left) || !domain.validPlane(right) || !domain.validSplit(split) || scratch == nil || regions == nil || !regions.Open() || report == nil {
		return false
	}
	overlap := split.Overlap()
	if !domain.validSupport(overlap) {
		return false
	}
	if support.Empty(overlap) {
		return true
	}
	values := domain.terminals.Begin()
	builder := domain.diagram.BeginWithTerminals(values)
	if values == nil || builder == nil {
		return false
	}
	_, ok := builder.MergeSoleFactor(left.root, right.root, overlap, overlap, overlap, scratch, regions,
		func(_ K, _ terminal.ID[V], selected terminal.ID[V]) (terminal.ID[V], bool) {
			return selected, true
		},
		func(first, second terminal.ID[V]) bool {
			return domain.equalTerminal(values, first, second)
		},
		report,
	)
	// The merge result is deliberately discarded. It exists only to drive the
	// one fused delta traversal; retaining it would turn replacement into a
	// masked reconstruction rather than an exact right-root installation.
	builder.Discard()
	return ok
}

func (domain *Domain[F, K, V]) mergeUnder(left, right Plane[F, K, V], leftSupport, rightSupport, referenceSupport support.Mask, split support.Split, scratch *diagram.SoleScratch[K, V], regions *support.Work, operation Binary[V], kind binaryKind, report diagram.SoleChange[K]) (Plane[F, K, V], bool) {
	if !domain.validPlane(left) || !domain.validPlane(right) || !domain.validSplit(split) || !domain.validSupport(leftSupport) || !domain.validSupport(rightSupport) || !domain.validSupport(referenceSupport) || scratch == nil || operation == nil || report != nil && (regions == nil || !regions.Open()) {
		return Plane[F, K, V]{}, false
	}
	work := domain.terminals.Begin()
	builder := domain.diagram.BeginWithTerminals(work)
	if work == nil || builder == nil {
		return Plane[F, K, V]{}, false
	}
	root, ok := builder.MergeSoleFactor(left.root, right.root, leftSupport, rightSupport, referenceSupport, scratch, regions, func(key K, first, second terminal.ID[V]) (terminal.ID[V], bool) {
		return domain.terminalsBinary(work, operation, kind, key)(first, second)
	}, func(first, second terminal.ID[V]) bool {
		return domain.equalTerminal(work, first, second)
	}, report)
	if !ok {
		builder.Discard()
		return Plane[F, K, V]{}, false
	}
	root, ok = builder.Seal(root)
	if !ok {
		return Plane[F, K, V]{}, false
	}
	return Plane[F, K, V]{root: root}, true
}

func (domain *Domain[F, K, V]) validSplit(split support.Split) bool {
	return domain != nil && split.Valid(domain.guards())
}
