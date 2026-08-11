package engine

import (
	"context"
	"sort"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
)

type SolveStatus uint8

const (
	SolveIncomplete SolveStatus = iota + 1
	SolveComplete
	SolveCanceled
	SolvePanicked
)

func (status SolveStatus) Complete() bool { return status == SolveComplete }

// producerEpoch is an epoch-local candidate cache in graph Group order. A
// generation marks it dirty; the runnable identity is always its output Point.
type producerEpoch struct {
	generation                uint64
	applied                   uint64
	version                   uint64
	candidate                 carrier.Contribution
	hasValue                  bool
	candidateTokens           []uint64
	scratchTokens             []uint64
	hasCandidateTokens        bool
	candidateEnvironmentToken uint64
	scratchEnvironmentToken   uint64
	inputs                    []carrier.Contribution
	inputStates               []carrier.State
	patches                   []carrier.Patch
	patchRows                 []contributionPatch
	reads                     []demand.Observation
}

// contributionPatch keeps carrier admission order separate from Rule callback
// order. Members execute in canonical graph order; only their already
// accepted patches are reordered into the carrier's physical slot order at
// the one contribution finish cut.
type contributionPatch struct {
	slot  shape.Slot
	patch carrier.Patch
}

// regionEpoch is a disposable localized-widening episode. exact is the last
// exact E⊔B head RHS; it never aliases a widened Point publication.
type regionEpoch struct {
	// phase belongs to this exact recurrence episode.  Nested regions may
	// restart independently: a child re-ascent must not turn an enclosing
	// narrowed episode into an ascent episode with its narrowed head retained.
	phase                  solvePhase
	exact                  carrier.Contribution
	hasExact               bool
	invalid                bool
	interfaces             []uint64
	ingress                []uint64
	backIngress            []uint64
	environmentIngress     []uint64
	environmentBackIngress []uint64
	factorIngress          []uint64
	factorBackIngress      []uint64
	snapshot               []uint64
}

type solvePhase uint8

const (
	phaseAscent solvePhase = iota + 1
	phaseNarrow
)

const (
	epochRunning uint32 = iota + 1
	epochCompleted
	epochIncomplete
)

// pointQueue is a dense, deduplicating Point queue. It is intentionally a
// readiness bitset: WTO events determine dequeue order, not Group ordering.
type pointQueue struct {
	ready []bool
	count int
}

func newPointQueue(count int) pointQueue { return pointQueue{ready: make([]bool, count)} }
func (queue *pointQueue) add(point int) bool {
	if queue == nil || point < 0 || point >= len(queue.ready) {
		return false
	}
	if !queue.ready[point] {
		queue.ready[point] = true
		queue.count++
	}
	return true
}
func (queue *pointQueue) take(point int) bool {
	if queue == nil || point < 0 || point >= len(queue.ready) || !queue.ready[point] {
		return false
	}
	queue.ready[point] = false
	queue.count--
	return true
}
func (queue *pointQueue) pending() bool { return queue != nil && queue.count != 0 }

type pointWTOFrame struct{ region int }

type executorEpoch struct {
	runtime *solverRuntime
	ctx     context.Context
	// report is a call-scoped first-failure sink. It is nil on the ordinary
	// Solve path, so the hot path does not allocate or retain diagnostics.
	report            *SolveReport
	work              *carrier.Work
	demand            *demand.Epoch
	points            []carrier.Contribution
	versions          []uint64
	producers         []producerEpoch
	regions           []regionEpoch
	queue             pointQueue
	structuralDirty   []bool
	postfixDirty      []bool
	postfixPending    []int
	postfixHead       int
	frames            []pointWTOFrame
	nested            []int
	regionScratch     []int
	terminal          atomic.Uint32
	activationPending bool
	activations       []equation.AcceptedMember
}

func (epoch *executorEpoch) recordFailure(reason SolveFailureReason, phase SolveFailurePhase, point, group, member, rule composition.Key) {
	if epoch == nil || epoch.report == nil {
		return
	}
	epoch.report.record(reason, phase, semanticKeyFromComposition(point), semanticKeyFromComposition(group), semanticKeyFromComposition(member), semanticKeyFromComposition(rule))
}

func (epoch *executorEpoch) recordPointFailure(reason SolveFailureReason, point equation.Point) {
	if epoch == nil || !point.Available() {
		return
	}
	epoch.recordFailure(reason, SolveFailurePhaseNone, point.Key(), composition.Key{}, composition.Key{}, composition.Key{})
}

func (epoch *executorEpoch) recordGroupFailure(reason SolveFailureReason, point equation.Point, group equation.GroupNode) {
	if epoch == nil {
		return
	}
	epoch.recordFailure(reason, SolveFailurePhaseNone, func() composition.Key {
		if point.Available() {
			return point.Key()
		}
		return composition.Key{}
	}(), func() composition.Key {
		if epoch.runtime != nil && epoch.runtime.graph != nil && epoch.runtime.graph.OwnsGroup(group) {
			return group.Key()
		}
		return composition.Key{}
	}(), composition.Key{}, composition.Key{})
}

func (epoch *executorEpoch) recordMemberFailure(reason SolveFailureReason, phase SolveFailurePhase, point equation.Point, group equation.GroupNode, member equation.RuleMember) {
	if epoch == nil {
		return
	}
	groupKey, memberKey, ruleKey := composition.Key{}, composition.Key{}, composition.Key{}
	if epoch.runtime != nil && epoch.runtime.graph != nil && epoch.runtime.graph.OwnsGroup(group) {
		groupKey = group.Key()
	}
	if epoch.runtime != nil && epoch.runtime.graph != nil && epoch.runtime.graph.OwnsMember(member) {
		memberKey, ruleKey = member.Key(), member.Rule()
	}
	pointKey := composition.Key{}
	if point.Available() {
		pointKey = point.Key()
	}
	epoch.recordFailure(reason, phase, pointKey, groupKey, memberKey, ruleKey)
}

func newRuntimeEpoch(runtime *solverRuntime, accepted []equation.AcceptedMember, ctx context.Context) (*executorEpoch, bool) {
	// Owner/liveness fence: do not allocate an epoch for a missing or canceled
	// call, or for a runtime with a missing owner-owned root.
	if runtime == nil || ctx == nil || ctx.Err() != nil {
		return nil, false
	}
	if runtime.carrier == nil || runtime.graph == nil || runtime.points == nil || runtime.demand == nil || runtime.topology == nil {
		return nil, false
	}

	// Accepted members must belong to this runtime generation's sealed
	// topology before any carrier or demand state is opened.
	if !validAcceptedActivations(runtime.topology, accepted) {
		return nil, false
	}

	// All dense runtime rows must remain aligned with their graph-owned
	// cardinalities. A mismatch is structural corruption, not an empty solve.
	pointCount := runtime.graph.PointCount()
	groupCount := runtime.graph.GroupCount()
	regionCount := runtime.graph.RegionCount()
	if len(runtime.producers) != groupCount ||
		len(runtime.activePoints) != pointCount ||
		len(runtime.pointInitials) != pointCount ||
		len(runtime.activeRegions) != regionCount ||
		len(runtime.regions) != regionCount ||
		len(runtime.regionChildren) != regionCount ||
		len(runtime.environmentIncoming) != pointCount ||
		len(runtime.factorIncoming) != pointCount {
		return nil, false
	}
	work, ok := runtime.carrier.NewWork()
	if !ok {
		return nil, false
	}
	var demandEpoch *demand.Epoch
	opened := false
	defer func() {
		if opened {
			return
		}
		if demandEpoch != nil {
			demandEpoch.Discard()
		}
		work.Close()
	}()
	demandEpoch, ok = demand.Open(runtime.demand)
	if !ok {
		return nil, false
	}
	empty, ok := support.FromGuard(runtime.carrier.Guards(), runtime.carrier.Guards().False())
	if !ok {
		return nil, false
	}
	epoch := &executorEpoch{runtime: runtime, ctx: ctx, work: work, demand: demandEpoch, points: make([]carrier.Contribution, runtime.graph.PointCount()), versions: make([]uint64, runtime.graph.PointCount()), producers: make([]producerEpoch, runtime.graph.GroupCount()), regions: make([]regionEpoch, len(runtime.regions)), queue: newPointQueue(runtime.graph.PointCount()), structuralDirty: make([]bool, runtime.graph.PointCount()), postfixDirty: make([]bool, runtime.graph.PointCount()), postfixPending: make([]int, 0, runtime.graph.PointCount()), frames: make([]pointWTOFrame, 0, runtime.graph.Schedule().RegionCount()), nested: make([]int, runtime.graph.RegionCount()), regionScratch: make([]int, 0, runtime.graph.RegionCount())}
	epoch.terminal.Store(epochRunning)
	if !work.SetCheckpoint(func() bool { return !epoch.canceled() }) {
		return nil, false
	}
	for index, region := range runtime.regions {
		if !runtime.activeRegions[index] {
			if region.active {
				return nil, false
			}
			continue
		}
		if !region.active {
			return nil, false
		}
		epoch.regions[index].phase = phaseAscent
		epoch.regions[index].interfaces = make([]uint64, len(region.faces))
		epoch.regions[index].ingress = make([]uint64, len(region.external))
		epoch.regions[index].backIngress = make([]uint64, len(region.back))
		epoch.regions[index].environmentIngress = make([]uint64, len(region.environmentExternal))
		epoch.regions[index].environmentBackIngress = make([]uint64, len(region.environmentBack))
		epoch.regions[index].factorIngress = make([]uint64, len(region.factorExternal))
		epoch.regions[index].factorBackIngress = make([]uint64, len(region.factorBack))
		epoch.regions[index].snapshot = make([]uint64, len(region.points))
	}
	for index := 0; index < runtime.points.PointCount(); index++ {
		point, pointOK := runtime.points.PointAt(index)
		pointIndex, indexed := runtime.graph.PointIndex(point)
		if !pointOK || !indexed || pointIndex < 0 || pointIndex >= len(epoch.points) || !runtime.activePoints[pointIndex] {
			return nil, false
		}
		if pointIndex >= len(runtime.pointScopes) || !runtime.pointScopes[pointIndex].Valid() {
			return nil, false
		}
		feasible := empty
		if point.HasInit() {
			feasible = runtime.pointInitials[pointIndex]
			if !feasible.Valid() {
				return nil, false
			}
		}
		state, initialized := carrier.NewState(runtime.carrier, runtime.pointScopes[pointIndex], feasible)
		if !initialized {
			return nil, false
		}
		initial, paired := work.EmptyContribution(state)
		if !paired {
			return nil, false
		}
		epoch.points[pointIndex] = initial
		if !epoch.markPostfixDirty(pointIndex) {
			return nil, false
		}
		for producerIndex := 0; producerIndex < runtime.graph.ProducerCount(point); producerIndex++ {
			group, groupOK := runtime.graph.ProducerAt(point, producerIndex)
			groupIndex, indexed := runtime.graph.GroupIndex(group)
			if !groupOK || !indexed || groupIndex < 0 || groupIndex >= len(epoch.producers) || runtime.producers[groupIndex].group.Output() != point {
				return nil, false
			}
			if epoch.producers[groupIndex].generation == 0 {
				metadata := runtime.producers[groupIndex]
				inputCount := metadata.group.InputCount()
				epoch.producers[groupIndex] = producerEpoch{generation: 1, candidateTokens: make([]uint64, inputCount), scratchTokens: make([]uint64, inputCount), inputs: make([]carrier.Contribution, inputCount), inputStates: make([]carrier.State, inputCount), patches: make([]carrier.Patch, 0, len(metadata.members)), patchRows: make([]contributionPatch, 0, len(metadata.members)), reads: make([]demand.Observation, 0, len(metadata.reads))}
			}
			if !epoch.enqueuePoint(pointIndex) {
				return nil, false
			}
		}
		if !epoch.markStructuralPoint(point) {
			return nil, false
		}
	}
	opened = true
	return epoch, true
}

func (epoch *executorEpoch) discard() {
	if epoch == nil {
		return
	}
	epoch.incomplete()
	if epoch.demand != nil {
		epoch.demand.Discard()
		epoch.demand = nil
	}
	if epoch.work != nil {
		epoch.work.Close()
		epoch.work = nil
	}
}
func (epoch *executorEpoch) canceled() bool {
	if epoch == nil || epoch.ctx == nil || epoch.terminal.Load() != epochRunning {
		return true
	}
	if epoch.ctx.Err() == nil {
		return false
	}
	epoch.terminal.CompareAndSwap(epochRunning, epochIncomplete)
	return true
}

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

func (epoch *executorEpoch) incomplete() {
	if epoch != nil {
		epoch.terminal.CompareAndSwap(epochRunning, epochIncomplete)
	}
}

func (epoch *executorEpoch) complete() bool {
	if epoch == nil || epoch.ctx == nil || epoch.ctx.Err() != nil {
		if epoch != nil {
			epoch.incomplete()
		}
		return false
	}
	return epoch.terminal.CompareAndSwap(epochRunning, epochCompleted)
}

// markPostfixDirty records the one Point whose established postfix proof may
// no longer be reused. The row is deduplicated: a producer can be woken many
// times before its Point gets another opportunity to prove its complete RHS.
func (epoch *executorEpoch) markPostfixDirty(point int) bool {
	if epoch == nil || epoch.runtime == nil || point < 0 || point >= len(epoch.postfixDirty) || point >= len(epoch.runtime.activePoints) || !epoch.runtime.activePoints[point] {
		return false
	}
	if epoch.postfixDirty[point] {
		return true
	}
	if epoch.postfixHead == len(epoch.postfixPending) {
		epoch.postfixPending = epoch.postfixPending[:0]
		epoch.postfixHead = 0
	}
	epoch.postfixDirty[point] = true
	epoch.postfixPending = append(epoch.postfixPending, point)
	return true
}

// markStructuralPoint is the one wakeup path for structural producers. Both
// environment and factor edges are graph-owned Point dependencies: a target
// must retain its structural proof bit while it is queued, then refreshPoint
// folds the complete incoming edge set before clearing that bit. Keeping this
// admission in one helper prevents a new structural edge family from falling
// through the no-candidate early return.
func (epoch *executorEpoch) markStructuralPoint(point equation.Point) bool {
	if epoch == nil || epoch.runtime == nil || !point.Available() || epoch.runtime.graph == nil {
		return false
	}
	pointIndex, indexed := epoch.runtime.graph.PointIndex(point)
	if !indexed || pointIndex < 0 || pointIndex >= len(epoch.structuralDirty) || pointIndex >= len(epoch.runtime.activePoints) {
		return false
	}
	// A structural predecessor may be active while its downstream consumer is
	// outside the demanded Point closure. That consumer has no epoch row to
	// wake; it is not a solver error and must not poison the demanded solve.
	if !epoch.runtime.activePoints[pointIndex] {
		return true
	}
	if epoch.runtime.graph.EnvironmentEdgeCount(point) == 0 && epoch.runtime.graph.FactorEdgeCount(point) == 0 {
		return true
	}
	epoch.structuralDirty[pointIndex] = true
	return epoch.markPostfixDirty(pointIndex) && epoch.enqueuePoint(pointIndex)
}

// markStructuralSuccessors applies the same Point wake protocol to every
// graph-owned structural edge family.  The edge-specific folds remain in
// foldPoint; scheduling never gets a second EnvironmentEdge or FactorEdge
// branch with its own dirty/version lifecycle.
func (epoch *executorEpoch) markStructuralSuccessors(source equation.Point) bool {
	if epoch == nil || epoch.runtime == nil || epoch.runtime.graph == nil || !source.Available() {
		return false
	}
	graph := epoch.runtime.graph
	for index := 0; index < graph.EnvironmentOutgoingCount(source); index++ {
		edge, ok := graph.EnvironmentOutgoingAt(source, index)
		if !ok || !epoch.markStructuralPoint(edge.Target()) {
			return false
		}
	}
	for index := 0; index < graph.FactorOutgoingCount(source); index++ {
		edge, ok := graph.FactorOutgoingAt(source, index)
		if !ok || !epoch.markStructuralPoint(edge.Target()) {
			return false
		}
	}
	for index := 0; index < graph.EnvironmentGroupCount(source); index++ {
		group, ok := graph.EnvironmentGroupAt(source, index)
		if !ok {
			return false
		}
		groupIndex, indexed := graph.GroupIndex(group)
		if !indexed || groupIndex < 0 || groupIndex >= len(epoch.runtime.producers) {
			return false
		}
		outputIndex, outputIndexed := graph.PointIndex(group.Output())
		if !outputIndexed || outputIndex < 0 || outputIndex >= len(epoch.runtime.activePoints) {
			return false
		}
		if !epoch.runtime.activePoints[outputIndex] {
			continue
		}
		if !epoch.markDirty(groupIndex) {
			return false
		}
	}
	return true
}

func (epoch *executorEpoch) postfixPoint() (int, bool) {
	if epoch == nil || epoch.postfixHead < 0 {
		return 0, false
	}
	for epoch.postfixHead < len(epoch.postfixPending) {
		point := epoch.postfixPending[epoch.postfixHead]
		if point >= 0 && point < len(epoch.postfixDirty) && epoch.postfixDirty[point] {
			return point, true
		}
		epoch.postfixHead++
	}
	return 0, false
}

func (epoch *executorEpoch) provePostfix(point int) bool {
	if epoch == nil || point < 0 || point >= len(epoch.postfixDirty) || !epoch.postfixDirty[point] {
		return false
	}
	epoch.postfixDirty[point] = false
	return true
}

func (epoch *executorEpoch) settlePostfix(point int) bool {
	if epoch == nil || point < 0 || point >= len(epoch.postfixDirty) {
		return false
	}
	return !epoch.postfixDirty[point] || epoch.provePostfix(point)
}

func (epoch *executorEpoch) markDirty(group int) bool {
	if epoch == nil || epoch.runtime == nil || epoch.canceled() || group < 0 || group >= len(epoch.producers) {
		return false
	}
	producer := epoch.runtime.producers[group]
	point, pointOK := epoch.runtime.graph.PointIndex(producer.group.Output())
	if !pointOK || point < 0 || point >= len(epoch.runtime.activePoints) || !epoch.runtime.activePoints[point] {
		return false
	}
	state := &epoch.producers[group]
	state.generation++
	if state.generation == 0 {
		return false
	}
	return epoch.markPostfixDirty(point) && epoch.enqueuePoint(point)
}

func (epoch *executorEpoch) activeRegion(region int) bool {
	return epoch != nil && epoch.runtime != nil && region >= 0 && region < len(epoch.runtime.regions) && region < len(epoch.runtime.activeRegions) && epoch.runtime.activeRegions[region] && epoch.runtime.regions[region].active
}

func (epoch *executorEpoch) updateNested(point int, delta int) bool {
	if epoch == nil || epoch.runtime == nil || point < 0 || point >= len(epoch.runtime.pointRegion) || point >= len(epoch.runtime.activePoints) || !epoch.runtime.activePoints[point] || delta == 0 {
		return false
	}
	region := epoch.runtime.pointRegion[point]
	for region != schedule.NoRegion {
		if !epoch.activeRegion(region) {
			return false
		}
		parent := epoch.runtime.regions[region].parent
		if parent == schedule.NoRegion {
			return true
		}
		if parent < 0 || parent >= len(epoch.nested) || epoch.nested[parent]+delta < 0 {
			return false
		}
		epoch.nested[parent] += delta
		region = parent
	}
	return true
}

func (epoch *executorEpoch) enqueuePoint(point int) bool {
	if epoch == nil || epoch.runtime == nil || point < 0 || point >= len(epoch.queue.ready) || point >= len(epoch.runtime.activePoints) || !epoch.runtime.activePoints[point] {
		return false
	}
	if epoch.queue.ready[point] {
		return true
	}
	return epoch.queue.add(point) && epoch.updateNested(point, 1)
}

func (epoch *executorEpoch) takePoint(point int) bool {
	if epoch == nil || epoch.runtime == nil || point < 0 || point >= len(epoch.queue.ready) || point >= len(epoch.runtime.activePoints) || !epoch.runtime.activePoints[point] || !epoch.queue.ready[point] {
		return false
	}
	return epoch.queue.take(point) && epoch.updateNested(point, -1)
}

// inputs snapshots and reindexes the full Group input vector once. Every
// member below receives this exact vector and the Group output target Scope.
func (epoch *executorEpoch) inputs(producer runtimeProducer, values []carrier.Contribution) ([]carrier.Contribution, bool) {
	if epoch == nil || epoch.work == nil || len(producer.inputs) != producer.group.InputCount() || len(values) != producer.group.InputCount() {
		return nil, false
	}
	for index := range values {
		input, inputOK := producer.group.InputAt(index)
		pointIndex, indexed := epoch.runtime.graph.PointIndex(input.Point())
		if !inputOK || !indexed || pointIndex < 0 || pointIndex >= len(epoch.points) {
			return nil, false
		}
		current := epoch.points[pointIndex]
		if !epoch.work.OwnsAdmittedContribution(current) {
			return nil, false
		}
		transport := producer.inputs[index]
		reindexed, ok := epoch.work.TransportContribution(current, transport.pre, transport.plan, transport.post)
		if !transport.valid() || !ok || !epoch.work.OwnsAdmittedContribution(reindexed) || !reindexed.State().Scope().Same(producer.outputScope) {
			return nil, false
		}
		values[index] = reindexed
	}
	return values, true
}

func (epoch *executorEpoch) environment(producer runtimeProducer) (carrier.Contribution, bool) {
	if epoch == nil || epoch.work == nil || producer.environment == nil || !producer.environment.valid() {
		return carrier.Contribution{}, producer.environment == nil
	}
	input, inputOK := producer.group.EnvironmentInput()
	pointIndex, indexed := epoch.runtime.graph.PointIndex(input.Point())
	if !inputOK || !indexed || pointIndex < 0 || pointIndex >= len(epoch.points) {
		return carrier.Contribution{}, false
	}
	current := epoch.points[pointIndex]
	if !epoch.work.OwnsAdmittedContribution(current) {
		return carrier.Contribution{}, false
	}
	transported, ok := epoch.work.TransportContribution(current, producer.environment.pre, producer.environment.plan, producer.environment.post)
	if !ok || !epoch.work.OwnsAdmittedContribution(transported) || !transported.State().Scope().Same(producer.outputScope) {
		return carrier.Contribution{}, false
	}
	return transported, true
}

// candidateTokens is the complete ordered input-version snapshot that
// justified one producer candidate.  A dirty generation is only a wakeup;
// these graph-issued Point versions are the evidence that the semantic input
// actually changed.  The caller supplies epoch-owned storage so evaluation
// adds no per-refold allocation.
func (epoch *executorEpoch) candidateTokens(producer runtimeProducer, tokens []uint64) bool {
	if epoch == nil || epoch.runtime == nil || len(tokens) != producer.group.InputCount() {
		return false
	}
	for index := range tokens {
		input, inputOK := producer.group.InputAt(index)
		point, indexed := epoch.runtime.graph.PointIndex(input.Point())
		if !inputOK || !indexed || point < 0 || point >= len(epoch.versions) || point >= len(epoch.runtime.activePoints) || !epoch.runtime.activePoints[point] {
			return false
		}
		tokens[index] = epoch.versions[point]
	}
	return true
}

func sameCandidateTokens(left, right []uint64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (epoch *executorEpoch) evaluate(producer runtimeProducer, cache *producerEpoch) (carrier.Contribution, []demand.Observation, bool) {
	if epoch == nil || epoch.work == nil || cache == nil || epoch.canceled() || len(producer.members) == 0 || !producer.outputScope.Valid() || !producer.premise.Valid() || producer.premise.Manager() != epoch.runtime.carrier.Guards() {
		return carrier.Contribution{}, nil, false
	}
	inputs, ok := epoch.inputs(producer, cache.inputs)
	if !ok {
		return carrier.Contribution{}, nil, false
	}
	var environment carrier.Contribution
	if producer.environment != nil {
		environment, ok = epoch.environment(producer)
		if !ok {
			return carrier.Contribution{}, nil, false
		}
	}
	if producer.environment != nil {
		input, inputOK := producer.group.EnvironmentInput()
		pointIndex, indexed := epoch.runtime.graph.PointIndex(input.Point())
		if !inputOK || !indexed || pointIndex < 0 || pointIndex >= len(epoch.versions) {
			return carrier.Contribution{}, nil, false
		}
		cache.scratchEnvironmentToken = epoch.versions[pointIndex]
	} else {
		cache.scratchEnvironmentToken = 0
	}
	if len(cache.inputStates) != len(inputs) {
		return carrier.Contribution{}, nil, false
	}
	for index := range inputs {
		cache.inputStates[index] = inputs[index].State()
	}
	var base carrier.ContributionBase
	// BeginContribution is the single authority that intersects all input
	// supports with this sealed group premise. The executor cannot precompute
	// or substitute a support result at a parallel boundary.
	if producer.environment != nil {
		base, ok = epoch.work.BeginContribution(producer.plan, producer.outputScope, inputs, producer.premise, environment)
	} else {
		base, ok = epoch.work.BeginContribution(producer.plan, producer.outputScope, inputs, producer.premise)
	}
	if !ok {
		return carrier.Contribution{}, nil, false
	}
	within := base.State().Support()
	if !within.Valid() {
		return carrier.Contribution{}, nil, false
	}
	patches := cache.patches[:0]
	patchRows := cache.patchRows[:0]
	reads := cache.reads[:0]
	retained := within
	supportPrune := false
	activations := make([]equation.AcceptedMember, 0)
	live := true
	defer func() {
		if live {
			_ = epoch.work.AbortContribution(base, patches)
		}
	}()
	for _, member := range producer.members {
		if epoch.canceled() {
			return carrier.Contribution{}, nil, false
		}
		result := member.execute(epoch.work, base, cache.inputStates, within)
		if !result.valid {
			epoch.recordMemberFailure(SolveFailureReasonExecution, result.phase, producer.group.Output(), producer.group, member.member())
			return carrier.Contribution{}, nil, false
		}
		reads = append(reads, result.reads...)
		if result.hasSupport {
			if !result.retained.Valid() || !result.retained.Entails(within) {
				return carrier.Contribution{}, nil, false
			}
			if supportPrune {
				var valid bool
				retained, valid = support.IntersectWithCheckpoint(func() bool { return !epoch.canceled() }, retained, result.retained)
				if !valid {
					return carrier.Contribution{}, nil, false
				}
			} else {
				// The first retained support is already proven to be a subset of
				// within above, so intersecting within with it only rebuilds the
				// result in a disposable guard Work.
				retained = result.retained
			}
			supportPrune = true
		}
		if len(result.activations) != 0 {
			if !canonicalAcceptedActivations(result.activations) {
				epoch.recordGroupFailure(SolveFailureReasonActivationMerge, producer.group.Output(), producer.group)
				return carrier.Contribution{}, nil, false
			}
			activations = append(activations, result.activations...)
		}
		if result.wrote {
			slot, writes := member.outputSlot()
			if !writes {
				return carrier.Contribution{}, nil, false
			}
			patches = append(patches, result.patch)
			patchRows = append(patchRows, contributionPatch{slot: slot, patch: result.patch})
		}
	}
	if len(activations) != 0 {
		if !epoch.observeActivations(activations) {
			epoch.recordGroupFailure(SolveFailureReasonActivationMerge, producer.group.Output(), producer.group)
			return carrier.Contribution{}, nil, false
		}
	}
	sort.Slice(patchRows, func(left, right int) bool { return patchRows[left].slot < patchRows[right].slot })
	for index, row := range patchRows {
		if row.slot < 0 || index > 0 && patchRows[index-1].slot >= row.slot {
			return carrier.Contribution{}, nil, false
		}
	}
	patches = patches[:0]
	for _, row := range patchRows {
		patches = append(patches, row.patch)
	}
	var next carrier.Contribution
	if supportPrune {
		next, ok = epoch.work.FinishContributionWithSupport(base, patches, retained)
	} else {
		next, ok = epoch.work.FinishContribution(base, patches)
	}
	if !ok {
		return carrier.Contribution{}, nil, false
	}
	// Finish consumed the one-shot base and every owned Patch atomically. Only
	// now disarm AbortContribution; cancellation before or during Finish keeps
	// the defer armed and deterministically drops the still-owned transaction.
	live = false
	if epoch.canceled() {
		return carrier.Contribution{}, nil, false
	}
	cache.patches, cache.patchRows, cache.reads = patches, patchRows, reads
	return next, reads, true
}

func (epoch *executorEpoch) pointBase(point equation.Point, pointIndex int) (carrier.Contribution, bool) {
	if epoch == nil || epoch.runtime == nil || pointIndex < 0 || pointIndex >= len(epoch.runtime.pointScopes) || pointIndex >= len(epoch.runtime.pointInitials) || !epoch.runtime.pointScopes[pointIndex].Valid() {
		return carrier.Contribution{}, false
	}
	feasible, ok := support.FromGuard(epoch.runtime.carrier.Guards(), epoch.runtime.carrier.Guards().False())
	if !ok {
		return carrier.Contribution{}, false
	}
	init, disposition, initialized := point.Init()
	if !initialized || !init.Available() {
		return carrier.Contribution{}, false
	}
	if disposition == equation.InitPresent {
		feasible = epoch.runtime.pointInitials[pointIndex]
		if !feasible.Valid() {
			return carrier.Contribution{}, false
		}
	}
	state, ok := carrier.NewState(epoch.runtime.carrier, epoch.runtime.pointScopes[pointIndex], feasible)
	if !ok {
		return carrier.Contribution{}, false
	}
	return epoch.work.EmptyContribution(state)
}

func (epoch *executorEpoch) pointBottom(pointIndex int) (carrier.Contribution, bool) {
	if epoch == nil || epoch.runtime == nil || pointIndex < 0 || pointIndex >= len(epoch.runtime.pointScopes) || !epoch.runtime.pointScopes[pointIndex].Valid() {
		return carrier.Contribution{}, false
	}
	feasible, ok := support.FromGuard(epoch.runtime.carrier.Guards(), epoch.runtime.carrier.Guards().False())
	if !ok {
		return carrier.Contribution{}, false
	}
	state, ok := carrier.NewState(epoch.runtime.carrier, epoch.runtime.pointScopes[pointIndex], feasible)
	if !ok {
		return carrier.Contribution{}, false
	}
	return epoch.work.EmptyContribution(state)
}

func (epoch *executorEpoch) foldGroups(base carrier.Contribution, groups []int) (carrier.Contribution, bool) {
	if epoch == nil || epoch.work == nil || !epoch.work.OwnsAdmittedContribution(base) {
		return carrier.Contribution{}, false
	}
	result := base
	for _, groupIndex := range groups {
		if groupIndex < 0 || groupIndex >= len(epoch.producers) {
			return carrier.Contribution{}, false
		}
		var ok bool
		if !epoch.producers[groupIndex].hasValue {
			return carrier.Contribution{}, false
		}
		result, _, ok = epoch.work.MergeContribution(result, epoch.producers[groupIndex].candidate)
		if !ok || epoch.canceled() {
			return carrier.Contribution{}, false
		}
	}
	return result, true
}

func (epoch *executorEpoch) foldEnvironmentEdges(base carrier.Contribution, edges []int) (carrier.Contribution, bool) {
	if epoch == nil || epoch.runtime == nil || epoch.work == nil || !epoch.work.OwnsAdmittedContribution(base) {
		return carrier.Contribution{}, false
	}
	result := base
	for _, edgeIndex := range edges {
		var ok bool
		result, ok = epoch.foldEnvironmentEdge(result, edgeIndex)
		if !ok {
			return carrier.Contribution{}, false
		}
	}
	return result, true
}

func (epoch *executorEpoch) foldEnvironmentEdge(base carrier.Contribution, edgeIndex int) (carrier.Contribution, bool) {
	if epoch == nil || epoch.runtime == nil || epoch.work == nil || !epoch.work.OwnsAdmittedContribution(base) || edgeIndex < 0 || edgeIndex >= len(epoch.runtime.environments) {
		return carrier.Contribution{}, false
	}
	edge := epoch.runtime.environments[edgeIndex]
	if edge.source < 0 || edge.source >= len(epoch.points) || edge.target < 0 || edge.target >= len(epoch.points) || !edge.input.valid() {
		return carrier.Contribution{}, false
	}
	transported, ok := epoch.work.TransportContribution(epoch.points[edge.source], edge.input.pre, edge.input.plan, edge.input.post)
	if !ok || !epoch.work.OwnsAdmittedContribution(transported) || !transported.State().Scope().Same(base.State().Scope()) {
		return carrier.Contribution{}, false
	}
	result, _, ok := epoch.work.MergeContribution(base, transported)
	if !ok || epoch.canceled() {
		return carrier.Contribution{}, false
	}
	return result, true
}

func (epoch *executorEpoch) foldFactorEdges(base carrier.Contribution, edges []int) (carrier.Contribution, bool) {
	if epoch == nil || epoch.runtime == nil || epoch.work == nil || !epoch.work.OwnsAdmittedContribution(base) {
		return carrier.Contribution{}, false
	}
	result := base
	for _, edgeIndex := range edges {
		var ok bool
		result, ok = epoch.foldFactorEdge(result, edgeIndex)
		if !ok {
			return carrier.Contribution{}, false
		}
	}
	return result, true
}

func (epoch *executorEpoch) foldFactorEdge(base carrier.Contribution, edgeIndex int) (carrier.Contribution, bool) {
	if epoch == nil || epoch.runtime == nil || epoch.work == nil || !epoch.work.OwnsAdmittedContribution(base) || edgeIndex < 0 || edgeIndex >= len(epoch.runtime.factorEdges) {
		return carrier.Contribution{}, false
	}
	edge := epoch.runtime.factorEdges[edgeIndex]
	if edge.source < 0 || edge.source >= len(epoch.points) || edge.target < 0 || edge.target >= len(epoch.points) || !edge.input.valid() {
		return carrier.Contribution{}, false
	}
	transported, ok := epoch.work.TransportContribution(epoch.points[edge.source], edge.input.pre, edge.input.plan, edge.input.post)
	if !ok || !epoch.work.OwnsAdmittedContribution(transported) || !transported.State().Scope().Same(base.State().Scope()) {
		return carrier.Contribution{}, false
	}
	projected, ok := epoch.work.ProjectContribution(transported, edge.slot)
	if !ok || !epoch.work.OwnsAdmittedContribution(projected) || !projected.State().Scope().Same(base.State().Scope()) {
		return carrier.Contribution{}, false
	}
	result, _, ok := epoch.work.MergeContribution(base, projected)
	if !ok || epoch.canceled() {
		return carrier.Contribution{}, false
	}
	return result, true
}

func (epoch *executorEpoch) foldPoint(base carrier.Contribution, point equation.Point) (carrier.Contribution, bool) {
	if epoch == nil || epoch.runtime == nil || !point.Available() || !epoch.work.OwnsAdmittedContribution(base) {
		return carrier.Contribution{}, false
	}
	result := base
	pointIndex, indexed := epoch.runtime.graph.PointIndex(point)
	if !indexed || pointIndex < 0 || pointIndex >= len(epoch.runtime.environmentIncoming) || pointIndex >= len(epoch.runtime.factorIncoming) {
		return carrier.Contribution{}, false
	}
	for _, edgeIndex := range epoch.runtime.environmentIncoming[pointIndex] {
		var merged bool
		result, merged = epoch.foldEnvironmentEdge(result, edgeIndex)
		if !merged || epoch.canceled() {
			return carrier.Contribution{}, false
		}
	}
	for _, edgeIndex := range epoch.runtime.factorIncoming[pointIndex] {
		var merged bool
		result, merged = epoch.foldFactorEdge(result, edgeIndex)
		if !merged || epoch.canceled() {
			return carrier.Contribution{}, false
		}
	}
	for index := 0; index < epoch.runtime.graph.ProducerCount(point); index++ {
		group, ok := epoch.runtime.graph.ProducerAt(point, index)
		groupIndex, indexed := epoch.runtime.graph.GroupIndex(group)
		if !ok || !indexed || groupIndex < 0 || groupIndex >= len(epoch.producers) {
			return carrier.Contribution{}, false
		}
		if !epoch.producers[groupIndex].hasValue {
			return carrier.Contribution{}, false
		}
		var merged bool
		result, _, merged = epoch.work.MergeContribution(result, epoch.producers[groupIndex].candidate)
		if !merged || epoch.canceled() {
			return carrier.Contribution{}, false
		}
	}
	return result, true
}

// regionRHS keeps recurrence operands private until one selected/exact carrier
// transition publishes the head. E includes Init and external producers; B
// starts at bottom and contains every back producer, including mixed Groups.
func (epoch *executorEpoch) regionRHS(point equation.Point, pointIndex int, region runtimeRegion, current carrier.Contribution) (ingress, exact, selected carrier.Contribution, ok bool) {
	if epoch == nil || !region.active || !epoch.work.OwnsAdmittedContribution(current) {
		return carrier.Contribution{}, carrier.Contribution{}, carrier.Contribution{}, false
	}
	base, ok := epoch.pointBase(point, pointIndex)
	if !ok {
		return carrier.Contribution{}, carrier.Contribution{}, carrier.Contribution{}, false
	}
	ingress, ok = epoch.foldEnvironmentEdges(base, region.environmentExternal) // E = base \sqcup environment ingress
	if !ok {
		return carrier.Contribution{}, carrier.Contribution{}, carrier.Contribution{}, false
	}
	ingress, ok = epoch.foldFactorEdges(ingress, region.factorExternal) // E = ... \sqcup Factor-edge ingress
	if !ok {
		return carrier.Contribution{}, carrier.Contribution{}, carrier.Contribution{}, false
	}
	ingress, ok = epoch.foldGroups(ingress, region.external) // E = ... \sqcup external
	if !ok {
		return carrier.Contribution{}, carrier.Contribution{}, carrier.Contribution{}, false
	}
	exact, ok = epoch.foldEnvironmentEdges(ingress, region.environmentBack) // R = E \sqcup environment back
	if !ok {
		return carrier.Contribution{}, carrier.Contribution{}, carrier.Contribution{}, false
	}
	exact, ok = epoch.foldFactorEdges(exact, region.factorBack) // R = ... \sqcup Factor-edge back
	if !ok {
		return carrier.Contribution{}, carrier.Contribution{}, carrier.Contribution{}, false
	}
	exact, ok = epoch.foldGroups(exact, region.back) // R = E \sqcup B
	if !ok || epoch.canceled() {
		return carrier.Contribution{}, carrier.Contribution{}, carrier.Contribution{}, false
	}
	selected, ok = epoch.foldEnvironmentEdges(current, region.environmentBack) // P = X \sqcup environment back
	if !ok {
		return carrier.Contribution{}, carrier.Contribution{}, carrier.Contribution{}, false
	}
	selected, ok = epoch.foldFactorEdges(selected, region.factorBack) // P = ... \sqcup Factor-edge back
	if !ok {
		return carrier.Contribution{}, carrier.Contribution{}, carrier.Contribution{}, false
	}
	selected, ok = epoch.foldGroups(selected, region.back) // P = X \sqcup B
	if !ok || epoch.canceled() {
		return carrier.Contribution{}, carrier.Contribution{}, carrier.Contribution{}, false
	}
	return ingress, exact, selected, true
}

func (epoch *executorEpoch) regionInterfacesChanged(region int) bool {
	if epoch == nil || !epoch.activeRegion(region) || region >= len(epoch.regions) {
		return true
	}
	bound, state := epoch.runtime.regions[region], epoch.regions[region]
	if !state.hasExact {
		return false
	}
	if len(bound.faces) != len(state.interfaces) || len(bound.external) != len(state.ingress) || len(bound.environmentExternal) != len(state.environmentIngress) || len(bound.factorExternal) != len(state.factorIngress) {
		return true
	}
	for index, point := range bound.faces {
		if point < 0 || point >= len(epoch.versions) || state.interfaces[index] != epoch.versions[point] {
			return true
		}
	}
	for index, group := range bound.external {
		if group < 0 || group >= len(epoch.producers) || state.ingress[index] != epoch.producers[group].version {
			return true
		}
	}
	for index, edge := range bound.environmentExternal {
		if edge < 0 || edge >= len(epoch.runtime.environments) || state.environmentIngress[index] != epoch.environmentVersion(edge) {
			return true
		}
	}
	for index, edge := range bound.factorExternal {
		if edge < 0 || edge >= len(epoch.runtime.factorEdges) || state.factorIngress[index] != epoch.factorEdgeVersion(edge) {
			return true
		}
	}
	return false
}

// regionExactInputsChanged checks the disposable proof recorded with the
// episode.exact head RHS.  The ordered producer and source-point versions are
// the complete semantic input list for E⊔B: queue readiness alone is not
// evidence that the stored exact carrier still describes the live recurrence.
// Faces and external ingress remain part of the interface proof because a
// changed enclosing point must restart the local episode before narrowing.
func (epoch *executorEpoch) regionExactInputsChanged(region int) bool {
	if epoch == nil || !epoch.activeRegion(region) || region >= len(epoch.regions) {
		return true
	}
	state := epoch.regions[region]
	if !state.hasExact || epoch.regionInterfacesChanged(region) {
		return true
	}
	bound := epoch.runtime.regions[region]
	if len(bound.back) != len(state.backIngress) || len(bound.environmentBack) != len(state.environmentBackIngress) || len(bound.factorBack) != len(state.factorBackIngress) {
		return true
	}
	for index, group := range bound.back {
		if group < 0 || group >= len(epoch.producers) || state.backIngress[index] != epoch.producers[group].version {
			return true
		}
	}
	for index, edge := range bound.environmentBack {
		if edge < 0 || edge >= len(epoch.runtime.environments) || state.environmentBackIngress[index] != epoch.environmentVersion(edge) {
			return true
		}
	}
	for index, edge := range bound.factorBack {
		if edge < 0 || edge >= len(epoch.runtime.factorEdges) || state.factorBackIngress[index] != epoch.factorEdgeVersion(edge) {
			return true
		}
	}
	return false
}

func (epoch *executorEpoch) rememberRegionInterfaces(region int) bool {
	if epoch == nil || !epoch.activeRegion(region) || region >= len(epoch.regions) {
		return false
	}
	bound, state := epoch.runtime.regions[region], &epoch.regions[region]
	if len(bound.faces) != len(state.interfaces) || len(bound.external) != len(state.ingress) || len(bound.back) != len(state.backIngress) || len(bound.environmentExternal) != len(state.environmentIngress) || len(bound.environmentBack) != len(state.environmentBackIngress) || len(bound.factorExternal) != len(state.factorIngress) || len(bound.factorBack) != len(state.factorBackIngress) {
		return false
	}
	for index, point := range bound.faces {
		if point < 0 || point >= len(epoch.versions) {
			return false
		}
		state.interfaces[index] = epoch.versions[point]
	}
	for index, group := range bound.external {
		if group < 0 || group >= len(epoch.producers) {
			return false
		}
		state.ingress[index] = epoch.producers[group].version
	}
	for index, group := range bound.back {
		if group < 0 || group >= len(epoch.producers) {
			return false
		}
		state.backIngress[index] = epoch.producers[group].version
	}
	for index, edge := range bound.environmentExternal {
		if edge < 0 || edge >= len(epoch.runtime.environments) {
			return false
		}
		state.environmentIngress[index] = epoch.environmentVersion(edge)
	}
	for index, edge := range bound.environmentBack {
		if edge < 0 || edge >= len(epoch.runtime.environments) {
			return false
		}
		state.environmentBackIngress[index] = epoch.environmentVersion(edge)
	}
	for index, edge := range bound.factorExternal {
		if edge < 0 || edge >= len(epoch.runtime.factorEdges) {
			return false
		}
		state.factorIngress[index] = epoch.factorEdgeVersion(edge)
	}
	for index, edge := range bound.factorBack {
		if edge < 0 || edge >= len(epoch.runtime.factorEdges) {
			return false
		}
		state.factorBackIngress[index] = epoch.factorEdgeVersion(edge)
	}
	return true
}

func (epoch *executorEpoch) environmentVersion(edge int) uint64 {
	if epoch == nil || epoch.runtime == nil || edge < 0 || edge >= len(epoch.runtime.environments) {
		return 0
	}
	return epoch.versions[epoch.runtime.environments[edge].source]
}

func (epoch *executorEpoch) factorEdgeVersion(edge int) uint64 {
	if epoch == nil || epoch.runtime == nil || edge < 0 || edge >= len(epoch.runtime.factorEdges) {
		return 0
	}
	return epoch.versions[epoch.runtime.factorEdges[edge].source]
}

// regionSubtree materializes one active recurrence subtree in executor scratch.
// The child rows are an assembly-time cache of immutable Region.Parent
// topology; they do not establish recurrence membership or semantic edges.
func (epoch *executorEpoch) regionSubtree(root int) ([]int, bool) {
	if epoch == nil || !epoch.activeRegion(root) || len(epoch.runtime.regionChildren) != len(epoch.runtime.regions) {
		return nil, false
	}
	stack := epoch.regionScratch[:0]
	stack = append(stack, root)
	for index := 0; index < len(stack); index++ {
		region := stack[index]
		if !epoch.activeRegion(region) || len(stack) > len(epoch.runtime.regions) {
			return nil, false
		}
		for _, child := range epoch.runtime.regionChildren[region] {
			if child < 0 || child >= len(epoch.runtime.regions) || epoch.runtime.regions[child].parent != region {
				return nil, false
			}
			stack = append(stack, child)
		}
	}
	epoch.regionScratch = stack
	return stack, true
}

// invalidatePostfixAncestors keeps a recurrence-head proof tied to the Point
// versions it summarizes. A Point publication can change the exact RHS of
// every enclosing head even before the head itself publishes again.
func (epoch *executorEpoch) invalidatePostfixAncestors(point int) bool {
	if epoch == nil || epoch.runtime == nil || point < 0 || point >= len(epoch.runtime.pointRegion) {
		return false
	}
	for region := epoch.runtime.pointRegion[point]; region != schedule.NoRegion; region = epoch.runtime.regions[region].parent {
		if !epoch.activeRegion(region) || !epoch.markPostfixDirty(epoch.runtime.regions[region].head) {
			return false
		}
	}
	return true
}

// restartRegion begins a fresh exact episode for one region and all nested
// regions.  Phase is owned by each region episode, so an independent child
// restart never changes an enclosing region from Narrow to Ascent while its
// narrowed head is still retained. Every selected Group rooted inside is made
// dirty before any later head widening can observe an old candidate.
func (epoch *executorEpoch) restartRegion(region int) bool {
	if epoch == nil || !epoch.activeRegion(region) || len(epoch.producers) != len(epoch.runtime.producers) {
		return false
	}
	subtree, subtreeOK := epoch.regionSubtree(region)
	if !subtreeOK {
		return false
	}
	for _, index := range subtree {
		// Every restarted descendant belongs to the new ascent episode. This
		// establishes parent-Ascent => descendant-Ascent before any local Point
		// or candidate can be queued.
		epoch.regions[index].phase = phaseAscent
		epoch.regions[index].exact = carrier.Contribution{}
		epoch.regions[index].hasExact = false
		epoch.regions[index].invalid = true
		clear(epoch.regions[index].interfaces)
		clear(epoch.regions[index].ingress)
		clear(epoch.regions[index].backIngress)
		clear(epoch.regions[index].environmentIngress)
		clear(epoch.regions[index].environmentBackIngress)
		clear(epoch.regions[index].factorIngress)
		clear(epoch.regions[index].factorBackIngress)
		clear(epoch.regions[index].snapshot)
	}
	// A fresh episode may not use an old local Point as a seed.  The Region's
	// event-point interval includes every nested descendant exactly once, so
	// this root row covers the full restarted subtree.  Reset through the sole
	// publication cut: observers must see the actual old-to-base delta rather
	// than a later base-to-recomputed delta (or no delta when it stays at base).
	bound := epoch.runtime.regions[region]
	for _, pointIndex := range bound.points {
		if pointIndex < 0 || pointIndex >= len(epoch.points) || !epoch.work.OwnsAdmittedContribution(epoch.points[pointIndex]) {
			return false
		}
		point, pointOK := epoch.runtime.graph.PointAt(schedule.Node(pointIndex))
		base, baseOK := epoch.pointBase(point, pointIndex)
		if !pointOK || !baseOK {
			return false
		}
		current := epoch.points[pointIndex]
		reset, changes, resetOK := epoch.work.ReplaceContribution(current, base)
		if !resetOK || epoch.canceled() {
			return false
		}
		if _, publishedOK := epoch.publish(pointIndex, current, reset, changes); !publishedOK || epoch.canceled() {
			return false
		}
		if !epoch.markPostfixDirty(pointIndex) {
			return false
		}
	}
	for _, pointIndex := range bound.points {
		point, pointOK := epoch.runtime.graph.PointAt(schedule.Node(pointIndex))
		if !pointOK {
			return false
		}
		for producerIndex := 0; producerIndex < epoch.runtime.graph.ProducerCount(point); producerIndex++ {
			group, groupOK := epoch.runtime.graph.ProducerAt(point, producerIndex)
			groupIndex, indexed := epoch.runtime.graph.GroupIndex(group)
			if !groupOK || !indexed || groupIndex < 0 || groupIndex >= len(epoch.producers) || epoch.runtime.producers[groupIndex].group.Output() != point {
				return false
			}
			cache := &epoch.producers[groupIndex]
			if cache.generation == 0 {
				continue
			}
			cache.candidate = carrier.Contribution{}
			cache.hasValue = false
			cache.hasCandidateTokens = false
			cache.candidateEnvironmentToken = 0
			cache.scratchEnvironmentToken = 0
			clear(cache.candidateTokens)
			clear(cache.scratchTokens)
			cache.applied = 0
			cache.patches = cache.patches[:0]
			cache.patchRows = cache.patchRows[:0]
			// Demand owns the live reverse relation independently of this cache.
			// Retract it before dropping cached observations so no pre-reset Product
			// read can wake the fresh region episode before its next refold.
			if epoch.demand == nil || !epoch.demand.Replace(groupIndex, nil) {
				return false
			}
			cache.reads = cache.reads[:0]
			if !epoch.markDirty(groupIndex) {
				return false
			}
		}
		if !epoch.markStructuralPoint(point) {
			return false
		}
	}
	return true
}

func (epoch *executorEpoch) regionCandidatesSettled(region int) bool {
	if epoch == nil || !epoch.activeRegion(region) {
		return false
	}
	for _, pointIndex := range epoch.runtime.regions[region].points {
		point, pointOK := epoch.runtime.graph.PointAt(schedule.Node(pointIndex))
		if !pointOK {
			return false
		}
		for producerIndex := 0; producerIndex < epoch.runtime.graph.ProducerCount(point); producerIndex++ {
			group, groupOK := epoch.runtime.graph.ProducerAt(point, producerIndex)
			groupIndex, indexed := epoch.runtime.graph.GroupIndex(group)
			if !groupOK || !indexed || groupIndex < 0 || groupIndex >= len(epoch.producers) || epoch.runtime.producers[groupIndex].group.Output() != point {
				return false
			}
			cache := epoch.producers[groupIndex]
			if cache.generation != 0 && cache.applied != cache.generation {
				return false
			}
		}
	}
	return true
}

func (epoch *executorEpoch) publish(point int, current, next carrier.Contribution, changes carrier.ChangeSet) (bool, bool) {
	if epoch == nil || epoch.canceled() || point < 0 || point >= len(epoch.points) || !epoch.work.OwnsAdmittedContribution(current) || !epoch.work.OwnsAdmittedContribution(next) || !epoch.runtime.carrier.OwnsChangeSet(changes) {
		return false, false
	}
	coverageChanges, coverageOK := epoch.work.CoverageChanges(current, next)
	if !coverageOK {
		return false, false
	}
	changed := !epoch.work.EqualContribution(current, next)
	epoch.points[point] = next
	if changed {
		epoch.versions[point]++
		if epoch.versions[point] == 0 {
			return false, false
		}
		if !epoch.markPostfixDirty(point) || !epoch.invalidatePostfixAncestors(point) {
			return false, false
		}
		sourcePoint, sourceOK := epoch.runtime.graph.PointAt(schedule.Node(point))
		if !sourceOK {
			return false, false
		}
		if !epoch.markStructuralSuccessors(sourcePoint) {
			return false, false
		}
	}
	wakes, ok := epoch.demand.RoutePoint(point, changes)
	if !ok {
		return false, false
	}
	for _, wake := range wakes {
		if !epoch.markDirty(wake.Group) {
			return false, false
		}
	}
	coverageWakes, ok := epoch.demand.RouteCoverage(point, coverageChanges)
	if !ok {
		return false, false
	}
	for _, wake := range coverageWakes {
		duplicate := false
		for _, semantic := range wakes {
			if semantic.Group == wake.Group {
				duplicate = true
				break
			}
		}
		if !duplicate && !epoch.markDirty(wake.Group) {
			return false, false
		}
	}
	return changed, true
}

// refreshPoint performs the only candidate replacement and sole Point
// publication. A region is admitted only for its head; nonheads exact-replace
// their complete RHS even while enclosed by the same WTO region.
func (epoch *executorEpoch) refreshPoint(point equation.Point, pointIndex, regionIndex int) (bool, bool) {
	if epoch == nil || epoch.canceled() || !point.Available() || pointIndex < 0 || pointIndex >= len(epoch.points) || pointIndex >= len(epoch.runtime.activePoints) || !epoch.runtime.activePoints[pointIndex] {
		return false, false
	}
	structuralChanged := pointIndex < len(epoch.structuralDirty) && epoch.structuralDirty[pointIndex]
	candidateChanged := false
	for index := 0; index < epoch.runtime.graph.ProducerCount(point); index++ {
		group, groupOK := epoch.runtime.graph.ProducerAt(point, index)
		groupIndex, indexed := epoch.runtime.graph.GroupIndex(group)
		if !groupOK || !indexed || groupIndex < 0 || groupIndex >= len(epoch.producers) {
			return false, false
		}
		producer := epoch.runtime.producers[groupIndex]
		state := &epoch.producers[groupIndex]
		if producer.group.Output() != point || state.applied == state.generation {
			continue
		}
		next, reads, ok := epoch.evaluate(producer, state)
		if !ok || epoch.canceled() || !epoch.candidateTokens(producer, state.scratchTokens) {
			if !ok {
				epoch.recordGroupFailure(SolveFailureReasonExecution, point, producer.group)
			}
			return false, false
		}
		if state.hasValue && (!epoch.work.LessOrEqUnder(state.candidate.State(), next.State()) || state.candidateEnvironmentToken != state.scratchEnvironmentToken) {
			// A wake generation is not semantic evidence. A candidate decrease or
			// incomparability is lawful only while an unchanged narrow episode is
			// propagating its smaller exact head around an internal edge. During
			// ascent, unchanged interfaces imply that every changed input belongs to
			// the same ascending Kleene chain; a monotone Rule cannot then decrease.
			// Fail that Rule law closed instead of restarting the identical episode
			// forever. A genuinely changed external interface still begins a fresh
			// episode below.
			if !state.hasCandidateTokens || sameCandidateTokens(state.candidateTokens, state.scratchTokens) {
				return false, false
			}
			region := epoch.runtime.pointRegion[pointIndex]
			if region == schedule.NoRegion || !epoch.activeRegion(region) {
				return false, false
			}
			phase := epoch.regions[region].phase
			if phase != phaseAscent && phase != phaseNarrow {
				return false, false
			}
			if epoch.regionInterfacesChanged(region) {
				if !epoch.restartRegion(region) {
					return false, false
				}
				return false, true
			}
			if phase != phaseNarrow {
				return false, false
			}
			// An unchanged narrow interface proves only where the wake came
			// from. It does not turn an incomparable Rule result into a
			// descent. Admit exactly next <= old; every other local result
			// fails closed instead of being published as an exact candidate.
			if !epoch.work.LessOrEqUnder(next.State(), state.candidate.State()) {
				return false, false
			}
		}
		if epoch.canceled() || !epoch.demand.Replace(groupIndex, reads) {
			return false, false
		}
		changed := !state.hasValue || !epoch.work.EqualContribution(state.candidate, next)
		state.candidate, state.hasValue, state.applied = next, true, state.generation
		copy(state.candidateTokens, state.scratchTokens)
		state.hasCandidateTokens = true
		state.candidateEnvironmentToken = state.scratchEnvironmentToken
		if changed {
			state.version++
			if state.version == 0 {
				return false, false
			}
		}
		if epoch.canceled() {
			return false, false
		}
		candidateChanged = candidateChanged || changed
	}
	if regionIndex == schedule.NoRegion && !candidateChanged && !structuralChanged {
		return false, epoch.settlePostfix(pointIndex)
	}
	if regionIndex == schedule.NoRegion {
		current := epoch.points[pointIndex]
		if !epoch.work.OwnsAdmittedContribution(current) {
			return false, false
		}
		base, ok := epoch.pointBase(point, pointIndex)
		if !ok {
			return false, false
		}
		rhs, ok := epoch.foldPoint(base, point)
		if !ok {
			return false, false
		}
		published, changes, publishedOK := epoch.work.ReplaceContribution(current, rhs)
		if !publishedOK || epoch.canceled() {
			return false, false
		}
		changed, publishedOK := epoch.publish(pointIndex, current, published, changes)
		if !publishedOK || !epoch.settlePostfix(pointIndex) {
			return false, false
		}
		epoch.structuralDirty[pointIndex] = false
		return changed, true
	}
	if !epoch.activeRegion(regionIndex) || epoch.runtime.regions[regionIndex].head != pointIndex {
		return false, false
	}
	episode := &epoch.regions[regionIndex]
	phase := episode.phase
	if phase != phaseAscent && phase != phaseNarrow {
		return false, false
	}
	if epoch.regionInterfacesChanged(regionIndex) {
		if !epoch.restartRegion(regionIndex) {
			return false, false
		}
		return false, true
	}
	if episode.invalid {
		if !epoch.regionCandidatesSettled(regionIndex) {
			return epoch.enqueuePoint(pointIndex), true
		}
		episode.invalid = false
	}
	current := epoch.points[pointIndex]
	if !epoch.work.OwnsAdmittedContribution(current) {
		return false, false
	}
	region := epoch.runtime.regions[regionIndex]
	ingress, exact, selected, exactOK := epoch.regionRHS(point, pointIndex, region, current)
	if !exactOK || epoch.canceled() {
		return false, false
	}
	if phase == phaseAscent && episode.hasExact && !epoch.work.LessOrEqUnder(ingress.State(), current.State()) {
		// New Init/external meaning begins a fresh episode before an inherited
		// widening step can observe a stale current head.
		if !epoch.restartRegion(regionIndex) {
			return false, false
		}
		return false, true
	}
	if phase == phaseNarrow {
		if !episode.hasExact {
			return false, false
		}
		// A descent may follow a smaller exact RHS.  A larger or incomparable
		// one, however, invalidates every narrowed history even if it still fits
		// below the current widened head.  Restart clears the complete region and
		// its descendants from Init before any new ascent publication.
		if !epoch.work.EqualContribution(episode.exact, exact) && !epoch.work.LessOrEqUnder(exact.State(), episode.exact.State()) {
			if !epoch.restartRegion(regionIndex) {
				return false, false
			}
			return false, true
		}
		if !epoch.work.LessOrEqUnder(exact.State(), current.State()) {
			if !epoch.restartRegion(regionIndex) {
				return false, false
			}
			return false, true
		}
	}
	var published carrier.Contribution
	var changes carrier.ChangeSet
	var publishedOK bool
	if phase == phaseAscent && !episode.hasExact {
		published, changes, publishedOK = epoch.work.ReplaceContribution(current, exact)
	} else if phase == phaseAscent && !epoch.work.LessOrEqUnder(episode.exact.State(), exact.State()) {
		// Interfaces were checked above. With them unchanged, an exact ascent
		// decrease is a broken monotonicity law, not a new episode. Retrying from
		// Init would reproduce the same RHS and allocate forever.
		return false, false
	} else if phase == phaseAscent {
		published, changes, publishedOK = epoch.work.MergeSelectedContribution(carrier.Widen, current, selected, exact, region.widen)
	} else {
		published, changes, publishedOK = epoch.work.MergeSelectedContribution(carrier.Narrow, current, exact, exact, region.narrow)
	}
	if !publishedOK || epoch.canceled() {
		return false, false
	}
	episode.exact, episode.hasExact = exact, true
	if epoch.canceled() || !epoch.rememberRegionInterfaces(regionIndex) {
		return false, false
	}
	changed, publishedOK := epoch.publish(pointIndex, current, published, changes)
	if !publishedOK || epoch.canceled() {
		return false, false
	}
	epoch.structuralDirty[pointIndex] = false
	if phase == phaseNarrow && changed && !epoch.enqueuePoint(pointIndex) {
		return false, false
	}
	return changed, true
}

func (epoch *executorEpoch) snapshotRegion(region int) bool {
	if epoch == nil || !epoch.activeRegion(region) || region >= len(epoch.regions) {
		return false
	}
	bound, state := epoch.runtime.regions[region], &epoch.regions[region]
	if len(bound.points) != len(state.snapshot) {
		return false
	}
	for index, point := range bound.points {
		if point < 0 || point >= len(epoch.versions) {
			return false
		}
		state.snapshot[index] = epoch.versions[point]
	}
	return true
}

func (epoch *executorEpoch) regionSnapshotChanged(region int) bool {
	if epoch == nil || !epoch.activeRegion(region) || region >= len(epoch.regions) {
		return true
	}
	bound, state := epoch.runtime.regions[region], epoch.regions[region]
	if len(bound.points) != len(state.snapshot) {
		return true
	}
	for index, point := range bound.points {
		if point < 0 || point >= len(epoch.versions) || state.snapshot[index] != epoch.versions[point] {
			return true
		}
	}
	return false
}

// regionPostfixed validates one already-drained region before EventExit. It
// never trusts an empty queue: every local candidate and exact-input version
// must still agree with the current phase. Once those disposable versions
// match, episode.exact is the already-computed E⊔B carrier; do not rebuild
// ingress, back edges, or factor joins merely to prove the same postfix.
func (epoch *executorEpoch) regionPostfixed(regionIndex int) (bool, bool) {
	if epoch == nil || epoch.canceled() || !epoch.activeRegion(regionIndex) {
		return false, false
	}
	region, episode := epoch.runtime.regions[regionIndex], &epoch.regions[regionIndex]
	phase := episode.phase
	if phase != phaseAscent && phase != phaseNarrow {
		return false, false
	}
	if episode.invalid || !epoch.regionCandidatesSettled(regionIndex) {
		return false, epoch.enqueuePoint(region.head)
	}
	if !episode.hasExact || epoch.regionExactInputsChanged(regionIndex) {
		return false, epoch.enqueuePoint(region.head)
	}
	_, headOK := epoch.runtime.graph.PointAt(schedule.Node(region.head))
	current := epoch.points[region.head]
	if !headOK || !epoch.work.OwnsAdmittedContribution(current) || !epoch.work.OwnsAdmittedContribution(episode.exact) {
		return false, false
	}
	exact := episode.exact
	if phase == phaseNarrow {
		if !epoch.work.LessOrEqUnder(exact.State(), current.State()) {
			if !epoch.restartRegion(regionIndex) {
				return false, false
			}
			return false, true
		}
	}
	if !epoch.work.LessOrEqUnder(exact.State(), current.State()) {
		return false, epoch.enqueuePoint(region.head)
	}
	return true, epoch.settlePostfix(region.head)
}

// demandedPostfix discharges only Point proofs invalidated since the last
// seal. An empty executor queue is not evidence: each affected row still
// checks the producer generations which justify its candidate, and an
// affected recurrence head uses its episode/interface proof before the row is
// cleared.
func (epoch *executorEpoch) demandedPostfix() (bool, bool) {
	if epoch == nil || epoch.canceled() || epoch.runtime == nil || epoch.runtime.points == nil {
		return false, false
	}
	for {
		pointIndex, pending := epoch.postfixPoint()
		if !pending {
			return !epoch.canceled(), !epoch.canceled()
		}
		if pointIndex < 0 || pointIndex >= len(epoch.points) || pointIndex >= len(epoch.runtime.activePoints) || !epoch.runtime.activePoints[pointIndex] {
			return false, false
		}
		point, pointOK := epoch.runtime.graph.PointAt(schedule.Node(pointIndex))
		if !pointOK {
			return false, false
		}
		for producerIndex := 0; producerIndex < epoch.runtime.graph.ProducerCount(point); producerIndex++ {
			group, groupOK := epoch.runtime.graph.ProducerAt(point, producerIndex)
			groupIndex, groupIndexed := epoch.runtime.graph.GroupIndex(group)
			if !groupOK || !groupIndexed || groupIndex < 0 || groupIndex >= len(epoch.producers) {
				return false, false
			}
			cache := epoch.producers[groupIndex]
			if cache.generation != 0 && cache.applied != cache.generation {
				return false, epoch.enqueuePoint(pointIndex)
			}
		}
		region := epoch.runtime.pointRegion[pointIndex]
		if region != schedule.NoRegion {
			if !epoch.activeRegion(region) {
				return false, false
			}
		}
		if region != schedule.NoRegion && epoch.runtime.regions[region].head == pointIndex {
			settled, valid := epoch.regionPostfixed(region)
			if !valid || !settled {
				return false, valid
			}
			continue
		}
		current := epoch.points[pointIndex]
		base, baseOK := epoch.pointBase(point, pointIndex)
		rhs, rhsOK := epoch.foldPoint(base, point)
		if !baseOK || !rhsOK || !epoch.work.OwnsAdmittedContribution(current) {
			return false, false
		}
		if !epoch.work.EqualContribution(rhs, current) {
			return false, epoch.enqueuePoint(pointIndex)
		}
		if !epoch.provePostfix(pointIndex) {
			return false, false
		}
	}
}

// advanceNarrow moves only currently-ascent recurrence episodes into their
// narrow phase.  The root-to-leaf traversal preserves the ownership invariant
// parent-Ascent => descendant-Ascent at every observable solver boundary.
// A child may be re-ascending beneath a narrowed parent after a localized
// restart; that parent remains narrow until the changed child result reaches
// its own exact-RHS check and causes its own reset.
func (epoch *executorEpoch) advanceNarrow() (advanced, ok bool) {
	if epoch == nil || epoch.runtime == nil || len(epoch.runtime.regions) != len(epoch.regions) || len(epoch.runtime.regionChildren) != len(epoch.runtime.regions) {
		return false, false
	}
	stack := epoch.regionScratch[:0]
	for index, region := range epoch.runtime.regions {
		if !epoch.activeRegion(index) {
			continue
		}
		if region.parent == schedule.NoRegion {
			stack = append(stack, index)
			continue
		}
		if region.parent < 0 || region.parent >= len(epoch.runtime.regions) || !epoch.activeRegion(region.parent) {
			return false, false
		}
	}
	for next := 0; next < len(stack); next++ {
		index := stack[next]
		if !epoch.activeRegion(index) {
			return false, false
		}
		region, episode := epoch.runtime.regions[index], &epoch.regions[index]
		switch episode.phase {
		case phaseAscent:
			if !episode.hasExact {
				return false, false
			}
			episode.phase = phaseNarrow
			if !epoch.markPostfixDirty(region.head) || !epoch.enqueuePoint(region.head) {
				return false, false
			}
			advanced = true
		case phaseNarrow:
			// The parent was already narrowed before this local episode. Its
			// child may still need a new narrow pass, but no parent history is
			// rewritten here.
		default:
			return false, false
		}
		for _, child := range epoch.runtime.regionChildren[index] {
			if child < 0 || child >= len(epoch.runtime.regions) || !epoch.activeRegion(child) || epoch.runtime.regions[child].parent != index {
				return false, false
			}
			stack = append(stack, child)
		}
	}
	epoch.regionScratch = stack
	return advanced, true
}

func (epoch *executorEpoch) allRegionsNarrow() bool {
	if epoch == nil || epoch.runtime == nil || len(epoch.runtime.regions) != len(epoch.regions) {
		return false
	}
	for index := range epoch.runtime.regions {
		if !epoch.activeRegion(index) {
			continue
		}
		if epoch.regions[index].phase != phaseNarrow {
			return false
		}
	}
	return true
}

func (epoch *executorEpoch) visitPoints() (visited bool, ok bool) {
	if epoch == nil || epoch.canceled() || epoch.runtime == nil || epoch.runtime.points == nil {
		return false, false
	}
	frames := epoch.frames[:0]
	defer func() { epoch.frames = frames[:0] }()
	for index := 0; index < epoch.runtime.points.EventCount(); index++ {
		if epoch.canceled() {
			return false, false
		}
		event, _, eventOK := epoch.runtime.points.EventAt(index)
		if !eventOK {
			return false, false
		}
		switch event.Kind {
		case schedule.EventEnter:
			region, regionOK := epoch.runtime.graph.Schedule().RegionAt(event.Region)
			parent := schedule.NoRegion
			if len(frames) != 0 {
				parent = frames[len(frames)-1].region
			}
			if !regionOK || !epoch.activeRegion(event.Region) || region.Head != event.Node || region.Parent != parent {
				return false, false
			}
			if !epoch.snapshotRegion(event.Region) {
				return false, false
			}
			frames = append(frames, pointWTOFrame{region: event.Region})
		case schedule.EventExit:
			if len(frames) == 0 || frames[len(frames)-1].region != event.Region {
				return false, false
			}
			region, regionOK := epoch.runtime.graph.Schedule().RegionAt(event.Region)
			if !regionOK || !epoch.activeRegion(event.Region) || region.Head != event.Node {
				return false, false
			}
			settled, valid := epoch.regionPostfixed(event.Region)
			if !valid {
				return false, false
			}
			if !settled {
				// Keep the child logically active. Its queued head contributes to
				// the enclosing nested count, so the parent cannot run ahead.
				return visited, true
			}
			changed := epoch.regionSnapshotChanged(event.Region)
			frames = frames[:len(frames)-1]
			if changed && len(frames) != 0 {
				parent := frames[len(frames)-1].region
				if !epoch.activeRegion(parent) || !epoch.enqueuePoint(epoch.runtime.regions[parent].head) {
					return false, false
				}
			}
		case schedule.EventNode:
			if (len(frames) == 0 && event.Region != schedule.NoRegion) || (len(frames) != 0 && event.Region != frames[len(frames)-1].region) {
				return false, false
			}
			point, pointOK := epoch.runtime.graph.PointAt(event.Node)
			pointIndex, indexed := epoch.runtime.graph.PointIndex(point)
			if !pointOK || !indexed || pointIndex < 0 || pointIndex >= len(epoch.runtime.activePoints) || !epoch.runtime.activePoints[pointIndex] {
				return false, false
			}
			if event.Region != schedule.NoRegion {
				if !epoch.activeRegion(event.Region) {
					return false, false
				}
				if epoch.runtime.regions[event.Region].head == pointIndex && epoch.nested[event.Region] != 0 {
					// The child frame owns progress first. Leaving this Point queued
					// makes the next iterative WTO pass revisit the parent head only
					// after all nested readiness has drained.
					continue
				}
			}
			if !epoch.takePoint(pointIndex) {
				continue
			}
			visited = true
			headRegion := schedule.NoRegion
			if event.Region != schedule.NoRegion {
				if !epoch.activeRegion(event.Region) {
					return false, false
				}
				candidate := epoch.runtime.regions[event.Region]
				if candidate.head == pointIndex {
					headRegion = event.Region
				}
			}
			if _, pointOK := epoch.refreshPoint(point, pointIndex, headRegion); !pointOK {
				epoch.recordPointFailure(SolveFailureReasonExecution, point)
				return false, false
			}
		default:
			return false, false
		}
	}
	return visited, len(frames) == 0
}

func (epoch *executorEpoch) run() bool {
	for epoch != nil && !epoch.canceled() {
		for epoch.queue.pending() {
			visited, ok := epoch.visitPoints()
			if !ok || !visited || epoch.canceled() {
				return false
			}
		}
		postfixed, ok := epoch.demandedPostfix()
		if !ok || epoch.canceled() {
			return false
		}
		if !postfixed {
			if !epoch.queue.pending() {
				return false
			}
			continue
		}
		if epoch.allRegionsNarrow() {
			return true
		}
		advanced, advancedOK := epoch.advanceNarrow()
		if !advancedOK || !advanced {
			return false
		}
	}
	return false
}

// completedState returns the runtime's installed immutable result only while
// the caller owns solver.mu. The retained work lease and State's existing
// completion authority bind it to this exact Solver revision.
func (solver *Solver) completedState(runtime *solverRuntime) *State {
	if solver == nil || runtime == nil || solver.runtime != runtime {
		return nil
	}
	state := runtime.completed
	if state == nil || runtime.retained == nil || !runtime.retained.Live() {
		return nil
	}
	if state.owner != solver || state.completion == nil || state.completion.solver != solver || state.completion.serial == 0 || state.completion.serial != solver.completion || state.completion.revision != solver.revision {
		return nil
	}
	return state
}

// publishCompleted is the one terminal publication cut.  Its inputs have
// already passed every fallible operation while epoch is Running: query
// materialization, retention of the new root arena, and eviction of any prior
// lease.  Once complete wins, these assignments are deliberately infallible;
// cancellation after that cut is non-operative.
func (solver *Solver) publishCompleted(epoch *executorEpoch, runtime *solverRuntime, state *State, completion uint64, retained *carrier.RetainedWork) bool {
	if solver == nil || epoch == nil || runtime == nil || state == nil || retained == nil || solver.runtime != runtime || state.owner != solver || state.completion == nil || state.completion.solver != solver || state.completion.serial != completion || state.completion.revision != solver.revision || !epoch.complete() {
		return false
	}
	runtime.retained = retained
	solver.completion = completion
	runtime.completed = state
	return true
}

// Solve executes runtime revisions iteratively. A newly admitted activation
// aborts its open Group and all epoch-local state, then the same compiler body
// builds a fresh runtime from Init for the merged relation set.
func (solver *Solver) Solve(ctx context.Context) (state *State, status SolveStatus) {
	return solver.solve(ctx, nil)
}

// SolveWithReport uses the same solve implementation as Solve and returns a
// detached first-failure certificate only when that call is incomplete. The
// report is call-local; the Solver never retains it.
func (solver *Solver) SolveWithReport(ctx context.Context) (state *State, status SolveStatus, report SolveReport) {
	state, status = solver.solve(ctx, &report)
	return state, status, report
}

// solve is the one execution route. report is nil for ordinary Solve, keeping
// its successful and failure paths free of diagnostic allocation.
func (solver *Solver) solve(ctx context.Context, report *SolveReport) (state *State, status SolveStatus) {
	// A callback can panic from anywhere beneath epoch.run or query
	// materialization. Keep both ownership forms reachable by recovery: Retain
	// moves Work ownership into prepared before the epoch becomes terminal.
	var current *executorEpoch
	var prepared *carrier.RetainedWork
	defer func() {
		if recover() != nil {
			if prepared != nil {
				prepared.Close()
				prepared = nil
			}
			if current != nil {
				current.incomplete()
				current.discard()
			}
			state, status = nil, SolvePanicked
		}
		if report == nil {
			return
		}
		if status == SolveIncomplete {
			if !report.Available() {
				report.record(SolveFailureReasonExecution, SolveFailurePhaseNone, SemanticKey{}, SemanticKey{}, SemanticKey{}, SemanticKey{})
			}
			return
		}
		// Canceled, Complete, and Panicked calls do not publish an incomplete
		// certificate, even if an earlier internal branch recorded a candidate
		// before the terminal status changed.
		*report = SolveReport{}
	}()
	if solver == nil || ctx == nil || ctx.Err() != nil {
		return nil, SolveCanceled
	}
	solver.mu.Lock()
	defer solver.mu.Unlock()
	for {
		runtime := solver.runtime
		if runtime == nil {
			return nil, SolveCanceled
		}
		if state = solver.completedState(runtime); state != nil {
			return state, SolveComplete
		}
		epoch, ok := newRuntimeEpoch(runtime, solver.accepted, ctx)
		if !ok {
			if ctx.Err() != nil {
				return nil, SolveCanceled
			}
			if report != nil {
				report.record(SolveFailureReasonEpoch, SolveFailurePhaseNone, SemanticKey{}, SemanticKey{}, SemanticKey{}, SemanticKey{})
			}
			return nil, SolveIncomplete
		}
		epoch.report = report
		current = epoch
		ran := epoch.run()
		if !ran {
			epoch.incomplete()
			epoch.discard()
			if ctx.Err() != nil {
				return nil, SolveCanceled
			}
			if report != nil {
				report.record(SolveFailureReasonExecution, SolveFailurePhaseNone, SemanticKey{}, SemanticKey{}, SemanticKey{}, SemanticKey{})
			}
			return nil, SolveIncomplete
		}
		if epoch.activationPending {
			frontier, canonical := canonicalizeAcceptedActivations(runtime.topology, epoch.activations)
			if !canonical {
				epoch.incomplete()
				epoch.discard()
				if report != nil {
					report.record(SolveFailureReasonActivationMerge, SolveFailurePhaseNone, SemanticKey{}, SemanticKey{}, SemanticKey{}, SemanticKey{})
				}
				return nil, SolveIncomplete
			}
			delta, subtracted := subtractAcceptedActivations(runtime.topology, frontier, solver.accepted)
			if !subtracted {
				epoch.incomplete()
				epoch.discard()
				if report != nil {
					report.record(SolveFailureReasonActivationMerge, SolveFailurePhaseNone, SemanticKey{}, SemanticKey{}, SemanticKey{}, SemanticKey{})
				}
				return nil, SolveIncomplete
			}
			epoch.activations = nil
			epoch.activationPending = len(delta) != 0
			if len(delta) == 0 {
				// Every observed Member and premise is already represented by the
				// committed relation. Keep this completed epoch for publication.
			} else {
				epoch.incomplete()
				epoch.discard()
				current = nil
				if ctx.Err() != nil {
					return nil, SolveCanceled
				}
				accepted, merged := mergeAcceptedActivations(runtime.topology, solver.accepted, delta)
				if !merged || sameAcceptedActivations(solver.accepted, accepted) {
					if report != nil {
						report.record(SolveFailureReasonActivationMerge, SolveFailurePhaseNone, SemanticKey{}, SemanticKey{}, SemanticKey{}, SemanticKey{})
					}
					return nil, SolveIncomplete
				}
				rebuilt, phase, built := solver.compiler.compile(accepted)
				if !built || rebuilt == nil {
					if report != nil {
						report.record(SolveFailureReasonActivationCompile, phase, SemanticKey{}, SemanticKey{}, SemanticKey{}, SemanticKey{})
					}
					return nil, SolveIncomplete
				}
				if ctx.Err() != nil {
					rebuilt = nil
					return nil, SolveCanceled
				}
				if solver.revision == ^uint64(0) {
					if report != nil {
						report.record(SolveFailureReasonActivationRevisionOverflow, SolveFailurePhaseNone, SemanticKey{}, SemanticKey{}, SemanticKey{}, SemanticKey{})
					}
					return nil, SolveIncomplete
				}
				runtime.completed = nil
				if runtime.retained != nil && !runtime.retained.Close() {
					if report != nil {
						report.record(SolveFailureReasonActivationRetainedClose, SolveFailurePhaseNone, SemanticKey{}, SemanticKey{}, SemanticKey{}, SemanticKey{})
					}
					return nil, SolveIncomplete
				}
				runtime.retained = nil
				solver.accepted = accepted
				solver.runtime = rebuilt
				solver.revision++
				if ctx.Err() != nil {
					return nil, SolveCanceled
				}
				continue
			}
		}
		results := make([]*queryResult, len(runtime.queries))
		for index, query := range runtime.queries {
			if epoch.canceled() {
				epoch.incomplete()
				epoch.discard()
				return nil, SolveCanceled
			}
			authority := query.queryAuthority()
			if !validQueryAuthority(authority) || authority.schema.composition != runtime.composition || !query.query().Key().Available() || index < 0 || index >= len(results) {
				epoch.incomplete()
				epoch.discard()
				reportFailureQuery(report, SolveFailureReasonQuery, SemanticKey{})
				return nil, SolveIncomplete
			}
			if results[index] != nil {
				epoch.incomplete()
				epoch.discard()
				reportFailureQuery(report, SolveFailureReasonQuery, SemanticKey{})
				return nil, SolveIncomplete
			}
			point := query.query().Point()
			pointIndex, indexed := runtime.graph.PointIndex(point)
			if !indexed || pointIndex < 0 || pointIndex >= len(epoch.points) {
				epoch.incomplete()
				epoch.discard()
				reportFailureQuery(report, SolveFailureReasonQuery, semanticKeyFromComposition(point.Key()))
				return nil, SolveIncomplete
			}
			result, ok := query.materialize(epoch.work, epoch.points[pointIndex].State())
			if !ok {
				if epoch.canceled() {
					epoch.incomplete()
					epoch.discard()
					return nil, SolveCanceled
				}
				epoch.incomplete()
				epoch.discard()
				reportFailureQuery(report, SolveFailureReasonQuery, semanticKeyFromComposition(point.Key()))
				return nil, SolveIncomplete
			}
			if epoch.canceled() {
				epoch.incomplete()
				epoch.discard()
				return nil, SolveCanceled
			}
			if result == nil || result.owner != authority || result.key != query.query().Key() {
				epoch.incomplete()
				epoch.discard()
				reportFailureQuery(report, SolveFailureReasonQuery, semanticKeyFromComposition(point.Key()))
				return nil, SolveIncomplete
			}
			results[index] = result
		}
		for _, result := range results {
			if result == nil {
				epoch.incomplete()
				epoch.discard()
				reportFailureQuery(report, SolveFailureReasonQuery, SemanticKey{})
				return nil, SolveIncomplete
			}
		}
		if epoch.canceled() {
			epoch.incomplete()
			epoch.discard()
			return nil, SolveCanceled
		}
		if solver.completion == ^uint64(0) {
			epoch.incomplete()
			epoch.discard()
			reportFailureQuery(report, SolveFailureReasonPublication, SemanticKey{})
			return nil, SolveIncomplete
		}
		nextCompletion := solver.completion + 1
		state = &State{owner: solver, completion: &completionAuthority{solver: solver, serial: nextCompletion, revision: solver.revision}, results: results}
		// Retain and eviction are preparation, not publication.  They must
		// finish while cancellation can still win the epoch terminal race.
		retained, retainedOK := epoch.work.Retain()
		if !retainedOK {
			epoch.discard()
			reportFailureQuery(report, SolveFailureReasonPublication, SemanticKey{})
			return nil, SolveIncomplete
		}
		epoch.work = nil
		prepared = retained
		if epoch.canceled() {
			retained.Close()
			prepared = nil
			epoch.discard()
			return nil, SolveCanceled
		}
		prior := runtime.retained
		if prior != nil && !prior.Close() {
			retained.Close()
			prepared = nil
			epoch.discard()
			reportFailureQuery(report, SolveFailureReasonPublication, SemanticKey{})
			return nil, SolveIncomplete
		}
		// A successfully evicted lease is no longer a cache, even if the
		// following cancellation prevents this candidate from publishing.
		runtime.retained = nil
		if epoch.canceled() {
			retained.Close()
			prepared = nil
			epoch.discard()
			return nil, SolveCanceled
		}
		if !solver.publishCompleted(epoch, runtime, state, nextCompletion, retained) {
			retained.Close()
			prepared = nil
			epoch.discard()
			return nil, SolveCanceled
		}
		prepared = nil
		epoch.discard()
		current = nil
		return state, SolveComplete
	}
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
