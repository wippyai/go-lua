// runtime_epoch.go holds the executor epoch vocabulary, the point queue, the failure recorders and epoch lifecycle.

package engine

import (
	"context"
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
	storePub          *solvedPublication
}

func (epoch *executorEpoch) recordFailure(reason SolveFailureReason, boundary solveBoundary, point, group, member, rule composition.Key) {
	if epoch == nil || epoch.report == nil {
		return
	}
	epoch.report.record(reason, boundary, reportedSemanticKey(point), reportedSemanticKey(group), reportedSemanticKey(member), reportedSemanticKey(rule))
}

func (epoch *executorEpoch) recordPointFailure(reason SolveFailureReason, point equation.Point) {
	if epoch == nil || !point.Available() {
		return
	}
	epoch.recordFailure(reason, boundaryNone, point.Key(), composition.Key{}, composition.Key{}, composition.Key{})
}

// recordRefreshPointFailure is the point-level fallback for refreshPoint.
// Member/group failures record their more specific certificate first, so this
// deliberately relies on SolveReport's first-wins record boundary.
func (epoch *executorEpoch) recordRefreshPointFailure(boundary solveBoundary, point equation.Point) {
	if epoch == nil {
		return
	}
	pointKey := composition.Key{}
	if point.Available() {
		pointKey = point.Key()
	}
	epoch.recordFailure(SolveFailureReasonExecution, boundary, pointKey, composition.Key{}, composition.Key{}, composition.Key{})
}

// recordRunFailure records a closed executor-loop boundary with no invented
// Point/Group coordinate. Cancellation and diagnostic cutoff are terminal
// statuses, not execution failures, and are intentionally handled by run's
// callers without reaching this helper.
func (epoch *executorEpoch) recordRunFailure(boundary solveBoundary) {
	if epoch == nil || epoch.canceled() {
		return
	}
	epoch.recordFailure(SolveFailureReasonExecution, boundary, composition.Key{}, composition.Key{}, composition.Key{}, composition.Key{})
}

func (epoch *executorEpoch) recordGroupFailure(reason SolveFailureReason, point equation.Point, group equation.GroupNode) {
	if epoch == nil {
		return
	}
	epoch.recordFailure(reason, boundaryNone, func() composition.Key {
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
func (epoch *executorEpoch) recordCandidateOrderFailure(boundary solveBoundary, point equation.Point, group equation.GroupNode) {
	if epoch == nil {
		return
	}
	if group.MemberCount() == 1 {
		if member, ok := group.MemberAt(0); ok {
			epoch.recordMemberFailure(SolveFailureReasonExecution, boundary, point, group, member)
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
	epoch.recordFailure(SolveFailureReasonExecution, boundary, pointKey, groupKey, composition.Key{}, composition.Key{})
}

func (epoch *executorEpoch) recordMemberFailure(reason SolveFailureReason, boundary solveBoundary, point equation.Point, group equation.GroupNode, member equation.RuleMember) {
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
	epoch.recordFailure(reason, boundary, pointKey, groupKey, memberKey, ruleKey)
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
	selectedPoints := make([]int, 0, len(runtime.activePoints))
	for pointIndex, active := range runtime.activePoints {
		if active {
			selectedPoints = append(selectedPoints, pointIndex)
		}
	}
	demandEpoch, ok = demand.OpenSelected(runtime.demand, selectedPoints)
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
		epoch.regions[index].ingress = make([]uint64, len(region.external))
		epoch.regions[index].backIngress = make([]uint64, len(region.back))
		epoch.regions[index].environmentIngress = make([]uint64, len(region.environmentExternal))
		epoch.regions[index].environmentBackIngress = make([]uint64, len(region.environmentBack))
		epoch.regions[index].factorIngress = make([]uint64, len(region.factorExternal))
		epoch.regions[index].factorBackIngress = make([]uint64, len(region.factorBack))
		epoch.regions[index].snapshot = make([]uint64, len(region.points))
	}
	for pointIndex, active := range runtime.activePoints {
		if !active {
			continue
		}
		point, pointOK := runtime.graph.PointAt(schedule.Node(pointIndex))
		if !pointOK || pointIndex < 0 || pointIndex >= len(epoch.points) {
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
				epoch.producers[groupIndex] = producerEpoch{generation: 1, candidateTokens: make([]uint64, inputCount), scratchTokens: make([]uint64, inputCount), inputs: make([]carrier.PointState, inputCount), inputStates: make([]carrier.State, inputCount), patches: make([]carrier.Patch, 0, metadata.span.count()), patchRows: make([]contributionPatch, 0, metadata.span.count()), reads: make([]demand.Observation, 0, len(metadata.reads))}
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
	epoch.storePub = nil
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
