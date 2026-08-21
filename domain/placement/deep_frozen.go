package placement

import (
	"sort"

	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

// deepFrozenLocalFacts is the header/edge evidence collected for one exact
// dense Heap root before graph propagation. Keeping the two negative cases
// separate matters: an exact FrozenMutable object proves the negation, while
// a mixed header or opaque edge only prevents a proof.
type deepFrozenLocalFacts struct {
	mutable bool
	frozen  bool
	unknown bool
	// invalid is distinct from unknown. Unknown is an authenticated opaque or
	// mixed graph fact; invalid means a Value/Heap read could not be
	// authenticated and must refuse the projection.
	invalid bool
}

// deepFrozenValueHeaders reduces all complete object headers in one Heap
// Value. WorldMany intentionally contributes both Recent and Summary; they
// are one complete world, not sibling coordinates that may be ignored.
func deepFrozenValueHeaders(value heapdomain.Value, facts *deepFrozenLocalFacts) bool {
	if facts == nil {
		return false
	}
	if !value.Valid() {
		facts.invalid = true
		return false
	}
	if value.IsTop() {
		// Top is a valid, authenticated graph fact. It is deliberately
		// preserved as Unknown rather than confused with a malformed read.
		facts.unknown = true
		return true
	}
	if value.WorldCount() < 0 {
		facts.invalid = true
		return false
	}
	for index := 0; index < value.WorldCount(); index++ {
		world, worldOK := value.WorldAt(index)
		if !worldOK || !world.Valid() {
			facts.invalid = true
			return false
		}
		switch world.Kind() {
		case heapdomain.WorldZero:
			// A zero allocation world has no object header or edge.
		case heapdomain.WorldExact:
			object, objectOK := world.Exact()
			if !objectOK || !deepFrozenObjectHeader(object, facts) {
				facts.invalid = true
				return false
			}
		case heapdomain.WorldOne:
			object, objectOK := world.Recent()
			if !objectOK || !deepFrozenObjectHeader(object, facts) {
				facts.invalid = true
				return false
			}
		case heapdomain.WorldMany:
			recent, recentOK := world.Recent()
			summary, summaryOK := world.Summary()
			if !recentOK || !summaryOK || !deepFrozenObjectHeader(recent, facts) || !deepFrozenObjectHeader(summary, facts) {
				facts.invalid = true
				return false
			}
		default:
			facts.invalid = true
			return false
		}
	}
	return true
}

func deepFrozenObjectHeader(object heapdomain.Object, facts *deepFrozenLocalFacts) bool {
	if facts == nil || !object.Valid() {
		if facts != nil {
			facts.invalid = true
		}
		return false
	}
	_, frozen, headerOK := object.Header()
	if !headerOK {
		facts.invalid = true
		return false
	}
	switch frozen {
	case heapdomain.FrozenMutable:
		facts.mutable = true
	case heapdomain.FrozenFrozen:
		facts.frozen = true
	case heapdomain.FrozenMutable | heapdomain.FrozenFrozen:
		facts.unknown = true
	default:
		facts.invalid = true
		return false
	}
	return true
}

func deepFrozenLocalState(facts deepFrozenLocalFacts) EvidenceState {
	if facts.invalid {
		return invalidEvidenceState
	}
	if facts.mutable {
		return EvidenceRefuted
	}
	if facts.unknown {
		return EvidenceUnknown
	}
	return EvidenceProven
}

// finiteDeepFrozenStates propagates local deep-frozen facts over an
// owner-authenticated root graph. SCC condensation is essential: an SCC whose
// members are all exactly FrozenFrozen is Proven when its outgoing closure is
// proven, rather than being downgraded merely because it is cyclic. An exact
// mutable member refutes the component and every predecessor; authenticated
// opaque/Top facts propagate Unknown. Malformed graph input refuses with nil;
// it is never widened into an Unknown result.
func finiteDeepFrozenStates(local []EvidenceState, adjacency [][]int) []EvidenceState {
	if !validDeepFrozenGraph(local, adjacency) {
		return nil
	}
	return finiteDeepFrozenStatesTrustedCanonical(local, adjacency)
}

// validDeepFrozenGraph authenticates the finite graph shape before the solver
// allocates any projection state. Rows are required to be sorted and unique;
// silently normalizing caller input would be a compensating graph rewrite.
//
// The solver's input alphabet is the decided triad: every local state is a
// header fact its producer derived from an owner-authenticated Heap relation.
// Absence is not in that alphabet, so an unwritten local row refuses the graph
// instead of being propagated as an opaque Unknown component.
func validDeepFrozenGraph(local []EvidenceState, adjacency [][]int) bool {
	if len(adjacency) != len(local) {
		return false
	}
	for _, state := range local {
		if !state.Valid() || state == EvidenceAbsent {
			return false
		}
	}
	for _, children := range adjacency {
		previous := -1
		for _, child := range children {
			if child < 0 || child >= len(local) || child <= previous {
				return false
			}
			previous = child
		}
	}
	return true
}

// finiteDeepFrozenStatesTrustedCanonical is the read-only solver seam for
// graph projections whose producer has already established all of the generic
// solver's preconditions: one adjacency row per local state, valid child
// coordinates, valid local states, and sorted duplicate-free rows. The seam
// rechecks those preconditions before touching the graph so even a direct
// malformed call refuses without a compensating Unknown vector.
func finiteDeepFrozenStatesTrustedCanonical(local []EvidenceState, adjacency [][]int) []EvidenceState {
	return finiteDeepFrozenStatesTrustedCanonicalWithScratch(local, adjacency, nil)
}

func finiteDeepFrozenStatesTrustedCanonicalWithScratch(local []EvidenceState, adjacency [][]int, scratch *containmentSCCScratch) []EvidenceState {
	if !validDeepFrozenGraph(local, adjacency) {
		return nil
	}
	count := len(local)
	states := make([]EvidenceState, count)
	if count == 0 {
		return states
	}
	componentOf, componentSizes := containmentSCCsWithScratch(adjacency, scratch)
	componentCount := len(componentSizes)
	componentRefuted := make([]bool, componentCount)
	componentUnknown := make([]bool, componentCount)
	// Keep the condensation DAG in flat CSR form. The former [][]int
	// representation allocated a backing slice for every non-empty component
	// (and another one for every component's reverse edge list), which made a
	// long acyclic Heap graph allocate once per root. Counts are reused as write
	// cursors after the CSR offsets have been sealed.
	componentChildCounts := make([]int, componentCount)
	componentParentCounts := make([]int, componentCount)
	componentEdgeMarks := make([]int, componentCount)
	componentNodeOffsets := make([]int, componentCount+1)
	for node, state := range local {
		component := componentOf[node]
		componentNodeOffsets[component+1]++
		switch state {
		case EvidenceRefuted:
			componentRefuted[component] = true
		case EvidenceUnknown:
			componentUnknown[component] = true
		case EvidenceProven:
		default:
			componentUnknown[component] = true
		}
	}
	for component := 0; component < componentCount; component++ {
		componentNodeOffsets[component+1] += componentNodeOffsets[component]
	}
	componentNodes := make([]int, count)
	componentNodeCursors := append([]int(nil), componentNodeOffsets[:componentCount]...)
	for node, component := range componentOf {
		componentNodes[componentNodeCursors[component]] = node
		componentNodeCursors[component]++
	}
	for component := 0; component < componentCount; component++ {
		mark := component + 1
		for _, node := range componentNodes[componentNodeOffsets[component]:componentNodeOffsets[component+1]] {
			for _, child := range adjacency[node] {
				if child < 0 || child >= count {
					// validDeepFrozenGraph already rejects this. Keep the
					// trusted seam fail-closed if a future caller mutates the
					// private graph between validation and projection.
					return nil
				}
				childComponent := componentOf[child]
				if childComponent == component {
					continue
				}
				if componentEdgeMarks[childComponent] == mark {
					continue
				}
				componentEdgeMarks[childComponent] = mark
				componentChildCounts[component]++
				componentParentCounts[childComponent]++
			}
		}
	}

	componentChildOffsets := make([]int, componentCount+1)
	componentParentOffsets := make([]int, componentCount+1)
	for component := 0; component < componentCount; component++ {
		componentChildOffsets[component+1] = componentChildOffsets[component] + componentChildCounts[component]
		componentParentOffsets[component+1] = componentParentOffsets[component] + componentParentCounts[component]
	}
	componentChildren := make([]int, componentChildOffsets[componentCount])
	componentParents := make([]int, componentParentOffsets[componentCount])

	// Reuse the count arrays as CSR cursors while materializing edges. The
	// source-node/component order and sorted input adjacency preserve the old
	// canonical traversal order; sorting each CSR row restores the exact
	// ascending component order previously produced by sortUniqueInts.
	for component := 0; component < componentCount; component++ {
		componentChildCounts[component] = componentChildOffsets[component]
		componentParentCounts[component] = componentParentOffsets[component]
	}
	for component := range componentEdgeMarks {
		componentEdgeMarks[component] = 0
	}
	for component := 0; component < componentCount; component++ {
		mark := component + 1
		for _, node := range componentNodes[componentNodeOffsets[component]:componentNodeOffsets[component+1]] {
			for _, child := range adjacency[node] {
				if child < 0 || child >= count {
					return nil
				}
				childComponent := componentOf[child]
				if childComponent == component || componentEdgeMarks[childComponent] == mark {
					continue
				}
				componentEdgeMarks[childComponent] = mark
				position := componentChildCounts[component]
				componentChildren[position] = childComponent
				componentChildCounts[component]++
			}
		}
	}
	for component := 0; component < componentCount; component++ {
		sort.Ints(componentChildren[componentChildOffsets[component]:componentChildOffsets[component+1]])
	}
	for parent := 0; parent < componentCount; parent++ {
		for _, child := range componentChildren[componentChildOffsets[parent]:componentChildOffsets[parent+1]] {
			if child < 0 || child >= componentCount {
				return nil
			}
			position := componentParentCounts[child]
			componentParents[position] = parent
			componentParentCounts[child]++
		}
	}

	componentState := make([]EvidenceState, componentCount)
	outstanding := make([]int, componentCount)
	queue := make([]int, 0, componentCount)
	for component := 0; component < componentCount; component++ {
		outstanding[component] = componentChildOffsets[component+1] - componentChildOffsets[component]
		if outstanding[component] == 0 {
			queue = append(queue, component)
		}
	}
	processed := 0
	for head := 0; head < len(queue); head++ {
		component := queue[head]
		processed++
		if componentRefuted[component] {
			componentState[component] = EvidenceRefuted
		} else if componentUnknown[component] {
			componentState[component] = EvidenceUnknown
		} else {
			componentState[component] = EvidenceProven
		}
		for index := componentParentOffsets[component]; index < componentParentOffsets[component+1]; index++ {
			parent := componentParents[index]
			if componentState[component] == EvidenceRefuted {
				componentRefuted[parent] = true
			} else if componentState[component] == EvidenceUnknown {
				componentUnknown[parent] = true
			}
			outstanding[parent]--
			if outstanding[parent] == 0 {
				queue = append(queue, parent)
			}
		}
	}
	if processed != componentCount {
		// containmentSCCs should make this impossible. Refuse if a future
		// graph representation violates the condensation DAG invariant.
		return nil
	}
	for node, component := range componentOf {
		states[node] = componentState[component]
	}
	return states
}
