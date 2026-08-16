package sourcecontrol

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/control"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Seal constructs the one assembly-local source-control proof. Source has
// already committed its authored Index, so arbitrary occurrence frontiers are
// queried through source.View rather than copied into another all-Term plane.
// Function Body availability is used only while solving reachability and is
// discarded before Result is returned.
func Seal(
	sourceView source.View,
	flow authored.View,
	bodies *body.Result,
	forest *containment.Result,
	shape *control.Shape,
	entry keyspace.Term,
	staticID identity.ContentID,
	moduleID identity.ContentID,
) (*Result, error) {
	sourceID := sourceView.Identity().ContentID()
	flowID := flow.Cold().ContentID()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() {
		return nil, errors.New("program/flow/sourcecontrol: owner identity is unavailable")
	}
	if !body.Matches(bodies, sourceID, flowID) {
		return nil, errors.New("program/flow/sourcecontrol: Body provenance disagrees with Source or Flow")
	}
	if !containment.Matches(forest, sourceID, flowID, staticID, moduleID) {
		return nil, errors.New("program/flow/sourcecontrol: containment provenance disagrees with Source, Flow, Static, or Module")
	}
	if !control.Matches(shape, sourceID, flowID, staticID, moduleID) {
		return nil, errors.New("program/flow/sourcecontrol: control provenance disagrees with Source, Flow, Static, or Module")
	}
	geometryResult, err := buildGeometry(sourceView, flow, bodies, forest, entry)
	if err != nil {
		return nil, err
	}
	witnesses, adjacency, err := buildArcs(flow, bodies, shape, forest, &geometryResult)
	if err != nil {
		return nil, err
	}
	reachable, activationRoots, err := solveReachability(sourceView, flow, bodies, forest, &geometryResult, &adjacency, entry)
	if err != nil {
		return nil, err
	}
	roots := dominanceRoots(&geometryResult, entry, activationRoots)
	proof, err := sealDominance(geometryResult.coordinates.nodeCount, adjacency, roots)
	if err != nil {
		return nil, err
	}
	result := &Result{
		sourceID:      sourceID,
		coordinates:   geometryResult.coordinates,
		resumes:       geometryResult.resumes,
		adjacency:     adjacency,
		witnesses:     witnesses,
		reachable:     reachable,
		dominance:     proof,
		flowID:        flowID,
		staticID:      staticID,
		moduleID:      moduleID,
		catalog:       &catalogLifecycle{phase: catalogUninstalled},
		outcomePhases: &outcomePhaseLifecycle{state: outcomePhaseUnissued},
	}
	result.catalog.owner = result
	result.outcomePhases.owner = result
	return result, nil
}

type availabilityRow struct {
	from uint32
	to   uint32
}

func solveReachability(
	sourceView source.View,
	flow authored.View,
	bodies *body.Result,
	forest *containment.Result,
	geometryResult *geometry,
	adjacency *adjacencyProof,
	entry keyspace.Term,
) ([]uint64, []uint32, error) {
	if geometryResult == nil || adjacency == nil || bodies == nil || forest == nil {
		return nil, nil, errors.New("program/flow/sourcecontrol: reachability owner is unavailable")
	}
	nodeCount := geometryResult.coordinates.nodeCount
	reachable := make([]uint64, (uint64(nodeCount)+63)/64)
	mark := func(node uint32) bool {
		if node >= nodeCount {
			return false
		}
		word, bit := node>>6, uint64(1)<<(node&63)
		if reachable[word]&bit != 0 {
			return false
		}
		reachable[word] |= bit
		return true
	}
	entryStart, ok := bodyStartForTerm(geometryResult, entry)
	if !ok {
		return nil, nil, errors.New("program/flow/sourcecontrol: Entry Body start is unavailable")
	}
	if !mark(entryStart) {
		return nil, nil, errors.New("program/flow/sourcecontrol: Entry Body start is invalid")
	}

	functions := flow.Functions()
	availability := make([]availabilityRow, 0, functions.Count())
	for index := 0; index < functions.Count(); index++ {
		function, ok := functions.At(index)
		if !ok {
			return nil, nil, errors.New("program/flow/sourcecontrol: Function view is unavailable")
		}
		owner, bodyTerm, _, rowOK := functions.Get(function)
		if !rowOK || keyspace.TermFamily(owner) != keyspace.FamilyBody || !validBody(bodyTerm, geometryResult.counts[keyspace.FamilyBody]) || owner == bodyTerm {
			return nil, nil, errors.New("program/flow/sourcecontrol: malformed Function Body authority")
		}
		activation, activationOK := bodies.Activation(bodyTerm)
		if !activationOK || activation != function {
			return nil, nil, errors.New("program/flow/sourcecontrol: Function Body activation disagrees with Body proof")
		}
		if forest.Static(function) {
			continue
		}
		from, fromOK := coordinateForTerm(sourceView, sourceView.Identity().ContentID(), &geometryResult.coordinates, function)
		to, toOK := bodyStartForTerm(geometryResult, bodyTerm)
		if !fromOK || !toOK {
			return nil, nil, errors.New("program/flow/sourcecontrol: Function lacks source-control availability coordinate")
		}
		availability = append(availability, availabilityRow{from: from, to: to})
	}
	availabilityOffsets := make([]uint32, int(nodeCount)+1)
	for _, row := range availability {
		if row.from >= nodeCount || row.to >= nodeCount {
			return nil, nil, errors.New("program/flow/sourcecontrol: Function availability escapes coordinates")
		}
		availabilityOffsets[row.from+1]++
	}
	for index := 1; index < len(availabilityOffsets); index++ {
		availabilityOffsets[index] += availabilityOffsets[index-1]
	}
	availabilityTargets := make([]uint32, len(availability))
	availabilityNext := append([]uint32(nil), availabilityOffsets[:len(availabilityOffsets)-1]...)
	for _, row := range availability {
		availabilityTargets[availabilityNext[row.from]] = row.to
		availabilityNext[row.from]++
	}

	activationMarks := make([]uint64, (uint64(nodeCount)+63)/64)
	markActivation := func(node uint32) {
		if node < nodeCount {
			activationMarks[node>>6] |= uint64(1) << (node & 63)
		}
	}
	work := make([]uint32, 0, int(nodeCount))
	work = append(work, entryStart)
	for len(work) != 0 {
		node := work[len(work)-1]
		work = work[:len(work)-1]
		for edge := adjacency.forwardOffsets[node]; edge < adjacency.forwardOffsets[node+1]; edge++ {
			if mark(adjacency.forwardTargets[edge]) {
				work = append(work, adjacency.forwardTargets[edge])
			}
		}
		for edge := availabilityOffsets[node]; edge < availabilityOffsets[node+1]; edge++ {
			to := availabilityTargets[edge]
			markActivation(to)
			if mark(to) {
				work = append(work, to)
			}
		}
	}
	activationRoots := make([]uint32, 0)
	for node := uint32(0); node < nodeCount; node++ {
		if bitSet(activationMarks, node) && bitSet(reachable, node) {
			activationRoots = append(activationRoots, node)
		}
	}
	return reachable, activationRoots, nil
}

func dominanceRoots(geometryResult *geometry, entry keyspace.Term, activationRoots []uint32) []uint32 {
	if geometryResult == nil {
		return nil
	}
	nodeCount := geometryResult.coordinates.nodeCount
	entryStart, ok := bodyStartForTerm(geometryResult, entry)
	if !ok || entryStart >= nodeCount {
		return nil
	}
	roots := make([]uint32, 0, len(activationRoots)+1)
	insertedEntry := false
	for _, root := range activationRoots {
		if root >= nodeCount || root == entryStart {
			continue
		}
		if !insertedEntry && entryStart < root {
			roots = append(roots, entryStart)
			insertedEntry = true
		}
		roots = append(roots, root)
	}
	if !insertedEntry {
		roots = append(roots, entryStart)
	}
	return roots
}

func bodyStartForTerm(geometryResult *geometry, body keyspace.Term) (uint32, bool) {
	if geometryResult == nil || keyspace.TermFamily(body) != keyspace.FamilyBody {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(body)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(geometryResult.coordinates.bodyOffsets)) {
		return 0, false
	}
	start := geometryResult.coordinates.bodyOffsets[ordinal-1]
	return start, start < geometryResult.coordinates.nodeCount
}

func bitSet(bits []uint64, node uint32) bool {
	word := node >> 6
	return uint64(word) < uint64(len(bits)) && bits[word]&(uint64(1)<<(node&63)) != 0
}
