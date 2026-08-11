package semantic

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/diagram"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
)

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
	}, binaryJoin, report)
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
