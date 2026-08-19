// runtime_epoch_activation.go canonicalizes accepted activations and installs the selected factor overlay.

package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	demandpkg "github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
)

// observeActivations appends detached evidence to this immutable graph
// generation's frontier. Canonicalization, premise union, and subtraction of
// the committed relation happen once after the epoch reaches its fixed point;
// doing those operations after every Group repeatedly copied the entire
// growing frontier.
func (epoch *executorEpoch) observeActivations(selected []equation.AcceptedMember) bool {
	if epoch == nil || epoch.runtime == nil || epoch.canceled() {
		return false
	}
	for _, value := range selected {
		if !value.Available() {
			return false
		}
	}
	epoch.activations = append(epoch.activations, selected...)
	epoch.activationPending = len(epoch.activations) != 0
	if epoch.diagnostics != nil {
		epoch.diagnostics.recordActivation(len(selected))
	}
	return true
}

// canonicalizeAcceptedActivations sorts one epoch-owned frontier in place and
// unions duplicate Member premises exactly once. The returned slice aliases
// values; no second frontier or retained index survives the generation cut.
func canonicalizeAcceptedActivations(topology *equation.Topology, values []equation.AcceptedMember) ([]equation.AcceptedMember, bool) {
	if topology == nil {
		return nil, false
	}
	for _, value := range values {
		if !value.Available() {
			return nil, false
		}
	}
	comparable := true
	sort.Slice(values, func(left, right int) bool {
		comparison, ok := values[left].Member().Compare(values[right].Member())
		if !ok {
			comparable = false
			return false
		}
		return comparison < 0
	})
	if !comparable {
		return nil, false
	}
	canonical := values[:0]
	for _, value := range values {
		if len(canonical) == 0 {
			canonical = append(canonical, value)
			continue
		}
		comparison, ok := canonical[len(canonical)-1].Member().Compare(value.Member())
		if !ok || comparison > 0 {
			return nil, false
		}
		if comparison < 0 {
			canonical = append(canonical, value)
			continue
		}
		if !canonical[len(canonical)-1].Member().Same(value.Member()) {
			return nil, false
		}
		merged, ok := topology.MergeAccepted(canonical[len(canonical)-1], value)
		if !ok {
			return nil, false
		}
		canonical[len(canonical)-1] = merged
	}
	return canonical, true
}

func canonicalAcceptedActivations(values []equation.AcceptedMember) bool {
	for index, value := range values {
		if !value.Available() {
			return false
		}
		if index > 0 {
			comparison, comparable := values[index-1].Member().Compare(value.Member())
			if !comparable || comparison >= 0 {
				return false
			}
		}
	}
	return true
}

func mergeAcceptedActivations(topology *equation.Topology, left, right []equation.AcceptedMember) ([]equation.AcceptedMember, bool) {
	if topology == nil || !canonicalAcceptedActivations(left) || !canonicalAcceptedActivations(right) {
		return nil, false
	}
	merged := make([]equation.AcceptedMember, 0, len(left)+len(right))
	leftIndex, rightIndex := 0, 0
	for leftIndex < len(left) || rightIndex < len(right) {
		comparison, comparable := 0, false
		if leftIndex < len(left) && rightIndex < len(right) {
			comparison, comparable = left[leftIndex].Member().Compare(right[rightIndex].Member())
		}
		if rightIndex == len(right) || leftIndex < len(left) && comparable && comparison < 0 {
			merged = append(merged, left[leftIndex])
			leftIndex++
			continue
		}
		if leftIndex == len(left) || comparable && comparison > 0 {
			merged = append(merged, right[rightIndex])
			rightIndex++
			continue
		}
		value, ok := topology.MergeAccepted(left[leftIndex], right[rightIndex])
		if !ok {
			return nil, false
		}
		merged = append(merged, value)
		leftIndex, rightIndex = leftIndex+1, rightIndex+1
	}
	return merged, true
}

func subtractAcceptedActivations(topology *equation.Topology, values, known []equation.AcceptedMember) ([]equation.AcceptedMember, bool) {
	if topology == nil || !canonicalAcceptedActivations(values) || !canonicalAcceptedActivations(known) {
		return nil, false
	}
	result := make([]equation.AcceptedMember, 0, len(values))
	valueIndex, knownIndex := 0, 0
	for valueIndex < len(values) {
		for knownIndex < len(known) {
			comparison, comparable := known[knownIndex].Member().Compare(values[valueIndex].Member())
			if !comparable {
				return nil, false
			}
			if comparison >= 0 {
				break
			}
			knownIndex++
		}
		if knownIndex == len(known) {
			result = append(result, values[valueIndex])
			valueIndex++
			continue
		}
		comparison, comparable := known[knownIndex].Member().Compare(values[valueIndex].Member())
		if !comparable {
			return nil, false
		}
		if comparison > 0 {
			result = append(result, values[valueIndex])
			valueIndex++
			continue
		}
		if comparison != 0 || !known[knownIndex].Member().Same(values[valueIndex].Member()) {
			return nil, false
		}
		merged, ok := topology.MergeAccepted(known[knownIndex], values[valueIndex])
		if !ok {
			return nil, false
		}
		if merged.Evidence() != known[knownIndex].Evidence() {
			result = append(result, values[valueIndex])
		}
		valueIndex++
	}
	return result, true
}

// installSelectedFactorOverlay publishes one prepared selected-edge
// delta into a settled running epoch. Every recoverable allocation and bounds
// check occurs
// in prepareSelectedFactorEpoch; the commit below has no fallible semantic
// operation, only assignments, map publication, and touched-bit updates.
func (epoch *executorEpoch) installSelectedFactorOverlay(overlay *preparedSelectedFactorOverlay) bool {
	if !epochSelectedOverlayInstallEligible(epoch, overlay) {
		return false
	}
	prepared, ok := epoch.prepareSelectedFactorEpoch(overlay)
	if !ok {
		return false
	}
	runtime := epoch.runtime
	if overlay.execution == nil || overlay.executionDemand == nil || overlay.demandEpoch == nil || len(overlay.activePoints) != len(runtime.activePoints) {
		return false
	}
	if overlay.grownFactorEdges != nil {
		runtime.factorEdges = overlay.grownFactorEdges
	} else if len(overlay.additions) != 0 {
		previous := len(runtime.factorEdges)
		runtime.factorEdges = runtime.factorEdges[:previous+len(overlay.additions)]
		for additionIndex, addition := range overlay.additions {
			runtime.factorEdges[previous+additionIndex] = addition.edge
		}
	}
	for _, replacement := range overlay.replacements {
		runtime.factorEdges[replacement.index] = replacement.edge
	}
	for _, row := range overlay.incomingRows {
		runtime.factorIncoming[row.point] = row.edges
	}
	for _, row := range overlay.outgoingRows {
		runtime.overlay.factorOutgoing[row.point] = row.edges
	}
	// Demand, schedule, recurrence rows, and newly selected dense state are
	// one prepared semantic view. Existing PointState/producer rows are left
	// untouched; only the activation records below fill previously inactive
	// indexes.
	if epoch.demand != nil {
		epoch.demand.Discard()
	}
	epoch.demand = overlay.demandEpoch
	runtime.points = overlay.executionDemand
	runtime.execution = overlay.execution
	runtime.executionDemand = overlay.executionDemand
	runtime.activePoints = overlay.activePoints
	runtime.regions = overlay.regions
	runtime.regionChildren = overlay.regionChildren
	runtime.pointRegion = overlay.pointRegion
	runtime.activeRegions = overlay.activeRegions
	for _, activation := range prepared.pointActivations {
		epoch.points[activation.index] = activation.state
		epoch.structuralDirty[activation.index] = true
		epoch.structural.inputs[activation.index] = structuralInputEpoch{}
	}
	for _, activation := range prepared.producerActivations {
		epoch.producers[activation.index] = activation.state
	}
	epoch.regions = prepared.regions
	epoch.candidatesPending = prepared.candidatesPending
	epoch.nested = prepared.nested
	epoch.frames = prepared.frames
	epoch.regionScratch = prepared.regionScratch
	if overlay.dependencyChanged {
		runtime.overlay.dependencyEdges = overlay.dependencyEdges
		runtime.overlay.dependencyAt = overlay.dependencyAt
	}
	for key, plan := range overlay.latePlans {
		runtime.overlay.latePlans[key] = plan
	}
	for origin, index := range overlay.newOrigins {
		runtime.overlay.originAt[origin] = index
	}
	runtime.overlay.directAt = cloneDirectCatalog(overlay.directCatalog)
	for _, target := range overlay.targets {
		epoch.structuralDirty[target] = true
		epoch.structural.inputs[target] = structuralInputEpoch{}
	}
	for _, point := range prepared.wakePoints {
		epoch.postfixDirty[point] = true
		epoch.queue.ready[point] = true
	}
	epoch.postfixPending = prepared.postfixPending
	epoch.postfixHead = 0
	epoch.queue.count = len(prepared.wakePoints)
	// matches proved the next stamp is representable before installation began.
	runtime.overlay.generation = runtime.overlay.generation.Next()
	return true
}

func epochSelectedOverlayInstallEligible(epoch *executorEpoch, overlay *preparedSelectedFactorOverlay) bool {
	if epoch == nil || overlay == nil || epoch.runtime == nil || epoch.work == nil || epoch.demand == nil || epoch.terminal.Load() != epochRunning || epoch.canceled() || epoch.queue.pending() || !epoch.demand.Live() || !runtimeSelectedOverlayEligible(epoch.runtime) {
		return false
	}
	return overlay.matches(epoch.runtime)
}

// matches is a constant-time stale-builder fence. Generation changes after
// every successful installation, so an old prepared delta cannot overwrite a
// newer factor/CSR view without rescanning all prior edges.
func (overlay *preparedSelectedFactorOverlay) matches(runtime *solverRuntime) bool {
	if overlay == nil || runtime == nil || runtime.graph == nil || overlay.runtime != runtime || !overlay.generation.Available() || overlay.generation != runtime.overlay.generation || !runtime.overlay.generation.Next().Available() {
		return false
	}
	nextEdgeCount := overlay.previousEdgeCount + len(overlay.additions)
	return overlay.previousEdgeCount == len(runtime.factorEdges) && nextEdgeCount >= overlay.previousEdgeCount && (len(overlay.additions) != 0 || len(overlay.replacements) != 0) &&
		validPreparedFactorCSRRows(overlay.incomingRows, runtime.graph.PointCount(), nextEdgeCount) &&
		validPreparedFactorCSRRows(overlay.outgoingRows, runtime.graph.PointCount(), nextEdgeCount)
}

type preparedSelectedFactorEpoch struct {
	postfixPending      []int
	wakePoints          []int
	pointActivations    []preparedPointActivation
	producerActivations []preparedProducerActivation
	regions             []regionEpoch
	candidatesPending   []uint64
	nested              []int
	frames              []pointWTOFrame
	regionScratch       []int
}

type preparedPointActivation struct {
	index int
	state carrier.PointState
}

type preparedProducerActivation struct {
	index int
	state producerEpoch
}

func (epoch *executorEpoch) prepareSelectedFactorEpoch(overlay *preparedSelectedFactorOverlay) (preparedSelectedFactorEpoch, bool) {
	if epoch == nil || overlay == nil || epoch.runtime == nil || epoch.work == nil || epoch.demand == nil || !epoch.demand.Live() || len(epoch.structural.inputs) != len(epoch.points) || len(epoch.structuralDirty) != len(epoch.points) || len(epoch.postfixDirty) != len(epoch.points) || len(epoch.queue.ready) != len(epoch.points) || epoch.postfixHead != len(epoch.postfixPending) || epoch.queue.count != 0 || len(overlay.activePoints) != len(epoch.points) || len(epoch.runtime.activePoints) != len(epoch.points) || len(overlay.selectedPoints) == 0 {
		return preparedSelectedFactorEpoch{}, false
	}
	pointActivations := make([]preparedPointActivation, 0)
	producerActivations := make([]preparedProducerActivation, 0)
	newPointSet := make(map[int]struct{})
	newProducers := make(map[int]struct{})
	producerActivationAt := make(map[int]producerEpoch)
	for pointIndex, active := range overlay.activePoints {
		if !active {
			if epoch.runtime.activePoints[pointIndex] {
				return preparedSelectedFactorEpoch{}, false
			}
			continue
		}
		if epoch.runtime.activePoints[pointIndex] {
			if !epoch.work.OwnsPointState(epoch.points[pointIndex]) {
				return preparedSelectedFactorEpoch{}, false
			}
			continue
		}
		point, pointOK := epoch.runtime.graph.PointAt(schedule.Node(pointIndex))
		state, stateOK := preparedRuntimePointState(epoch, pointIndex, point)
		if !pointOK || !stateOK {
			return preparedSelectedFactorEpoch{}, false
		}
		pointActivations = append(pointActivations, preparedPointActivation{index: pointIndex, state: state})
		newPointSet[pointIndex] = struct{}{}
		for producerIndex := 0; producerIndex < epoch.runtime.graph.ProducerCount(point); producerIndex++ {
			group, groupOK := epoch.runtime.graph.ProducerAt(point, producerIndex)
			groupIndex, indexed := epoch.runtime.graph.GroupIndex(group)
			if !groupOK || !indexed || groupIndex < 0 || groupIndex >= len(epoch.producers) || group.Output() != point {
				return preparedSelectedFactorEpoch{}, false
			}
			if epoch.producers[groupIndex].generation != 0 {
				return preparedSelectedFactorEpoch{}, false
			}
			if _, duplicate := newProducers[groupIndex]; duplicate {
				return preparedSelectedFactorEpoch{}, false
			}
			metadata := epoch.runtime.producers[groupIndex]
			inputCount := metadata.group.InputCount()
			cache := producerEpoch{generation: 1, candidateTokens: make([]uint64, inputCount), scratchTokens: make([]uint64, inputCount), inputs: make([]carrier.PointState, inputCount), inputStates: make([]carrier.State, inputCount), patches: make([]carrier.Patch, 0, metadata.span.count()), patchRows: make([]contributionPatch, 0, metadata.span.count()), reads: make([]demandpkg.Observation, 0, len(metadata.reads))}
			producerActivations = append(producerActivations, preparedProducerActivation{index: groupIndex, state: cache})
			producerActivationAt[groupIndex] = cache
			newProducers[groupIndex] = struct{}{}
		}
	}
	for _, target := range overlay.targets {
		if target < 0 || target >= len(epoch.points) || !overlay.activePoints[target] {
			return preparedSelectedFactorEpoch{}, false
		}
		if epoch.runtime.activePoints[target] && (epoch.structuralDirty[target] || epoch.postfixDirty[target] || epoch.queue.ready[target]) {
			return preparedSelectedFactorEpoch{}, false
		}
	}
	for _, addition := range overlay.additions {
		edge := addition.edge
		_, sourcePrepared := newPointSet[edge.source]
		if edge.index < overlay.previousEdgeCount || edge.index >= overlay.previousEdgeCount+len(overlay.additions) || edge.source < 0 || edge.source >= len(epoch.points) || edge.target < 0 || edge.target >= len(epoch.points) || !overlay.activePoints[edge.target] || !epoch.runtime.activePoints[edge.source] && !sourcePrepared || epoch.runtime.activePoints[edge.source] && !epoch.work.OwnsPointState(epoch.points[edge.source]) {
			return preparedSelectedFactorEpoch{}, false
		}
	}
	for _, replacement := range overlay.replacements {
		_, sourcePrepared := newPointSet[replacement.edge.source]
		if replacement.index < 0 || replacement.index >= overlay.previousEdgeCount || replacement.edge.index != replacement.index || replacement.edge.source < 0 || replacement.edge.source >= len(epoch.points) || replacement.edge.target < 0 || replacement.edge.target >= len(epoch.points) || !overlay.activePoints[replacement.edge.target] || !epoch.runtime.activePoints[replacement.edge.source] && !sourcePrepared || epoch.runtime.activePoints[replacement.edge.source] && !epoch.work.OwnsPointState(epoch.points[replacement.edge.source]) {
			return preparedSelectedFactorEpoch{}, false
		}
	}
	wakePoints := append([]int(nil), overlay.targets...)
	for pointIndex := range pointActivations {
		wakePoints = append(wakePoints, pointActivations[pointIndex].index)
	}
	if overlay.execution == nil || overlay.executionDemand == nil || len(overlay.regions) != overlay.execution.RegionCount() || len(overlay.activeRegions) != len(overlay.regions) || len(overlay.pointRegion) != len(epoch.points) {
		return preparedSelectedFactorEpoch{}, false
	}
	for index, region := range overlay.regions {
		if !overlay.activeRegions[index] {
			if region.active {
				return preparedSelectedFactorEpoch{}, false
			}
			continue
		}
		if !region.active || region.head < 0 || region.head >= len(epoch.points) || !overlay.activePoints[region.head] {
			return preparedSelectedFactorEpoch{}, false
		}
		wakePoints = append(wakePoints, region.head)
	}
	sort.Ints(wakePoints)
	unique := wakePoints[:0]
	for _, point := range wakePoints {
		if len(unique) == 0 || unique[len(unique)-1] != point {
			unique = append(unique, point)
		}
	}
	wakePoints = unique
	for _, point := range wakePoints {
		if point < 0 || point >= len(epoch.points) || point >= len(overlay.activePoints) || !overlay.activePoints[point] {
			return preparedSelectedFactorEpoch{}, false
		}
		if epoch.runtime.activePoints[point] && (epoch.structuralDirty[point] || epoch.postfixDirty[point] || epoch.queue.ready[point]) {
			return preparedSelectedFactorEpoch{}, false
		}
	}
	result := preparedSelectedFactorEpoch{postfixPending: append([]int(nil), wakePoints...), wakePoints: wakePoints, pointActivations: pointActivations, producerActivations: producerActivations, regions: make([]regionEpoch, len(overlay.regions)), candidatesPending: make([]uint64, len(overlay.regions)), frames: make([]pointWTOFrame, 0, len(overlay.regions)), regionScratch: make([]int, 0, len(overlay.regions))}
	var nestedOK bool
	result.nested, nestedOK = preparedSelectedOverlayNested(wakePoints, overlay.pointRegion, overlay.regions, overlay.activeRegions)
	if !nestedOK {
		return preparedSelectedFactorEpoch{}, false
	}
	for index, region := range overlay.regions {
		episode := &result.regions[index]
		if !overlay.activeRegions[index] {
			continue
		}
		episode.phase = phaseAscent
		episode.episode = 1
		episode.invalid = true
		episode.ingress = make([]uint64, len(region.external))
		episode.backIngress = make([]uint64, len(region.back))
		episode.environmentIngress = make([]uint64, len(region.environmentExternal))
		episode.environmentBackIngress = make([]uint64, len(region.environmentBack))
		episode.factorIngress = make([]uint64, len(region.factorExternal))
		episode.factorBackIngress = make([]uint64, len(region.factorBack))
		episode.snapshot = make([]uint64, len(region.points))
		var pending uint64
		for _, pointIndex := range region.points {
			if pointIndex < 0 || pointIndex >= len(epoch.points) || !overlay.activePoints[pointIndex] {
				return preparedSelectedFactorEpoch{}, false
			}
			point, pointOK := epoch.runtime.graph.PointAt(schedule.Node(pointIndex))
			if !pointOK {
				return preparedSelectedFactorEpoch{}, false
			}
			for producerIndex := 0; producerIndex < epoch.runtime.graph.ProducerCount(point); producerIndex++ {
				group, groupOK := epoch.runtime.graph.ProducerAt(point, producerIndex)
				groupIndex, indexed := epoch.runtime.graph.GroupIndex(group)
				if !groupOK || !indexed || groupIndex < 0 || groupIndex >= len(epoch.producers) {
					return preparedSelectedFactorEpoch{}, false
				}
				cache := epoch.producers[groupIndex]
				if activation, activated := producerActivationAt[groupIndex]; activated {
					cache = activation
				}
				if cache.generation != cache.applied {
					if pending == ^uint64(0) {
						return preparedSelectedFactorEpoch{}, false
					}
					pending++
				}
			}
		}
		result.candidatesPending[index] = pending
	}
	nextCount := overlay.previousEdgeCount + len(overlay.additions)
	if nextCount < overlay.previousEdgeCount {
		return preparedSelectedFactorEpoch{}, false
	}
	if !validPreparedFactorCSRRows(overlay.incomingRows, len(epoch.points), nextCount) || !validPreparedFactorCSRRows(overlay.outgoingRows, len(epoch.points), nextCount) {
		return preparedSelectedFactorEpoch{}, false
	}
	demandEpoch, demandOK := epoch.demand.Widen(overlay.selectedPoints)
	if !demandOK {
		return preparedSelectedFactorEpoch{}, false
	}
	overlay.demandEpoch = demandEpoch
	return result, true
}

func preparedRuntimePointState(epoch *executorEpoch, pointIndex int, point equation.Point) (carrier.PointState, bool) {
	if epoch == nil || epoch.runtime == nil || epoch.work == nil || !point.Available() || pointIndex < 0 || pointIndex >= len(epoch.runtime.pointScopes) || pointIndex >= len(epoch.runtime.pointInitials) || !epoch.runtime.pointScopes[pointIndex].Valid() {
		return carrier.PointState{}, false
	}
	feasible, ok := support.FromGuard(epoch.runtime.carrier.Guards(), epoch.runtime.carrier.Guards().False())
	if !ok {
		return carrier.PointState{}, false
	}
	if point.HasInit() {
		feasible = epoch.runtime.pointInitials[pointIndex]
		if !feasible.Valid() {
			return carrier.PointState{}, false
		}
	}
	state, initialized := carrier.NewState(epoch.runtime.carrier, epoch.runtime.pointScopes[pointIndex], feasible)
	if !initialized {
		return carrier.PointState{}, false
	}
	empty, paired := epoch.work.EmptyContribution(state)
	if !paired {
		return carrier.PointState{}, false
	}
	rule, paired := epoch.work.AsRuleContribution(empty)
	if !paired {
		return carrier.PointState{}, false
	}
	pointState, paired := epoch.work.PointStateFromRuleContribution(rule)
	return pointState, paired
}

// preparedSelectedOverlayNested derives the exact parent readiness counters
// that updateNested(point, +1) would establish for a freshly installed
// candidate WTO. It is deliberately preparation-only: installation publishes
// the already-derived counters together with the already-ready queue bits.
func preparedSelectedOverlayNested(wakePoints, pointRegion []int, regions []runtimeRegion, activeRegions []bool) ([]int, bool) {
	if len(activeRegions) != len(regions) {
		return nil, false
	}
	nested := make([]int, len(regions))
	maxInt := int(^uint(0) >> 1)
	for _, point := range wakePoints {
		if point < 0 || point >= len(pointRegion) {
			return nil, false
		}
		region := pointRegion[point]
		for depth := 0; region != schedule.NoRegion; depth++ {
			if depth >= len(regions) || region < 0 || region >= len(regions) || !activeRegions[region] || !regions[region].active {
				return nil, false
			}
			parent := regions[region].parent
			if parent == schedule.NoRegion {
				break
			}
			if parent < 0 || parent >= len(regions) || !activeRegions[parent] || !regions[parent].active || nested[parent] == maxInt {
				return nil, false
			}
			nested[parent]++
			region = parent
		}
	}
	return nested, true
}

func sameAcceptedActivations(left, right []equation.AcceptedMember) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !left[index].Member().Same(right[index].Member()) || left[index].Evidence() != right[index].Evidence() {
			return false
		}
	}
	return true
}
