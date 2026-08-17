// runtime_epoch_activation.go canonicalizes accepted activations and installs the selected factor overlay.

package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
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

// installSelectedFactorOverlay publishes one prepared acyclic selected-edge
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
	if overlay.execution != nil && overlay.execution.RegionCount() != 0 {
		runtime.execution = overlay.execution
		runtime.executionDemand = overlay.executionDemand
		runtime.regions = overlay.regions
		runtime.regionChildren = overlay.regionChildren
		runtime.pointRegion = overlay.pointRegion
		runtime.activeRegions = overlay.activeRegions
		epoch.regions = prepared.regions
		epoch.candidatesPending = prepared.candidatesPending
		epoch.nested = prepared.nested
		epoch.frames = prepared.frames
		epoch.regionScratch = prepared.regionScratch
	}
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
	if len(epoch.regions) != 0 || len(epoch.runtime.regions) != 0 || len(epoch.runtime.activeRegions) != 0 || len(epoch.nested) != 0 {
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
	postfixPending    []int
	wakePoints        []int
	regions           []regionEpoch
	candidatesPending []uint64
	nested            []int
	frames            []pointWTOFrame
	regionScratch     []int
}

func (epoch *executorEpoch) prepareSelectedFactorEpoch(overlay *preparedSelectedFactorOverlay) (preparedSelectedFactorEpoch, bool) {
	if epoch == nil || overlay == nil || epoch.runtime == nil || len(epoch.structural.inputs) != len(epoch.points) || len(epoch.structuralDirty) != len(epoch.points) || len(epoch.postfixDirty) != len(epoch.points) || len(epoch.queue.ready) != len(epoch.points) || epoch.postfixHead != len(epoch.postfixPending) || epoch.queue.count != 0 {
		return preparedSelectedFactorEpoch{}, false
	}
	for _, target := range overlay.targets {
		if target < 0 || target >= len(epoch.points) || target >= len(epoch.runtime.activePoints) || !epoch.runtime.activePoints[target] || epoch.structuralDirty[target] || epoch.postfixDirty[target] || epoch.queue.ready[target] {
			return preparedSelectedFactorEpoch{}, false
		}
	}
	for _, addition := range overlay.additions {
		edge := addition.edge
		if edge.index < overlay.previousEdgeCount || edge.index >= overlay.previousEdgeCount+len(overlay.additions) || edge.source < 0 || edge.source >= len(epoch.points) || edge.target < 0 || edge.target >= len(epoch.points) || epoch.runtime.activePoints[edge.target] && !epoch.work.OwnsPointState(epoch.points[edge.source]) {
			return preparedSelectedFactorEpoch{}, false
		}
	}
	for _, replacement := range overlay.replacements {
		if replacement.index < 0 || replacement.index >= overlay.previousEdgeCount || replacement.edge.index != replacement.index || replacement.edge.source < 0 || replacement.edge.source >= len(epoch.points) || replacement.edge.target < 0 || replacement.edge.target >= len(epoch.points) || epoch.runtime.activePoints[replacement.edge.target] && !epoch.work.OwnsPointState(epoch.points[replacement.edge.source]) {
			return preparedSelectedFactorEpoch{}, false
		}
	}
	wakePoints := append([]int(nil), overlay.targets...)
	if overlay.execution != nil && overlay.execution.RegionCount() != 0 {
		if len(epoch.regions) != 0 || len(epoch.runtime.regions) != 0 || epoch.runtime.execution != nil || epoch.runtime.executionDemand != nil || overlay.executionDemand == nil || len(overlay.regions) != overlay.execution.RegionCount() || len(overlay.activeRegions) != len(overlay.regions) {
			return preparedSelectedFactorEpoch{}, false
		}
		for index, region := range overlay.regions {
			if !overlay.activeRegions[index] {
				if region.active {
					return preparedSelectedFactorEpoch{}, false
				}
				continue
			}
			if !region.active || region.head < 0 || region.head >= len(epoch.points) {
				return preparedSelectedFactorEpoch{}, false
			}
			wakePoints = append(wakePoints, region.head)
		}
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
		if point < 0 || point >= len(epoch.points) || point >= len(epoch.runtime.activePoints) || !epoch.runtime.activePoints[point] || epoch.structuralDirty[point] || epoch.postfixDirty[point] || epoch.queue.ready[point] {
			return preparedSelectedFactorEpoch{}, false
		}
	}
	result := preparedSelectedFactorEpoch{postfixPending: append([]int(nil), wakePoints...), wakePoints: wakePoints}
	if overlay.execution != nil && overlay.execution.RegionCount() != 0 {
		result.regions = make([]regionEpoch, len(overlay.regions))
		result.candidatesPending = make([]uint64, len(overlay.regions))
		// The overlay is committed by direct assignment below, rather than by
		// enqueuePoint. Seed nested exactly as those already-ready Points would
		// have done after the candidate WTO became live. In particular, a Point
		// in a child region contributes to each enclosing parent's nested count;
		// otherwise the first takePoint would underflow that parent.
		var nestedOK bool
		result.nested, nestedOK = preparedSelectedOverlayNested(wakePoints, overlay.pointRegion, overlay.regions, overlay.activeRegions)
		if !nestedOK {
			return preparedSelectedFactorEpoch{}, false
		}
		result.frames = make([]pointWTOFrame, 0, len(overlay.regions))
		result.regionScratch = make([]int, 0, len(overlay.regions))
		for index, region := range overlay.regions {
			episode := &result.regions[index]
			episode.phase = phaseAscent
			episode.episode = 1
			episode.invalid = true
			episode.interfaces = make([]uint64, len(region.faces))
			episode.ingress = make([]uint64, len(region.external))
			episode.backIngress = make([]uint64, len(region.back))
			episode.environmentIngress = make([]uint64, len(region.environmentExternal))
			episode.environmentBackIngress = make([]uint64, len(region.environmentBack))
			episode.factorIngress = make([]uint64, len(region.factorExternal))
			episode.factorBackIngress = make([]uint64, len(region.factorBack))
			episode.snapshot = make([]uint64, len(region.points))
		}
	}
	nextCount := overlay.previousEdgeCount + len(overlay.additions)
	if nextCount < overlay.previousEdgeCount {
		return preparedSelectedFactorEpoch{}, false
	}
	if !validPreparedFactorCSRRows(overlay.incomingRows, len(epoch.points), nextCount) || !validPreparedFactorCSRRows(overlay.outgoingRows, len(epoch.points), nextCount) {
		return preparedSelectedFactorEpoch{}, false
	}
	return result, true
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
