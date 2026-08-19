package diagram

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/terminal"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

type trackedTriple[V any] struct {
	base, current *node[V]
	when, within  support.Mask
}

type trackedResult[V any] struct {
	value   *node[V]
	changed support.Mask
}

// trackedInlineDepth keeps an ordinary column rewrite entirely on the
// scratch's own storage. A sparse fact column reconverges over a handful of
// states, and hashing a four-operand key costs more than scanning them; the
// spill map remains for a genuinely wide diagram, so the memo never caps how
// much sharing a traversal may prove.
const trackedInlineDepth = 16

type trackedEntry[V any] struct {
	triple trackedTriple[V]
	result trackedResult[V]
}

// trackedMemo is the exact state memo of one tracked traversal. It is reset
// per traversal and never shared between two of them: the key names operands
// only, never the rewrite applied to them.
type trackedMemo[V any] struct {
	inline [trackedInlineDepth]trackedEntry[V]
	count  int
	spill  map[trackedTriple[V]]trackedResult[V]
}

func (memo *trackedMemo[V]) reset() {
	clear(memo.inline[:memo.count])
	memo.count = 0
	clear(memo.spill)
}

func (memo *trackedMemo[V]) lookup(triple trackedTriple[V]) (trackedResult[V], bool) {
	for index := 0; index < memo.count; index++ {
		if memo.inline[index].triple == triple {
			return memo.inline[index].result, true
		}
	}
	if memo.spill == nil {
		return trackedResult[V]{}, false
	}
	result, found := memo.spill[triple]
	return result, found
}

func (memo *trackedMemo[V]) store(triple trackedTriple[V], result trackedResult[V]) {
	if memo.count < len(memo.inline) {
		memo.inline[memo.count] = trackedEntry[V]{triple: triple, result: result}
		memo.count++
		return
	}
	if memo.spill == nil {
		memo.spill = make(map[trackedTriple[V]]trackedResult[V], trackedInlineDepth*2)
	}
	memo.spill[triple] = result
}

// trackedState is one resolved traversal state: either the final result for
// that state, or the atom and the two successor states to fold.
type trackedState[V any] struct {
	result    trackedResult[V]
	atom      guard.Atom
	low, high trackedTriple[V]
	complete  bool
}

type trackedFrame[V any] struct {
	triple    trackedTriple[V]
	atom      guard.Atom
	low, high trackedTriple[V]
	phase     uint8
}

// TrackedTransform rewrites current exactly where when holds, retains every
// terminal outside it, and computes the net semantic difference from base
// over within in the same iterative FDD/BDD traversal. regions is the
// caller-owned Boolean transaction and scratch is its reusable explicit
// traversal storage; neither is allocated or published per write.
func (builder *Builder[F, K, V]) TrackedTransform(base, current Value[V], when, within support.Mask, regions *support.Work, scratch *SoleScratch[K, V], operation Transform[V], equal func(terminal.ID[V], terminal.ID[V]) bool) (Value[V], support.Mask, bool) {
	return builder.trackedRewrite(base, current, when, within, false, regions, scratch, operation, equal)
}

// TrackedMaskTransform rewrites current where when holds and writes the
// undefined terminal everywhere outside it, computing the same net semantic
// difference from base over within in that one traversal. It is the masked
// half of the identical primitive: restricting a column to its authored
// region and rewriting the terminals it retains is one traversal, never a
// mask followed by a second pointwise pass.
func (builder *Builder[F, K, V]) TrackedMaskTransform(base, current Value[V], when, within support.Mask, regions *support.Work, scratch *SoleScratch[K, V], operation Transform[V], equal func(terminal.ID[V], terminal.ID[V]) bool) (Value[V], support.Mask, bool) {
	return builder.trackedRewrite(base, current, when, within, true, regions, scratch, operation, equal)
}

// trackedRewrite is the one iterative traversal behind both tracked writes.
// erase selects what is written beyond when: the current terminal exactly, or
// the undefined terminal. Because when entails within, an erased subtree
// outside within is uniformly undefined and needs no descent.
func (builder *Builder[F, K, V]) trackedRewrite(base, current Value[V], when, within support.Mask, erase bool, regions *support.Work, scratch *SoleScratch[K, V], operation Transform[V], equal func(terminal.ID[V], terminal.ID[V]) bool) (Value[V], support.Mask, bool) {
	if builder == nil || !builder.open || !builder.validValue(base) || !builder.validValue(current) ||
		regions == nil || !regions.Open() || !regions.Valid(when) || !regions.Valid(within) ||
		when.Manager() != builder.diagram.guards || within.Manager() != builder.diagram.guards || !when.Entails(within) ||
		scratch == nil || operation == nil || equal == nil {
		return Value[V]{}, support.Mask{}, false
	}
	whole := regions.True()
	scratch.tracked.reset()
	clear(scratch.trackedFrames)
	scratch.trackedFrames = scratch.trackedFrames[:0]
	root := trackedTriple[V]{base: base.node, current: current.node, when: when, within: within}
	scratch.trackedFrames = append(scratch.trackedFrames, trackedFrame[V]{triple: root})
	for len(scratch.trackedFrames) != 0 {
		index := len(scratch.trackedFrames) - 1
		frame := &scratch.trackedFrames[index]
		if _, found := scratch.tracked.lookup(frame.triple); found {
			scratch.trackedFrames = scratch.trackedFrames[:index]
			continue
		}
		switch frame.phase {
		case 0:
			state, ok := builder.trackedStep(frame.triple, regions, whole, erase, operation, equal)
			if !ok {
				return Value[V]{}, support.Mask{}, false
			}
			if state.complete {
				scratch.tracked.store(frame.triple, state.result)
				scratch.trackedFrames = scratch.trackedFrames[:index]
				continue
			}
			frame.atom, frame.low, frame.high, frame.phase = state.atom, state.low, state.high, 1
			scratch.trackedFrames = append(scratch.trackedFrames, trackedFrame[V]{triple: state.low})
		case 1:
			frame.phase = 2
			scratch.trackedFrames = append(scratch.trackedFrames, trackedFrame[V]{triple: frame.high})
		default:
			low, lowOK := scratch.tracked.lookup(frame.low)
			high, highOK := scratch.tracked.lookup(frame.high)
			if !lowOK || !highOK {
				return Value[V]{}, support.Mask{}, false
			}
			output := builder.decisionOrExisting(frame.atom, low.value, high.value, frame.triple.current, frame.triple.base)
			changed, ok := regions.Decision(frame.atom, low.changed, high.changed)
			if !ok {
				return Value[V]{}, support.Mask{}, false
			}
			scratch.tracked.store(frame.triple, trackedResult[V]{value: output, changed: changed})
			scratch.trackedFrames = scratch.trackedFrames[:index]
		}
	}
	answer, ok := scratch.tracked.lookup(root)
	if !ok {
		return Value[V]{}, support.Mask{}, false
	}
	return Value[V]{owner: builder.diagram.owner, node: answer.value}, answer.changed, true
}

// trackedStep resolves one traversal state. The four operands are decomposed
// and ranked exactly once here: whether the state is a leaf and, when it is
// not, which atom it splits on are the same question over the same views.
func (builder *Builder[F, K, V]) trackedStep(triple trackedTriple[V], regions *support.Work, whole support.Mask, erase bool, operation Transform[V], equal func(terminal.ID[V], terminal.ID[V]) bool) (trackedState[V], bool) {
	// The unconstrained region decomposes to the same constant view at every
	// state, and a rewrite over whole support reaches it at every one of them.
	// Recognizing that handle keeps the ordinary case a two-operand walk.
	withinView, withinOK := support.Decomposition{Terminal: true, Value: true}, true
	if triple.within != whole {
		withinView, withinOK = regions.Decompose(triple.within)
	}
	whenView, whenOK := withinView, true
	if triple.when != triple.within {
		whenView, whenOK = support.Decomposition{Terminal: true, Value: true}, true
		if triple.when != whole {
			whenView, whenOK = regions.Decompose(triple.when)
		}
	}
	if !withinOK || !whenOK {
		return trackedState[V]{}, false
	}
	if withinView.Terminal && !withinView.Value {
		value := sparseNode(builder, triple.current)
		if erase {
			// when entails within, so this whole subtree lies beyond the
			// rewritten region and becomes sparse absence. Keeping an already
			// undefined current node preserves the exact predecessor pointer,
			// which is what lets an unmoved rewrite republish its input.
			value = undefinedOrZero(builder, triple.current)
		}
		return trackedState[V]{complete: true, result: trackedResult[V]{value: value, changed: regions.False()}}, true
	}
	baseRank, baseOK := builder.diagram.nodeRank(triple.base)
	currentRank, currentOK := builder.diagram.nodeRank(triple.current)
	whenRank, whenRankOK := builder.diagram.regionRank(whenView)
	withinRank, withinRankOK := builder.diagram.regionRank(withinView)
	if !baseOK || !currentOK || !whenRankOK || !withinRankOK {
		return trackedState[V]{}, false
	}
	if rank := minimumMergeRank(baseRank, currentRank, whenRank, withinRank); rank != noRelationRank {
		return builder.trackedBranches(triple, withinView, whenView, baseRank, currentRank, whenRank, withinRank, rank)
	}
	baseID, baseValid := builder.trackedTerminal(triple.base)
	currentID, currentValid := builder.trackedTerminal(triple.current)
	if !baseValid || !currentValid || !withinView.Terminal || !withinView.Value || !whenView.Terminal {
		return trackedState[V]{}, false
	}
	outputID := currentID
	if erase {
		outputID = terminal.ID[V]{}
	}
	if whenView.Value {
		var accepted bool
		outputID, accepted = operation(currentID)
		if !accepted || !builder.validTerminal(outputID) {
			return trackedState[V]{}, false
		}
	}
	changed := regions.False()
	if !equal(baseID, outputID) {
		changed = regions.True()
	}
	return trackedState[V]{complete: true, result: trackedResult[V]{value: builder.adoptTerminal(outputID, triple.current, triple.base), changed: changed}}, true
}

func (builder *Builder[F, K, V]) trackedTerminal(value *node[V]) (terminal.ID[V], bool) {
	if value == nil {
		return terminal.ID[V]{}, true
	}
	return value.value, value.terminal && builder.validTerminal(value.value)
}

// trackedBranches splits one traversal state at the earliest atom any of its
// four operands tests. Its views and ranks come from trackedStep, which has
// already decomposed and ranked them to decide that this state is not a leaf.
func (builder *Builder[F, K, V]) trackedBranches(triple trackedTriple[V], withinView, whenView support.Decomposition, baseRank, currentRank, whenRank, withinRank, rank uint64) (trackedState[V], bool) {
	var atom guard.Atom
	switch {
	case withinRank == rank:
		atom = withinView.Atom
	case whenRank == rank:
		atom = whenView.Atom
	case baseRank == rank:
		atom = triple.base.atom
	default:
		atom = triple.current.atom
	}
	lowWhen, highWhen := triple.when, triple.when
	if whenRank == rank {
		lowWhen, highWhen = whenView.Low, whenView.High
	}
	lowWithin, highWithin := triple.within, triple.within
	if withinRank == rank {
		lowWithin, highWithin = withinView.Low, withinView.High
	}
	return trackedState[V]{
		atom: atom,
		low:  trackedTriple[V]{base: branchNode(triple.base, baseRank, rank, false), current: branchNode(triple.current, currentRank, rank, false), when: lowWhen, within: lowWithin},
		high: trackedTriple[V]{base: branchNode(triple.base, baseRank, rank, true), current: branchNode(triple.current, currentRank, rank, true), when: highWhen, within: highWithin},
	}, true
}

func sparseNode[F ~uint64, K scalar.Key, V any](builder *Builder[F, K, V], value *node[V]) *node[V] {
	if value != nil {
		return value
	}
	return builder.terminal(terminal.ID[V]{})
}

// undefinedOrZero returns the undefined terminal, preferring the node the
// caller already holds. A reduced diagram encodes a uniformly undefined
// region as exactly one terminal node, so this is pointer preservation, not a
// second encoding of absence.
func undefinedOrZero[F ~uint64, K scalar.Key, V any](builder *Builder[F, K, V], value *node[V]) *node[V] {
	if value != nil && value.terminal && value.value == (terminal.ID[V]{}) {
		return value
	}
	return builder.terminal(terminal.ID[V]{})
}
