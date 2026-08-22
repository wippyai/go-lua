// runtime_epoch_queue.go marks dirty and structural work, proves postfix and moves points through the queue.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
)

// markPostfixDirty records the one Point whose established postfix proof may
// no longer be reused. The row is deduplicated: a producer can be woken many
// times before its Point gets another opportunity to prove its complete RHS.
func (epoch *executorEpoch) markPostfixDirty(point int) bool {
	if epoch == nil || epoch.runtime == nil || point < 0 || point >= len(epoch.postfixDirty) || !epoch.activeState(point) {
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

// markStructuralState is the compact-state counterpart used by the mounted
// acyclic vertical. It proves the underlying graph Point once, but records the
// wake on the exact StateOrdinal row so two contexts cannot share a structural
// dirty bit. Source rows are resolved later from the retained execution plan.
func (epoch *executorEpoch) markStructuralState(state int, point equation.Point) bool {
	if epoch == nil || epoch.runtime == nil || epoch.runtime.graph == nil || !point.Available() || !epoch.activeState(state) || state < 0 || state >= len(epoch.structuralDirty) {
		return false
	}
	pointIndex, indexed := epoch.runtime.graph.PointIndex(point)
	if !indexed || pointIndex < 0 || pointIndex >= epoch.runtime.graph.PointCount() {
		return false
	}
	contextIncoming := state >= 0 && state < len(epoch.runtime.contextTransportIncoming) && len(epoch.runtime.contextTransportIncoming[state]) != 0
	if len(epoch.runtime.environmentIncoming[pointIndex]) == 0 && ((!epoch.runtime.artifactBacked && len(epoch.runtime.factorIncoming[pointIndex]) == 0) || (epoch.runtime.artifactBacked && (state < 0 || state >= len(epoch.runtime.stateFactorIncoming) || len(epoch.runtime.stateFactorIncoming[state]) == 0) && !contextIncoming)) {
		return true
	}
	epoch.structuralDirty[state] = true
	return epoch.markPostfixDirty(state) && epoch.enqueuePoint(state)
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

// markStructuralSuccessorsState is the compact acyclic counterpart. Every
// graph edge is resolved in the source state's context through executionPlan;
// no graph Point row is used as a mutable target address.
func (epoch *executorEpoch) markStructuralSuccessorsState(sourceState int, source equation.Point) bool {
	if epoch == nil || epoch.runtime == nil || !epoch.runtime.artifactBacked || epoch.runtime.graph == nil || !epoch.activeState(sourceState) || !source.Available() {
		return false
	}
	graph := epoch.runtime.graph
	_, sourcePointIndex, sourceContext, sourceOK := epoch.runtime.graphPointAtState(sourceState)
	if !sourceOK {
		return false
	}
	providedSourceIndex, providedSourceOK := graph.PointIndex(source)
	if !providedSourceOK || providedSourceIndex != sourcePointIndex {
		return false
	}
	for index := 0; index < graph.EnvironmentOutgoingCount(source); index++ {
		edge, ok := graph.EnvironmentOutgoingAt(source, index)
		if !ok {
			return false
		}
		if edge.TransportOnly() {
			continue
		}
		targetIndex, indexed := graph.PointIndex(edge.Target())
		if !indexed {
			return false
		}
		targetStates, stateOK := epoch.runtime.stateRowsForGraphPoint(sourceState, targetIndex)
		if !stateOK {
			return false
		}
		for _, targetState := range targetStates {
			if epoch.runtime.activeState(targetState) && !epoch.markStructuralState(targetState, edge.Target()) {
				return false
			}
		}
	}
	if sourceState < 0 || sourceState >= len(epoch.runtime.stateFactorOutgoing) {
		return false
	}
	for _, rowIndex := range epoch.runtime.stateFactorOutgoing[sourceState] {
		if rowIndex < 0 || rowIndex >= len(epoch.runtime.stateFactorRows) {
			return false
		}
		row := epoch.runtime.stateFactorRows[rowIndex]
		if row.edge < 0 || row.edge >= len(epoch.runtime.factorEdges) || row.source != sourceState {
			return false
		}
		edge := epoch.runtime.factorEdges[row.edge]
		target, targetOK := graph.PointAt(schedule.Node(edge.target))
		if !targetOK || row.target < 0 || row.target >= len(epoch.points) {
			return false
		}
		if epoch.runtime.activeState(row.target) && !epoch.markStructuralState(row.target, target) {
			return false
		}
	}
	// Context transports carry an exact source PointState to one authenticated
	// target StateOrdinal. Their wake is a direct state edge; no graph-point
	// fan-out or module/context inference is allowed.
	if sourceState >= 0 && sourceState < len(epoch.runtime.contextTransportOutgoing) {
		for _, transportIndex := range epoch.runtime.contextTransportOutgoing[sourceState] {
			if transportIndex < 0 || transportIndex >= len(epoch.runtime.contextTransports) {
				return false
			}
			transport := epoch.runtime.contextTransports[transportIndex]
			if transport.from != sourceState || transport.sourcePoint != sourcePointIndex || transport.sourceContext != sourceContext || transport.to < 0 || transport.to >= len(epoch.points) {
				return false
			}
			mappedSource, mappedOK := epoch.runtime.contextTransportSourceState(transport.to, transport.sourcePoint)
			if !mappedOK || mappedSource != transport.from {
				return false
			}
			target, targetOK := graph.PointAt(schedule.Node(transport.targetPoint))
			_, targetPointIndex, targetContext, targetStateOK := epoch.runtime.graphPointAtState(transport.to)
			if !targetOK || !targetStateOK || targetPointIndex != transport.targetPoint || targetContext != transport.targetContext || epoch.runtime.activeState(transport.to) && !epoch.markStructuralState(transport.to, target) {
				return false
			}
		}
	}
	// Ordinary Group inputs are semantic dependencies too. The singular Graph
	// owns their consumer incidence, while the Link plan supplies the exact
	// contextual output row to wake. Structural environment/factor edges above
	// cannot substitute for this relation: omitting it lets a mounted summary
	// consumer retain a clean candidate after its input StateOrdinal changes.
	for index := 0; index < graph.ConsumerCount(source); index++ {
		group, ok := graph.ConsumerAt(source, index)
		if !ok {
			return false
		}
		groupIndex, indexed := graph.GroupIndex(group)
		outputIndex, outputIndexed := graph.PointIndex(group.Output())
		if !indexed || !outputIndexed || groupIndex < 0 || groupIndex >= len(epoch.runtime.producers) {
			return false
		}
		targetStates, stateOK := epoch.runtime.stateRowsForGraphPoint(sourceState, outputIndex)
		if !stateOK {
			return false
		}
		for _, targetState := range targetStates {
			if epoch.runtime.activeState(targetState) && !epoch.markDirtyIfCleanForState(targetState, groupIndex) {
				return false
			}
		}
	}
	for index := 0; index < graph.EnvironmentGroupCount(source); index++ {
		group, ok := graph.EnvironmentGroupAt(source, index)
		if !ok {
			return false
		}
		groupIndex, indexed := graph.GroupIndex(group)
		outputIndex, outputIndexed := graph.PointIndex(group.Output())
		if !indexed || !outputIndexed || groupIndex < 0 || groupIndex >= len(epoch.runtime.producers) {
			return false
		}
		targetStates, stateOK := epoch.runtime.stateRowsForGraphPoint(sourceState, outputIndex)
		if !stateOK {
			return false
		}
		for _, targetState := range targetStates {
			if epoch.runtime.activeState(targetState) && !epoch.markDirtyForState(targetState, groupIndex) {
				return false
			}
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

// invalidateRegionPostfix drops only the relation certificate.  The exact
// episode, its input evidence, and the live Point values remain epoch-owned;
// callers still decide whether the corresponding head must be queued.
func (epoch *executorEpoch) invalidateRegionPostfix(region int) bool {
	if epoch == nil || !epoch.activeRegion(region) || region >= len(epoch.regions) {
		return false
	}
	epoch.regions[region].postfixAt = 0
	return true
}

// regionPostfixProved reads the one stamp that certifies the already checked
// exact<=current relation. The certificate is a clock position rather than a
// tuple of counters: every fact the tuple used to carry -- the episode, the
// phase, the recomputed exact RHS, the ingress that justified it and the head
// publication -- either marks an operand this Region owns or drops the stamp
// outright, so a stamp that still dominates every mark on this Region's rows
// is exactly the old conjunction.
func (epoch *executorEpoch) regionPostfixProved(region int) bool {
	if epoch == nil || !epoch.activeRegion(region) || region >= len(epoch.regions) {
		return false
	}
	episode := &epoch.regions[region]
	if !episode.hasExact || episode.episode == 0 || episode.postfixAt == 0 {
		return false
	}
	return episode.externalAt <= episode.postfixAt && episode.backAt <= episode.postfixAt && episode.pointsAt <= episode.postfixAt
}

func (epoch *executorEpoch) rememberRegionPostfix(region int) bool {
	if epoch == nil || !epoch.activeRegion(region) || region >= len(epoch.regions) {
		return false
	}
	episode := &epoch.regions[region]
	if !episode.hasExact || episode.episode == 0 {
		return false
	}
	episode.postfixAt = epoch.operands.advance()
	return true
}

// markDirtyForState advances one compact producer occurrence. It is the only
// artifact cache wake path: the static graph Group identifies metadata, while
// the retained plan identifies the state row that owns the mutable candidate.
func (epoch *executorEpoch) markDirtyForState(stateIndex, group int) bool {
	if epoch == nil || epoch.runtime == nil || epoch.canceled() || !epoch.runtime.artifactBacked || stateIndex < 0 || !epoch.activeState(stateIndex) || group < 0 {
		return false
	}
	state, ok := epoch.producerCache(contextfiber.StateOrdinal(stateIndex), group)
	if !ok {
		return false
	}
	if state.generation == ^uint64(0) {
		return false
	}
	clean := state.applied == state.generation
	if clean && !epoch.updateCandidatesPending(stateIndex, 1) {
		return false
	}
	state.generation++
	return epoch.markPostfixDirty(stateIndex) && epoch.enqueuePoint(stateIndex)
}

func (epoch *executorEpoch) markDirty(group int) bool {
	if epoch == nil || epoch.runtime == nil || epoch.canceled() || group < 0 || group >= len(epoch.producers) {
		return false
	}
	producer := &epoch.runtime.producers[group]
	point, pointOK := epoch.runtime.graph.PointIndex(producer.group.Output())
	if !pointOK || point < 0 || point >= epoch.runtime.graph.PointCount() {
		return false
	}
	stateIndex := point
	var state *producerEpoch
	if epoch.runtime.artifactBacked {
		if epoch.currentState < 0 {
			return false
		}
		stateIndex, pointOK = epoch.runtime.stateForGraphPoint(epoch.currentState, point)
		if !pointOK {
			return false
		}
		return epoch.markDirtyForState(stateIndex, group)
	} else {
		if !epoch.activeState(stateIndex) {
			return false
		}
		state = &epoch.producers[group]
	}
	if !epoch.activeState(stateIndex) {
		return false
	}
	clean := state.applied == state.generation
	if state.generation == ^uint64(0) {
		return false
	}
	if clean && !epoch.updateCandidatesPending(stateIndex, 1) {
		return false
	}
	state.generation++
	return epoch.markPostfixDirty(stateIndex) && epoch.enqueuePoint(stateIndex)
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
	if epoch.runtime != nil && epoch.runtime.artifactBacked {
		if epoch.currentState < 0 {
			return false
		}
		return epoch.markDirtyIfCleanForState(epoch.currentState, group)
	}
	if state.applied != state.generation {
		return true
	}
	return epoch.markDirty(group)
}

// markDirtyIfCleanForState is the contextual form of the published-input
// deduplication law. Multiple input positions may name the same source, but a
// producer occurrence already pending in this exact StateOrdinal will read
// the newest point value and must not acquire a redundant generation.
func (epoch *executorEpoch) markDirtyIfCleanForState(stateIndex, group int) bool {
	if epoch == nil || epoch.runtime == nil || !epoch.runtime.artifactBacked || stateIndex < 0 || !epoch.activeState(stateIndex) || group < 0 {
		return false
	}
	state, ok := epoch.producerCache(contextfiber.StateOrdinal(stateIndex), group)
	if !ok {
		return false
	}
	if state.applied != state.generation {
		return true
	}
	return epoch.markDirtyForState(stateIndex, group)
}

func (epoch *executorEpoch) markPublishedInputConsumers(source equation.Point) bool {
	if epoch == nil || epoch.runtime == nil || epoch.runtime.graph == nil || !source.Available() {
		return false
	}
	if epoch.runtime.artifactBacked {
		return true
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
	if epoch == nil || epoch.runtime == nil || point < 0 || point >= len(epoch.runtime.pointRegion) || !epoch.activeState(point) || delta != 1 && delta != -1 || len(epoch.candidatesPending) != len(epoch.runtime.regions) {
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
	if epoch == nil || epoch.runtime == nil || point < 0 || point >= len(epoch.runtime.pointRegion) || !epoch.activeState(point) || delta != 1 && delta != -1 || len(epoch.nested) != len(epoch.runtime.regions) {
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
	if epoch == nil || epoch.runtime == nil || point < 0 || point >= len(epoch.queue.ready) || !epoch.activeState(point) {
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
	if epoch == nil || epoch.runtime == nil || point < 0 || point >= len(epoch.queue.ready) || !epoch.activeState(point) || !epoch.queue.ready[point] {
		return false
	}
	// Mirror enqueuePoint: a malformed counter must leave the ready bit set so
	// a failed dequeue cannot strand the scheduler with unaccounted work.
	return epoch.updateNested(point, -1) && epoch.queue.take(point)
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

func (epoch *executorEpoch) recordPointDescent(point int) bool {
	if epoch == nil || point < 0 || point >= len(epoch.structural.pointDescent) || epoch.structural.descent == ^uint64(0) {
		return false
	}
	epoch.structural.descent++
	epoch.structural.pointDescent[point] = epoch.structural.descent
	return true
}
