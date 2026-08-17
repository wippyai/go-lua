// runtime_epoch_queue.go marks dirty and structural work, proves postfix and moves points through the queue.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
)

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
