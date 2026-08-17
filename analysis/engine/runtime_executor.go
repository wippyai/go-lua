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
	"github.com/wippyai/go-lua/analysis/identity"
)

type SolveStatus uint8

const (
	SolveIncomplete SolveStatus = iota + 1
	SolveComplete
	SolveCanceled
	SolvePanicked
	// SolveInvalid is returned only by a diagnostic entry point before any
	// solver work when its closed options value is invalid.
	SolveInvalid
)

// producerEpoch is an epoch-local candidate cache in graph Group order. A
// generation marks it dirty; the runnable identity is always its output Point.
type producerEpoch struct {
	generation                uint64
	applied                   uint64
	version                   uint64
	candidate                 carrier.RuleContribution
	hasValue                  bool
	candidateTokens           []uint64
	scratchTokens             []uint64
	hasCandidateTokens        bool
	candidateEnvironmentToken uint64
	scratchEnvironmentToken   uint64
	inputs                    []carrier.PointState
	inputStates               []carrier.State
	patches                   []carrier.Patch
	patchRows                 []contributionPatch
	reads                     []demand.Observation
}

// regionPostfixProof is the disposable relation certificate for one exact
// recurrence episode.  It contains no carrier state: exact revision/input
// evidence and head publication version are the immutable evidence that the
// already checked exact<=current relation still describes this head.
type regionPostfixProof struct {
	valid         bool
	episode       uint64
	phase         solvePhase
	exactInputs   uint64
	exactRevision uint64
	headVersion   uint64
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
	phase              solvePhase
	episode            uint64
	exact              carrier.PointRHS
	hasExact           bool
	exactInputsVersion uint64
	exactRevision      uint64
	postfix            regionPostfixProof
	invalid            bool
	// interfaceRefreshPending is an epoch-local barrier.  A stale boundary
	// version first dirties its ordinary Group owners and waits for their
	// candidate generations to settle; only then may the head refold and take
	// a new version snapshot.  Keeping the old snapshot during that interval
	// prevents a second head visit from re-dirtying the same Groups.
	interfaceRefreshPending bool
	interfaces              []uint64
	ingress                 []uint64
	backIngress             []uint64
	environmentIngress      []uint64
	environmentBackIngress  []uint64
	factorIngress           []uint64
	factorBackIngress       []uint64
	snapshot                []uint64
}

type solvePhase uint8

const (
	phaseAscent solvePhase = iota + 1
	phaseNarrow
)

type pointPublication uint8

const (
	publicationAscending pointPublication = iota + 1
	publicationMayDescend
)

// structuralInputEpoch records the newest global descent generation among the
// structural sources incorporated by one point's exact fold.
type structuralInputEpoch struct {
	descent uint64
	seeded  bool
}

type structuralEpoch struct {
	descent      uint64
	pointDescent []uint64
	inputs       []structuralInputEpoch
}

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
	report             *SolveReport
	diagnostics        *solveDiagnosticState
	diagnosticRevision identity.Generation
	work               *carrier.Work
	demand             *demand.Epoch
	points             []carrier.PointState
	versions           []uint64
	producers          []producerEpoch
	regions            []regionEpoch
	// candidatesPending is one dense counter per active Region. It counts the
	// producer Groups in that Region's complete event-point interval (and thus
	// in every nested descendant) whose latest wake generation has not yet
	// been evaluated. The counter is the only hot-path candidate-settled
	// witness; Group rows remain the source of the generation/applied state.
	candidatesPending []uint64
	queue             pointQueue
	structuralDirty   []bool
	structural        structuralEpoch
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

// recordRefreshPointFailure is the point-level fallback for refreshPoint.
// Member/group failures record their more specific certificate first, so this
// deliberately relies on SolveReport's first-wins record boundary.
func (epoch *executorEpoch) recordRefreshPointFailure(phase SolveFailurePhase, point equation.Point) {
	if epoch == nil {
		return
	}
	pointKey := composition.Key{}
	if point.Available() {
		pointKey = point.Key()
	}
	epoch.recordFailure(SolveFailureReasonExecution, phase, pointKey, composition.Key{}, composition.Key{}, composition.Key{})
}

// recordRunFailure records a closed executor-loop boundary with no invented
// Point/Group coordinate. Cancellation and diagnostic cutoff are terminal
// statuses, not execution failures, and are intentionally handled by run's
// callers without reaching this helper.
func (epoch *executorEpoch) recordRunFailure(phase SolveFailurePhase) {
	if epoch == nil || epoch.canceled() {
		return
	}
	epoch.recordFailure(SolveFailureReasonExecution, phase, composition.Key{}, composition.Key{}, composition.Key{}, composition.Key{})
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

// recordCandidateOrderFailure keeps the producer certificate at the exact
// order boundary. Program-artifact groups are singleton, so their Rule role
// remains classifiable by AnalyzeDiagnostics; multi-member groups retain only
// their Group identity rather than inventing one culprit.
func (epoch *executorEpoch) recordCandidateOrderFailure(phase SolveFailurePhase, point equation.Point, group equation.GroupNode) {
	if epoch == nil {
		return
	}
	if group.MemberCount() == 1 {
		if member, ok := group.MemberAt(0); ok {
			epoch.recordMemberFailure(SolveFailureReasonExecution, phase, point, group, member)
			return
		}
	}
	pointKey, groupKey := composition.Key{}, composition.Key{}
	if point.Available() {
		pointKey = point.Key()
	}
	if epoch.runtime != nil && epoch.runtime.graph != nil && epoch.runtime.graph.OwnsGroup(group) {
		groupKey = group.Key()
	}
	epoch.recordFailure(SolveFailureReasonExecution, phase, pointKey, groupKey, composition.Key{}, composition.Key{})
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

func newRuntimeEpoch(runtime *solverRuntime, relation equation.Relation, ctx context.Context) (*executorEpoch, bool) {
	// Owner/liveness fence: do not allocate an epoch for a missing or canceled
	// call, or for a runtime with a missing owner-owned root.
	if runtime == nil {
		return nil, false
	}
	if ctx == nil {
		return nil, false
	}
	if ctx.Err() != nil {
		return nil, false
	}
	if runtime.carrier == nil || runtime.graph == nil || runtime.points == nil || runtime.demand == nil || runtime.topology == nil {
		return nil, false
	}

	// Accepted members must belong to this runtime generation's sealed
	// topology before any carrier or demand state is opened.
	if !relation.OwnedBy(runtime.topology) {
		return nil, false
	}

	// All dense runtime rows must remain aligned with their graph-owned
	// cardinalities. A mismatch is structural corruption, not an empty solve.
	pointCount := runtime.graph.PointCount()
	groupCount := runtime.graph.GroupCount()
	regionCount := len(runtime.regions)
	if len(runtime.producers) != groupCount ||
		len(runtime.activePoints) != pointCount ||
		len(runtime.pointInitials) != pointCount ||
		len(runtime.activeRegions) != regionCount ||
		len(runtime.regions) != regionCount ||
		len(runtime.regionChildren) != regionCount ||
		len(runtime.environmentIncoming) != pointCount ||
		len(runtime.factorIncoming) != pointCount ||
		len(runtime.overlay.factorOutgoing) != pointCount {
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
	epoch := &executorEpoch{
		runtime:           runtime,
		ctx:               ctx,
		work:              work,
		demand:            demandEpoch,
		points:            make([]carrier.PointState, runtime.graph.PointCount()),
		versions:          make([]uint64, runtime.graph.PointCount()),
		producers:         make([]producerEpoch, runtime.graph.GroupCount()),
		regions:           make([]regionEpoch, len(runtime.regions)),
		candidatesPending: make([]uint64, len(runtime.regions)),
		queue:             newPointQueue(runtime.graph.PointCount()),
		structuralDirty:   make([]bool, runtime.graph.PointCount()),
		structural: structuralEpoch{
			pointDescent: make([]uint64, runtime.graph.PointCount()),
			inputs:       make([]structuralInputEpoch, runtime.graph.PointCount()),
		},
		postfixDirty:   make([]bool, runtime.graph.PointCount()),
		postfixPending: make([]int, 0, runtime.graph.PointCount()),
		frames:         make([]pointWTOFrame, 0, regionCount),
		nested:         make([]int, regionCount),
		regionScratch:  make([]int, 0, regionCount),
	}
	epoch.terminal.Store(epochRunning)
	if !work.SetCheckpoint(epoch.checkpoint) {
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
		epoch.regions[index].episode = 1
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
		emptyContribution, paired := work.EmptyContribution(state)
		if !paired {
			return nil, false
		}
		emptyRule, paired := work.AsRuleContribution(emptyContribution)
		if !paired {
			return nil, false
		}
		initial, paired := work.PointStateFromRuleContribution(emptyRule)
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
				epoch.producers[groupIndex] = producerEpoch{generation: 1, candidateTokens: make([]uint64, inputCount), scratchTokens: make([]uint64, inputCount), inputs: make([]carrier.PointState, inputCount), inputStates: make([]carrier.State, inputCount), patches: make([]carrier.Patch, 0, len(metadata.members)), patchRows: make([]contributionPatch, 0, len(metadata.members)), reads: make([]demand.Observation, 0, len(metadata.reads))}
			}
			if !epoch.enqueuePoint(pointIndex) {
				return nil, false
			}
		}
		if !epoch.markStructuralPoint(point) {
			return nil, false
		}
	}
	// Every producer starts with generation 1 and applied 0. Seed each active
	// Region's counter from its immutable event-point interval; that interval
	// includes every nested descendant point, so a Group is intentionally
	// counted once for each active Region that encloses its output.
	for regionIndex, region := range runtime.regions {
		if !runtime.activeRegions[regionIndex] {
			continue
		}
		if !region.active {
			return nil, false
		}
		var pending uint64
		for _, pointIndex := range region.points {
			if pointIndex < 0 || pointIndex >= runtime.graph.PointCount() {
				return nil, false
			}
			point, pointOK := runtime.graph.PointAt(schedule.Node(pointIndex))
			if !pointOK {
				return nil, false
			}
			producerCount := runtime.graph.ProducerCount(point)
			if producerCount < 0 || uint64(producerCount) > ^uint64(0)-pending {
				return nil, false
			}
			pending += uint64(producerCount)
		}
		epoch.candidatesPending[regionIndex] = pending
	}
	opened = true
	return epoch, true
}

func (runtime *solverRuntime) executionEventCount() int {
	if runtime == nil {
		return 0
	}
	if runtime.executionDemand != nil {
		return runtime.executionDemand.EventCount()
	}
	if runtime.points == nil {
		return 0
	}
	return runtime.points.EventCount()
}

func (runtime *solverRuntime) executionEventAt(index int) (schedule.Event, bool) {
	if runtime == nil {
		return schedule.Event{}, false
	}
	if runtime.executionDemand != nil {
		event, _, ok := runtime.executionDemand.EventAt(index)
		return event, ok
	}
	if runtime.points == nil {
		return schedule.Event{}, false
	}
	event, _, ok := runtime.points.EventAt(index)
	return event, ok
}

func (runtime *solverRuntime) executionRegionAt(index int) (schedule.Region, bool) {
	if runtime == nil {
		return schedule.Region{}, false
	}
	if runtime.execution != nil {
		return runtime.execution.RegionAt(index)
	}
	if runtime.graph == nil || runtime.graph.Schedule() == nil {
		return schedule.Region{}, false
	}
	return runtime.graph.Schedule().RegionAt(index)
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

// checkpoint is the cancellation-only evaluator liveness probe shared by
// ordinary carrier work and nested rule/query frames.
func (epoch *executorEpoch) checkpoint() bool {
	return epoch != nil && !epoch.canceled()
}

// diagnosticCheckpoint is installed only for flagged diagnostic solves after
// epoch construction succeeds. Ordinary carrier and diagram probes retain the
// cancellation-only checkpoint above.
func (epoch *executorEpoch) diagnosticCheckpoint() bool {
	if epoch == nil || epoch.canceled() {
		return false
	}
	if epoch.diagnostics == nil || epoch.diagnostics.checkpoint() {
		return true
	}
	epoch.terminal.CompareAndSwap(epochRunning, epochIncomplete)
	return false
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
// environment and factor edges are Point dependencies: a target must retain
// its structural proof bit while it is queued, then refreshPoint folds the
// complete incoming edge set before clearing that bit. Selected FactorEdges
// share this same runtime-owned incoming CSR, so no dynamic family can fall
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
	if len(epoch.runtime.environmentIncoming[pointIndex]) == 0 && len(epoch.runtime.factorIncoming[pointIndex]) == 0 {
		return true
	}
	epoch.structuralDirty[pointIndex] = true
	return epoch.markPostfixDirty(pointIndex) && epoch.enqueuePoint(pointIndex)
}

// markStructuralSuccessors applies the same Point wake protocol to every
// graph-owned EnvironmentEdges and runtime-owned FactorEdges. The edge-
// specific folds remain in foldPoint; scheduling never gets a second dynamic
// FactorEdge lifecycle.
func (epoch *executorEpoch) markStructuralSuccessors(source equation.Point) bool {
	if epoch == nil || epoch.runtime == nil || epoch.runtime.graph == nil || !source.Available() {
		return false
	}
	graph := epoch.runtime.graph
	for index := 0; index < graph.EnvironmentOutgoingCount(source); index++ {
		edge, ok := graph.EnvironmentOutgoingAt(source, index)
		if !ok {
			return false
		}
		if edge.TransportOnly() {
			continue
		}
		if !epoch.markStructuralPoint(edge.Target()) {
			return false
		}
	}
	sourceIndex, sourceIndexed := graph.PointIndex(source)
	if !sourceIndexed || sourceIndex < 0 || sourceIndex >= len(epoch.runtime.overlay.factorOutgoing) {
		return false
	}
	for _, edgeIndex := range epoch.runtime.overlay.factorOutgoing[sourceIndex] {
		if edgeIndex < 0 || edgeIndex >= len(epoch.runtime.factorEdges) {
			return false
		}
		edge := epoch.runtime.factorEdges[edgeIndex]
		target, targetOK := graph.PointAt(schedule.Node(edge.target))
		if !targetOK || !epoch.markStructuralPoint(target) {
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

func (episode *regionEpoch) nextExactRevision() bool {
	if episode == nil {
		return false
	}
	if episode.exactRevision == ^uint64(0) {
		episode.postfix = regionPostfixProof{}
		return false
	}
	episode.postfix = regionPostfixProof{}
	episode.exactRevision++
	return true
}

// invalidateRegionPostfix drops only the relation certificate.  The exact
// episode, its input evidence, and the live Point values remain epoch-owned;
// callers still decide whether the corresponding head must be queued.
func (epoch *executorEpoch) invalidateRegionPostfix(region int) bool {
	if epoch == nil || !epoch.activeRegion(region) || region >= len(epoch.regions) {
		return false
	}
	epoch.regions[region].postfix = regionPostfixProof{}
	return true
}

func (epoch *executorEpoch) regionPostfixProved(region int) bool {
	if epoch == nil || !epoch.activeRegion(region) || region >= len(epoch.regions) {
		return false
	}
	bound, episode := epoch.runtime.regions[region], &epoch.regions[region]
	if !episode.hasExact || episode.episode == 0 || episode.exactInputsVersion == 0 || episode.exactRevision == 0 || bound.head < 0 || bound.head >= len(epoch.versions) {
		return false
	}
	proof := episode.postfix
	return proof.valid && proof.episode == episode.episode && proof.phase == episode.phase && proof.exactInputs == episode.exactInputsVersion && proof.exactRevision == episode.exactRevision && proof.headVersion == epoch.versions[bound.head]
}

func (epoch *executorEpoch) rememberRegionPostfix(region int) bool {
	if epoch == nil || !epoch.activeRegion(region) || region >= len(epoch.regions) {
		return false
	}
	bound, episode := epoch.runtime.regions[region], &epoch.regions[region]
	if !episode.hasExact || episode.episode == 0 || episode.exactInputsVersion == 0 || episode.exactRevision == 0 || bound.head < 0 || bound.head >= len(epoch.versions) {
		return false
	}
	episode.postfix = regionPostfixProof{valid: true, episode: episode.episode, phase: episode.phase, exactInputs: episode.exactInputsVersion, exactRevision: episode.exactRevision, headVersion: epoch.versions[bound.head]}
	return true
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
	clean := state.applied == state.generation
	if state.generation == ^uint64(0) {
		return false
	}
	if clean && !epoch.updateCandidatesPending(point, 1) {
		return false
	}
	state.generation++
	return epoch.markPostfixDirty(point) && epoch.enqueuePoint(point)
}

// markDirtyIfClean is used for published input-identity propagation. A pending
// candidate already reads the newest source PointState, so creating another
// generation would only force a duplicate evaluation; a clean candidate must
// be woken to carry the new State+C identity through ordinary consumers.
func (epoch *executorEpoch) markDirtyIfClean(group int) bool {
	if epoch == nil || group < 0 || group >= len(epoch.producers) {
		return false
	}
	state := &epoch.producers[group]
	if state.applied != state.generation {
		return true
	}
	return epoch.markDirty(group)
}

func (epoch *executorEpoch) markPublishedInputConsumers(source equation.Point) bool {
	if epoch == nil || epoch.runtime == nil || epoch.runtime.graph == nil || !source.Available() {
		return false
	}
	graph := epoch.runtime.graph
	for index := 0; index < graph.ConsumerCount(source); index++ {
		group, ok := graph.ConsumerAt(source, index)
		groupIndex, indexed := graph.GroupIndex(group)
		outputIndex, outputIndexed := graph.PointIndex(group.Output())
		if !ok || !indexed || !outputIndexed || groupIndex < 0 || groupIndex >= len(epoch.runtime.producers) || outputIndex < 0 || outputIndex >= len(epoch.runtime.activePoints) {
			return false
		}
		if !epoch.runtime.activePoints[outputIndex] {
			continue
		}
		if !epoch.markDirtyIfClean(groupIndex) {
			return false
		}
	}
	return true
}

// updateCandidatesPending applies a candidate clean<->pending transition to
// the innermost active Region containing point and every active ancestor. The
// validation pass completes before mutation, making counter overflow and
// underflow fences transactional: a rejected transition cannot leave one
// ancestor observing a different count from another.
func (epoch *executorEpoch) updateCandidatesPending(point, delta int) bool {
	if epoch == nil || epoch.runtime == nil || point < 0 || point >= len(epoch.runtime.pointRegion) || point >= len(epoch.runtime.activePoints) || !epoch.runtime.activePoints[point] || delta != 1 && delta != -1 || len(epoch.candidatesPending) != len(epoch.runtime.regions) {
		return false
	}
	region := epoch.runtime.pointRegion[point]
	if region == schedule.NoRegion {
		return true
	}
	for region != schedule.NoRegion {
		if !epoch.activeRegion(region) {
			return false
		}
		pending := epoch.candidatesPending[region]
		if delta > 0 {
			if pending == ^uint64(0) {
				return false
			}
		} else if pending == 0 {
			return false
		}
		parent := epoch.runtime.regions[region].parent
		if parent < schedule.NoRegion || parent >= len(epoch.runtime.regions) {
			return false
		}
		region = parent
	}
	region = epoch.runtime.pointRegion[point]
	for region != schedule.NoRegion {
		if delta > 0 {
			epoch.candidatesPending[region]++
		} else {
			epoch.candidatesPending[region]--
		}
		region = epoch.runtime.regions[region].parent
	}
	return true
}

func (epoch *executorEpoch) activeRegion(region int) bool {
	return epoch != nil && epoch.runtime != nil && region >= 0 && region < len(epoch.runtime.regions) && region < len(epoch.runtime.activeRegions) && epoch.runtime.activeRegions[region] && epoch.runtime.regions[region].active
}

func (epoch *executorEpoch) updateNested(point int, delta int) bool {
	if epoch == nil || epoch.runtime == nil || point < 0 || point >= len(epoch.runtime.pointRegion) || point >= len(epoch.runtime.activePoints) || !epoch.runtime.activePoints[point] || delta != 1 && delta != -1 || len(epoch.nested) != len(epoch.runtime.regions) {
		return false
	}
	region := epoch.runtime.pointRegion[point]
	for depth := 0; region != schedule.NoRegion; depth++ {
		if depth >= len(epoch.runtime.regions) {
			return false
		}
		if !epoch.activeRegion(region) {
			return false
		}
		parent := epoch.runtime.regions[region].parent
		if parent == schedule.NoRegion {
			break
		}
		if parent < 0 || parent >= len(epoch.nested) || !epoch.activeRegion(parent) || delta > 0 && epoch.nested[parent] == int(^uint(0)>>1) || delta < 0 && epoch.nested[parent] == 0 {
			return false
		}
		region = parent
	}
	region = epoch.runtime.pointRegion[point]
	for region != schedule.NoRegion {
		parent := epoch.runtime.regions[region].parent
		if parent == schedule.NoRegion {
			break
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
	// updateNested is validated and applied before the ready bit. queue.add
	// cannot fail after the bounds/readiness checks above, so a rejected
	// counter transition cannot mint a queue entry without its ancestor debt.
	if !epoch.updateNested(point, 1) {
		return false
	}
	if !epoch.queue.add(point) {
		return false
	}
	if epoch.diagnostics != nil {
		epoch.diagnostics.observeQueue(epoch.queue.count)
	}
	return true
}

func (epoch *executorEpoch) takePoint(point int) bool {
	if epoch == nil || epoch.runtime == nil || point < 0 || point >= len(epoch.queue.ready) || point >= len(epoch.runtime.activePoints) || !epoch.runtime.activePoints[point] || !epoch.queue.ready[point] {
		return false
	}
	// Mirror enqueuePoint: a malformed counter must leave the ready bit set so
	// a failed dequeue cannot strand the scheduler with unaccounted work.
	return epoch.updateNested(point, -1) && epoch.queue.take(point)
}

// inputs snapshots and reindexes the full Group input vector once. Every
// member below receives this exact vector and the Group output target Scope.
func (epoch *executorEpoch) inputs(producer runtimeProducer, values []carrier.PointState) ([]carrier.PointState, bool) {
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
		if !current.Valid() {
			return nil, false
		}
		transport := producer.inputs[index]
		point, ok := epoch.work.TransportPointState(current, transport.pre, transport.plan, transport.post)
		if !transport.valid() || !ok || !point.Valid() || !point.Scope().Same(producer.outputScope) {
			return nil, false
		}
		values[index] = point
	}
	return values, true
}

func (epoch *executorEpoch) environment(producer runtimeProducer) (carrier.PointState, bool) {
	if epoch == nil || epoch.work == nil || producer.environment == nil || !producer.environment.valid() {
		return carrier.PointState{}, producer.environment == nil
	}
	input, inputOK := producer.group.EnvironmentInput()
	pointIndex, indexed := epoch.runtime.graph.PointIndex(input.Point())
	if !inputOK || !indexed || pointIndex < 0 || pointIndex >= len(epoch.points) {
		return carrier.PointState{}, false
	}
	current := epoch.points[pointIndex]
	if !current.Valid() {
		return carrier.PointState{}, false
	}
	transported, ok := epoch.work.TransportPointState(current, producer.environment.pre, producer.environment.plan, producer.environment.post)
	if !ok || !transported.Valid() || !transported.Scope().Same(producer.outputScope) {
		return carrier.PointState{}, false
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

func (epoch *executorEpoch) evaluate(producer runtimeProducer, cache *producerEpoch) (result carrier.RuleContribution, reads []demand.Observation, ok bool) {
	if epoch != nil && epoch.diagnostics != nil && epoch.diagnostics.scheduleEnabled() {
		defer func() { epoch.diagnostics.recordEvaluate(&ok) }()
	}
	if epoch == nil || epoch.work == nil || cache == nil || epoch.canceled() || len(producer.members) == 0 || !producer.outputScope.Valid() || !producer.premise.Valid() || producer.premise.Manager() != epoch.runtime.carrier.Guards() {
		return carrier.RuleContribution{}, nil, false
	}
	inputs, ok := epoch.inputs(producer, cache.inputs)
	if !ok {
		return carrier.RuleContribution{}, nil, false
	}
	var environment carrier.PointState
	if producer.environment != nil {
		environment, ok = epoch.environment(producer)
		if !ok {
			return carrier.RuleContribution{}, nil, false
		}
	}
	if producer.environment != nil {
		input, inputOK := producer.group.EnvironmentInput()
		pointIndex, indexed := epoch.runtime.graph.PointIndex(input.Point())
		if !inputOK || !indexed || pointIndex < 0 || pointIndex >= len(epoch.versions) {
			return carrier.RuleContribution{}, nil, false
		}
		cache.scratchEnvironmentToken = epoch.versions[pointIndex]
	} else {
		cache.scratchEnvironmentToken = 0
	}
	if len(cache.inputStates) != len(inputs) {
		return carrier.RuleContribution{}, nil, false
	}
	for index := range inputs {
		cache.inputStates[index] = inputs[index].State()
	}
	var base carrier.RuleContributionBase
	// BeginContribution is the single authority that intersects all input
	// supports with this sealed group premise. The executor cannot precompute
	// or substitute a support result at a parallel boundary.
	if producer.environment != nil {
		base, ok = epoch.work.BeginRuleContribution(producer.plan, producer.outputScope, inputs, producer.premise, environment)
	} else {
		base, ok = epoch.work.BeginRuleContribution(producer.plan, producer.outputScope, inputs, producer.premise)
	}
	if !ok {
		return carrier.RuleContribution{}, nil, false
	}
	within := base.State().Support()
	if !within.Valid() {
		return carrier.RuleContribution{}, nil, false
	}
	patches := cache.patches[:0]
	patchRows := cache.patchRows[:0]
	reads = cache.reads[:0]
	retained := within
	supportPrune := false
	activations := make([]equation.AcceptedMember, 0)
	live := true
	defer func() {
		if live {
			_ = epoch.work.AbortRuleContribution(base, patches)
		}
	}()
	for _, member := range producer.members {
		if epoch.canceled() {
			return carrier.RuleContribution{}, nil, false
		}
		result := member.execute(epoch.work, base, cache.inputStates, within)
		if !result.valid {
			epoch.recordMemberFailure(SolveFailureReasonExecution, result.phase, producer.group.Output(), producer.group, member.member())
			return carrier.RuleContribution{}, nil, false
		}
		reads = append(reads, result.reads...)
		if result.hasSupport {
			if !result.retained.Valid() || !result.retained.Entails(within) {
				return carrier.RuleContribution{}, nil, false
			}
			if supportPrune {
				var valid bool
				retained, valid = support.IntersectWithCheckpoint(func() bool { return !epoch.canceled() }, retained, result.retained)
				if !valid {
					return carrier.RuleContribution{}, nil, false
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
				return carrier.RuleContribution{}, nil, false
			}
			activations = append(activations, result.activations...)
		}
		if result.wrote {
			slot, writes := member.outputSlot()
			if !writes {
				return carrier.RuleContribution{}, nil, false
			}
			patches = append(patches, result.patch)
			patchRows = append(patchRows, contributionPatch{slot: slot, patch: result.patch})
		}
	}
	if len(activations) != 0 {
		if !epoch.observeActivations(activations) {
			epoch.recordGroupFailure(SolveFailureReasonActivationMerge, producer.group.Output(), producer.group)
			return carrier.RuleContribution{}, nil, false
		}
	}
	sort.Slice(patchRows, func(left, right int) bool { return patchRows[left].slot < patchRows[right].slot })
	for index, row := range patchRows {
		if row.slot < 0 || index > 0 && patchRows[index-1].slot >= row.slot {
			return carrier.RuleContribution{}, nil, false
		}
	}
	patches = patches[:0]
	for _, row := range patchRows {
		patches = append(patches, row.patch)
	}
	var next carrier.RuleContribution
	if supportPrune {
		next, ok = epoch.work.FinishRuleContributionWithSupport(base, patches, retained)
	} else {
		next, ok = epoch.work.FinishRuleContribution(base, patches)
	}
	if !ok {
		return carrier.RuleContribution{}, nil, false
	}
	// Finish consumed the one-shot base and every owned Patch atomically. Only
	// now disarm AbortContribution; cancellation before or during Finish keeps
	// the defer armed and deterministically drops the still-owned transaction.
	live = false
	if epoch.canceled() {
		return carrier.RuleContribution{}, nil, false
	}
	cache.patches, cache.patchRows, cache.reads = patches, patchRows, reads
	return next, reads, true
}

func (epoch *executorEpoch) pointBase(point equation.Point, pointIndex int) (carrier.PointRHS, bool) {
	if epoch == nil || epoch.runtime == nil || pointIndex < 0 || pointIndex >= len(epoch.runtime.pointScopes) || pointIndex >= len(epoch.runtime.pointInitials) || !epoch.runtime.pointScopes[pointIndex].Valid() {
		return carrier.PointRHS{}, false
	}
	feasible, ok := support.FromGuard(epoch.runtime.carrier.Guards(), epoch.runtime.carrier.Guards().False())
	if !ok {
		return carrier.PointRHS{}, false
	}
	init, disposition, initialized := point.Init()
	if !initialized || !init.Available() {
		return carrier.PointRHS{}, false
	}
	if disposition == equation.InitPresent {
		feasible = epoch.runtime.pointInitials[pointIndex]
		if !feasible.Valid() {
			return carrier.PointRHS{}, false
		}
	}
	state, ok := carrier.NewState(epoch.runtime.carrier, epoch.runtime.pointScopes[pointIndex], feasible)
	if !ok {
		return carrier.PointRHS{}, false
	}
	pointState, ok := epoch.work.EmptyPointState(state)
	if !ok {
		return carrier.PointRHS{}, false
	}
	return epoch.work.PointRHSFromPointState(pointState)
}

func (epoch *executorEpoch) addPointFoldEnvironmentEdge(edgeIndex int) bool {
	if epoch == nil || epoch.runtime == nil || epoch.work == nil || edgeIndex < 0 || edgeIndex >= len(epoch.runtime.environments) {
		return false
	}
	edge := epoch.runtime.environments[edgeIndex]
	if edge.source < 0 || edge.source >= len(epoch.points) || edge.target < 0 || edge.target >= len(epoch.points) || !edge.input.valid() {
		return false
	}
	transported, ok := epoch.work.TransportPointState(epoch.points[edge.source], edge.input.pre, edge.input.plan, edge.input.post)
	return ok && epoch.work.AddPointFoldEnvironment(transported) && !epoch.canceled()
}

// addPointFoldFactorEdgeWithBoundary preserves the existing projection,
// transport, and one transaction admission while returning only the failed
// boundary for the owning refresh diagnostic.
func (epoch *executorEpoch) addPointFoldFactorEdgeWithBoundary(edgeIndex int) (pointFoldBoundary, bool) {
	if epoch == nil || epoch.runtime == nil || epoch.work == nil || edgeIndex < 0 || edgeIndex >= len(epoch.runtime.factorEdges) {
		return pointFoldBoundaryFactorValidation, false
	}
	edge := epoch.runtime.factorEdges[edgeIndex]
	if edge.source < 0 || edge.source >= len(epoch.points) || edge.target < 0 || edge.target >= len(epoch.points) || !edge.input.valid() {
		return pointFoldBoundaryFactorValidation, false
	}
	// Factor projection commutes with the point boundary: support and guard
	// transport are shared by every plane, while typed reindex is factor-local.
	// Project first so a one-Factor edge never reconstructs the unrelated
	// Value/Call/Heap/Pack/Effect roots merely to discard them afterward.
	projected, ok := epoch.work.ProjectPointState(epoch.points[edge.source], edge.slot)
	if !ok || !epoch.work.OwnsPointState(projected) {
		return pointFoldBoundaryFactorProjection, false
	}
	transported, transportBoundary, ok := epoch.work.TransportPointStateWithBoundary(projected, edge.input.pre, edge.input.plan, edge.input.post)
	if !ok || !epoch.work.OwnsPointState(transported) {
		return pointFoldBoundaryFromTransport(transportBoundary), false
	}
	if !epoch.work.AddPointFoldEnvironment(transported) {
		return pointFoldBoundaryFactorAdmission, false
	}
	return pointFoldBoundaryNone, !epoch.canceled()
}

func (epoch *executorEpoch) addPointFoldGroup(group int) bool {
	return epoch != nil && group >= 0 && group < len(epoch.producers) && epoch.producers[group].hasValue && epoch.work.AddPointFoldRule(epoch.producers[group].candidate) && !epoch.canceled()
}

// foldPointInputs is the runtime's one canonical fixed-order RHS assembly.
// Environment PointStates, projected Factor PointStates, and closed producer
// contributions remain nominally distinct at admission; carrier borrows only
// their opaque root/support/coverage surfaces for one synchronized commit.
func (epoch *executorEpoch) foldPointInputs(reference carrier.PointState, base carrier.PointRHS, environments, factors, groups []int) (result carrier.PointRHS, ok bool) {
	return epoch.foldPointTerms(reference, base, environments, factors, groups, equation.Point{})
}

func (epoch *executorEpoch) structuralInputDescent(pointIndex int, selfDescent uint64) (uint64, bool) {
	if epoch == nil || pointIndex < 0 || pointIndex >= len(epoch.runtime.environmentIncoming) || pointIndex >= len(epoch.runtime.factorIncoming) || pointIndex >= len(epoch.structural.inputs) {
		return 0, false
	}
	newest := uint64(0)
	consider := func(source int) bool {
		if source < 0 || source >= len(epoch.structural.pointDescent) {
			return false
		}
		descent := epoch.structural.pointDescent[source]
		if source == pointIndex {
			descent = selfDescent
		}
		if descent > newest {
			newest = descent
		}
		return true
	}
	for _, edge := range epoch.runtime.environmentIncoming[pointIndex] {
		if edge < 0 || edge >= len(epoch.runtime.environments) || !consider(epoch.runtime.environments[edge].source) {
			return 0, false
		}
	}
	for _, edge := range epoch.runtime.factorIncoming[pointIndex] {
		if edge < 0 || edge >= len(epoch.runtime.factorEdges) || !consider(epoch.runtime.factorEdges[edge].source) {
			return 0, false
		}
	}
	return newest, true
}

func (epoch *executorEpoch) rememberStructuralInputs(pointIndex int, selfDescent uint64) bool {
	newest, ok := epoch.structuralInputDescent(pointIndex, selfDescent)
	if !ok {
		return false
	}
	epoch.structural.inputs[pointIndex] = structuralInputEpoch{descent: newest, seeded: true}
	return true
}

func (epoch *executorEpoch) invalidateStructuralInputs(pointIndex int) bool {
	if epoch == nil || pointIndex < 0 || pointIndex >= len(epoch.structural.inputs) {
		return false
	}
	epoch.structural.inputs[pointIndex] = structuralInputEpoch{}
	return true
}

// structuralInputsAscending is the cheap executor certificate for one exact
// acyclic refold. Global descent generations increase strictly, so equality
// with the stored maximum proves that no incoming source descended.
func (epoch *executorEpoch) structuralInputsAscending(pointIndex int) (bool, bool) {
	if epoch == nil || pointIndex < 0 || pointIndex >= len(epoch.structural.inputs) {
		return false, false
	}
	snapshot := epoch.structural.inputs[pointIndex]
	newest, ok := epoch.structuralInputDescent(pointIndex, epoch.structural.pointDescent[pointIndex])
	if !ok || snapshot.seeded && newest < snapshot.descent {
		return false, false
	}
	return !snapshot.seeded || newest == snapshot.descent, true
}

func (epoch *executorEpoch) foldPoint(reference carrier.PointState, base carrier.PointRHS, point equation.Point) (carrier.PointRHS, bool) {
	result, _, ok := epoch.foldPointWithBoundary(reference, base, point)
	return result, ok
}

// foldPointWithBoundary exposes only whether the outer Point ownership check
// reached the existing canonical terms fold. Refresh diagnostics use that
// scalar to distinguish an invalid foldPoint boundary from a failure inside
// foldPointTerms; it creates no additional fold authority or data path.
func (epoch *executorEpoch) foldPointWithBoundary(reference carrier.PointState, base carrier.PointRHS, point equation.Point) (carrier.PointRHS, pointFoldBoundary, bool) {
	if epoch == nil || epoch.runtime == nil || !point.Available() || !epoch.work.OwnsPointState(reference) || !epoch.work.OwnsPointRHS(base) {
		return carrier.PointRHS{}, pointFoldBoundaryNone, false
	}
	pointIndex, indexed := epoch.runtime.graph.PointIndex(point)
	if !indexed || pointIndex < 0 || pointIndex >= len(epoch.runtime.environmentIncoming) || pointIndex >= len(epoch.runtime.factorIncoming) {
		return carrier.PointRHS{}, pointFoldBoundaryNone, false
	}
	return epoch.foldPointTermsWithBoundary(reference, base, epoch.runtime.environmentIncoming[pointIndex], epoch.runtime.factorIncoming[pointIndex], nil, point)
}

// pointFoldBoundary is private diagnostic provenance for the one existing
// canonical Point-RHS transaction. It is not a second fold representation.
type pointFoldBoundary uint8

const (
	pointFoldBoundaryNone pointFoldBoundary = iota
	pointFoldBoundaryBegin
	pointFoldBoundaryEnvironment
	pointFoldBoundaryFactorValidation
	pointFoldBoundaryFactorProjection
	pointFoldBoundaryFactorTransportPreflight
	pointFoldBoundaryFactorTransportCoordinatePreSupport
	pointFoldBoundaryFactorTransportCoordinateReindexSupport
	pointFoldBoundaryFactorTransportCoordinatePostSupport
	pointFoldBoundaryFactorTransportCoordinateCoverage
	pointFoldBoundaryFactorTransportCoordinateAdmission
	pointFoldBoundaryFactorTransportGeneralPreFilter
	pointFoldBoundaryFactorTransportGeneralReindexSupport
	pointFoldBoundaryFactorTransportGeneralReindexTypedSlots
	pointFoldBoundaryFactorTransportGeneralReindexCommit
	pointFoldBoundaryFactorTransportGeneralPostFilter
	pointFoldBoundaryFactorTransportGeneralCoverage
	pointFoldBoundaryFactorTransportGeneralAdmission
	pointFoldBoundaryFactorAdmission
	pointFoldBoundaryProducer
	pointFoldBoundaryFinish
)

func pointFoldBoundaryFromTransport(boundary carrier.PointTransportBoundary) pointFoldBoundary {
	switch boundary {
	case carrier.PointTransportBoundaryPreflight:
		return pointFoldBoundaryFactorTransportPreflight
	case carrier.PointTransportBoundaryCoordinatePreSupport:
		return pointFoldBoundaryFactorTransportCoordinatePreSupport
	case carrier.PointTransportBoundaryCoordinateReindexSupport:
		return pointFoldBoundaryFactorTransportCoordinateReindexSupport
	case carrier.PointTransportBoundaryCoordinatePostSupport:
		return pointFoldBoundaryFactorTransportCoordinatePostSupport
	case carrier.PointTransportBoundaryCoordinateCoverage:
		return pointFoldBoundaryFactorTransportCoordinateCoverage
	case carrier.PointTransportBoundaryCoordinateAdmission:
		return pointFoldBoundaryFactorTransportCoordinateAdmission
	case carrier.PointTransportBoundaryGeneralPreFilter:
		return pointFoldBoundaryFactorTransportGeneralPreFilter
	case carrier.PointTransportBoundaryGeneralReindexSupport:
		return pointFoldBoundaryFactorTransportGeneralReindexSupport
	case carrier.PointTransportBoundaryGeneralReindexTypedSlots:
		return pointFoldBoundaryFactorTransportGeneralReindexTypedSlots
	case carrier.PointTransportBoundaryGeneralReindexCommit:
		return pointFoldBoundaryFactorTransportGeneralReindexCommit
	case carrier.PointTransportBoundaryGeneralPostFilter:
		return pointFoldBoundaryFactorTransportGeneralPostFilter
	case carrier.PointTransportBoundaryGeneralCoverage:
		return pointFoldBoundaryFactorTransportGeneralCoverage
	case carrier.PointTransportBoundaryGeneralAdmission:
		return pointFoldBoundaryFactorTransportGeneralAdmission
	default:
		return pointFoldBoundaryFactorTransportPreflight
	}
}

func refreshAcyclicFoldPhase(boundary pointFoldBoundary) SolveFailurePhase {
	switch boundary {
	case pointFoldBoundaryBegin:
		return SolveFailurePhaseRefreshAcyclicFoldBegin
	case pointFoldBoundaryEnvironment:
		return SolveFailurePhaseRefreshAcyclicFoldEnvironment
	case pointFoldBoundaryFactorValidation:
		return SolveFailurePhaseRefreshAcyclicFoldFactorValidation
	case pointFoldBoundaryFactorProjection:
		return SolveFailurePhaseRefreshAcyclicFoldFactorProjection
	case pointFoldBoundaryFactorTransportPreflight:
		return SolveFailurePhaseRefreshAcyclicFoldFactorTransportPreflight
	case pointFoldBoundaryFactorTransportCoordinatePreSupport:
		return SolveFailurePhaseRefreshAcyclicFoldFactorTransportCoordinatePreSupport
	case pointFoldBoundaryFactorTransportCoordinateReindexSupport:
		return SolveFailurePhaseRefreshAcyclicFoldFactorTransportCoordinateReindexSupport
	case pointFoldBoundaryFactorTransportCoordinatePostSupport:
		return SolveFailurePhaseRefreshAcyclicFoldFactorTransportCoordinatePostSupport
	case pointFoldBoundaryFactorTransportCoordinateCoverage:
		return SolveFailurePhaseRefreshAcyclicFoldFactorTransportCoordinateCoverage
	case pointFoldBoundaryFactorTransportCoordinateAdmission:
		return SolveFailurePhaseRefreshAcyclicFoldFactorTransportCoordinateAdmission
	case pointFoldBoundaryFactorTransportGeneralPreFilter:
		return SolveFailurePhaseRefreshAcyclicFoldFactorTransportGeneralPreFilter
	case pointFoldBoundaryFactorTransportGeneralReindexSupport:
		return SolveFailurePhaseRefreshAcyclicFoldFactorTransportGeneralReindexSupport
	case pointFoldBoundaryFactorTransportGeneralReindexTypedSlots:
		return SolveFailurePhaseRefreshAcyclicFoldFactorTransportGeneralReindexTypedSlots
	case pointFoldBoundaryFactorTransportGeneralReindexCommit:
		return SolveFailurePhaseRefreshAcyclicFoldFactorTransportGeneralReindexCommit
	case pointFoldBoundaryFactorTransportGeneralPostFilter:
		return SolveFailurePhaseRefreshAcyclicFoldFactorTransportGeneralPostFilter
	case pointFoldBoundaryFactorTransportGeneralCoverage:
		return SolveFailurePhaseRefreshAcyclicFoldFactorTransportGeneralCoverage
	case pointFoldBoundaryFactorTransportGeneralAdmission:
		return SolveFailurePhaseRefreshAcyclicFoldFactorTransportGeneralAdmission
	case pointFoldBoundaryFactorAdmission:
		return SolveFailurePhaseRefreshAcyclicFoldFactorAdmission
	case pointFoldBoundaryProducer:
		return SolveFailurePhaseRefreshAcyclicFoldProducer
	case pointFoldBoundaryFinish:
		return SolveFailurePhaseRefreshAcyclicFoldFinish
	default:
		return SolveFailurePhaseRefreshAcyclicFoldPoint
	}
}

func (epoch *executorEpoch) foldPointTerms(reference carrier.PointState, base carrier.PointRHS, environments, factors, groups []int, producerPoint equation.Point) (result carrier.PointRHS, ok bool) {
	result, _, ok = epoch.foldPointTermsWithBoundary(reference, base, environments, factors, groups, producerPoint)
	return result, ok
}

// foldPointTermsWithBoundary executes the same one-shot canonical fold as
// foldPointTerms while returning only its first failed transaction boundary.
// The marker is consumed immediately by refresh diagnostics and retains no
// Point, carrier state, or fold rows.
func (epoch *executorEpoch) foldPointTermsWithBoundary(reference carrier.PointState, base carrier.PointRHS, environments, factors, groups []int, producerPoint equation.Point) (result carrier.PointRHS, boundary pointFoldBoundary, ok bool) {
	if epoch != nil && epoch.diagnostics != nil {
		epoch.diagnostics.recordFold()
	}
	if epoch == nil || epoch.runtime == nil || epoch.work == nil || !epoch.work.OwnsPointState(reference) || !epoch.work.OwnsPointRHS(base) {
		return carrier.PointRHS{}, pointFoldBoundaryBegin, false
	}
	producerCount := len(groups)
	if producerPoint.Available() {
		producerCount = epoch.runtime.graph.ProducerCount(producerPoint)
	}
	if len(environments) == 0 && len(factors) == 0 && producerCount == 0 {
		return base, pointFoldBoundaryNone, true
	}
	if !epoch.work.BeginPointRHSFold(reference, base) {
		return carrier.PointRHS{}, pointFoldBoundaryBegin, false
	}
	active := true
	defer func() {
		if active {
			_ = epoch.work.AbortPointRHSFold()
		}
	}()
	boundary = pointFoldBoundaryEnvironment
	for _, edge := range environments {
		if !epoch.addPointFoldEnvironmentEdge(edge) {
			return carrier.PointRHS{}, boundary, false
		}
	}
	for _, edge := range factors {
		factorBoundary, factorOK := epoch.addPointFoldFactorEdgeWithBoundary(edge)
		if !factorOK {
			boundary = factorBoundary
			return carrier.PointRHS{}, boundary, false
		}
	}
	boundary = pointFoldBoundaryProducer
	if producerPoint.Available() {
		for index := 0; index < epoch.runtime.graph.ProducerCount(producerPoint); index++ {
			group, present := epoch.runtime.graph.ProducerAt(producerPoint, index)
			groupIndex, indexed := epoch.runtime.graph.GroupIndex(group)
			if !present || !indexed || !epoch.addPointFoldGroup(groupIndex) {
				return carrier.PointRHS{}, boundary, false
			}
		}
	} else {
		for _, group := range groups {
			if !epoch.addPointFoldGroup(group) {
				return carrier.PointRHS{}, boundary, false
			}
		}
	}
	boundary = pointFoldBoundaryFinish
	result, ok = epoch.work.FinishPointRHSFold()
	active = false
	return result, boundary, ok && !epoch.canceled()
}

// regionRHS keeps recurrence operands private until one exact carrier
// transition publishes the head. E includes Init and external producers; B
// starts at bottom and contains every back producer, including mixed Groups.
func (epoch *executorEpoch) regionRHS(point equation.Point, pointIndex int, region runtimeRegion, current carrier.PointState) (ingress, exact carrier.PointRHS, ok bool) {
	if epoch != nil && epoch.diagnostics != nil {
		epoch.diagnostics.recordRegionRHS()
	}
	if epoch == nil || !region.active || !epoch.work.OwnsPointState(current) {
		return carrier.PointRHS{}, carrier.PointRHS{}, false
	}
	base, ok := epoch.pointBase(point, pointIndex)
	if !ok {
		return carrier.PointRHS{}, carrier.PointRHS{}, false
	}
	ingress, ok = epoch.foldPointInputs(current, base, region.environmentExternal, region.factorExternal, region.external) // E = base \sqcup external ingress
	if !ok {
		return carrier.PointRHS{}, carrier.PointRHS{}, false
	}
	exact, ok = epoch.foldPointInputs(current, ingress, region.environmentBack, region.factorBack, region.back) // R = E \sqcup B
	if !ok || epoch.canceled() {
		return carrier.PointRHS{}, carrier.PointRHS{}, false
	}
	return ingress, exact, true
}

// regionSelected is the ordinary ascent widening surface. It intentionally
// retains X+B, not E+B: external ingress is already checked against the
// current head before this fold. A pending interface refresh uses its newly
// rebuilt exact R directly as selected, avoiding a redundant fold.
func (epoch *executorEpoch) regionSelected(current carrier.PointState, region runtimeRegion) (carrier.PointRHS, bool) {
	if epoch == nil || epoch.work == nil || !epoch.work.OwnsPointState(current) {
		return carrier.PointRHS{}, false
	}
	currentRHS, ok := epoch.work.PointRHSFromPointState(current)
	if !ok {
		return carrier.PointRHS{}, false
	}
	return epoch.foldPointInputs(current, currentRHS, region.environmentBack, region.factorBack, region.back) // P = X \sqcup B
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
		_ = epoch.invalidateRegionPostfix(region)
		return true
	}
	for index, point := range bound.faces {
		if point < 0 || point >= len(epoch.versions) || state.interfaces[index] != epoch.versions[point] {
			_ = epoch.invalidateRegionPostfix(region)
			return true
		}
	}
	for index, group := range bound.external {
		if group < 0 || group >= len(epoch.producers) || state.ingress[index] != epoch.producers[group].version {
			_ = epoch.invalidateRegionPostfix(region)
			return true
		}
	}
	for index, edge := range bound.environmentExternal {
		if edge < 0 || edge >= len(epoch.runtime.environments) || state.environmentIngress[index] != epoch.environmentVersion(edge) {
			_ = epoch.invalidateRegionPostfix(region)
			return true
		}
	}
	for index, edge := range bound.factorExternal {
		if edge < 0 || edge >= len(epoch.runtime.factorEdges) || state.factorIngress[index] != epoch.factorEdgeVersion(edge) {
			_ = epoch.invalidateRegionPostfix(region)
			return true
		}
	}
	return false
}

// beginRegionInterfaceRefresh opens the localized ascent barrier for one
// stale boundary. Existing publication routing has already woken every
// ordinary consumer (including raw State+C-only changes) and structural wake
// paths have already covered EnvironmentInput/edge rows. This barrier only
// prevents the head from refolding until those candidate generations settle;
// the authoritative interface snapshot remains untouched until publication.
func (epoch *executorEpoch) beginRegionInterfaceRefresh(region int) bool {
	if epoch == nil || !epoch.activeRegion(region) || region >= len(epoch.regions) || region >= len(epoch.runtime.regions) {
		return false
	}
	state := &epoch.regions[region]
	bound := epoch.runtime.regions[region]
	if !state.hasExact || state.interfaceRefreshPending {
		return state.interfaceRefreshPending
	}
	state.interfaceRefreshPending = true
	state.invalid = true
	if !epoch.invalidateRegionPostfix(region) || !epoch.markPostfixDirty(bound.head) || !epoch.enqueuePoint(bound.head) {
		return false
	}
	if epoch.diagnostics != nil {
		epoch.diagnostics.recordInterfaceRefreshBegin(epoch, region, epoch.interfaceRefreshChangedFaces(region))
	}
	return true
}

// interfaceRefreshChangedFaces is diagnostics-only evidence. Publication has
// already routed all consumers; this bounded version walk records only the
// stale face count without adding a second dependency structure or a hot-path
// allocation.
func (epoch *executorEpoch) interfaceRefreshChangedFaces(region int) uint64 {
	if epoch == nil || epoch.runtime == nil || epoch.runtime.graph == nil || !epoch.activeRegion(region) || region >= len(epoch.runtime.regions) || region >= len(epoch.regions) {
		return 0
	}
	bound, state := epoch.runtime.regions[region], epoch.regions[region]
	if len(bound.faces) != len(state.interfaces) {
		return 0
	}
	var changedFaces uint64
	for index, pointIndex := range bound.faces {
		if pointIndex < 0 || pointIndex >= len(epoch.versions) || state.interfaces[index] == epoch.versions[pointIndex] {
			continue
		}
		changedFaces++
	}
	return changedFaces
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
		_ = epoch.invalidateRegionPostfix(region)
		return true
	}
	for index, group := range bound.back {
		if group < 0 || group >= len(epoch.producers) || state.backIngress[index] != epoch.producers[group].version {
			_ = epoch.invalidateRegionPostfix(region)
			return true
		}
	}
	for index, edge := range bound.environmentBack {
		if edge < 0 || edge >= len(epoch.runtime.environments) || state.environmentBackIngress[index] != epoch.environmentVersion(edge) {
			_ = epoch.invalidateRegionPostfix(region)
			return true
		}
	}
	for index, edge := range bound.factorBack {
		if edge < 0 || edge >= len(epoch.runtime.factorEdges) || state.factorBackIngress[index] != epoch.factorEdgeVersion(edge) {
			_ = epoch.invalidateRegionPostfix(region)
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
	if epoch.diagnostics != nil {
		epoch.diagnostics.rememberRegionInterfaces(epoch, region)
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
		if !epoch.activeRegion(region) || !epoch.invalidateRegionPostfix(region) || !epoch.markPostfixDirty(epoch.runtime.regions[region].head) {
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
func (epoch *executorEpoch) restartRegion(region int, callSite SolveDiagnosticRestartCallSite, reason SolveDiagnosticRestartReason, pendingGroup int, pending carrier.RuleContribution) (ok bool) {
	var sample solveDiagnosticRestartSample
	if epoch != nil && epoch.diagnostics != nil && epoch.diagnostics.restartEnabled() {
		sample = epoch.diagnostics.beginRestart(epoch, region, callSite, reason, pendingGroup, pending)
		defer func() { epoch.diagnostics.finishRestart(sample, ok) }()
	}
	if epoch == nil || !epoch.activeRegion(region) || len(epoch.producers) != len(epoch.runtime.producers) {
		return false
	}
	subtree, subtreeOK := epoch.regionSubtree(region)
	if !subtreeOK {
		return false
	}
	for _, index := range subtree {
		if epoch.regions[index].episode == ^uint64(0) {
			return false
		}
	}
	for _, index := range subtree {
		// Every restarted descendant belongs to the new ascent episode. This
		// establishes parent-Ascent => descendant-Ascent before any local Point
		// or candidate can be queued.
		epoch.regions[index].phase = phaseAscent
		epoch.regions[index].episode++
		if epoch.diagnostics != nil {
			epoch.diagnostics.observeEpisode(epoch.regions[index].episode)
		}
		epoch.regions[index].exact = carrier.PointRHS{}
		epoch.regions[index].hasExact = false
		epoch.regions[index].exactInputsVersion = 0
		epoch.regions[index].exactRevision = 0
		epoch.regions[index].postfix = regionPostfixProof{}
		epoch.regions[index].invalid = true
		epoch.regions[index].interfaceRefreshPending = false
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
		if pointIndex < 0 || pointIndex >= len(epoch.points) || !epoch.work.OwnsPointState(epoch.points[pointIndex]) {
			return false
		}
		point, pointOK := epoch.runtime.graph.PointAt(schedule.Node(pointIndex))
		base, baseOK := epoch.pointBase(point, pointIndex)
		if !pointOK || !baseOK {
			return false
		}
		current := epoch.points[pointIndex]
		reset, changes, resetOK := epoch.work.ReplacePointWithRHS(current, base)
		if !resetOK || epoch.canceled() {
			return false
		}
		if epoch.diagnostics != nil && epoch.diagnostics.restartEnabled() {
			sample.resetPoints++
			representationChanged := !epoch.work.ExactSamePointRepresentation(current, reset)
			semanticChanged := !epoch.work.EqualPointState(current, reset)
			if representationChanged {
				sample.representationResets++
				if !semanticChanged {
					sample.representationOnlyResets++
				}
			}
			if semanticChanged {
				sample.semanticResets++
				if !current.Support().Equal(reset.Support()) {
					sample.semanticSupportResets++
				} else {
					sample.semanticValueResets++
				}
			}
		}
		if _, publishedOK := epoch.publish(pointIndex, current, reset, changes, publicationMayDescend); !publishedOK || epoch.canceled() {
			return false
		}
		if !epoch.invalidateStructuralInputs(pointIndex) {
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
			// Mark the old candidate pending before clearing applied.  If this
			// Group was settled, markDirty performs the clean->pending counter
			// transition; if it was already pending, the wake is deduplicated by
			// the same generation/applied relation.  Clearing applied first would
			// hide that transition and undercount every restarted ancestor.
			if !epoch.markDirty(groupIndex) {
				return false
			}
			if epoch.diagnostics != nil && epoch.diagnostics.restartEnabled() {
				sample.resetProducers++
			}
			cache.candidate = carrier.RuleContribution{}
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
		}
		if !epoch.markStructuralPoint(point) {
			return false
		}
	}
	return true
}

func (epoch *executorEpoch) regionCandidatesSettled(region int) bool {
	if epoch == nil || !epoch.activeRegion(region) || region >= len(epoch.candidatesPending) {
		return false
	}
	return epoch.candidatesPending[region] == 0
}

func (epoch *executorEpoch) recordPointDescent(point int) bool {
	if epoch == nil || point < 0 || point >= len(epoch.structural.pointDescent) || epoch.structural.descent == ^uint64(0) {
		return false
	}
	epoch.structural.descent++
	epoch.structural.pointDescent[point] = epoch.structural.descent
	return true
}

func (epoch *executorEpoch) publish(point int, current, next carrier.PointState, changes carrier.ChangeSet, order pointPublication) (bool, bool) {
	if epoch == nil || epoch.canceled() || point < 0 || point >= len(epoch.points) || point >= len(epoch.structural.pointDescent) || !epoch.work.OwnsPointState(current) || !epoch.work.OwnsPointState(next) || !epoch.runtime.carrier.OwnsChangeSet(changes) || order != publicationAscending && order != publicationMayDescend {
		return false, false
	}
	coverageChanges, coverageOK := epoch.work.CoverageWakeChangesPointStates(current, next)
	if !coverageOK {
		return false, false
	}
	semanticChanged := !epoch.work.EqualPointState(current, next)
	// A compact Target-row alias can preserve the lifted semantic Point while
	// replacing the exact State+C header. Structural consumers cache a source
	// version, not an extensional quotient, so that replacement must advance
	// the publication generation and wake their canonical refold. Otherwise a
	// later additive ticket could be replayed over the stale alias.
	changed := !epoch.work.ExactSamePointRepresentation(current, next)
	if epoch.diagnostics != nil {
		epoch.diagnostics.recordPublication(semanticChanged, changed)
	}
	epoch.points[point] = next
	var sourcePoint equation.Point
	if changed {
		epoch.versions[point]++
		if epoch.versions[point] == 0 {
			return false, false
		}
		if epoch.diagnostics != nil {
			epoch.diagnostics.recordVersionBump()
		}
		if order == publicationMayDescend && semanticChanged && !epoch.recordPointDescent(point) {
			return false, false
		}
		if !epoch.markPostfixDirty(point) || !epoch.invalidatePostfixAncestors(point) {
			return false, false
		}
		var sourceOK bool
		sourcePoint, sourceOK = epoch.runtime.graph.PointAt(schedule.Node(point))
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
	if epoch.diagnostics != nil {
		epoch.diagnostics.recordWakes(len(wakes), len(coverageWakes))
	}
	if changed && !epoch.markPublishedInputConsumers(sourcePoint) {
		return false, false
	}
	return changed, true
}

// publishAcyclicExact installs one already-complete PointRHS without deriving
// an old-to-new typed ChangeSet. Acyclic scheduling already owns the exact
// static consumer incidence: conservatively waking that row is semantically
// equivalent to routing every changed Unit, while avoiding a second FDD
// zipper solely for invalidation evidence. This changes scheduling precision,
// never abstract-state precision. Recurrence publication retains the exact
// ChangeSet path above for its phase/order proofs.
func (epoch *executorEpoch) publishAcyclicExact(point int, current, next carrier.PointState, order pointPublication) (bool, bool) {
	if epoch == nil || epoch.canceled() || point < 0 || point >= len(epoch.points) || point >= len(epoch.structural.pointDescent) ||
		!epoch.work.OwnsPointState(current) || !epoch.work.OwnsPointState(next) || order != publicationAscending && order != publicationMayDescend {
		return false, false
	}
	semanticChanged := !epoch.work.EqualPointState(current, next)
	// See publish: raw compact-C replacement is a versioned structural event
	// even when the lifted Point value is equal. The acyclic path has no typed
	// ChangeSet routing, so its conservative graph wake is the only barrier
	// that prevents a stale cursor from bridging the representation change.
	changed := !epoch.work.ExactSamePointRepresentation(current, next)
	if epoch.diagnostics != nil {
		epoch.diagnostics.recordPublication(semanticChanged, changed)
	}
	// Install the exact RHS representation even when it is observationally
	// equal under current support. Replace has the same law: a later support
	// growth must see the newly recomputed latent representation, not the old
	// PointState's hidden branch.
	epoch.points[point] = next
	if !changed {
		return semanticChanged, true
	}
	epoch.versions[point]++
	if epoch.versions[point] == 0 {
		return false, false
	}
	if epoch.diagnostics != nil {
		epoch.diagnostics.recordVersionBump()
	}
	if order == publicationMayDescend && semanticChanged && !epoch.recordPointDescent(point) {
		return false, false
	}
	sourcePoint, sourceOK := epoch.runtime.graph.PointAt(schedule.Node(point))
	if !sourceOK || !epoch.markStructuralSuccessors(sourcePoint) {
		return false, false
	}
	if !epoch.markPostfixDirty(point) || !epoch.invalidatePostfixAncestors(point) {
		return false, false
	}
	// Every exact or dynamic typed read of this Point belongs to one ordinary
	// graph consumer. Waking the canonical consumer row subsumes unit/factor
	// routing without inventing a parallel dependency graph. Clean-only wake is
	// safe after typed/coverage routing has already scheduled any pending row.
	if !epoch.markPublishedInputConsumers(sourcePoint) {
		return false, false
	}
	return true, true
}

// refreshPoint performs the only candidate replacement and sole Point
// publication. A region is admitted only for its head; nonheads exact-replace
// their complete RHS even while enclosed by the same WTO region.
func (epoch *executorEpoch) refreshPoint(point equation.Point, pointIndex, regionIndex int) (changed, ok bool) {
	refreshPhase := SolveFailurePhaseRefreshValidation
	defer func() {
		if !ok {
			epoch.recordRefreshPointFailure(refreshPhase, point)
		}
	}()
	if epoch == nil || epoch.canceled() || !point.Available() || pointIndex < 0 || pointIndex >= len(epoch.points) || pointIndex >= len(epoch.runtime.activePoints) || !epoch.runtime.activePoints[pointIndex] {
		return false, false
	}
	if epoch.diagnostics != nil {
		epoch.diagnostics.recordRefresh()
	}
	current := epoch.points[pointIndex]
	if !epoch.work.OwnsPointState(current) {
		return false, false
	}
	structuralChanged := pointIndex < len(epoch.structuralDirty) && epoch.structuralDirty[pointIndex]
	anyCandidateChanged := false
	candidateRequiresCanonicalFold := false
	// Outside recurrence, an ascending replacement c<=c' may be installed in
	// the existing exact point aggregate by joining the complete c'. If
	// X=base join rest join c, then X join c'=base join rest join c'. This is
	// the seminaive scalar law; it needs neither subtraction nor a retained
	// operand tree. Structural changes remain on the exact rebuild path below
	// because a source Point may have descended during a narrow episode.
	appended, appendedOK := epoch.work.PointRHSFromPointState(current)
	if !appendedOK {
		return false, false
	}
	refreshPhase = SolveFailurePhaseRefreshCandidate
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
		// The only acyclic append proof is carrier-owned. Let its success prove
		// both lifted order and raw compact-row additivity in one traversal; on
		// failure, run the ordinary order check once solely to distinguish a
		// lawful raw-alias replacement (canonical fold) from a broken Rule law.
		refreshPhase = SolveFailurePhaseRefreshCandidateOrder
		thisCandidateChanged := !state.hasValue
		candidateAppendable := true
		candidateOrdered := true
		if state.hasValue {
			thisCandidateChanged = !epoch.work.ExactSameRuleContributionRepresentation(state.candidate, next)
			if regionIndex == schedule.NoRegion {
				if thisCandidateChanged {
					candidateAppendable = epoch.work.CanAppendAscendingRuleContribution(state.candidate, next)
					candidateOrdered = candidateAppendable
					if !candidateOrdered {
						candidateOrdered = epoch.work.LessOrEqRuleContribution(state.candidate, next)
					}
				}
			} else {
				// Region RHS is always canonically rebuilt, so it needs only the
				// ordinary monotonicity law, not a raw-row append certificate.
				candidateOrdered = epoch.work.LessOrEqRuleContribution(state.candidate, next)
			}
		}
		environmentChanged := state.candidateEnvironmentToken != state.scratchEnvironmentToken
		if state.hasValue && !candidateOrdered {
			// A wake generation is not semantic evidence. A candidate decrease or
			// incomparability is lawful only while an unchanged narrow episode is
			// propagating its smaller exact head around an internal edge. During
			// ascent, unchanged interfaces imply that every changed input belongs to
			// the same ascending Kleene chain; a monotone Rule cannot then decrease.
			// Fail that Rule law closed instead of restarting the identical episode
			// forever. A genuinely changed external interface still begins a fresh
			// episode below.
			if (!state.hasCandidateTokens || sameCandidateTokens(state.candidateTokens, state.scratchTokens)) && !environmentChanged {
				refreshPhase = SolveFailurePhaseRefreshCandidateOrderStableInputs
				epoch.recordCandidateOrderFailure(refreshPhase, point, producer.group)
				return false, false
			}
			region := epoch.runtime.pointRegion[pointIndex]
			if region == schedule.NoRegion || !epoch.activeRegion(region) {
				refreshPhase = SolveFailurePhaseRefreshCandidateOrderRegion
				epoch.recordCandidateOrderFailure(refreshPhase, point, producer.group)
				return false, false
			}
			phase := epoch.regions[region].phase
			if phase != phaseAscent && phase != phaseNarrow {
				refreshPhase = SolveFailurePhaseRefreshCandidateOrderRegion
				epoch.recordCandidateOrderFailure(refreshPhase, point, producer.group)
				return false, false
			}
			if epoch.regionInterfacesChanged(region) {
				if !epoch.restartRegion(region, SolveDiagnosticRestartCandidateInterface, SolveDiagnosticRestartCandidateNotOrdered, groupIndex, next) {
					return false, false
				}
				return false, true
			}
			if phase != phaseNarrow {
				refreshPhase = SolveFailurePhaseRefreshCandidateOrderRegion
				epoch.recordCandidateOrderFailure(refreshPhase, point, producer.group)
				return false, false
			}
			// An unchanged narrow interface proves only where the wake came
			// from. It does not turn an incomparable Rule result into a
			// descent. Admit exactly next <= old; every other local result
			// fails closed instead of being published as an exact candidate.
			if !epoch.work.LessOrEqRuleContribution(next, state.candidate) {
				refreshPhase = SolveFailurePhaseRefreshCandidateOrderDescent
				epoch.recordCandidateOrderFailure(refreshPhase, point, producer.group)
				return false, false
			}
		}
		refreshPhase = SolveFailurePhaseRefreshDemandCommit
		if epoch.canceled() || !epoch.demand.Replace(groupIndex, reads) {
			return false, false
		}
		// Candidate identity is State+C, not merely its lifted extensional
		// value. An alias replacement can be semantically equal while changing
		// the compact row retained by the current Point RHS; appending the new
		// candidate to that old RHS would retain both aliases. The carrier owns
		// the raw-additivity proof, and a failure selects the existing canonical
		// fold rather than a compensating merge path.
		changed := thisCandidateChanged
		appendable := candidateAppendable
		if !epoch.updateCandidatesPending(pointIndex, -1) {
			return false, false
		}
		state.candidate, state.hasValue, state.applied = next, true, state.generation
		copy(state.candidateTokens, state.scratchTokens)
		state.hasCandidateTokens = true
		state.candidateEnvironmentToken = state.scratchEnvironmentToken
		if changed {
			state.version++
			if state.version == 0 {
				return false, false
			}
			// A dirty structural row takes the complete canonical fold below,
			// which already reads every freshly installed producer candidate.
			// Building an append-only RHS here would publish intermediate roots
			// only to discard them immediately before that exact reconstruction.
			if regionIndex == schedule.NoRegion && !structuralChanged && !appendable {
				candidateRequiresCanonicalFold = true
			}
			// Once any replacement is non-additive, ignore any partial append
			// already accumulated this refresh and rebuild in canonical input
			// order below. This is one Point RHS authority, not a side cache.
			if regionIndex == schedule.NoRegion && !structuralChanged && !candidateRequiresCanonicalFold {
				var merged bool
				appended, merged = epoch.work.AddRuleContribution(appended, next)
				if !merged || epoch.canceled() {
					return false, false
				}
			}
		}
		if epoch.canceled() {
			return false, false
		}
		anyCandidateChanged = anyCandidateChanged || changed
		refreshPhase = SolveFailurePhaseRefreshCandidate
	}
	if regionIndex == schedule.NoRegion && !anyCandidateChanged && !structuralChanged {
		refreshPhase = SolveFailurePhaseRefreshAcyclicPublication
		return false, epoch.settlePostfix(pointIndex)
	}
	if regionIndex == schedule.NoRegion {
		rhs := appended
		order := publicationAscending
		if structuralChanged || candidateRequiresCanonicalFold {
			refreshPhase = SolveFailurePhaseRefreshAcyclicStructuralInputs
			ascending, valid := epoch.structuralInputsAscending(pointIndex)
			if !valid {
				return false, false
			}
			refreshPhase = SolveFailurePhaseRefreshAcyclicPointBase
			base, ok := epoch.pointBase(point, pointIndex)
			if !ok {
				return false, false
			}
			refreshPhase = SolveFailurePhaseRefreshAcyclicFoldPoint
			foldBoundary := pointFoldBoundaryNone
			rhs, foldBoundary, ok = epoch.foldPointWithBoundary(current, base, point)
			if !ok {
				refreshPhase = refreshAcyclicFoldPhase(foldBoundary)
				return false, false
			}
			if !ascending && !epoch.work.LessOrEqPointStateRHS(current, rhs) {
				order = publicationMayDescend
			}
		}
		refreshPhase = SolveFailurePhaseRefreshAcyclicPublication
		selfDescent := epoch.structural.pointDescent[pointIndex]
		published, ok := epoch.work.PublishPointRHS(rhs)
		if !ok || epoch.canceled() {
			return false, false
		}
		changed, publishedOK := epoch.publishAcyclicExact(pointIndex, current, published, order)
		if !publishedOK || !epoch.settlePostfix(pointIndex) {
			return false, false
		}
		if structuralChanged && !epoch.rememberStructuralInputs(pointIndex, selfDescent) {
			return false, false
		}
		epoch.structuralDirty[pointIndex] = false
		return changed, true
	}
	refreshPhase = SolveFailurePhaseRefreshRegionInterface
	if !epoch.activeRegion(regionIndex) || epoch.runtime.regions[regionIndex].head != pointIndex {
		return false, false
	}
	episode := &epoch.regions[regionIndex]
	phase := episode.phase
	if phase != phaseAscent && phase != phaseNarrow {
		return false, false
	}
	interfacesChanged := epoch.regionInterfacesChanged(regionIndex)
	if interfacesChanged {
		if phase == phaseAscent && episode.hasExact {
			// A stale ascent boundary is a localized refresh, not automatically a
			// new exact episode. Publication routing has already dirtied ordinary
			// consumers; the barrier waits for their candidates before rebuilding E/R.
			if !episode.interfaceRefreshPending && !epoch.beginRegionInterfaceRefresh(regionIndex) {
				return false, false
			}
		} else {
			if !epoch.restartRegion(regionIndex, SolveDiagnosticRestartHeadInterface, SolveDiagnosticRestartInterfaceChanged, -1, carrier.RuleContribution{}) {
				return false, false
			}
			return false, true
		}
	}
	exactInputsChanged := episode.hasExact && epoch.regionExactInputsChanged(regionIndex)
	if episode.invalid {
		if !epoch.regionCandidatesSettled(regionIndex) {
			return epoch.enqueuePoint(pointIndex), true
		}
		episode.invalid = false
	}
	region := epoch.runtime.regions[regionIndex]
	// An unchanged ascent RHS is already represented by episode.exact and the
	// current widened Point. Re-running Widen would only rebuild the same roots
	// and coverage before proving the same postfix relation again.
	if phase == phaseAscent && episode.hasExact && !exactInputsChanged {
		epoch.structuralDirty[pointIndex] = false
		return false, epoch.settlePostfix(pointIndex)
	}
	refreshPending := phase == phaseAscent && episode.hasExact && episode.interfaceRefreshPending
	refreshOldExact := episode.exact
	var ingress, exact, selected carrier.PointRHS
	var exactOK bool
	structuralFolded := false
	refreshPhase = SolveFailurePhaseRefreshRegionRHS
	if phase == phaseAscent && episode.hasExact {
		// A changed Region must rebuild its complete E+B carrier in canonical
		// input order. Reusing episode.exact and appending changed back Groups
		// loses the exact compact Target-row surface when another contributor
		// expands it, so the acyclic ticket proof intentionally does not cross
		// this recurrence boundary.
		ingress, exact, exactOK = epoch.regionRHS(point, pointIndex, region, current)
		structuralFolded = exactOK
		if exactOK {
			if refreshPending {
				// The carrier law for an ascending cached exact proves
				// Widen(current, R, R) preserves current and carries R. Avoid
				// rebuilding the ordinary X+B selected fold in this refresh.
				selected = exact
			} else {
				selected, exactOK = epoch.regionSelected(current, region)
			}
		}
	} else if phase == phaseNarrow && episode.hasExact && !exactInputsChanged {
		// Narrow may need several semantic descents against one unchanged exact
		// RHS. Reuse that owner-issued carrier rather than reconstructing E+B on
		// every narrow step.
		exact, selected, exactOK = episode.exact, episode.exact, true
	} else {
		ingress, exact, exactOK = epoch.regionRHS(point, pointIndex, region, current)
		structuralFolded = exactOK
	}
	if !exactOK || epoch.canceled() {
		return false, false
	}
	refreshPhase = SolveFailurePhaseRefreshRegionOrder
	if phase == phaseAscent && episode.hasExact && !episode.interfaceRefreshPending && !epoch.work.LessOrEqPointRHSPoint(ingress, current) {
		// New Init/external meaning begins a fresh episode before an inherited
		// widening step can observe a stale current head.
		if !epoch.restartRegion(regionIndex, SolveDiagnosticRestartAscentIngress, SolveDiagnosticRestartIngressNotBelowCurrent, -1, carrier.RuleContribution{}) {
			return false, false
		}
		return false, true
	}
	if phase == phaseAscent && episode.hasExact && !epoch.work.LessOrEqPointRHS(episode.exact, exact) {
		// An interface refresh may continue only when its complete exact RHS
		// grows from the cached episode RHS. A decrease or incomparable result
		// is a genuine non-monotone boundary. Only a pending interface refresh
		// has enough fresh-boundary evidence to restart; an unchanged episode
		// retains the existing fail-closed Rule-law behavior.
		if !refreshPending {
			return false, false
		}
		if epoch.diagnostics != nil {
			epoch.diagnostics.recordInterfaceRefreshOutcome(epoch, regionIndex, refreshOldExact, exact, false, true)
		}
		if !epoch.restartRegion(regionIndex, SolveDiagnosticRestartAscentIngress, SolveDiagnosticRestartExactIncomparable, -1, carrier.RuleContribution{}) {
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
		if !epoch.work.EqualPointRHS(episode.exact, exact) && !epoch.work.LessOrEqPointRHS(exact, episode.exact) {
			if !epoch.restartRegion(regionIndex, SolveDiagnosticRestartNarrowExact, SolveDiagnosticRestartExactIncomparable, -1, carrier.RuleContribution{}) {
				return false, false
			}
			return false, true
		}
		if !epoch.work.LessOrEqPointRHSPoint(exact, current) {
			if !epoch.restartRegion(regionIndex, SolveDiagnosticRestartNarrowCurrent, SolveDiagnosticRestartExactNotBelowCurrent, -1, carrier.RuleContribution{}) {
				return false, false
			}
			return false, true
		}
	}
	refreshPhase = SolveFailurePhaseRefreshRegionMerge
	var published carrier.PointState
	var changes carrier.ChangeSet
	var publishedOK bool
	if phase == phaseAscent && !episode.hasExact {
		published, changes, publishedOK = epoch.work.ReplacePointWithRHS(current, exact)
	} else if phase == phaseAscent {
		published, changes, publishedOK = epoch.work.MergeSelectedPointState(carrier.Widen, current, selected, exact, region.widen)
	} else {
		published, changes, publishedOK = epoch.work.MergeSelectedPointState(carrier.Narrow, current, exact, exact, region.narrow)
	}
	if !publishedOK || epoch.canceled() {
		return false, false
	}
	refreshPhase = SolveFailurePhaseRefreshRegionPublication
	if !episode.nextExactRevision() {
		return false, false
	}
	if !episode.hasExact || exactInputsChanged {
		episode.exactInputsVersion++
		if episode.exactInputsVersion == 0 {
			return false, false
		}
	}
	order := publicationAscending
	if phase == phaseNarrow {
		order = publicationMayDescend
	}
	selfDescent := epoch.structural.pointDescent[pointIndex]
	changed, publishedOK = epoch.publish(pointIndex, current, published, changes, order)
	if !publishedOK || epoch.canceled() {
		return false, false
	}
	episode.exact, episode.hasExact = exact, true
	if epoch.canceled() || !epoch.rememberRegionInterfaces(regionIndex) {
		return false, false
	}
	episode.interfaceRefreshPending = false
	if refreshPending && epoch.diagnostics != nil {
		// A refresh is complete only after the new head publication and its
		// authoritative interface/version snapshot both succeed.
		epoch.diagnostics.recordInterfaceRefreshOutcome(epoch, regionIndex, refreshOldExact, exact, true, false)
	}
	if structuralFolded && !epoch.rememberStructuralInputs(pointIndex, selfDescent) {
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
		_ = epoch.invalidateRegionPostfix(regionIndex)
		return false, epoch.enqueuePoint(region.head)
	}
	if !episode.hasExact || epoch.regionExactInputsChanged(regionIndex) {
		_ = epoch.invalidateRegionPostfix(regionIndex)
		return false, epoch.enqueuePoint(region.head)
	}
	_, headOK := epoch.runtime.graph.PointAt(schedule.Node(region.head))
	if region.head < 0 || region.head >= len(epoch.points) || region.head >= len(epoch.versions) {
		return false, false
	}
	current := epoch.points[region.head]
	if !headOK || !epoch.work.OwnsPointState(current) || !epoch.work.OwnsPointRHS(episode.exact) {
		return false, false
	}
	if epoch.regionPostfixProved(regionIndex) {
		return true, epoch.settlePostfix(region.head)
	}
	exact := episode.exact
	if !epoch.work.LessOrEqPointRHSPoint(exact, current) {
		if phase == phaseNarrow {
			if !epoch.restartRegion(regionIndex, SolveDiagnosticRestartPostfixExact, SolveDiagnosticRestartExactNotBelowCurrent, -1, carrier.RuleContribution{}) {
				return false, false
			}
			return false, true
		}
		return false, epoch.enqueuePoint(region.head)
	}
	if !epoch.rememberRegionPostfix(regionIndex) {
		return false, false
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
		rhs, rhsOK := epoch.foldPoint(current, base, point)
		if !baseOK || !rhsOK || !epoch.work.OwnsPointState(current) {
			return false, false
		}
		if !epoch.work.LessOrEqPointRHSPoint(rhs, current) || !epoch.work.LessOrEqPointStateRHS(current, rhs) {
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
			episode.postfix = regionPostfixProof{}
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
	if epoch.diagnostics != nil {
		epoch.diagnostics.recordPass(len(epoch.frames))
	}
	frames := epoch.frames[:0]
	defer func() { epoch.frames = frames[:0] }()
	for index := 0; index < epoch.runtime.executionEventCount(); index++ {
		if epoch.canceled() {
			return false, false
		}
		event, eventOK := epoch.runtime.executionEventAt(index)
		if !eventOK {
			return false, false
		}
		switch event.Kind {
		case schedule.EventEnter:
			region, regionOK := epoch.runtime.executionRegionAt(event.Region)
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
			region, regionOK := epoch.runtime.executionRegionAt(event.Region)
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
	for epoch != nil && epoch.checkpoint() {
		for epoch.queue.pending() {
			visited, ok := epoch.visitPoints()
			if epoch.canceled() {
				return false
			}
			if !ok {
				epoch.recordRunFailure(SolveFailurePhaseRunVisitInvalid)
				return false
			}
			if !visited {
				epoch.recordRunFailure(SolveFailurePhaseRunVisitNoProgress)
				return false
			}
		}
		postfixed, ok := epoch.demandedPostfix()
		if epoch.canceled() {
			return false
		}
		if !ok {
			epoch.recordRunFailure(SolveFailurePhaseRunPostfixInvalid)
			return false
		}
		if !postfixed {
			if !epoch.queue.pending() {
				epoch.recordRunFailure(SolveFailurePhaseRunPostfixStalled)
				return false
			}
			continue
		}
		if epoch.allRegionsNarrow() {
			return true
		}
		advanced, advancedOK := epoch.advanceNarrow()
		if !advancedOK {
			epoch.recordRunFailure(SolveFailurePhaseRunNarrowInvalid)
			return false
		}
		if !advanced {
			epoch.recordRunFailure(SolveFailurePhaseRunNarrowNoProgress)
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
	if state.completion == nil || state.completion.store != solver.store || !state.completion.serial.Available() || state.completion.serial != solver.completion || state.completion.relation != solver.relation.Generation() {
		return nil
	}
	return state
}

// publishCompleted is the one terminal publication cut.  Its inputs have
// already passed every fallible operation while epoch is Running: query
// materialization, retention of the new root arena, and eviction of any prior
// lease.  Once complete wins, these assignments are deliberately infallible;
// cancellation after that cut is non-operative.
func (solver *Solver) publishCompleted(epoch *executorEpoch, runtime *solverRuntime, state *State, completion identity.Generation, retained *carrier.RetainedWork) bool {
	if solver == nil || epoch == nil || runtime == nil || state == nil || retained == nil || solver.runtime != runtime || state.completion == nil || state.completion.store != solver.store || state.completion.serial != completion || state.completion.relation != solver.relation.Generation() || !epoch.complete() {
		return false
	}
	runtime.retained = retained
	solver.completion = completion
	runtime.completed = state
	return true
}

// Solve executes runtime revisions iteratively. A structural-only acyclic
// activation can extend a settled total-demand epoch in place; every other
// activation still takes the canonical compiler path from Init.
func (solver *Solver) Solve(ctx context.Context) (state *State, status SolveStatus) {
	return solver.solve(ctx, nil, nil)
}

// SolveWithReport uses the same solve implementation as Solve and returns a
// detached first-failure certificate only when that call is incomplete. The
// report is call-local; the Solver never retains it.
func (solver *Solver) SolveWithReport(ctx context.Context) (state *State, status SolveStatus, report SolveReport) {
	state, status = solver.solve(ctx, &report, nil)
	return state, status, report
}

// SolveWithDiagnostics executes one solve with a detached, bounded aggregate
// collector and its existing first-incomplete certificate. A zero flag
// selection avoids aggregate collection but still returns that scalar
// certificate if the single solve is incomplete. Invalid options return
// SolveInvalid with an empty snapshot before any solver work begins.
func (solver *Solver) SolveWithDiagnostics(ctx context.Context, options SolveDiagnosticOptions) (state *State, status SolveStatus, diagnostics SolveDiagnostics) {
	if !options.Valid() {
		return nil, SolveInvalid, SolveDiagnostics{}
	}
	collector := newSolveDiagnosticState(options)
	var failure SolveReport
	state, status = solver.solve(ctx, &failure, collector)
	if status == SolveCanceled && collector != nil {
		collector.clearCutoff()
	}
	if collector != nil {
		diagnostics = collector.snapshot()
	}
	diagnostics.Failure = failure
	return state, status, diagnostics
}

// solve is the one execution route. report is nil for ordinary Solve, keeping
// its successful and failure paths free of diagnostic allocation.
func (solver *Solver) solve(ctx context.Context, report *SolveReport, diagnostics *solveDiagnosticState) (state *State, status SolveStatus) {
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
		if diagnostics != nil && diagnostics.cutoff {
			// Work cutoff is an intentional diagnostic stop, not a semantic
			// execution defect; it must not publish a false failure witness.
			*report = SolveReport{}
			return
		}
		if status == SolveIncomplete {
			if !report.Available() {
				report.record(SolveFailureReasonExecution, SolveFailurePhaseNone, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{})
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
runtimeRevisions:
	for {
		runtime := solver.runtime
		if runtime == nil {
			return nil, SolveCanceled
		}
		if state = solver.completedState(runtime); state != nil {
			return state, SolveComplete
		}
		epoch, ok := newRuntimeEpoch(runtime, solver.relation, ctx)
		if !ok {
			if ctx.Err() != nil {
				return nil, SolveCanceled
			}
			if report != nil {
				report.record(SolveFailureReasonEpoch, SolveFailurePhaseNone, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{})
			}
			return nil, SolveIncomplete
		}
		epoch.report = report
		epoch.diagnostics = diagnostics
		epoch.diagnosticRevision = solver.relation.Generation()
		if diagnostics != nil && !epoch.work.SetCheckpoint(epoch.diagnosticCheckpoint) {
			epoch.incomplete()
			epoch.discard()
			if ctx.Err() != nil {
				return nil, SolveCanceled
			}
			return nil, SolveIncomplete
		}
		if diagnostics != nil {
			diagnostics.epochStarted(epoch, solver.relation.Generation())
		}
		current = epoch
		for {
			if !epoch.run() {
				epoch.incomplete()
				epoch.discard()
				if ctx.Err() != nil {
					return nil, SolveCanceled
				}
				if diagnostics != nil && diagnostics.cutoff {
					return nil, SolveIncomplete
				}
				if report != nil {
					report.record(SolveFailureReasonExecution, SolveFailurePhaseNone, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{})
				}
				return nil, SolveIncomplete
			}
			if !epoch.activationPending {
				break
			}
			frontier, canonical := canonicalizeAcceptedActivations(runtime.topology, epoch.activations)
			if !canonical {
				epoch.incomplete()
				epoch.discard()
				if report != nil {
					report.record(SolveFailureReasonActivationMerge, SolveFailurePhaseNone, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{})
				}
				return nil, SolveIncomplete
			}
			delta, subtracted := subtractAcceptedActivations(runtime.topology, frontier, solver.relation.Rows())
			if !subtracted {
				epoch.incomplete()
				epoch.discard()
				if report != nil {
					report.record(SolveFailureReasonActivationMerge, SolveFailurePhaseNone, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{})
				}
				return nil, SolveIncomplete
			}
			epoch.activations = nil
			epoch.activationPending = false
			if len(delta) == 0 {
				// Every observed Member and premise is already represented by the
				// committed relation. Keep this completed epoch for publication.
				break
			}
			accepted, merged := mergeAcceptedActivations(runtime.topology, solver.relation.Rows(), delta)
			if !merged || sameAcceptedActivations(solver.relation.Rows(), accepted) || !runtime.topology.ValidAccepted(accepted) {
				epoch.incomplete()
				epoch.discard()
				if report != nil {
					report.record(SolveFailureReasonActivationMerge, SolveFailurePhaseNone, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{})
				}
				return nil, SolveIncomplete
			}
			// The next relation is derived exactly once, here: Publish stamps the
			// following Generation and stores the structural digest derived for
			// it. Both installation paths below consume that one publication and
			// neither re-derives its identity. A saturated stamp fails closed.
			published, publishedOK := runtime.topology.Publish(solver.relation, accepted)
			// The live path is deliberately narrower than activation validity: it
			// must prove total demand, no recurrence, structural-only FactorEdges,
			// and an acyclic combined dependency relation. A false result is not a
			// weakened solve; it selects the unchanged cold compiler below.
			if publishedOK {
				overlay, preparedOverlay := runtime.prepareSelectedFactorOverlay(delta, published)
				installedOverlay := preparedOverlay && overlay != nil && epoch.installSelectedFactorOverlay(overlay)
				if installedOverlay {
					solver.relation = published
					epoch.diagnosticRevision = published.Generation()

					if diagnostics != nil {
						diagnostics.observeRevision(published.Generation())
						diagnostics.resetRevisionEvidence()
					}
					if ctx.Err() != nil {
						epoch.incomplete()
						epoch.discard()
						current = nil
						return nil, SolveCanceled
					}
					continue
				}
			}

			epoch.incomplete()
			epoch.discard()
			current = nil
			if ctx.Err() != nil {
				return nil, SolveCanceled
			}
			if !publishedOK {
				if report != nil {
					report.record(SolveFailureReasonActivationRevisionOverflow, SolveFailurePhaseNone, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{})
				}
				return nil, SolveIncomplete
			}
			rebuilt, phase, built := solver.compiler.compile(published)
			if !built || rebuilt == nil {
				if report != nil {
					report.record(SolveFailureReasonActivationCompile, phase, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{})
				}
				return nil, SolveIncomplete
			}
			if ctx.Err() != nil {
				return nil, SolveCanceled
			}
			runtime.completed = nil
			if runtime.retained != nil && !runtime.retained.Close() {
				if report != nil {
					report.record(SolveFailureReasonActivationRetainedClose, SolveFailurePhaseNone, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{})
				}
				return nil, SolveIncomplete
			}
			runtime.retained = nil
			solver.relation = published
			solver.runtime = rebuilt

			if diagnostics != nil {
				diagnostics.observeRevision(published.Generation())
				diagnostics.resetRevisionEvidence()
			}
			if ctx.Err() != nil {
				return nil, SolveCanceled
			}
			continue runtimeRevisions
		}
		results := make([]*queryResult, len(runtime.queries))
		for index, query := range runtime.queries {
			if epoch.canceled() {
				epoch.incomplete()
				epoch.discard()
				return nil, SolveCanceled
			}
			owner := query.queryOwner()
			if owner == nil || !owner.validQueryOwner(runtime, query.query()) || !query.query().Key().Available() || index < 0 || index >= len(results) {
				epoch.incomplete()
				epoch.discard()
				reportFailureQuery(report, SolveFailureReasonQuery, identity.SemanticKey{})
				return nil, SolveIncomplete
			}
			if results[index] != nil {
				epoch.incomplete()
				epoch.discard()
				reportFailureQuery(report, SolveFailureReasonQuery, identity.SemanticKey{})
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
			if result == nil || result.owner != owner || result.key != query.query().Key() {
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
				reportFailureQuery(report, SolveFailureReasonQuery, identity.SemanticKey{})
				return nil, SolveIncomplete
			}
		}
		observationResults := make([]*observationResult, len(runtime.observations))
		for index, observation := range runtime.observations {
			if epoch.canceled() {
				epoch.incomplete()
				epoch.discard()
				return nil, SolveCanceled
			}
			if observation == nil || index < 0 || index >= len(observationResults) {
				epoch.incomplete()
				epoch.discard()
				reportFailureQuery(report, SolveFailureReasonQuery, identity.SemanticKey{})
				return nil, SolveIncomplete
			}
			owner, id, point := observation.observationOwner(), observation.observationID(), observation.observationPoint()
			pointIndex, indexed := runtime.graph.PointIndex(point)
			if owner == nil || !owner.valid(runtime) || !id.Available() || !indexed || pointIndex < 0 || pointIndex >= len(epoch.points) {
				epoch.incomplete()
				epoch.discard()
				reportFailureQuery(report, SolveFailureReasonQuery, semanticKeyFromComposition(point.Key()))
				return nil, SolveIncomplete
			}
			result, observationPhase, ok := observation.materializeObservation(epoch.work, epoch.points[pointIndex].State())
			if !ok || result == nil || result.owner != owner || result.id != id || result.value == nil {
				if epoch.canceled() {
					epoch.incomplete()
					epoch.discard()
					return nil, SolveCanceled
				}
				epoch.incomplete()
				epoch.discard()
				if report != nil {
					report.record(SolveFailureReasonQuery, observationPhase, semanticKeyFromComposition(point.Key()), identity.SemanticKey{}, identity.SemanticKey{}, identity.SemanticKey{})
				}
				return nil, SolveIncomplete
			}
			observationResults[index] = result
		}
		for _, result := range observationResults {
			if result == nil {
				epoch.incomplete()
				epoch.discard()
				reportFailureQuery(report, SolveFailureReasonQuery, identity.SemanticKey{})
				return nil, SolveIncomplete
			}
		}
		if epoch.canceled() {
			epoch.incomplete()
			epoch.discard()
			return nil, SolveCanceled
		}
		nextCompletion := solver.completion.Next()
		if !nextCompletion.Available() {
			epoch.incomplete()
			epoch.discard()
			reportFailureQuery(report, SolveFailureReasonPublication, identity.SemanticKey{})
			return nil, SolveIncomplete
		}
		state = &State{completion: &completionAuthority{store: solver.store, serial: nextCompletion, relation: solver.relation.Generation()}, results: results, observations: observationResults}
		// Retain and eviction are preparation, not publication.  They must
		// finish while cancellation can still win the epoch terminal race.
		retained, retainedOK := epoch.work.Retain()
		if !retainedOK {
			epoch.discard()
			reportFailureQuery(report, SolveFailureReasonPublication, identity.SemanticKey{})
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
			reportFailureQuery(report, SolveFailureReasonPublication, identity.SemanticKey{})
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
