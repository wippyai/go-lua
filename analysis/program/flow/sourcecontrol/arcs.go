package sourcecontrol

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/control"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func buildArcs(
	flow authored.View,
	bodies *body.Result,
	shape *control.Shape,
	forest *containment.Result,
	geometryResult *geometry,
) (witnessProof, adjacencyProof, error) {
	var emptyWitness witnessProof
	var emptyAdjacency adjacencyProof
	if bodies == nil || shape == nil || forest == nil || geometryResult == nil {
		return emptyWitness, emptyAdjacency, errors.New("program/flow/sourcecontrol: structural owner is unavailable")
	}
	rows := make([]Arc, 0)
	add := func(from, to uint32, sourceTerm, targetTerm, decision keyspace.Term, truth bool) error {
		if from >= geometryResult.coordinates.nodeCount || to >= geometryResult.coordinates.nodeCount || sourceTerm == 0 || targetTerm == 0 {
			return errors.New("program/flow/sourcecontrol: malformed structural arc endpoint")
		}
		if decision == 0 && truth {
			return errors.New("program/flow/sourcecontrol: unguarded arc has truth meaning")
		}
		if decision != 0 && keyspace.TermFamily(decision) != keyspace.FamilyBranch && keyspace.TermFamily(decision) != keyspace.FamilyLoop {
			return errors.New("program/flow/sourcecontrol: arc decision is not Branch or Loop")
		}
		rows = append(rows, Arc{From: from, To: to, Source: sourceTerm, Target: targetTerm, Decision: decision, Truth: truth})
		return nil
	}
	bodyCount := geometryResult.counts[keyspace.FamilyBody]
	controlView := flow.Control()
	for bodyOrdinal := uint32(1); bodyOrdinal <= bodyCount; bodyOrdinal++ {
		owner := keyspace.MakeTerm(keyspace.FamilyBody, bodyOrdinal)
		rootCount, ok := bodies.RootCount(owner)
		if !ok || rootCount < 0 {
			return emptyWitness, emptyAdjacency, errors.New("program/flow/sourcecontrol: Body root range is unavailable")
		}
		for cursor := uint32(0); cursor < uint32(rootCount); cursor++ {
			root, rootOK := bodies.RootAt(owner, int(cursor))
			if !rootOK {
				return emptyWitness, emptyAdjacency, errors.New("program/flow/sourcecontrol: Body root is unavailable")
			}
			if forest.Static(root) {
				continue
			}
			from := geometryResult.coordinates.bodyOffsets[bodyOrdinal-1] + cursor
			next := from + 1
			nextTarget, nextTargetOK := nextRootOrBody(bodies, owner, cursor, uint32(rootCount))
			if !nextTargetOK {
				return emptyWitness, emptyAdjacency, errors.New("program/flow/sourcecontrol: next Body root is unavailable")
			}
			switch keyspace.TermFamily(root) {
			case keyspace.FamilyBind, keyspace.FamilyAssign, keyspace.FamilyCall:
				if err := add(from, next, root, nextTarget, 0, false); err != nil {
					return emptyWitness, emptyAdjacency, err
				}
			case keyspace.FamilyReturn:
				// Return ends at the current activation boundary.
			case keyspace.FamilyBreak:
				loop, selected := shape.BreakLoop(root)
				if !selected || keyspace.TermFamily(loop) != keyspace.FamilyLoop {
					return emptyWitness, emptyAdjacency, errors.New("program/flow/sourcecontrol: Break has no selected Loop")
				}
				if forest.Static(loop) {
					return emptyWitness, emptyAdjacency, errors.New("program/flow/sourcecontrol: dynamic Break targets static Loop")
				}
				loopNode, loopOK := rootCoordinate(geometryResult, loop)
				if !loopOK {
					return emptyWitness, emptyAdjacency, errors.New("program/flow/sourcecontrol: Break target Loop lacks a coordinate")
				}
				if err := add(from, loopNode+1, root, loop, 0, false); err != nil {
					return emptyWitness, emptyAdjacency, err
				}
			case keyspace.FamilyGoto:
				_, label, rowOK := controlView.Gotos().Get(root)
				if !rowOK || keyspace.TermFamily(label) != keyspace.FamilyLabel {
					return emptyWitness, emptyAdjacency, errors.New("program/flow/sourcecontrol: Goto has no Label target")
				}
				if _, bodyOK := shape.GotoTargetBody(root); !bodyOK {
					return emptyWitness, emptyAdjacency, errors.New("program/flow/sourcecontrol: Goto lexical target is unavailable")
				}
				labelNode, labelOK := rootCoordinate(geometryResult, label)
				if !labelOK {
					return emptyWitness, emptyAdjacency, errors.New("program/flow/sourcecontrol: Goto target Label lacks a coordinate")
				}
				if err := add(from, labelNode, root, label, 0, false); err != nil {
					return emptyWitness, emptyAdjacency, err
				}
			case keyspace.FamilyBody:
				child := root
				if !validBody(child, bodyCount) {
					return emptyWitness, emptyAdjacency, errors.New("program/flow/sourcecontrol: nested Body root is invalid")
				}
				childOrdinal := keyspace.TermOrdinal(child)
				childStart, childTail := bodyStart(geometryResult, childOrdinal), bodyTail(geometryResult, childOrdinal)
				if err := add(from, childStart, child, child, 0, false); err != nil {
					return emptyWitness, emptyAdjacency, err
				}
				if err := add(childTail, next, child, nextTarget, 0, false); err != nil {
					return emptyWitness, emptyAdjacency, err
				}
			case keyspace.FamilyBranch:
				_, _, whenTrue, whenFalse, rowOK := controlView.Branches().Get(root)
				if !rowOK || !validBody(whenTrue, bodyCount) || !validBody(whenFalse, bodyCount) || whenTrue == whenFalse {
					return emptyWitness, emptyAdjacency, errors.New("program/flow/sourcecontrol: Branch arm is invalid")
				}
				trueStart, falseStart := bodyStart(geometryResult, keyspace.TermOrdinal(whenTrue)), bodyStart(geometryResult, keyspace.TermOrdinal(whenFalse))
				trueTail, falseTail := bodyTail(geometryResult, keyspace.TermOrdinal(whenTrue)), bodyTail(geometryResult, keyspace.TermOrdinal(whenFalse))
				if err := add(from, trueStart, root, whenTrue, root, true); err != nil {
					return emptyWitness, emptyAdjacency, err
				}
				if err := add(from, falseStart, root, whenFalse, root, false); err != nil {
					return emptyWitness, emptyAdjacency, err
				}
				if err := add(trueTail, next, whenTrue, nextTarget, 0, false); err != nil {
					return emptyWitness, emptyAdjacency, err
				}
				if err := add(falseTail, next, whenFalse, nextTarget, 0, false); err != nil {
					return emptyWitness, emptyAdjacency, err
				}
			case keyspace.FamilyLoop:
				_, loopBody, loopKind, _, rowOK := controlView.Loops().Get(root)
				if !rowOK || !validBody(loopBody, bodyCount) || !validLoop(loopKind) {
					return emptyWitness, emptyAdjacency, errors.New("program/flow/sourcecontrol: Loop body or kind is invalid")
				}
				childOrdinal := keyspace.TermOrdinal(loopBody)
				childStart, childTail := bodyStart(geometryResult, childOrdinal), bodyTail(geometryResult, childOrdinal)
				switch loopKind {
				case kind.LoopWhile:
					if err := add(from, childStart, root, loopBody, root, true); err != nil {
						return emptyWitness, emptyAdjacency, err
					}
					if err := add(from, next, root, nextTarget, root, false); err != nil {
						return emptyWitness, emptyAdjacency, err
					}
					if err := add(childTail, from, loopBody, root, 0, false); err != nil {
						return emptyWitness, emptyAdjacency, err
					}
				case kind.LoopRepeat:
					decision := geometryResult.coordinates.loopDecision[keyspace.TermOrdinal(root)]
					if decision == noNode {
						return emptyWitness, emptyAdjacency, errors.New("program/flow/sourcecontrol: Repeat lacks hidden decision")
					}
					if err := add(from, childStart, root, loopBody, 0, false); err != nil {
						return emptyWitness, emptyAdjacency, err
					}
					if err := add(childTail, decision, loopBody, root, 0, false); err != nil {
						return emptyWitness, emptyAdjacency, err
					}
					// Repeat's false branch re-enters the Body; true exits.
					if err := add(decision, childStart, root, loopBody, root, false); err != nil {
						return emptyWitness, emptyAdjacency, err
					}
					if err := add(decision, next, root, nextTarget, root, true); err != nil {
						return emptyWitness, emptyAdjacency, err
					}
				case kind.LoopNumericFor, kind.LoopGenericFor:
					decision := geometryResult.coordinates.loopDecision[keyspace.TermOrdinal(root)]
					if decision == noNode {
						return emptyWitness, emptyAdjacency, errors.New("program/flow/sourcecontrol: For loop lacks hidden decision")
					}
					if err := add(from, decision, root, root, 0, false); err != nil {
						return emptyWitness, emptyAdjacency, err
					}
					if err := add(decision, childStart, root, loopBody, root, true); err != nil {
						return emptyWitness, emptyAdjacency, err
					}
					if err := add(decision, next, root, nextTarget, root, false); err != nil {
						return emptyWitness, emptyAdjacency, err
					}
					if err := add(childTail, decision, loopBody, root, 0, false); err != nil {
						return emptyWitness, emptyAdjacency, err
					}
				default:
					return emptyWitness, emptyAdjacency, errors.New("program/flow/sourcecontrol: unsupported Loop kind")
				}
			default:
				return emptyWitness, emptyAdjacency, errors.New("program/flow/sourcecontrol: unsupported Body root family")
			}
		}
	}
	if uint64(len(rows)) > uint64(^uint32(0)) {
		return emptyWitness, emptyAdjacency, errors.New("program/flow/sourcecontrol: structural arc denominator overflow")
	}
	witness, err := buildWitnessIndex(rows, geometryResult.counts)
	if err != nil {
		return emptyWitness, emptyAdjacency, err
	}
	adjacency, err := buildAdjacency(geometryResult.coordinates.nodeCount, rows)
	if err != nil {
		return emptyWitness, emptyAdjacency, err
	}
	return witness, adjacency, nil
}

func buildWitnessIndex(rows []Arc, counts [keyspace.FamilyCount]uint32) (witnessProof, error) {
	proof := witnessProof{rows: rows}
	for _, family := range arcSourceFamilyList() {
		if counts[family] == 0 {
			continue
		}
		proof.sourceOffsets[family] = make([]uint32, counts[family]+1)
	}
	for _, row := range rows {
		family, ordinal := keyspace.TermFamily(row.Source), keyspace.TermOrdinal(row.Source)
		if !arcSourceFamily(family) || ordinal == 0 || uint64(ordinal) > uint64(counts[family]) {
			return witnessProof{}, errors.New("program/flow/sourcecontrol: arc Source family is not indexable")
		}
		targetFamily, targetOrdinal := keyspace.TermFamily(row.Target), keyspace.TermOrdinal(row.Target)
		if targetFamily <= keyspace.FamilyInvalid || targetFamily >= keyspace.FamilyCount || targetFamily == keyspace.FamilyOutcome || targetOrdinal == 0 || uint64(targetOrdinal) > uint64(counts[targetFamily]) {
			return witnessProof{}, errors.New("program/flow/sourcecontrol: arc Target anchor is invalid")
		}
		if row.Decision != 0 {
			decisionFamily, decisionOrdinal := keyspace.TermFamily(row.Decision), keyspace.TermOrdinal(row.Decision)
			if (decisionFamily != keyspace.FamilyBranch && decisionFamily != keyspace.FamilyLoop) || decisionOrdinal == 0 || uint64(decisionOrdinal) > uint64(counts[decisionFamily]) {
				return witnessProof{}, errors.New("program/flow/sourcecontrol: arc Decision anchor is invalid")
			}
		} else if row.Truth {
			return witnessProof{}, errors.New("program/flow/sourcecontrol: unguarded arc has truth meaning")
		}
		proof.sourceOffsets[family][ordinal]++
	}
	for _, family := range arcSourceFamilyList() {
		offsets := proof.sourceOffsets[family]
		for index := 1; index < len(offsets); index++ {
			offsets[index] += offsets[index-1]
		}
	}
	for _, family := range arcSourceFamilyList() {
		offsets := proof.sourceOffsets[family]
		if len(offsets) != 0 {
			proof.sourceIndexes[family] = make([]uint32, offsets[len(offsets)-1])
		}
	}
	next := [keyspace.FamilyCount][]uint32{}
	for _, family := range arcSourceFamilyList() {
		if len(proof.sourceOffsets[family]) != 0 {
			next[family] = make([]uint32, len(proof.sourceOffsets[family])-1)
			copy(next[family], proof.sourceOffsets[family][:len(proof.sourceOffsets[family])-1])
		}
	}
	for index, row := range rows {
		family, ordinal := keyspace.TermFamily(row.Source), keyspace.TermOrdinal(row.Source)
		if !arcSourceFamily(family) || ordinal == 0 || uint64(ordinal) >= uint64(len(next[family])+1) {
			return witnessProof{}, errors.New("program/flow/sourcecontrol: arc Source row disappeared during indexing")
		}
		at := next[family][ordinal-1]
		indexes := proof.sourceIndexes[family]
		if uint64(at) >= uint64(len(indexes)) {
			return witnessProof{}, errors.New("program/flow/sourcecontrol: arc Source index escaped family range")
		}
		indexes[at] = uint32(index)
		next[family][ordinal-1]++
	}
	return proof, nil
}

func arcSourceFamilyList() [8]keyspace.Family {
	return [...]keyspace.Family{
		keyspace.FamilyBody, keyspace.FamilyBind, keyspace.FamilyAssign,
		keyspace.FamilyCall, keyspace.FamilyBranch, keyspace.FamilyLoop,
		keyspace.FamilyBreak, keyspace.FamilyGoto,
	}
}

func arcSourceFamily(family keyspace.Family) bool {
	switch family {
	case keyspace.FamilyBody, keyspace.FamilyBind, keyspace.FamilyAssign,
		keyspace.FamilyCall, keyspace.FamilyBranch, keyspace.FamilyLoop,
		keyspace.FamilyBreak, keyspace.FamilyGoto:
		return true
	default:
		return false
	}
}

func buildAdjacency(nodeCount uint32, rows []Arc) (adjacencyProof, error) {
	var adjacency adjacencyProof
	if uint64(len(rows)) > uint64(^uint32(0)) {
		return adjacency, errors.New("program/flow/sourcecontrol: structural adjacency denominator overflow")
	}
	if nodeCount == 0 {
		if len(rows) != 0 {
			return adjacency, errors.New("program/flow/sourcecontrol: adjacency row has no coordinate space")
		}
		return adjacency, nil
	}

	// Raw rows are first bucketed by source. The sealed structural algebra has
	// at most two destinations per source; this is an invariant, not a safety
	// cap. Canonicalization below handles the only possible row cardinalities
	// directly and never invokes a general-purpose sort.
	forwardRawOffsets := make([]uint32, int(nodeCount)+1)
	for _, row := range rows {
		if row.From >= nodeCount || row.To >= nodeCount {
			return adjacency, errors.New("program/flow/sourcecontrol: structural adjacency endpoint is invalid")
		}
		forwardRawOffsets[row.From+1]++
	}
	for index := 1; index < len(forwardRawOffsets); index++ {
		forwardRawOffsets[index] += forwardRawOffsets[index-1]
	}
	for node := uint32(0); node < nodeCount; node++ {
		if forwardRawOffsets[node+1]-forwardRawOffsets[node] > 2 {
			return adjacency, errors.New("program/flow/sourcecontrol: structural source outdegree exceeds two")
		}
	}
	forwardRaw := make([]uint32, len(rows))
	forwardNext := append([]uint32(nil), forwardRawOffsets[:len(forwardRawOffsets)-1]...)
	for _, row := range rows {
		at := forwardNext[row.From]
		forwardRaw[at] = row.To
		forwardNext[row.From]++
	}

	forwardOffsets := make([]uint32, int(nodeCount)+1)
	forwardTargets := make([]uint32, 0, len(rows))
	for node := uint32(0); node < nodeCount; node++ {
		start, end := forwardRawOffsets[node], forwardRawOffsets[node+1]
		switch end - start {
		case 0:
		case 1:
			forwardTargets = append(forwardTargets, forwardRaw[start])
		case 2:
			left, right := forwardRaw[start], forwardRaw[start+1]
			if left > right {
				left, right = right, left
			}
			forwardTargets = append(forwardTargets, left)
			if right != left {
				forwardTargets = append(forwardTargets, right)
			}
		}
		forwardOffsets[node+1] = uint32(len(forwardTargets))
	}
	adjacency.forwardOffsets, adjacency.forwardTargets = forwardOffsets, forwardTargets

	// Forward sources are scanned in ascending order. Thus each reverse target
	// bucket receives strictly ascending, already-deduplicated source nodes.
	reverseOffsets := make([]uint32, int(nodeCount)+1)
	for _, to := range forwardTargets {
		reverseOffsets[to+1]++
	}
	for index := 1; index < len(reverseOffsets); index++ {
		reverseOffsets[index] += reverseOffsets[index-1]
	}
	reverseTargets := make([]uint32, len(forwardTargets))
	reverseNext := append([]uint32(nil), reverseOffsets[:len(reverseOffsets)-1]...)
	for from := uint32(0); from < nodeCount; from++ {
		for edge := forwardOffsets[from]; edge < forwardOffsets[from+1]; edge++ {
			to := forwardTargets[edge]
			reverseTargets[reverseNext[to]] = from
			reverseNext[to]++
		}
	}
	adjacency.reverseOffsets, adjacency.reverseTargets = reverseOffsets, reverseTargets
	return adjacency, nil
}

func nextRootOrBody(bodies *body.Result, owner keyspace.Term, cursor, rootCount uint32) (keyspace.Term, bool) {
	if cursor+1 < rootCount {
		root, ok := bodies.RootAt(owner, int(cursor+1))
		if !ok || root == 0 {
			return 0, false
		}
		return root, true
	}
	return owner, owner != 0
}

func rootCoordinate(result *geometry, term keyspace.Term) (uint32, bool) {
	if result == nil {
		return 0, false
	}
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	var nodes []uint32
	switch family {
	case keyspace.FamilyLoop:
		nodes = result.loopNodes
	case keyspace.FamilyLabel:
		nodes = result.labelNodes
	default:
		return 0, false
	}
	if ordinal == 0 || uint64(ordinal) >= uint64(len(nodes)) {
		return 0, false
	}
	node := nodes[ordinal]
	return node, node != noNode && node < result.coordinates.nodeCount
}

func bodyStart(result *geometry, ordinal uint32) uint32 {
	return result.coordinates.bodyOffsets[ordinal-1]
}

func bodyTail(result *geometry, ordinal uint32) uint32 {
	return result.coordinates.bodyOffsets[ordinal] - 1
}

func validLoop(loopKind kind.LoopKind) bool {
	return loopKind >= kind.LoopWhile && loopKind <= kind.LoopGenericFor
}
