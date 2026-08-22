// runtime_epoch_activation.go canonicalizes accepted activations and installs the selected factor overlay.

package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	demandpkg "github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/change"
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

// nextFactorSources is the edge-source column of the frontier this overlay
// publishes, served from the prepared rows before any of them is assigned to
// the runtime. A grown backing store already carries both the appended and
// the replaced rows; an in-place frontier keeps them beside the installed
// ones until the commit writes them.
func (overlay *preparedSelectedFactorOverlay) nextFactorSources(runtime *solverRuntime) factorSourceColumn {
	if overlay.grownFactorEdges != nil {
		return installedFactorSources(overlay.grownFactorEdges)
	}
	return factorSourceColumn{installed: runtime.factorEdges, additions: overlay.additions, replacements: overlay.replacements}
}

// installSelectedFactorOverlay publishes one prepared selected-edge delta
// into a settled running epoch. Every admission the installation can refuse -
// the prepared activation epoch, the operand plane over the rows about to be
// published, the operand epoch, and every point index it installs - completes
// on prepared values before the first mutation; the commit below then has no
// fallible semantic operation, only assignments, map publication, and
// touched-bit updates.
func (epoch *executorEpoch) installSelectedFactorOverlay(overlay *preparedSelectedFactorOverlay) (bool, solveBoundary) {
	if !epochSelectedOverlayInstallEligible(epoch, overlay) {
		return false, selectedOverlayRefused("install-eligibility")
	}
	if epoch.runtime.artifactBacked {
		return epoch.installSelectedFactorOverlayArtifact(overlay)
	}
	prepared, ok := epoch.prepareSelectedFactorEpoch(overlay)
	if !ok {
		return false, selectedOverlayRefused("install-epoch")
	}
	runtime := epoch.runtime
	if overlay.execution == nil || overlay.executionDemand == nil || overlay.demandEpoch == nil || len(overlay.activePoints) != len(runtime.activePoints) {
		return false, selectedOverlayRefused("install-frontier")
	}
	// Region ordinals are per-frontier and private, so the operand transpose
	// over them is too. It is derived from the edge and region rows this
	// overlay publishes rather than from the rows the runtime still holds,
	// which is what keeps the last fallible derivation ahead of the commit,
	// and it is published together with those rows before any reader can
	// reach a row of one frontier through a transpose of another.
	operands, planed := buildOperandPlane(runtime.graph, runtime.producers, runtime.environments, overlay.nextFactorSources(runtime), overlay.regions)
	if !planed {
		return false, selectedOverlayRefused("install-operand-plane")
	}
	if !epoch.operands.openable(operands) {
		return false, selectedOverlayRefused("install-operand-epoch")
	}
	// The frontier's per-region change-fact is derived here, against the rows
	// the runtime still holds, because it is the only place both frontiers are
	// readable. It is what the tick space and the region episodes are carried
	// under below; nothing is retained that it does not prove.
	repointed := make(map[int]struct{}, len(overlay.replacements))
	for _, replacement := range overlay.replacements {
		repointed[replacement.index] = struct{}{}
	}
	previousOf, carry, classified := regionFrontierCarry(runtime.regions, overlay.regions, runtime.activeRegions, overlay.activeRegions, repointed)
	if !classified {
		return false, selectedOverlayRefused("install-carry")
	}
	if len(prepared.regions) != len(overlay.regions) || len(overlay.regionChildren) != len(overlay.regions) || len(carry) != len(overlay.regions) {
		return false, selectedOverlayRefused("install-region-width")
	}
	// An in-place frontier appends into reserved capacity. prepareEdgeBacking
	// grows the store whenever that capacity is short, so the reslice below is
	// admitted here rather than assumed across the two calls.
	if overlay.grownFactorEdges == nil && cap(runtime.factorEdges) < overlay.previousEdgeCount+len(overlay.additions) {
		return false, selectedOverlayRefused("install-edge-capacity")
	}
	for _, activation := range prepared.pointActivations {
		if !epoch.installablePoint(activation.index) {
			return false, selectedOverlayRefused("install-point")
		}
	}
	directAt := cloneDirectCatalog(overlay.directCatalog)
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
	runtime.operands = operands
	// The region episodes and the tick space are carried under one change-fact
	// and nothing after this point may observe one without the other, so the
	// two refuse through the epoch fail-stop rather than by returning a
	// half-installed frontier to the caller.
	regions, carried := epoch.carryRegionEpisodes(prepared.regions, previousOf, carry)
	if !carried {
		return epoch.fail(), selectedOverlayRefused("install-region-episode")
	}
	epoch.regions = regions
	if !epoch.markCarriedRegionOperands(epoch.operands.openAdmitted(operands, previousOf, carry), carry) {
		return epoch.fail(), selectedOverlayRefused("install-carried-operand")
	}
	for _, activation := range prepared.pointActivations {
		// An activation installs a point without publishing a
		// predecessor-to-successor transition, so it issues no classification.
		epoch.installAdmittedPoint(activation.index, activation.state, change.Set{})
		epoch.structuralDirty[activation.index] = true
	}
	for _, activation := range prepared.producerActivations {
		epoch.producers[activation.index] = activation.state
	}
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
	runtime.overlay.directAt = directAt
	for _, target := range overlay.targets {
		epoch.structuralDirty[target] = true
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
	return true, boundaryNone
}

// installSelectedFactorOverlayArtifact commits a prepared contextual
// frontier. Graph-shaped fields are retained only as detached mirrors for
// existing non-artifact inspection; all mounted epoch state, wake rows,
// factor occurrences and schedule events are StateOrdinal-indexed.
func (epoch *executorEpoch) installSelectedFactorOverlayArtifact(overlay *preparedSelectedFactorOverlay) (bool, solveBoundary) {
	if epoch == nil || overlay == nil || epoch.runtime == nil || !epoch.runtime.artifactBacked || overlay.stateExecution == nil || len(overlay.stateActive) != len(epoch.points) || len(overlay.stateTargets) == 0 || len(overlay.stateSelected) == 0 {
		return false, selectedOverlayRefused("artifact-instance")
	}
	prepared, ok := epoch.prepareSelectedFactorEpochArtifact(overlay)
	if !ok || overlay.demandEpoch == nil || !overlay.demandEpoch.Live() {
		return false, selectedOverlayRefused("artifact-epoch")
	}
	runtime := epoch.runtime
	if overlay.execution == nil || overlay.executionDemand == nil || len(overlay.activePoints) != len(runtime.activePoints) || len(overlay.stateFactorIncoming) != len(epoch.points) || len(overlay.stateFactorOutgoing) != len(epoch.points) || len(overlay.statePointRegion) != len(epoch.points) || len(overlay.stateRegions) != len(overlay.stateActiveRegions) {
		return false, selectedOverlayRefused("artifact-frontier")
	}
	// The graph view remains a cold consistency witness for an overlay built
	// from the same frontier. Mounted execution below uses only state rows, but
	// a caller cannot replace the graph witness with an unrelated region table
	// and still claim this is that prepared frontier.
	nextEdgeCount := overlay.previousEdgeCount + len(overlay.additions)
	if overlay.execution.RegionCount() != len(overlay.regions) || nextEdgeCount < overlay.previousEdgeCount {
		return false, selectedOverlayRefused("artifact-region-width")
	}
	for _, region := range overlay.regions {
		for _, edge := range append(append([]int(nil), region.factorExternal...), region.factorBack...) {
			if edge < 0 || edge >= nextEdgeCount {
				return false, selectedOverlayRefused("artifact-region-edge")
			}
		}
	}
	operands, planed := buildStateOperandPlane(runtime, stateFactorSources(overlay.stateFactorRows), overlay.stateRegions)
	if !planed || !epoch.operands.openable(operands) {
		return false, selectedOverlayRefused("artifact-operand-plane")
	}
	for _, activation := range prepared.pointActivations {
		if !epoch.installablePoint(activation.index) {
			return false, selectedOverlayRefused("artifact-point")
		}
	}
	for _, activation := range prepared.producerActivations {
		if activation.index < 0 || activation.index >= len(epoch.producers) || activation.stateIndex < 0 || activation.stateIndex >= len(epoch.points) || activation.group < 0 || activation.group >= len(runtime.producers) {
			return false, selectedOverlayRefused("artifact-producer")
		}
	}
	if overlay.grownFactorEdges == nil && cap(runtime.factorEdges) < overlay.previousEdgeCount+len(overlay.additions) {
		return false, selectedOverlayRefused("artifact-edge-capacity")
	}

	// All fallible derivation is complete. The following assignments publish
	// one coherent contextual frontier.
	if epoch.demand != nil {
		epoch.demand.Discard()
	}
	epoch.demand = overlay.demandEpoch
	runtime.points = overlay.executionDemand
	runtime.execution = overlay.execution
	runtime.executionDemand = overlay.executionDemand
	runtime.stateExecution = overlay.stateExecution
	runtime.stateExecutionEvents = overlay.stateExecutionEvents
	runtime.activePoints = overlay.activePoints
	runtime.activeStates = overlay.stateActive
	runtime.stateSelected = overlay.stateSelected
	runtime.stateFactorRows = overlay.stateFactorRows
	runtime.stateFactorIncoming = overlay.stateFactorIncoming
	runtime.stateFactorOutgoing = overlay.stateFactorOutgoing
	runtime.regions = overlay.stateRegions
	runtime.regionChildren = overlay.stateRegionChildren
	runtime.pointRegion = overlay.statePointRegion
	runtime.activeRegions = overlay.stateActiveRegions
	runtime.operands = operands
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
	// The old graph CSR is a detached metadata mirror. Mounted fold/wake paths
	// use stateFactorIncoming/stateFactorOutgoing above and never resolve a
	// mutable row by graph Point alone.
	epoch.operands.open(operands)
	epoch.regions = prepared.regions
	for _, activation := range prepared.pointActivations {
		epoch.installAdmittedPoint(activation.index, activation.state, change.Set{})
		epoch.structuralDirty[activation.index] = true
	}
	for _, activation := range prepared.producerActivations {
		epoch.producers[activation.index] = activation.state
	}
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
	for _, target := range overlay.stateTargets {
		epoch.structuralDirty[target] = true
	}
	for _, point := range prepared.wakePoints {
		if point < 0 || point >= len(epoch.points) {
			return false, selectedOverlayRefused("artifact-wake-point")
		}
		epoch.postfixDirty[point] = true
		epoch.queue.ready[point] = true
	}
	epoch.postfixPending = prepared.postfixPending
	epoch.postfixHead = 0
	epoch.queue.count = len(prepared.wakePoints)
	runtime.overlay.generation = runtime.overlay.generation.Next()
	return true, boundaryNone
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
	// index is the compact producer-row ordinal for artifact epochs and the
	// singular graph Group ordinal for the explicit non-artifact construction.
	index int
	// stateIndex/group identify the contextual occurrence when artifact-backed.
	stateIndex int
	group      int
	state      producerEpoch
}

func (epoch *executorEpoch) prepareSelectedFactorEpoch(overlay *preparedSelectedFactorOverlay) (preparedSelectedFactorEpoch, bool) {
	if epoch != nil && epoch.runtime != nil && epoch.runtime.artifactBacked {
		return epoch.prepareSelectedFactorEpochArtifact(overlay)
	}
	if epoch == nil || overlay == nil || epoch.runtime == nil || epoch.work == nil || epoch.demand == nil || !epoch.demand.Live() || len(epoch.structural.pointDescent) != len(epoch.points) || len(epoch.structuralDirty) != len(epoch.points) || len(epoch.postfixDirty) != len(epoch.points) || len(epoch.queue.ready) != len(epoch.points) || epoch.postfixHead != len(epoch.postfixPending) || epoch.queue.count != 0 || len(overlay.activePoints) != len(epoch.points) || len(epoch.runtime.activePoints) != len(epoch.points) || len(overlay.selectedPoints) == 0 {
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
			metadata := &epoch.runtime.producers[groupIndex]
			inputCount := metadata.group.InputCount()
			cache := producerEpoch{generation: 1, inputs: make([]carrier.PointState, inputCount), inputStates: make([]carrier.State, inputCount), patches: make([]carrier.Patch, 0, metadata.span.count()), patchRows: make([]contributionPatch, 0, metadata.span.count()), reads: make([]demandpkg.Observation, 0, len(metadata.reads))}
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

// prepareSelectedFactorEpochArtifact is the contextual activation cut. It
// never interprets a graph Point ordinal as an epoch row: every new Point and
// producer occurrence is admitted through the compact overlay state index.
func (epoch *executorEpoch) prepareSelectedFactorEpochArtifact(overlay *preparedSelectedFactorOverlay) (preparedSelectedFactorEpoch, bool) {
	if epoch == nil || overlay == nil || epoch.runtime == nil || !epoch.runtime.artifactBacked || epoch.work == nil || epoch.demand == nil || !epoch.demand.Live() || overlay.stateExecution == nil || overlay.stateExecution.NodeCount() != len(epoch.points) || len(overlay.stateActive) != len(epoch.points) || len(epoch.runtime.activeStates) != len(epoch.points) || len(epoch.structural.pointDescent) != len(epoch.points) || len(epoch.structuralDirty) != len(epoch.points) || len(epoch.postfixDirty) != len(epoch.points) || len(epoch.queue.ready) != len(epoch.points) || epoch.postfixHead != len(epoch.postfixPending) || epoch.queue.count != 0 || len(overlay.stateSelected) == 0 {
		return preparedSelectedFactorEpoch{}, false
	}
	pointActivations := make([]preparedPointActivation, 0)
	producerActivations := make([]preparedProducerActivation, 0)
	newStates := make(map[int]struct{})
	producerActivationAt := make(map[stateGroupKey]producerEpoch)
	for stateIndex, active := range overlay.stateActive {
		if !active {
			if epoch.runtime.activeStates[stateIndex] {
				return preparedSelectedFactorEpoch{}, false
			}
			continue
		}
		if epoch.runtime.activeStates[stateIndex] {
			if !epoch.work.OwnsPointState(epoch.points[stateIndex]) {
				return preparedSelectedFactorEpoch{}, false
			}
			continue
		}
		point, _, _, pointOK := epoch.runtime.graphPointAtState(stateIndex)
		state, stateOK := preparedRuntimeStateState(epoch, stateIndex, point)
		if !pointOK || !stateOK {
			return preparedSelectedFactorEpoch{}, false
		}
		pointActivations = append(pointActivations, preparedPointActivation{index: stateIndex, state: state})
		newStates[stateIndex] = struct{}{}
		for producerIndex := 0; producerIndex < epoch.runtime.graph.ProducerCount(point); producerIndex++ {
			groupNode, groupOK := epoch.runtime.graph.ProducerAt(point, producerIndex)
			groupIndex, indexed := epoch.runtime.graph.GroupIndex(groupNode)
			row, rowOK := epoch.runtime.producerRows.row(contextfiber.StateOrdinal(stateIndex), groupIndex)
			if !groupOK || !indexed || !rowOK || groupIndex < 0 || groupIndex >= len(epoch.runtime.producers) || groupNode.Output() != point {
				return preparedSelectedFactorEpoch{}, false
			}
			cache := newPreparedProducerEpoch(epoch.runtime, stateIndex, groupIndex)
			if cache.generation == 0 {
				return preparedSelectedFactorEpoch{}, false
			}
			key := stateGroupKey{state: contextfiber.StateOrdinal(stateIndex), group: groupIndex}
			if _, duplicate := producerActivationAt[key]; duplicate {
				return preparedSelectedFactorEpoch{}, false
			}
			producerActivationAt[key] = cache
			producerActivations = append(producerActivations, preparedProducerActivation{index: row, stateIndex: stateIndex, group: groupIndex, state: cache})
		}
	}
	for _, target := range overlay.stateTargets {
		if target < 0 || target >= len(epoch.points) || !overlay.stateActive[target] {
			return preparedSelectedFactorEpoch{}, false
		}
		if epoch.runtime.activeStates[target] && (epoch.structuralDirty[target] || epoch.postfixDirty[target] || epoch.queue.ready[target]) {
			return preparedSelectedFactorEpoch{}, false
		}
	}
	// Every lifted factor row naming this frontier must have exact active
	// source/target states. This also fences replacements whose graph pair is
	// valid but whose context occurrence is not executable.
	for _, row := range overlay.stateFactorRows {
		if row.edge < 0 || row.edge >= overlay.previousEdgeCount+len(overlay.additions) || row.source < 0 || row.source >= len(epoch.points) || row.target < 0 || row.target >= len(epoch.points) {
			return preparedSelectedFactorEpoch{}, false
		}
		if !overlay.stateActive[row.target] {
			continue
		}
		if row.edge >= overlay.previousEdgeCount || containsPreparedReplacement(overlay.replacements, row.edge) {
			_, sourcePrepared := newStates[row.source]
			if (!epoch.runtime.activeStates[row.source] && !sourcePrepared) || epoch.runtime.activeStates[row.source] && !epoch.work.OwnsPointState(epoch.points[row.source]) {
				return preparedSelectedFactorEpoch{}, false
			}
		} else if !overlay.stateActive[row.source] {
			return preparedSelectedFactorEpoch{}, false
		}
	}
	wakePoints := append([]int(nil), overlay.stateTargets...)
	for _, activation := range pointActivations {
		wakePoints = append(wakePoints, activation.index)
	}
	sort.Ints(wakePoints)
	wakePoints = uniqueInts(wakePoints)
	for _, point := range wakePoints {
		if point < 0 || point >= len(epoch.points) || !overlay.stateActive[point] {
			return preparedSelectedFactorEpoch{}, false
		}
		if epoch.runtime.activeStates[point] && (epoch.structuralDirty[point] || epoch.postfixDirty[point] || epoch.queue.ready[point]) {
			return preparedSelectedFactorEpoch{}, false
		}
	}
	if len(overlay.stateRegions) != len(overlay.stateActiveRegions) || len(overlay.stateRegionChildren) != len(overlay.stateRegions) || len(overlay.statePointRegion) != len(epoch.points) || len(overlay.stateExecutionEvents) == 0 {
		return preparedSelectedFactorEpoch{}, false
	}
	for _, region := range overlay.stateRegions {
		if !region.active {
			continue
		}
		if region.head < 0 || region.head >= len(epoch.points) || !overlay.stateActive[region.head] {
			return preparedSelectedFactorEpoch{}, false
		}
		wakePoints = append(wakePoints, region.head)
	}
	sort.Ints(wakePoints)
	wakePoints = uniqueInts(wakePoints)
	for _, point := range wakePoints {
		if point < 0 || point >= len(epoch.points) || !overlay.stateActive[point] {
			return preparedSelectedFactorEpoch{}, false
		}
		if epoch.runtime.activeStates[point] && (epoch.structuralDirty[point] || epoch.postfixDirty[point] || epoch.queue.ready[point]) {
			return preparedSelectedFactorEpoch{}, false
		}
	}
	result := preparedSelectedFactorEpoch{postfixPending: append([]int(nil), wakePoints...), wakePoints: wakePoints, pointActivations: pointActivations, producerActivations: producerActivations, regions: make([]regionEpoch, len(overlay.stateRegions)), candidatesPending: make([]uint64, len(overlay.stateRegions)), frames: make([]pointWTOFrame, 0, len(overlay.stateRegions)), regionScratch: make([]int, 0, len(overlay.stateRegions))}
	var nestedOK bool
	result.nested, nestedOK = preparedSelectedOverlayNested(wakePoints, overlay.statePointRegion, overlay.stateRegions, overlay.stateActiveRegions)
	if !nestedOK {
		return preparedSelectedFactorEpoch{}, false
	}
	for regionIndex, region := range overlay.stateRegions {
		if !overlay.stateActiveRegions[regionIndex] {
			if region.active {
				return preparedSelectedFactorEpoch{}, false
			}
			continue
		}
		if !region.active {
			return preparedSelectedFactorEpoch{}, false
		}
		result.regions[regionIndex].phase = phaseAscent
		result.regions[regionIndex].episode = 1
		result.regions[regionIndex].invalid = true
		var pending uint64
		for _, stateIndex := range region.points {
			if stateIndex < 0 || stateIndex >= len(epoch.points) || !overlay.stateActive[stateIndex] {
				return preparedSelectedFactorEpoch{}, false
			}
			point, _, _, pointOK := epoch.runtime.graphPointAtState(stateIndex)
			if !pointOK {
				return preparedSelectedFactorEpoch{}, false
			}
			for producerIndex := 0; producerIndex < epoch.runtime.graph.ProducerCount(point); producerIndex++ {
				groupNode, groupOK := epoch.runtime.graph.ProducerAt(point, producerIndex)
				groupIndex, indexed := epoch.runtime.graph.GroupIndex(groupNode)
				cache, cacheOK := epoch.producerCache(contextfiber.StateOrdinal(stateIndex), groupIndex)
				if !groupOK || !indexed || !cacheOK {
					return preparedSelectedFactorEpoch{}, false
				}
				if activation, activated := producerActivationAt[stateGroupKey{state: contextfiber.StateOrdinal(stateIndex), group: groupIndex}]; activated {
					cache = &activation
				}
				if cache.generation != cache.applied {
					if pending == ^uint64(0) {
						return preparedSelectedFactorEpoch{}, false
					}
					pending++
				}
			}
		}
		result.candidatesPending[regionIndex] = pending
	}
	// Artifact demand is represented by stateActive/stateSelected. The graph
	// demand epoch is widened only as a detached lifecycle fence at install;
	// it is not consulted for mounted routing or scheduling.
	demandEpoch, demandOK := epoch.demand.Widen(overlay.selectedPoints)
	if !demandOK {
		return preparedSelectedFactorEpoch{}, false
	}
	overlay.demandEpoch = demandEpoch
	return result, true
}

func newPreparedProducerEpoch(runtime *solverRuntime, stateIndex, groupIndex int) producerEpoch {
	if runtime == nil || groupIndex < 0 || groupIndex >= len(runtime.producers) {
		return producerEpoch{}
	}
	metadata := &runtime.producers[groupIndex]
	return producerEpoch{state: contextfiber.StateOrdinal(stateIndex), group: groupIndex, generation: 1, inputs: make([]carrier.PointState, metadata.group.InputCount()), inputStates: make([]carrier.State, metadata.group.InputCount()), patches: make([]carrier.Patch, 0, metadata.span.count()), patchRows: make([]contributionPatch, 0, metadata.span.count()), reads: make([]demandpkg.Observation, 0, len(metadata.reads))}
}

func containsPreparedReplacement(replacements []preparedFactorReplacement, index int) bool {
	for _, replacement := range replacements {
		if replacement.index == index {
			return true
		}
	}
	return false
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

func preparedRuntimeStateState(epoch *executorEpoch, stateIndex int, point equation.Point) (carrier.PointState, bool) {
	if epoch == nil || epoch.runtime == nil || epoch.work == nil || stateIndex < 0 || !point.Available() {
		return carrier.PointState{}, false
	}
	_, pointIndex, _, pointOK := epoch.runtime.graphPointAtState(stateIndex)
	if !pointOK || pointIndex < 0 || pointIndex >= len(epoch.runtime.pointScopes) || !epoch.runtime.pointScopes[pointIndex].Valid() {
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
