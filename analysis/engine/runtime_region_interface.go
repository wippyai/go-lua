// runtime_region_interface.go computes region right-hand sides, refreshes interfaces, restarts regions and settles postfix.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
)

// regionRHS keeps recurrence operands private until one exact carrier
// transition publishes the head. E includes Init and external producers; B
// starts at bottom and contains every back producer, including mixed Groups.
func (epoch *executorEpoch) regionRHS(point equation.Point, pointIndex int, region runtimeRegion, current carrier.PointState) (carrier.PointRHS, carrier.ChangeSet, bool) {
	if epoch != nil && epoch.diagnostics != nil {
		epoch.diagnostics.recordRegionRHS()
	}
	if epoch == nil || !region.active || !epoch.work.OwnsPointState(current) {
		return carrier.PointRHS{}, carrier.ChangeSet{}, false
	}
	base, ok := epoch.pointBase(point, pointIndex)
	if !ok {
		return carrier.PointRHS{}, carrier.ChangeSet{}, false
	}
	sets := pointFoldTermSets{
		first: pointFoldTermSet{
			environments: region.environmentExternal,
			factors:      region.factorExternal,
			groups:       region.external,
		},
		second: pointFoldTermSet{
			environments: region.environmentBack,
			factors:      region.factorBack,
			groups:       region.back,
		},
		count: 2,
	}
	exact, changes, _, foldOK := epoch.foldPointTermSetsWithBoundary(current, base, sets, equation.Point{}) // R = base \sqcup E \sqcup B
	if !foldOK || epoch.canceled() {
		return carrier.PointRHS{}, carrier.ChangeSet{}, false
	}
	return exact, changes, true
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

// regionExternalIngressChanged checks only the direct producer and structural
// ingress versions that feed this Region head. Interior input versions are
// owned by the producer candidate token snapshot; they are classified at the
// candidate-order boundary instead of being copied into a Region face plane.
func (epoch *executorEpoch) regionExternalIngressChanged(region int) bool {
	if epoch == nil || !epoch.activeRegion(region) || region >= len(epoch.regions) {
		return true
	}
	bound, state := epoch.runtime.regions[region], epoch.regions[region]
	if !state.hasExact {
		return false
	}
	if len(bound.external) != len(state.ingress) || len(bound.environmentExternal) != len(state.environmentIngress) || len(bound.factorExternal) != len(state.factorIngress) {
		_ = epoch.invalidateRegionPostfix(region)
		return true
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
		epoch.diagnostics.recordInterfaceRefreshBegin(epoch, region)
	}
	return true
}

// regionExactInputsChanged checks the disposable proof recorded with the
// episode.exact head RHS.  The ordered producer and source-point versions are
// the complete semantic input list for E⊔B: queue readiness alone is not
// evidence that the stored exact carrier still describes the live recurrence.
// Direct external ingress remains part of the interface proof because a
// changed source outside the Region must restart the local episode before
// narrowing. Interior producer inputs are checked by candidate tokens at
// their evaluation boundary.
func (epoch *executorEpoch) regionExactInputsChanged(region int) bool {
	if epoch == nil || !epoch.activeRegion(region) || region >= len(epoch.regions) {
		return true
	}
	state := epoch.regions[region]
	if !state.hasExact || epoch.regionExternalIngressChanged(region) {
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
	if len(bound.external) != len(state.ingress) || len(bound.back) != len(state.backIngress) || len(bound.environmentExternal) != len(state.environmentIngress) || len(bound.environmentBack) != len(state.environmentBackIngress) || len(bound.factorExternal) != len(state.factorIngress) || len(bound.factorBack) != len(state.factorBackIngress) {
		return false
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

// restartRegion begins a fresh exact episode for one region and all nested
// regions.  Phase is owned by each region episode, so an independent child
// restart never changes an enclosing region from Narrow to Ascent while its
// narrowed head is still retained. Every selected Group rooted inside is made
// dirty before any later head widening can observe an old candidate.
func (epoch *executorEpoch) restartRegion(region int, callSite solveDiagnosticRestartCallSite, reason solveDiagnosticRestartReason, pendingGroup int, pending carrier.RuleContribution) (ok bool) {
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
			if !epoch.restartRegion(regionIndex, solveDiagnosticRestartPostfixExact, solveDiagnosticRestartExactNotBelowCurrent, -1, carrier.RuleContribution{}) {
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
