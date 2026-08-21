package placement

import (
	"sort"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	internalhash "github.com/wippyai/go-lua/internal/hash"
)

// StaticContainmentCache is the one-entry, owner-lifetime memo for the
// authenticated Heap containment projection. It retains no OrderedCells or
// callback-generation capability: the entry owns only immutable Heap values
// copied from a complete observation and the resulting immutable projection.
//
// The cache is deliberately Placement-owned. Placement and Heap are one exact
// coordinate authority, while a runtime Heap vector is recurrent state and
// therefore belongs in the entry key rather than in Schema identity alone.
type StaticContainmentCache struct {
	schema Schema
	entry  atomic.Pointer[staticContainmentCacheEntry]
}

type staticContainmentCacheEntry struct {
	schema     Schema
	hash       uint64
	values     []heapdomain.Value
	present    []bool
	projection staticContainmentProjection
}

// staticContainmentProjection is the immutable result needed by the query
// fold after one authenticated graph build and the two SCC projections. The
// graph remains retained so future extensions can reuse canonical adjacency;
// all consumers read it, never mutate it.
type staticContainmentProjection struct {
	graph      staticHeapGraph
	depths     []uint32
	knownDepth []bool
	deepStates []EvidenceState
}

// NewStaticContainmentCache creates a cache fenced to one exact Placement
// owner. A cold/unavailable Schema cannot issue a cache capability.
func NewStaticContainmentCache(schema Schema) *StaticContainmentCache {
	if !schema.Valid() {
		return nil
	}
	return &StaticContainmentCache{schema: schema}
}

func (entry *staticContainmentCacheEntry) matches(schema Schema, hash uint64, values []heapdomain.Value, present []bool) bool {
	if entry == nil || entry.schema != schema || entry.hash != hash || len(entry.values) != len(values) || len(entry.present) != len(present) {
		return false
	}
	for index, value := range values {
		if entry.present[index] != present[index] || !heapdomain.Equal(entry.values[index], value) {
			return false
		}
	}
	return true
}

// staticHeapGraph is the private, immutable projection shared by Placement's
// containment-depth and DeepFrozen columns. The dense Heap coordinate is the
// node ID: Heap has already authenticated the complete root directory, so no
// second coordinate map or caller-provided relation is admitted here.
//
// The graph is built once per heterogeneous Heap fold. Its rows are
// canonicalized before publication to the two private solvers; projections use
// the solvers' trusted-canonical seams so this immutable graph is not copied or
// normalized a second time.
type staticHeapGraph struct {
	evidence            []AllocationEvidence
	allocationDense     []int
	allocationOrdinal   []int
	adjacency           [][]int
	allocationAdjacency [][]int
	deepLocal           []EvidenceState
	cellsComplete       bool
	depthComplete       bool
}

// buildStaticHeapGraph authenticates every dense Heap root and visits each
// complete value's containment stream exactly once. It intentionally keeps
// Boot roots in adjacency/local evidence so a mutable Boot descendant refutes
// an allocation's DeepFrozen proof, while the allocation-only depth graph is
// derived as a private projection afterward.
func buildStaticHeapGraph(schema Schema, cells engine.OrderedCells[heapdomain.Value]) (staticHeapGraph, bool) {
	return buildStaticHeapGraphRows(schema, cells.Count(), cells.At)
}

// buildStaticHeapGraphRows is the same authenticated builder over a detached
// row accessor. The cache snapshots one complete OrderedCells vector first,
// so graph construction cannot observe a different callback-generation row
// than the key it authenticated.
func buildStaticHeapGraphRows(schema Schema, cellCount int, at func(int) (heapdomain.Value, bool, bool)) (staticHeapGraph, bool) {
	if !schema.Valid() || at == nil {
		return staticHeapGraph{}, false
	}
	heapSchema := schema.Heap()
	denseCount := heapSchema.KeyCount()
	graph := staticHeapGraph{
		evidence:          make([]AllocationEvidence, denseCount),
		allocationOrdinal: make([]int, denseCount),
		adjacency:         make([][]int, denseCount),
		deepLocal:         make([]EvidenceState, denseCount),
		cellsComplete:     cellCount == denseCount,
		depthComplete:     cellCount == denseCount,
	}
	for index := range graph.allocationOrdinal {
		graph.allocationOrdinal[index] = -1
	}
	for dense := 0; dense < denseCount; dense++ {
		key, keyOK := heapSchema.KeyAt(dense)
		if !keyOK || (key.Kind() != heapdomain.RootAllocation && key.Kind() != heapdomain.RootBoot) {
			return staticHeapGraph{}, false
		}
		if key.Kind() == heapdomain.RootAllocation {
			graph.allocationOrdinal[dense] = len(graph.allocationDense)
			graph.allocationDense = append(graph.allocationDense, dense)
			canonical, canonicalOK := allocationEvidenceForKey(schema, key, Bottom, false)
			if !canonicalOK {
				return staticHeapGraph{}, false
			}
			graph.evidence[dense] = canonical
		}
	}
	if !graph.cellsComplete {
		// A partial/revoked vector cannot establish either a finite depth or a
		// negative DeepFrozen proof. Canonical allocation rows remain useful to
		// the caller, with both optional columns left absent/Unknown.
		return graph, true
	}

	for dense := 0; dense < denseCount; dense++ {
		key, keyOK := heapSchema.KeyAt(dense)
		if !keyOK {
			return staticHeapGraph{}, false
		}
		value, present, available := at(dense)
		if !available || !value.Valid() || value.IsTop() || !heapSchema.Admits(key, value) {
			graph.deepLocal[dense] = EvidenceUnknown
			if key.Kind() == heapdomain.RootAllocation {
				graph.depthComplete = false
			}
			continue
		}
		if !present {
			if key.Kind() == heapdomain.RootAllocation {
				// Heap's admitted sparse default is Bottom: no worlds and no
				// containment edges, so depth is an exact zero. A non-Bottom
				// absent token is an unavailable relation and cannot be used.
				if !value.IsBottom() {
					graph.depthComplete = false
					graph.deepLocal[dense] = EvidenceUnknown
				} else {
					graph.deepLocal[dense] = EvidenceProven
				}
			} else {
				// A sealed Boot root has an object policy even when its cell is
				// sparse; absence therefore cannot prove immutable publication.
				graph.deepLocal[dense] = EvidenceUnknown
			}
			continue
		}
		if value.IsBottom() {
			if key.Kind() == heapdomain.RootAllocation {
				graph.deepLocal[dense] = EvidenceProven
			} else {
				graph.deepLocal[dense] = EvidenceUnknown
			}
			continue
		}

		facts := deepFrozenLocalFacts{}
		if key.Kind() == heapdomain.RootBoot {
			bootFrozen, bootFrozenOK := heapSchema.BootFrozen(key)
			if !bootFrozenOK {
				facts.unknown = true
			} else {
				switch bootFrozen {
				case heapdomain.FrozenMutable:
					facts.mutable = true
				case heapdomain.FrozenFrozen:
					facts.frozen = true
				default:
					facts.unknown = true
				}
			}
		}
		if !deepFrozenValueHeaders(value, &facts) {
			facts.unknown = true
		}
		local := deepFrozenLocalState(facts)
		deepUnknown := local == EvidenceUnknown
		if local == EvidenceRefuted {
			// A mutable header is an exact refutation and must dominate any
			// opaque edge discovered below.
			deepUnknown = false
		}
		depthRowComplete := true
		visited := heapSchema.VisitContainments(value, func(observation heapdomain.ContainmentVisit) bool {
			if !observation.Valid() {
				depthRowComplete = false
				deepUnknown = true
				return false
			}
			switch observation.Kind() {
			case heapdomain.ContainmentNone:
				return true
			case heapdomain.ContainmentUnknown:
				depthRowComplete = false
				deepUnknown = true
				// Continue collecting exact edges for DeepFrozen: an exact
				// mutable descendant later in the stream is still a valid
				// refutation even when another edge is opaque.
				return true
			case heapdomain.ContainmentExact:
				reference, referenceOK := observation.Reference()
				childKey, _, childOK := reference.Key()
				if !referenceOK || !childOK || !heapSchema.OwnsKey(childKey) {
					depthRowComplete = false
					deepUnknown = true
					return true
				}
				childDense, childDenseOK := heapSchema.KeyIndex(childKey)
				if !childDenseOK || childDense < 0 || childDense >= denseCount {
					depthRowComplete = false
					deepUnknown = true
					return true
				}
				graph.adjacency[dense] = append(graph.adjacency[dense], childDense)
				return true
			default:
				depthRowComplete = false
				deepUnknown = true
				return true
			}
		})
		if !visited {
			depthRowComplete = false
			deepUnknown = true
		}
		if key.Kind() == heapdomain.RootAllocation && !depthRowComplete {
			graph.depthComplete = false
		}
		if deepUnknown && local != EvidenceRefuted {
			local = EvidenceUnknown
		}
		graph.deepLocal[dense] = local
	}

	graph.allocationAdjacency = make([][]int, len(graph.allocationDense))
	for dense := range graph.adjacency {
		graph.adjacency[dense] = sortUniqueInts(graph.adjacency[dense])
		ordinal := graph.allocationOrdinal[dense]
		if ordinal < 0 {
			continue
		}
		for _, childDense := range graph.adjacency[dense] {
			childOrdinal := graph.allocationOrdinal[childDense]
			if childOrdinal >= 0 {
				graph.allocationAdjacency[ordinal] = append(graph.allocationAdjacency[ordinal], childOrdinal)
			}
		}
	}
	for index := range graph.allocationAdjacency {
		graph.allocationAdjacency[index] = sortUniqueInts(graph.allocationAdjacency[index])
	}
	return graph, true
}

func containmentCacheSnapshot(schema Schema, cells engine.OrderedCells[heapdomain.Value]) ([]heapdomain.Value, []bool, uint64, bool) {
	if !schema.Valid() {
		return nil, nil, 0, false
	}
	denseCount := schema.Heap().KeyCount()
	if cells.Count() != denseCount {
		return nil, nil, 0, false
	}
	values := make([]heapdomain.Value, denseCount)
	present := make([]bool, denseCount)
	hash := uint64(0xcbf29ce484222325)
	for _, byteValue := range schema.ContentID() {
		hash = internalhash.MixHash(hash, uint64(byteValue))
	}
	hash = internalhash.MixHash(hash, uint64(denseCount))
	heapSchema := schema.Heap()
	for index := 0; index < denseCount; index++ {
		value, isPresent, available := cells.At(index)
		fingerprint, fingerprintOK := heapSchema.Fingerprint(value)
		if !available || !fingerprintOK {
			// The ordinary builder intentionally retains conservative unknown
			// evidence for revoked or malformed rows. Such a row has no stable
			// cache key, so let the uncached path preserve that behavior.
			return nil, nil, 0, false
		}
		values[index], present[index] = value, isPresent
		hash = internalhash.MixHash(hash, uint64(index))
		hash = internalhash.MixHash(hash, fingerprint)
		if isPresent {
			hash = internalhash.MixHash(hash, 1)
		} else {
			hash = internalhash.MixHash(hash, 0)
		}
	}
	return values, present, hash, true
}

func projectStaticContainmentGraph(graph staticHeapGraph) staticContainmentProjection {
	var scratch containmentSCCScratch
	depths, knownDepth := graph.depthStatesWithScratch(&scratch)
	deepStates := graph.deepStatesWithScratch(&scratch)
	return staticContainmentProjection{
		graph:      graph,
		depths:     depths,
		knownDepth: knownDepth,
		deepStates: deepStates,
	}
}

// projection resolves one complete, owner-authenticated Heap vector through
// the one-entry memo. A miss snapshots the callback rows before graph work;
// the cache never retains the borrowed OrderedCells capability itself.
func (cache *StaticContainmentCache) projection(schema Schema, cells engine.OrderedCells[heapdomain.Value]) (staticContainmentProjection, bool) {
	if cache == nil || cache.schema != schema {
		return staticContainmentProjection{}, false
	}
	values, present, hash, snapshotOK := containmentCacheSnapshot(schema, cells)
	if !snapshotOK {
		return staticContainmentProjection{}, false
	}
	if prior := cache.entry.Load(); prior != nil && prior.matches(schema, hash, values, present) {
		return prior.projection, true
	}
	graph, graphOK := buildStaticHeapGraphRows(schema, len(values), func(index int) (heapdomain.Value, bool, bool) {
		if index < 0 || index >= len(values) {
			return heapdomain.Value{}, false, false
		}
		return values[index], present[index], true
	})
	if !graphOK {
		return staticContainmentProjection{}, false
	}
	entry := &staticContainmentCacheEntry{
		schema:     schema,
		hash:       hash,
		values:     values,
		present:    present,
		projection: projectStaticContainmentGraph(graph),
	}
	// A racing miss may replace this entry before Store. Both entries are
	// independently authenticated and immutable; replacing one complete entry
	// cannot expose a partially built graph or solver slice.
	cache.entry.Store(entry)
	return entry.projection, true
}

func (graph staticHeapGraph) depthStates() ([]uint32, []bool) {
	return graph.depthStatesWithScratch(nil)
}

func (graph staticHeapGraph) depthStatesWithScratch(scratch *containmentSCCScratch) ([]uint32, []bool) {
	if !graph.cellsComplete || !graph.depthComplete {
		return nil, nil
	}
	// buildStaticHeapGraph authenticates and canonicalizes every allocation
	// row. finiteContainmentDepths is read-only, so the immutable projection can
	// be solved directly without a defensive adjacency clone.
	return finiteContainmentDepthsWithScratch(graph.allocationAdjacency, scratch)
}

func (graph staticHeapGraph) deepStates() []EvidenceState {
	return graph.deepStatesWithScratch(nil)
}

func (graph staticHeapGraph) deepStatesWithScratch(scratch *containmentSCCScratch) []EvidenceState {
	if !graph.cellsComplete {
		return nil
	}
	// buildStaticHeapGraph supplies valid, sorted, duplicate-free rows and valid
	// local states. The trusted seam skips the generic solver's in-place
	// normalization, preserving this graph as an immutable shared projection.
	return finiteDeepFrozenStatesTrustedCanonicalWithScratch(graph.deepLocal, graph.adjacency, scratch)
}

// AccumulatePlacementSummaryContainment is the domain fold bridge used by the
// heterogeneous Placement+Heap query. It consumes only the engine-issued
// Heap vector and joins finite authenticated depth evidence into the already
// accumulated Placement observation. The graph solver and evidence storage
// remain private to this package. The query invokes projections synchronously,
// so this bridge owns the current mutable fold plane and may refine it in
// place; callers that need a detached answer use the frozen result boundary.
func AccumulatePlacementSummaryContainment(schema Schema, observation PlacementSummaryObservation, cells engine.OrderedCells[heapdomain.Value]) (PlacementSummaryObservation, bool) {
	if !summaryObservationShape(schema, observation) {
		return PlacementSummaryObservation{}, false
	}
	graph, graphOK := buildStaticHeapGraph(schema, cells)
	if !graphOK {
		return PlacementSummaryObservation{}, false
	}
	return accumulateStaticContainmentProjection(schema, observation, projectStaticContainmentGraph(graph))
}

// AccumulatePlacementSummaryContainmentCached is the owner-level query seam.
// It preserves the uncached builder's fail-closed behavior when the incoming
// vector is incomplete or cannot receive a stable authenticated key.
func AccumulatePlacementSummaryContainmentCached(cache *StaticContainmentCache, schema Schema, observation PlacementSummaryObservation, cells engine.OrderedCells[heapdomain.Value]) (PlacementSummaryObservation, bool) {
	if !summaryObservationShape(schema, observation) {
		return PlacementSummaryObservation{}, false
	}
	if projection, cachedOK := cache.projection(schema, cells); cachedOK {
		return accumulateStaticContainmentProjection(schema, observation, projection)
	}
	return AccumulatePlacementSummaryContainment(schema, observation, cells)
}

func accumulateStaticContainmentProjection(schema Schema, observation PlacementSummaryObservation, projection staticContainmentProjection) (PlacementSummaryObservation, bool) {
	graph := projection.graph
	if len(graph.evidence) != len(observation.evidence) {
		return PlacementSummaryObservation{}, false
	}
	result := observation
	for ordinal, dense := range graph.allocationDense {
		evidence := graph.evidence[dense]
		if ordinal < len(projection.knownDepth) && projection.knownDepth[ordinal] {
			evidence.Depth = projection.depths[ordinal]
			evidence.HasDepth = true
		}
		if dense < len(projection.deepStates) {
			evidence.DeepFrozen = projection.deepStates[dense]
		}
		if !evidence.HasDepth && evidence.DeepFrozen == EvidenceUnknown {
			continue
		}
		merged := result.evidence[dense].Merge(evidence)
		if !merged.Valid() {
			return PlacementSummaryObservation{}, false
		}
		result.evidence[dense] = merged
	}
	return result, true
}

func sortUniqueInts(values []int) []int {
	if len(values) < 2 {
		return values
	}
	sort.Ints(values)
	write := 1
	for _, value := range values[1:] {
		if value == values[write-1] {
			continue
		}
		values[write] = value
		write++
	}
	return values[:write]
}

// finiteContainmentDepths solves the complete allocation graph.  The SCC
// walk is iterative so a long recursive Heap graph cannot consume the Go call
// stack.  A cyclic SCC and every allocation reachable from it are left
// unknown: treating a recursive path as one finite integer would be an
// unsound depth proof.  Acyclic components retain exact longest-path depths.
func finiteContainmentDepths(adjacency [][]int) ([]uint32, []bool) {
	return finiteContainmentDepthsWithScratch(adjacency, nil)
}

func finiteContainmentDepthsWithScratch(adjacency [][]int, scratch *containmentSCCScratch) ([]uint32, []bool) {
	count := len(adjacency)
	depths := make([]uint32, count)
	known := make([]bool, count)
	if count == 0 {
		return depths, known
	}
	for _, children := range adjacency {
		for _, child := range children {
			if child < 0 || child >= count {
				// A child outside the solved allocation denominator is the
				// graph equivalent of a missing/foreign root.  Never let an
				// unchecked relation turn into a proof or a panic.
				return depths, known
			}
		}
	}
	componentOf, componentSizes := containmentSCCsWithScratch(adjacency, scratch)
	cyclic := make([]bool, len(componentSizes))
	for node, component := range componentOf {
		if componentSizes[component] > 1 {
			cyclic[component] = true
			continue
		}
		for _, child := range adjacency[node] {
			if child == node {
				cyclic[component] = true
				break
			}
		}
	}

	// A depth path that enters a recursive component is not finite.  Marking
	// its forward closure keeps unrelated acyclic allocation trees precise.
	unknown := make([]bool, count)
	work := make([]int, 0)
	for node, component := range componentOf {
		if !cyclic[component] || unknown[node] {
			continue
		}
		unknown[node] = true
		work = append(work, node)
	}
	for len(work) != 0 {
		node := work[0]
		work = work[1:]
		for _, child := range adjacency[node] {
			if unknown[child] {
				continue
			}
			unknown[child] = true
			work = append(work, child)
		}
	}
	// A representational overflow is another source of non-finite evidence.
	// Reuse the closure worklist so every descendant of the overflowing edge
	// is downgraded, including descendants that also have a shorter alternate
	// path. The longest path is still not representable in that case.
	markUnknownReachable := func(start int) int {
		work = work[:0]
		work = append(work, start)
		added := 0
		for len(work) != 0 {
			node := work[0]
			work = work[1:]
			if unknown[node] {
				continue
			}
			unknown[node] = true
			added++
			for _, child := range adjacency[node] {
				if !unknown[child] {
					work = append(work, child)
				}
			}
		}
		return added
	}

	indegree := make([]int, count)
	knownCount := 0
	for node := 0; node < count; node++ {
		if unknown[node] {
			continue
		}
		knownCount++
		for _, child := range adjacency[node] {
			if !unknown[child] {
				indegree[child]++
			}
		}
	}
	// Any topological order produces the same longest-path join. A FIFO queue
	// keeps the solver linear in V+E; a min-heap would add an unnecessary
	// log(V) factor without changing the published depths.
	queue := make([]int, 0, knownCount)
	for node := 0; node < count; node++ {
		if !unknown[node] && indegree[node] == 0 {
			queue = append(queue, node)
		}
	}
	processed := 0
	for head := 0; head < len(queue); head++ {
		node := queue[head]
		known[node] = true
		processed++
		for _, child := range adjacency[node] {
			if unknown[child] {
				continue
			}
			candidate := depths[node]
			if candidate == ^uint32(0) {
				// The wire scalar cannot represent this path length.  Keep
				// the entire forward closure conservative instead of wrapping
				// to zero or proving a shorter alternate path.
				knownCount -= markUnknownReachable(child)
				continue
			}
			candidate++
			if candidate > depths[child] {
				depths[child] = candidate
			}
			indegree[child]--
			if indegree[child] == 0 {
				queue = append(queue, child)
			}
		}
	}
	if processed != knownCount {
		// This is defensive (Tarjan already removed every SCC), but retaining
		// Unknown is preferable to publishing a partial topological proof.
		for node := range known {
			known[node] = false
			depths[node] = 0
		}
	}
	return depths, known
}

type containmentDFSFrame struct {
	node int
	next int
}

// containmentSCCScratch owns only mutable Tarjan work storage. It is never
// part of a staticHeapGraph or a published result: one query creates one
// instance and consumes it synchronously for the allocation and full-Heap
// projections. The slices are deliberately retained between those two calls
// so a long graph does not pay for a second set of SCC growth allocations.
type containmentSCCScratch struct {
	visitIndex     []int
	low            []int
	onStack        []bool
	tarjanStack    []int
	componentOf    []int
	componentSizes []int
	frames         []containmentDFSFrame
}

func (scratch *containmentSCCScratch) prepare(count int) {
	if cap(scratch.visitIndex) < count {
		scratch.visitIndex = make([]int, count)
	} else {
		scratch.visitIndex = scratch.visitIndex[:count]
	}
	if cap(scratch.low) < count {
		scratch.low = make([]int, count)
	} else {
		scratch.low = scratch.low[:count]
	}
	if cap(scratch.onStack) < count {
		scratch.onStack = make([]bool, count)
	} else {
		scratch.onStack = scratch.onStack[:count]
		clear(scratch.onStack)
	}
	if cap(scratch.tarjanStack) < count {
		scratch.tarjanStack = make([]int, 0, count)
	} else {
		scratch.tarjanStack = scratch.tarjanStack[:0]
	}
	if cap(scratch.componentOf) < count {
		scratch.componentOf = make([]int, count)
	} else {
		scratch.componentOf = scratch.componentOf[:count]
	}
	componentSizeCapacity := count
	if componentSizeCapacity > 64 {
		componentSizeCapacity = 64
	}
	if cap(scratch.componentSizes) < componentSizeCapacity {
		scratch.componentSizes = make([]int, 0, componentSizeCapacity)
	} else {
		scratch.componentSizes = scratch.componentSizes[:0]
	}
	// A small starter capacity keeps shallow/disconnected graphs cheap; a
	// reused query-local scratch retains any growth needed by a deeper graph.
	frameCapacity := count
	if frameCapacity > 64 {
		frameCapacity = 64
	}
	if cap(scratch.frames) < frameCapacity {
		scratch.frames = make([]containmentDFSFrame, 0, frameCapacity)
	} else {
		scratch.frames = scratch.frames[:0]
	}
}

// containmentSCCs is an iterative Tarjan walk. Nodes and adjacency are
// already canonical Heap order, so component IDs are deterministic and do not
// depend on map iteration or callback ordering. Returning compact component
// labels/sizes avoids one heap allocation per singleton component in a large
// acyclic graph.
func containmentSCCs(adjacency [][]int) ([]int, []int) {
	return containmentSCCsWithScratch(adjacency, nil)
}

func containmentSCCsWithScratch(adjacency [][]int, scratch *containmentSCCScratch) ([]int, []int) {
	if scratch == nil {
		var local containmentSCCScratch
		scratch = &local
	}
	count := len(adjacency)
	scratch.prepare(count)
	visitIndex := scratch.visitIndex
	for index := range visitIndex {
		visitIndex[index] = -1
	}
	low := scratch.low
	onStack := scratch.onStack
	tarjanStack := scratch.tarjanStack
	componentOf := scratch.componentOf
	componentSizes := scratch.componentSizes
	frames := scratch.frames
	nextIndex := 0
	for root := 0; root < count; root++ {
		if visitIndex[root] >= 0 {
			continue
		}
		visitIndex[root] = nextIndex
		low[root] = nextIndex
		nextIndex++
		tarjanStack = append(tarjanStack, root)
		onStack[root] = true
		frames = append(frames[:0], containmentDFSFrame{node: root})
		for len(frames) != 0 {
			frameIndex := len(frames) - 1
			frame := &frames[frameIndex]
			if frame.next < len(adjacency[frame.node]) {
				child := adjacency[frame.node][frame.next]
				frame.next++
				if visitIndex[child] < 0 {
					visitIndex[child] = nextIndex
					low[child] = nextIndex
					nextIndex++
					tarjanStack = append(tarjanStack, child)
					onStack[child] = true
					frames = append(frames, containmentDFSFrame{node: child})
					continue
				}
				if onStack[child] && visitIndex[child] < low[frame.node] {
					low[frame.node] = visitIndex[child]
				}
				continue
			}
			finishedNode := frame.node
			frames = frames[:frameIndex]
			if len(frames) != 0 {
				parent := frames[len(frames)-1].node
				if low[finishedNode] < low[parent] {
					low[parent] = low[finishedNode]
				}
			}
			if low[finishedNode] != visitIndex[finishedNode] {
				continue
			}
			componentID := len(componentSizes)
			componentSize := 0
			for {
				last := len(tarjanStack) - 1
				node := tarjanStack[last]
				tarjanStack = tarjanStack[:last]
				onStack[node] = false
				componentOf[node] = componentID
				componentSize++
				if node == finishedNode {
					break
				}
			}
			componentSizes = append(componentSizes, componentSize)
		}
	}
	scratch.tarjanStack = tarjanStack
	scratch.componentOf = componentOf
	scratch.componentSizes = componentSizes
	scratch.frames = frames
	return componentOf, componentSizes
}
