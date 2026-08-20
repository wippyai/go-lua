// runtime_region_interface.go computes region right-hand sides, refreshes interfaces, restarts regions and settles postfix.

package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/change"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
)

// regionRHS keeps recurrence operands private until one exact carrier
// transition publishes the head. E includes Init and external producers; B
// starts at bottom and contains every back producer, including mixed Groups.
//
// The row it admits is the delta the operand plane issues whenever the
// retained accumulator legalizes reuse, and the complete E+B row otherwise.
// Both admit their operands in the one sealed canonical order, so the fold
// itself never learns which of the two it is running.
func (epoch *executorEpoch) regionRHS(point equation.Point, pointIndex, regionIndex int, region *runtimeRegion, current carrier.PointState) (carrier.PointRHS, carrier.ChangeSet, bool) {
	if epoch != nil && epoch.diagnostics != nil {
		epoch.diagnostics.recordRegionRHS()
	}
	if epoch == nil || !region.active || !epoch.work.OwnsPointState(current) || regionIndex < 0 || regionIndex >= len(epoch.regions) {
		return carrier.PointRHS{}, carrier.ChangeSet{}, false
	}
	episode := &epoch.regions[regionIndex]
	base, sets, ok := epoch.regionOperandTerms(point, pointIndex, regionIndex, region, episode)
	if !ok {
		return carrier.PointRHS{}, carrier.ChangeSet{}, false
	}
	exact, changes, _, foldOK := epoch.foldPointTermSetsWithBoundary(current, base, sets, equation.Point{}) // R = base \sqcup E \sqcup B
	if !foldOK || epoch.canceled() {
		return carrier.PointRHS{}, carrier.ChangeSet{}, false
	}
	// The raw fold, before the support-axis discharge the head publication
	// applies to it, is the accumulator the next delta folds onto. Only an
	// ascent retains one: a narrow episode's row is a descent, and folding a
	// later operand onto a retained descent would publish an upper bound where
	// the narrow order laws demand the exact one.
	episode.accumulator, episode.hasAccumulator = exact, episode.phase == phaseAscent
	if !episode.hasAccumulator {
		dbgEngine.DropNarrowFold++
		episode.accumulator = carrier.PointRHS{}
	}
	return exact, changes, true
}

// regionAccumulatorEvidenceAdmits is the evidence half of the accumulator
// reuse predicate: a retained accumulator plus classified, non-descending
// evidence for every ingress operand that moved since this episode last
// remembered its interfaces. Admits is fail-closed, so an operand whose
// producer classified no direction refuses reuse exactly as a descent does.
func regionAccumulatorEvidenceAdmits(episode *regionEpoch) bool {
	return episode != nil && episode.phase == phaseAscent && episode.hasAccumulator && episode.pending.Admits()
}

// regionOperandTerms chooses the operands one head refold must admit.
//
// Reuse is admitted by one predicate and nothing else: the accumulated
// evidence of every ingress mark since this episode last remembered its
// interfaces. Admits is fail-closed, so an operand whose producer classified
// no direction, and any operand that descended, rebuilds the complete row
// from Init. Under an admitted ascent the retained accumulator already
// contains every unmoved operand, so folding the moved ones onto it is the
// same value as folding all of them onto Init.
func (epoch *executorEpoch) regionOperandTerms(point equation.Point, pointIndex, regionIndex int, region *runtimeRegion, episode *regionEpoch) (carrier.PointRHS, pointFoldTermSets, bool) {
	dbgRegionReuseRefusal(epoch, episode)
	if regionAccumulatorEvidenceAdmits(episode) && epoch.work.OwnsPointRHS(episode.accumulator) {
		rows := [6]struct {
			kind    operandKind
			members []int
		}{
			{operandExternalEnvironment, region.environmentExternal},
			{operandExternalFactor, region.factorExternal},
			{operandExternalProducer, region.external},
			{operandBackEnvironment, region.environmentBack},
			{operandBackFactor, region.factorBack},
			{operandBackProducer, region.back},
		}
		var bounds [7]int
		scratch, admitted := epoch.operandScratch[:0], true
		for index, row := range rows {
			bounds[index] = len(scratch)
			scratch, admitted = epoch.changedRegionOperands(regionIndex, row.kind, row.members, episode.rememberAt, scratch)
			if !admitted {
				dbgEngine.RefuseChangedRow++
				break
			}
		}
		bounds[6] = len(scratch)
		epoch.operandScratch = scratch
		if admitted {
			dbgEngine.ReuseAdmit++
			dbgEngine.ReuseTerms += uint64(bounds[6])
			return episode.accumulator, pointFoldTermSets{
				first: pointFoldTermSet{
					environments: scratch[bounds[0]:bounds[1]],
					factors:      scratch[bounds[1]:bounds[2]],
					groups:       scratch[bounds[2]:bounds[3]],
				},
				second: pointFoldTermSet{
					environments: scratch[bounds[3]:bounds[4]],
					factors:      scratch[bounds[4]:bounds[5]],
					groups:       scratch[bounds[5]:bounds[6]],
				},
				count: 2,
			}, true
		}
	}
	dbgEngine.ReuseRefuse++
	dbgEngine.RebuildTerms += uint64(len(region.environmentExternal) + len(region.factorExternal) + len(region.external) + len(region.environmentBack) + len(region.factorBack) + len(region.back))
	base, ok := epoch.pointBase(point, pointIndex)
	if !ok {
		return carrier.PointRHS{}, pointFoldTermSets{}, false
	}
	return base, pointFoldTermSets{
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
	}, true
}

// regionSelected is the ordinary ascent widening surface. It intentionally
// retains X+B, not E+B: external ingress is already checked against the
// current head before this fold. A pending interface refresh uses its newly
// rebuilt exact R directly as selected, avoiding a redundant fold.
func (epoch *executorEpoch) regionSelected(current carrier.PointState, region *runtimeRegion) (carrier.PointRHS, bool) {
	if epoch == nil || epoch.work == nil || !epoch.work.OwnsPointState(current) {
		return carrier.PointRHS{}, false
	}
	currentRHS, ok := epoch.work.PointRHSFromPointState(current)
	if !ok {
		return carrier.PointRHS{}, false
	}
	return epoch.foldPointInputs(current, currentRHS, region.environmentBack, region.factorBack, region.back) // P = X \sqcup B
}

// regionExternalIngressChanged reads the one stamp the operand plane keeps
// for this Region's external ingress rows. Interior input changes are not its
// question: they are classified at the candidate-order boundary from the same
// plane, so no Region copies a face version to answer this.
func (epoch *executorEpoch) regionExternalIngressChanged(region int) bool {
	if epoch == nil || !epoch.activeRegion(region) || region >= len(epoch.regions) {
		return true
	}
	state := &epoch.regions[region]
	if !state.hasExact || state.externalAt <= state.rememberAt {
		return false
	}
	_ = epoch.invalidateRegionPostfix(region)
	return true
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
	bound := &epoch.runtime.regions[region]
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
	state := &epoch.regions[region]
	if !state.hasExact || epoch.regionExternalIngressChanged(region) {
		return true
	}
	if state.backAt <= state.rememberAt {
		return false
	}
	_ = epoch.invalidateRegionPostfix(region)
	return true
}

// rememberRegionInterfaces closes this Region's interface epoch. It is one
// clock advance and one evidence reset: the ordered producer, environment and
// factor version vectors it used to copy carried nothing the operand plane
// does not already stamp, and the two edge-version readers they fed are gone
// with them.
func (epoch *executorEpoch) rememberRegionInterfaces(region int) bool {
	if epoch == nil || !epoch.activeRegion(region) || region >= len(epoch.regions) {
		return false
	}
	state := &epoch.regions[region]
	state.rememberAt = epoch.operands.advance()
	state.pending = change.Classified()
	if epoch.diagnostics != nil {
		epoch.diagnostics.rememberRegionInterfaces(epoch, region)
	}
	return true
}

// carryRegionEpisodes installs the region episodes of one frontier over the
// episodes the epoch is running. A region the frontier proves unchanged keeps
// the episode it settled: its operands are the operands it folded, so its
// exact row, its remember stamp and its retained accumulator all still
// describe the live recurrence and nothing about the frontier refuses them.
//
// Every other continued region opens a new ascent episode, and so does every
// region nested inside one: a parent that re-ascends may not hold a narrowed
// descendant, which is the same ownership invariant restartRegion keeps. A
// region this frontier opens keeps the episode the preparation built for it.
func (epoch *executorEpoch) carryRegionEpisodes(fresh []regionEpoch, previousOf []int, carry []regionRowCarry) ([]regionEpoch, bool) {
	count := len(fresh)
	if epoch == nil || epoch.runtime == nil || len(carry) != count || len(previousOf) != count || len(epoch.runtime.regionChildren) != count {
		return nil, false
	}
	reascend := make([]bool, count)
	stack := make([]int, 0, count)
	for index := 0; index < count; index++ {
		// A region this frontier opens ascends from Init, and one it changed
		// opens a new ascent episode. Both are roots of the descendant closure
		// below, because a new region can be nested between an enclosing region
		// and a region that was already nested inside it.
		if previousOf[index] < 0 || carry[index] != regionRowRetained {
			reascend[index], stack = true, append(stack, index)
		}
	}
	for next := 0; next < len(stack); next++ {
		for _, child := range epoch.runtime.regionChildren[stack[next]] {
			if child < 0 || child >= count || reascend[child] {
				continue
			}
			reascend[child], stack = true, append(stack, child)
		}
	}
	dbgEngine.CarryInstalls++
	for index := 0; index < count; index++ {
		source := previousOf[index]
		if source < 0 || source >= len(epoch.regions) {
			dbgEngine.CarryOpened++
			continue
		}
		switch carry[index] {
		case regionRowRetained:
			dbgEngine.CarryRetained++
		case regionRowExtended:
			dbgEngine.CarryExtended++
		default:
			dbgEngine.CarryRebuilt++
		}
		fresh[index] = epoch.regions[source]
		if reascend[index] {
			epoch.reascendRegionEpisode(&fresh[index])
		}
	}
	return fresh, true
}

// reascendRegionEpisode opens one new ascent episode over a region the
// frontier changed. The episode is new, but the row the previous one settled
// is not thrown away: it is the fold of operands that have not moved, so it is
// the lower bound the next delta folds onto. Only a narrow episode's exact row
// carries that way -- an ascent episode's exact row is the discharged one, and
// only its own raw accumulator is the fold.
func (epoch *executorEpoch) reascendRegionEpisode(state *regionEpoch) {
	seed, hasSeed := state.accumulator, state.hasAccumulator && state.phase == phaseAscent
	if state.phase == phaseNarrow && state.hasExact {
		seed, hasSeed = state.exact, true
	}
	episode := state.episode
	if episode != ^uint64(0) {
		episode++
	}
	*state = regionEpoch{
		phase:          phaseAscent,
		episode:        episode,
		accumulator:    seed,
		hasAccumulator: hasSeed,
		invalid:        true,
		rememberAt:     epoch.operands.advance(),
		pending:        change.Classified(),
	}
	if epoch.diagnostics != nil {
		epoch.diagnostics.observeEpisode(episode)
	}
}

// markCarriedRegionOperands routes the evidence of the operands a frontier
// installation stamped. The plane stamps the tick; this is the same mark on
// the region's own evidence axis, so an installed operand reaches the
// accumulator through the one classification route a publication uses.
//
// An appended operand is a join term the row did not have, so the row ascends
// and a retained accumulator still bounds it from below. A row whose operands
// moved position or changed source is classified by nobody, so it reaches the
// axis unclassified and Admits refuses the reuse.
func (epoch *executorEpoch) markCarriedRegionOperands(stamped []uint8, carry []regionRowCarry) bool {
	if epoch == nil || len(stamped) != len(carry) {
		return false
	}
	clock := epoch.operands.clock
	for region, kinds := range stamped {
		if kinds == 0 || region >= len(epoch.regions) || !epoch.activeRegion(region) {
			continue
		}
		state := &epoch.regions[region]
		for kind := operandKind(0); kind < operandKindCount; kind++ {
			if kinds&(1<<uint(kind)) == 0 {
				continue
			}
			evidence := change.Set{Reasons: change.ChangedFactor, Direction: change.Known | change.Ascends}
			if carry[region] != regionRowExtended {
				evidence = change.Set{Reasons: change.ChangedFactor}
			}
			switch {
			case kind.external():
				state.externalAt = clock
				state.pending = state.pending.Union(evidence)
			case kind.back():
				state.backAt = clock
				state.pending = state.pending.Union(evidence)
			default:
				state.pointsAt = clock
			}
		}
	}
	return true
}

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
		if epoch.regions[index].hasAccumulator {
			dbgEngine.DropRestart++
		}
		epoch.regions[index].accumulator = carrier.PointRHS{}
		epoch.regions[index].hasAccumulator = false
		epoch.regions[index].postfixAt = 0
		epoch.regions[index].invalid = true
		epoch.regions[index].interfaceRefreshPending = false
		// A fresh episode retains no interface history. Opening a new remember
		// epoch is that drop: every mark this Region has taken now lies at or
		// below the stamp, and hasExact gates its readers until the first exact
		// row of the new episode is rebuilt.
		epoch.regions[index].rememberAt = epoch.operands.advance()
		epoch.regions[index].pending = change.Classified()
	}
	// A fresh episode may not use an old local Point as a seed.  The Region's
	// event-point interval includes every nested descendant exactly once, so
	// this root row covers the full restarted subtree.  Reset through the sole
	// publication cut: observers must see the actual old-to-base delta rather
	// than a later base-to-recomputed delta (or no delta when it stays at base).
	bound := &epoch.runtime.regions[region]
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
			cache.rememberAt = 0
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

// snapshotRegion opens one WTO pass over this Region's interior. The pass
// asks a single question at its exit -- did any interior Point publish -- and
// the operand plane already stamps that, so the pass keeps a clock position
// instead of a copy of every interior version.
func (epoch *executorEpoch) snapshotRegion(region int) bool {
	if epoch == nil || !epoch.activeRegion(region) || region >= len(epoch.regions) {
		return false
	}
	epoch.regions[region].enterAt = epoch.operands.advance()
	return true
}

func (epoch *executorEpoch) regionSnapshotChanged(region int) bool {
	if epoch == nil || !epoch.activeRegion(region) || region >= len(epoch.regions) {
		return true
	}
	state := &epoch.regions[region]
	return state.pointsAt > state.enterAt
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
	region, episode := &epoch.runtime.regions[regionIndex], &epoch.regions[regionIndex]
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
	if region.head < 0 || region.head >= len(epoch.points) {
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
