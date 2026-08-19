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

type trackedFrame[V any] struct {
	triple    trackedTriple[V]
	atom      guard.Atom
	low, high trackedTriple[V]
	phase     uint8
}

// TrackedTransform rewrites current exactly where when holds and computes the
// net semantic difference from base over within in the same iterative
// FDD/BDD traversal. regions is the Patch-owned Boolean transaction and
// scratch is its reusable explicit traversal storage; neither is allocated or
// published per write.
func (builder *Builder[F, K, V]) TrackedTransform(base, current Value[V], when, within support.Mask, regions *support.Work, scratch *SoleScratch[K, V], operation Transform[V], equal func(terminal.ID[V], terminal.ID[V]) bool) (Value[V], support.Mask, bool) {
	if builder == nil || !builder.open || !builder.validValue(base) || !builder.validValue(current) ||
		regions == nil || !regions.Open() || !regions.Valid(when) || !regions.Valid(within) ||
		when.Manager() != builder.diagram.guards || within.Manager() != builder.diagram.guards || !when.Entails(within) ||
		scratch == nil || operation == nil || equal == nil {
		return Value[V]{}, support.Mask{}, false
	}
	clear(scratch.tracked)
	clear(scratch.trackedFrames)
	scratch.trackedFrames = scratch.trackedFrames[:0]
	root := trackedTriple[V]{base: base.node, current: current.node, when: when, within: within}
	scratch.trackedFrames = append(scratch.trackedFrames, trackedFrame[V]{triple: root})
	for len(scratch.trackedFrames) != 0 {
		index := len(scratch.trackedFrames) - 1
		frame := &scratch.trackedFrames[index]
		if _, found := scratch.tracked[frame.triple]; found {
			scratch.trackedFrames = scratch.trackedFrames[:index]
			continue
		}
		switch frame.phase {
		case 0:
			complete, value, changed, ok := builder.trackedLeaf(frame.triple, regions, operation, equal)
			if !ok {
				return Value[V]{}, support.Mask{}, false
			}
			if complete {
				if scratch.tracked == nil {
					scratch.tracked = make(map[trackedTriple[V]]trackedResult[V])
				}
				scratch.tracked[frame.triple] = trackedResult[V]{value: value, changed: changed}
				scratch.trackedFrames = scratch.trackedFrames[:index]
				continue
			}
			atom, low, high, ok := builder.trackedBranches(frame.triple, regions)
			if !ok {
				return Value[V]{}, support.Mask{}, false
			}
			frame.atom, frame.low, frame.high, frame.phase = atom, low, high, 1
			scratch.trackedFrames = append(scratch.trackedFrames, trackedFrame[V]{triple: low})
		case 1:
			frame.phase = 2
			scratch.trackedFrames = append(scratch.trackedFrames, trackedFrame[V]{triple: frame.high})
		default:
			low, lowOK := scratch.tracked[frame.low]
			high, highOK := scratch.tracked[frame.high]
			if !lowOK || !highOK {
				return Value[V]{}, support.Mask{}, false
			}
			output := builder.decisionOrExisting(frame.atom, low.value, high.value, frame.triple.current, frame.triple.base)
			changed, ok := regions.Decision(frame.atom, low.changed, high.changed)
			if !ok {
				return Value[V]{}, support.Mask{}, false
			}
			scratch.tracked[frame.triple] = trackedResult[V]{value: output, changed: changed}
			scratch.trackedFrames = scratch.trackedFrames[:index]
		}
	}
	answer, ok := scratch.tracked[root]
	if !ok {
		return Value[V]{}, support.Mask{}, false
	}
	return Value[V]{owner: builder.diagram.owner, node: answer.value}, answer.changed, true
}

func (builder *Builder[F, K, V]) trackedLeaf(triple trackedTriple[V], regions *support.Work, operation Transform[V], equal func(terminal.ID[V], terminal.ID[V]) bool) (bool, *node[V], support.Mask, bool) {
	withinView, withinOK := regions.Decompose(triple.within)
	whenView, whenOK := regions.Decompose(triple.when)
	if !withinOK || !whenOK {
		return false, nil, support.Mask{}, false
	}
	if withinView.Terminal && !withinView.Value {
		return true, sparseNode(builder, triple.current), regions.False(), true
	}
	baseRank, baseOK := builder.diagram.nodeRank(triple.base)
	currentRank, currentOK := builder.diagram.nodeRank(triple.current)
	whenRank, whenRankOK := builder.diagram.regionRank(whenView)
	withinRank, withinRankOK := builder.diagram.regionRank(withinView)
	if !baseOK || !currentOK || !whenRankOK || !withinRankOK {
		return false, nil, support.Mask{}, false
	}
	if minimumMergeRank(baseRank, currentRank, whenRank, withinRank) != noRelationRank {
		return false, nil, support.Mask{}, true
	}
	baseID, baseValid := builder.trackedTerminal(triple.base)
	currentID, currentValid := builder.trackedTerminal(triple.current)
	if !baseValid || !currentValid || !withinView.Terminal || !withinView.Value || !whenView.Terminal {
		return false, nil, support.Mask{}, false
	}
	outputID := currentID
	if whenView.Value {
		var accepted bool
		outputID, accepted = operation(currentID)
		if !accepted || !builder.validTerminal(outputID) {
			return false, nil, support.Mask{}, false
		}
	}
	output := builder.terminal(outputID)
	if triple.current != nil && triple.current.terminal && triple.current.value == outputID {
		output = triple.current
	} else if triple.base != nil && triple.base.terminal && triple.base.value == outputID {
		output = triple.base
	}
	changed := regions.False()
	if !equal(baseID, outputID) {
		changed = regions.True()
	}
	return true, output, changed, true
}

func (builder *Builder[F, K, V]) trackedTerminal(value *node[V]) (terminal.ID[V], bool) {
	if value == nil {
		return terminal.ID[V]{}, true
	}
	return value.value, value.terminal && builder.validTerminal(value.value)
}

func (builder *Builder[F, K, V]) trackedBranches(triple trackedTriple[V], regions *support.Work) (guard.Atom, trackedTriple[V], trackedTriple[V], bool) {
	withinView, withinOK := regions.Decompose(triple.within)
	whenView, whenOK := regions.Decompose(triple.when)
	if !withinOK || !whenOK {
		return 0, trackedTriple[V]{}, trackedTriple[V]{}, false
	}
	baseRank, baseOK := builder.diagram.nodeRank(triple.base)
	currentRank, currentOK := builder.diagram.nodeRank(triple.current)
	whenRank, whenOK := builder.diagram.regionRank(whenView)
	withinRank, withinOK := builder.diagram.regionRank(withinView)
	if !baseOK || !currentOK || !whenOK || !withinOK {
		return 0, trackedTriple[V]{}, trackedTriple[V]{}, false
	}
	rank := minimumMergeRank(baseRank, currentRank, whenRank, withinRank)
	if rank == noRelationRank {
		return 0, trackedTriple[V]{}, trackedTriple[V]{}, false
	}
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
	return atom,
		trackedTriple[V]{base: branchNode(triple.base, baseRank, rank, false), current: branchNode(triple.current, currentRank, rank, false), when: lowWhen, within: lowWithin},
		trackedTriple[V]{base: branchNode(triple.base, baseRank, rank, true), current: branchNode(triple.current, currentRank, rank, true), when: highWhen, within: highWithin},
		true
}

func sparseNode[F ~uint64, K scalar.Key, V any](builder *Builder[F, K, V], value *node[V]) *node[V] {
	if value != nil {
		return value
	}
	return builder.terminal(terminal.ID[V]{})
}
