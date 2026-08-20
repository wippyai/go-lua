package sourcecontrol

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// buildResumeProjection seals the only two structural continuation families
// that cannot be selected from a normal executable root after Source has been
// published.  Construction may inspect Source order once through the exact
// Index position, but the resulting query retains only one dense Term slice
// for each authored family.  No source order, Outcome, edge, or generic node
// relation is copied into the proof.
func buildResumeProjection(
	sourceView source.View,
	flow authored.View,
	bodies *body.Result,
	forest *containment.Result,
	geometryResult *geometry,
) (resumeProof, error) {
	var empty resumeProof
	if !sourceView.Identity().ContentID().Available() || !flow.ContentID().Available() ||
		bodies == nil || forest == nil || geometryResult == nil {
		return empty, errors.New("program/flow/sourcecontrol: resume owner is unavailable")
	}

	counts := geometryResult.counts
	empty.labels = make([]keyspace.Term, counts[keyspace.FamilyLabel])
	empty.loops = make([]keyspace.Term, counts[keyspace.FamilyLoop])

	labels := flow.Control().Labels()
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyLabel]; ordinal++ {
		label := keyspace.MakeTerm(keyspace.FamilyLabel, ordinal)
		owner, ok := labels.Get(label)
		if !ok || !validBody(owner, counts[keyspace.FamilyBody]) {
			return resumeProof{}, errors.New("program/flow/sourcecontrol: malformed Label owner")
		}
		cursor, err := validateResumeSourcePosition(sourceView, bodies, forest, geometryResult, label, owner, geometryResult.labelNodes)
		if err != nil {
			return resumeProof{}, err
		}
		rootCount, ok := bodies.RootCount(owner)
		if !ok || rootCount < 0 || uint64(cursor) > uint64(rootCount) {
			return resumeProof{}, errors.New("program/flow/sourcecontrol: Label cursor is invalid")
		}
		empty.labels[ordinal-1], err = nextDynamicRootOrBody(bodies, forest, owner, cursor, uint32(rootCount), counts)
		if err != nil {
			return resumeProof{}, err
		}
	}

	loops := flow.Control().Loops()
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyLoop]; ordinal++ {
		loop := keyspace.MakeTerm(keyspace.FamilyLoop, ordinal)
		owner, loopBody, _, _, ok := loops.Get(loop)
		if !ok || !validBody(owner, counts[keyspace.FamilyBody]) || !validBody(loopBody, counts[keyspace.FamilyBody]) || owner == loopBody {
			return resumeProof{}, errors.New("program/flow/sourcecontrol: malformed Loop owner")
		}
		bodyParent, bodyParentOK := forest.Parent(loopBody)
		if !bodyParentOK || bodyParent != owner {
			return resumeProof{}, errors.New("program/flow/sourcecontrol: Loop Body owner disagrees with containment")
		}
		cursor, err := validateResumeSourcePosition(sourceView, bodies, forest, geometryResult, loop, owner, geometryResult.loopNodes)
		if err != nil {
			return resumeProof{}, err
		}
		rootCount, ok := bodies.RootCount(owner)
		if !ok || rootCount < 0 || cursor >= uint32(rootCount) {
			return resumeProof{}, errors.New("program/flow/sourcecontrol: Loop cursor is invalid")
		}
		root, rootOK := bodies.RootAt(owner, int(cursor))
		if !rootOK || root != loop {
			return resumeProof{}, errors.New("program/flow/sourcecontrol: Loop is not its exact Body root")
		}
		empty.loops[ordinal-1] = owner
		if cursor+1 < uint32(rootCount) {
			next, nextOK := bodies.RootAt(owner, int(cursor+1))
			if !nextOK || !validPreOutcomeTerm(next, counts) || !body.RootFamily(keyspace.TermFamily(next)) {
				return resumeProof{}, errors.New("program/flow/sourcecontrol: Loop next root is invalid")
			}
			parent, parentOK := forest.Parent(next)
			if !parentOK || parent != owner {
				return resumeProof{}, errors.New("program/flow/sourcecontrol: Loop next root owner disagrees with containment")
			}
			empty.loops[ordinal-1] = next
		}
	}
	return empty, nil
}

// validateResumeSourcePosition checks both halves of the exact direct source
// anchor: Source must report the authored term itself at the supplied Body
// order offset, and the coordinate installed while mapping Source roots must
// agree with Position's root cursor.  This rejects an owner-correct term that
// was moved to a different source-root slot.
func validateResumeSourcePosition(
	sourceView source.View,
	bodies *body.Result,
	forest *containment.Result,
	geometryResult *geometry,
	term keyspace.Term,
	owner keyspace.Term,
	nodes []uint32,
) (uint32, error) {
	root, rootOK := sourceView.Index().Root(term)
	bodyTerm, offset, cursor, positionOK := sourceView.Index().Position(term)
	if !rootOK || root != term || !positionOK || bodyTerm != owner || offset < 0 || cursor < 0 {
		return 0, errors.New("program/flow/sourcecontrol: resume Source position is not an exact root")
	}
	length, lengthOK := sourceView.Order().BodyLen(owner)
	if !lengthOK || offset >= length {
		return 0, errors.New("program/flow/sourcecontrol: resume Source offset is invalid")
	}
	ordered, orderedOK := sourceView.Order().BodyAt(owner, offset)
	if !orderedOK || ordered != term {
		return 0, errors.New("program/flow/sourcecontrol: resume Source order disagrees with Position")
	}
	parent, parentOK := forest.Parent(term)
	if !parentOK || parent != owner {
		return 0, errors.New("program/flow/sourcecontrol: resume Source owner disagrees with containment")
	}
	rootCount, rootCountOK := bodies.RootCount(owner)
	if !rootCountOK || rootCount < 0 || uint64(cursor) > uint64(rootCount) {
		return 0, errors.New("program/flow/sourcecontrol: resume Source cursor is invalid")
	}
	ownerOrdinal := keyspace.TermOrdinal(owner)
	termOrdinal := keyspace.TermOrdinal(term)
	if ownerOrdinal == 0 || uint64(ownerOrdinal) >= uint64(len(geometryResult.coordinates.bodyOffsets)) ||
		termOrdinal == 0 || uint64(termOrdinal) >= uint64(len(nodes)) {
		return 0, errors.New("program/flow/sourcecontrol: resume coordinate denominator is invalid")
	}
	start, end := geometryResult.coordinates.bodyOffsets[ownerOrdinal-1], geometryResult.coordinates.bodyOffsets[ownerOrdinal]
	if end <= start || uint64(cursor) >= uint64(end-start) || start+uint32(cursor) >= geometryResult.coordinates.nodeCount ||
		nodes[termOrdinal] != start+uint32(cursor) {
		return 0, errors.New("program/flow/sourcecontrol: resume Source root cursor disagrees with geometry")
	}
	return uint32(cursor), nil
}

func nextDynamicRootOrBody(
	bodies *body.Result,
	forest *containment.Result,
	owner keyspace.Term,
	cursor,
	rootCount uint32,
	counts [keyspace.FamilyCount]uint32,
) (keyspace.Term, error) {
	for at := cursor; at < rootCount; at++ {
		root, ok := bodies.RootAt(owner, int(at))
		if !ok || !validPreOutcomeTerm(root, counts) || !body.RootFamily(keyspace.TermFamily(root)) {
			return 0, errors.New("program/flow/sourcecontrol: Label next root is invalid")
		}
		parent, parentOK := forest.Parent(root)
		if !parentOK || parent != owner {
			return 0, errors.New("program/flow/sourcecontrol: Label next root owner disagrees with containment")
		}
		if !forest.Static(root) {
			return root, nil
		}
	}
	return owner, nil
}
