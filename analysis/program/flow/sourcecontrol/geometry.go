package sourcecontrol

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// geometry is Seal-local construction state. The Loop and Label coordinate
// lookup slices are discarded after witness construction; Result retains only
// the exact body/loop coordinate sidecars and the narrow resume projection.
type geometry struct {
	coordinates coordinateProof
	loopNodes   []uint32
	labelNodes  []uint32
	resumes     resumeProof
	counts      [keyspace.FamilyCount]uint32
}

func buildGeometry(
	sourceView source.View,
	flow authored.View,
	bodies *body.Result,
	forest *containment.Result,
	entry keyspace.Term,
) (geometry, error) {
	var empty geometry
	counts, err := geometryCounts(sourceView, flow, bodies, forest, entry)
	if err != nil {
		return empty, err
	}
	result := geometry{counts: counts}
	bodyCount := counts[keyspace.FamilyBody]
	result.coordinates.bodyOffsets = make([]uint32, bodyCount+1)
	result.coordinates.repeatBody = make([]uint32, bodyCount+1)
	for ordinal := range result.coordinates.repeatBody {
		result.coordinates.repeatBody[ordinal] = noNode
	}
	result.loopNodes = make([]uint32, counts[keyspace.FamilyLoop]+1)
	for ordinal := range result.loopNodes {
		result.loopNodes[ordinal] = noNode
	}
	result.labelNodes = make([]uint32, counts[keyspace.FamilyLabel]+1)
	for ordinal := range result.labelNodes {
		result.labelNodes[ordinal] = noNode
	}

	var nodeCount uint64
	for bodyOrdinal := uint32(1); bodyOrdinal <= bodyCount; bodyOrdinal++ {
		bodyTerm := keyspace.MakeTerm(keyspace.FamilyBody, bodyOrdinal)
		rootCount, ok := bodies.RootCount(bodyTerm)
		if !ok || rootCount < 0 {
			return empty, errors.New("program/flow/sourcecontrol: Body root denominator is invalid")
		}
		if nodeCount > uint64(^uint32(0)) || nodeCount > uint64(maxInt()) {
			return empty, errors.New("program/flow/sourcecontrol: coordinate denominator overflow")
		}
		result.coordinates.bodyOffsets[bodyOrdinal-1] = uint32(nodeCount)
		nodeCount += uint64(rootCount) + 1
		if nodeCount > uint64(^uint32(0)) || nodeCount > uint64(maxInt()) {
			return empty, errors.New("program/flow/sourcecontrol: coordinate denominator overflow")
		}
	}
	result.coordinates.bodyOffsets[bodyCount] = uint32(nodeCount)
	if nodeCount == 0 {
		return empty, errors.New("program/flow/sourcecontrol: empty coordinate space")
	}

	if err := installSourceCoordinates(sourceView, &result); err != nil {
		return empty, err
	}
	loops := flow.Control().Loops()
	result.coordinates.loopDecision = make([]uint32, counts[keyspace.FamilyLoop]+1)
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyLoop]; ordinal++ {
		result.coordinates.loopDecision[ordinal] = noNode
	}
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyLoop]; ordinal++ {
		loop := keyspace.MakeTerm(keyspace.FamilyLoop, ordinal)
		owner, child, loopKind, _, ok := loops.Get(loop)
		if !ok || !validBody(owner, bodyCount) || !validBody(child, bodyCount) || owner == child {
			return empty, errors.New("program/flow/sourcecontrol: malformed Loop row")
		}
		if forest.Static(loop) || !dynamicLoopKind(loopKind) {
			continue
		}
		if nodeCount >= uint64(^uint32(0)) || nodeCount >= uint64(maxInt()) {
			return empty, errors.New("program/flow/sourcecontrol: hidden decision denominator overflow")
		}
		decision := uint32(nodeCount)
		result.coordinates.loopDecision[ordinal] = decision
		nodeCount++
		if loopKind == kind.LoopRepeat {
			childOrdinal := keyspace.TermOrdinal(child)
			if result.coordinates.repeatBody[childOrdinal] != noNode {
				return empty, errors.New("program/flow/sourcecontrol: Repeat Body has multiple decisions")
			}
			result.coordinates.repeatBody[childOrdinal] = decision
		}
	}
	if nodeCount > uint64(^uint32(0)) || nodeCount > uint64(maxInt()) {
		return empty, errors.New("program/flow/sourcecontrol: final coordinate denominator overflow")
	}
	result.coordinates.nodeCount = uint32(nodeCount)
	resumes, err := buildResumeProjection(sourceView, flow, bodies, forest, &result)
	if err != nil {
		return empty, err
	}
	result.resumes = resumes
	return result, nil
}

// installSourceCoordinates seals every declared Loop and Label term's exact
// node coordinate through the already sealed source.Index, the same read
// resume.go uses for the narrow resume projection. Index seals each term's
// (Body, Cursor) position once, at Source construction, against the authored
// Body root sequence; this package only converts that sealed pair into its
// own dense node numbering.
func installSourceCoordinates(sourceView source.View, result *geometry) error {
	if result == nil {
		return errors.New("program/flow/sourcecontrol: missing geometry owner")
	}
	index := sourceView.Index()
	for ordinal := uint32(1); ordinal <= result.counts[keyspace.FamilyLoop]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyLoop, ordinal)
		node, err := directSourceCoordinate(index, result, term)
		if err != nil {
			return err
		}
		if err := installRoot(result, term, node); err != nil {
			return err
		}
	}
	for ordinal := uint32(1); ordinal <= result.counts[keyspace.FamilyLabel]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyLabel, ordinal)
		node, err := directSourceCoordinate(index, result, term)
		if err != nil {
			return err
		}
		if err := installRoot(result, term, node); err != nil {
			return err
		}
	}
	return validateLoopCoordinates(result)
}

// directSourceCoordinate converts one declared term's sealed Index position
// into this package's dense node numbering. Root(term) == term proves term is
// its own direct Source occurrence rather than an inherited descendant.
func directSourceCoordinate(index source.Index, result *geometry, term keyspace.Term) (uint32, error) {
	root, rootOK := index.Root(term)
	owner, offset, cursor, posOK := index.Position(term)
	if !rootOK || root != term || !posOK || offset < 0 || cursor < 0 || !validBody(owner, result.counts[keyspace.FamilyBody]) {
		return 0, errors.New("program/flow/sourcecontrol: Source coordinate is not an exact root")
	}
	ownerOrdinal := keyspace.TermOrdinal(owner)
	if ownerOrdinal == 0 || uint64(ownerOrdinal) >= uint64(len(result.coordinates.bodyOffsets)) {
		return 0, errors.New("program/flow/sourcecontrol: Source coordinate denominator is invalid")
	}
	start, end := result.coordinates.bodyOffsets[ownerOrdinal-1], result.coordinates.bodyOffsets[ownerOrdinal]
	if end <= start || uint64(cursor) >= uint64(end-start) {
		return 0, errors.New("program/flow/sourcecontrol: Source coordinate cursor is invalid")
	}
	return start + uint32(cursor), nil
}

// validateLoopCoordinates closes the one root family whose coordinates are
// consumed by structural arc construction but are not covered by the direct
// metadata counters above. Every authored Loop ordinal must have exactly one
// mapped Source coordinate; an unreferenced hole must fail the seal.
func validateLoopCoordinates(result *geometry) error {
	if result == nil {
		return errors.New("program/flow/sourcecontrol: missing geometry owner")
	}
	loopCount := result.counts[keyspace.FamilyLoop]
	if uint64(len(result.loopNodes)) != uint64(loopCount)+1 {
		return errors.New("program/flow/sourcecontrol: Loop coordinate denominator is invalid")
	}
	for ordinal := uint32(1); ordinal <= loopCount; ordinal++ {
		if result.loopNodes[ordinal] == noNode {
			return errors.New("program/flow/sourcecontrol: Loop declaration lacks a coordinate")
		}
	}
	return nil
}

func installRoot(result *geometry, term keyspace.Term, node uint32) error {
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if result == nil || (family != keyspace.FamilyLoop && family != keyspace.FamilyLabel) || ordinal == 0 || node >= result.coordinates.bodyOffsets[len(result.coordinates.bodyOffsets)-1] {
		return errors.New("program/flow/sourcecontrol: Source coordinate is invalid")
	}
	var nodes []uint32
	if family == keyspace.FamilyLoop {
		nodes = result.loopNodes
	} else {
		nodes = result.labelNodes
	}
	if uint64(ordinal) >= uint64(len(nodes)) {
		return errors.New("program/flow/sourcecontrol: Source coordinate is invalid")
	}
	if nodes[ordinal] != noNode {
		return errors.New("program/flow/sourcecontrol: duplicate Source coordinate")
	}
	nodes[ordinal] = node
	return nil
}

func geometryCounts(
	sourceView source.View,
	flow authored.View,
	bodies *body.Result,
	forest *containment.Result,
	entry keyspace.Term,
) ([keyspace.FamilyCount]uint32, error) {
	var counts [keyspace.FamilyCount]uint32
	identity := sourceView.Identity()
	if !identity.ContentID().Available() || !flow.ContentID().Available() || bodies == nil || forest == nil {
		return counts, errors.New("program/flow/sourcecontrol: owner view or proof is unavailable")
	}
	var preOutcome uint64
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := identity.FamilyCount(family)
		if count < 0 || uint64(count) > uint64(keyspace.MaxTermOrdinal) {
			return counts, errors.New("program/flow/sourcecontrol: invalid Source family cardinality")
		}
		counts[family] = uint32(count)
		if family != keyspace.FamilyOutcome {
			preOutcome += uint64(count)
		}
	}
	if preOutcome != uint64(forest.Count()) || uint64(identity.TermCount()) != preOutcome+uint64(counts[keyspace.FamilyOutcome]) {
		return counts, errors.New("program/flow/sourcecontrol: Source/containment denominator mismatch")
	}
	bodyCount := counts[keyspace.FamilyBody]
	if bodyCount == 0 || bodies.BodyCount() != int(bodyCount) || !validBody(entry, bodyCount) {
		return counts, errors.New("program/flow/sourcecontrol: Body denominator mismatch")
	}
	if err := validateBodyParentAgreement(bodies, forest, entry, bodyCount); err != nil {
		return counts, err
	}
	checks := [...]struct {
		family keyspace.Family
		count  int
	}{
		{keyspace.FamilyBind, flow.Storage().Binds().Count()},
		{keyspace.FamilyAssign, flow.Storage().Assigns().Count()},
		{keyspace.FamilyCall, flow.Calls().Count()},
		{keyspace.FamilyFunction, flow.Functions().Count()},
		{keyspace.FamilyReturn, flow.Control().Returns().Count()},
		{keyspace.FamilyBreak, flow.Control().Breaks().Count()},
		{keyspace.FamilyLabel, flow.Control().Labels().Count()},
		{keyspace.FamilyGoto, flow.Control().Gotos().Count()},
		{keyspace.FamilyBranch, flow.Control().Branches().Count()},
		{keyspace.FamilyLoop, flow.Control().Loops().Count()},
	}
	for _, check := range checks {
		if check.count < 0 || uint64(check.count) != uint64(counts[check.family]) {
			return counts, errors.New("program/flow/sourcecontrol: authored family denominator mismatch")
		}
	}
	return counts, nil
}

// validateBodyParentAgreement closes the Body authority splice at the
// earliest sourcecontrol boundary. Body roots and all later structural work
// may have equal cardinalities while carrying a foreign parent image, so the
// two independent Body/containment proofs must agree for every Body before
// any Source coordinate is consumed.
func validateBodyParentAgreement(
	bodies *body.Result,
	forest *containment.Result,
	entry keyspace.Term,
	bodyCount uint32,
) error {
	if bodies == nil || forest == nil || !validBody(entry, bodyCount) {
		return errors.New("program/flow/sourcecontrol: Body parent owner is unavailable")
	}
	for ordinal := uint32(1); ordinal <= bodyCount; ordinal++ {
		bodyTerm := keyspace.MakeTerm(keyspace.FamilyBody, ordinal)
		bodyParent, bodyHasParent := bodies.Parent(bodyTerm)
		forestParent, forestHasParent := forest.Parent(bodyTerm)
		if bodyTerm == entry {
			if bodyHasParent || forestHasParent || bodyParent != 0 || forestParent != 0 {
				return errors.New("program/flow/sourcecontrol: Entry Body has a lexical or containment parent")
			}
			continue
		}
		if !bodyHasParent || !forestHasParent || bodyParent == 0 || bodyParent != forestParent ||
			!validBody(bodyParent, bodyCount) || bodyParent == bodyTerm {
			return errors.New("program/flow/sourcecontrol: Body parent disagrees with containment")
		}
	}
	return nil
}

func validBody(term keyspace.Term, count uint32) bool {
	return keyspace.TermFamily(term) == keyspace.FamilyBody && keyspace.TermOrdinal(term) != 0 && keyspace.TermOrdinal(term) <= count
}

func validPreOutcomeTerm(term keyspace.Term, counts [keyspace.FamilyCount]uint32) bool {
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	return family > keyspace.FamilyInvalid && family < keyspace.FamilyCount && family != keyspace.FamilyOutcome && ordinal != 0 && ordinal <= counts[family]
}

func dynamicLoopKind(loopKind kind.LoopKind) bool {
	return loopKind == kind.LoopRepeat || loopKind == kind.LoopNumericFor || loopKind == kind.LoopGenericFor
}

func maxInt() int { return int(^uint(0) >> 1) }
