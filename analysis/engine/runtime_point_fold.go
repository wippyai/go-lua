// runtime_point_fold.go gathers producer inputs, evaluates rules and folds point terms.

package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

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
			epoch.recordMemberFailure(SolveFailureReasonExecution, result.boundary, producer.group.Output(), producer.group, member.member())
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
func (epoch *executorEpoch) addPointFoldFactorEdgeWithBoundary(edgeIndex int) (solveBoundary, bool) {
	if epoch == nil || epoch.runtime == nil || epoch.work == nil || edgeIndex < 0 || edgeIndex >= len(epoch.runtime.factorEdges) {
		return refused(SolveFailureFamilyRefresh, "acyclic-fold-factor-validation"), false
	}
	edge := epoch.runtime.factorEdges[edgeIndex]
	if edge.source < 0 || edge.source >= len(epoch.points) || edge.target < 0 || edge.target >= len(epoch.points) || !edge.input.valid() {
		return refused(SolveFailureFamilyRefresh, "acyclic-fold-factor-validation"), false
	}
	// Factor projection commutes with the point boundary: support and guard
	// transport are shared by every plane, while typed reindex is factor-local.
	// Project first so a one-Factor edge never reconstructs the unrelated
	// Factor roots merely to discard them afterward.
	projected, ok := epoch.work.ProjectPointState(epoch.points[edge.source], edge.slot)
	if !ok || !epoch.work.OwnsPointState(projected) {
		return refused(SolveFailureFamilyRefresh, "acyclic-fold-factor-projection"), false
	}
	transported, transportBoundary, ok := epoch.work.TransportPointStateWithBoundary(projected, edge.input.pre, edge.input.plan, edge.input.post)
	if !ok || !epoch.work.OwnsPointState(transported) {
		return refused(SolveFailureFamilyRefresh, "acyclic-fold-factor-transport").withTransport(transportBoundary), false
	}
	if !epoch.work.AddPointFoldEnvironment(transported) {
		return refused(SolveFailureFamilyRefresh, "acyclic-fold-factor-admission"), false
	}
	return boundaryNone, !epoch.canceled()
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

func (epoch *executorEpoch) foldPoint(reference carrier.PointState, base carrier.PointRHS, point equation.Point) (carrier.PointRHS, bool) {
	result, _, ok := epoch.foldPointWithBoundary(reference, base, point)
	return result, ok
}

// foldPointWithBoundary exposes only whether the outer Point ownership check
// reached the existing canonical terms fold. Refresh diagnostics use that
// scalar to distinguish an invalid foldPoint boundary from a failure inside
// foldPointTerms; it creates no additional fold authority or data path.
func (epoch *executorEpoch) foldPointWithBoundary(reference carrier.PointState, base carrier.PointRHS, point equation.Point) (carrier.PointRHS, solveBoundary, bool) {
	if epoch == nil || epoch.runtime == nil || !point.Available() || !epoch.work.OwnsPointState(reference) || !epoch.work.OwnsPointRHS(base) {
		return carrier.PointRHS{}, refused(SolveFailureFamilyRefresh, "acyclic-fold-point"), false
	}
	pointIndex, indexed := epoch.runtime.graph.PointIndex(point)
	if !indexed || pointIndex < 0 || pointIndex >= len(epoch.runtime.environmentIncoming) || pointIndex >= len(epoch.runtime.factorIncoming) {
		return carrier.PointRHS{}, refused(SolveFailureFamilyRefresh, "acyclic-fold-point"), false
	}
	return epoch.foldPointTermsWithBoundary(reference, base, epoch.runtime.environmentIncoming[pointIndex], epoch.runtime.factorIncoming[pointIndex], nil, point)
}

func (epoch *executorEpoch) foldPointTerms(reference carrier.PointState, base carrier.PointRHS, environments, factors, groups []int, producerPoint equation.Point) (result carrier.PointRHS, ok bool) {
	result, _, ok = epoch.foldPointTermsWithBoundary(reference, base, environments, factors, groups, producerPoint)
	return result, ok
}

// foldPointTermsWithBoundary executes the same one-shot canonical fold as
// foldPointTerms while returning only its first failed transaction boundary.
// The marker is consumed immediately by refresh diagnostics and retains no
// Point, carrier state, or fold rows.
func (epoch *executorEpoch) foldPointTermsWithBoundary(reference carrier.PointState, base carrier.PointRHS, environments, factors, groups []int, producerPoint equation.Point) (result carrier.PointRHS, boundary solveBoundary, ok bool) {
	if epoch != nil && epoch.diagnostics != nil {
		epoch.diagnostics.recordFold()
	}
	if epoch == nil || epoch.runtime == nil || epoch.work == nil || !epoch.work.OwnsPointState(reference) || !epoch.work.OwnsPointRHS(base) {
		return carrier.PointRHS{}, refused(SolveFailureFamilyRefresh, "acyclic-fold-begin"), false
	}
	producerCount := len(groups)
	if producerPoint.Available() {
		producerCount = epoch.runtime.graph.ProducerCount(producerPoint)
	}
	if len(environments) == 0 && len(factors) == 0 && producerCount == 0 {
		return base, boundaryNone, true
	}
	if !epoch.work.BeginPointRHSFold(reference, base) {
		return carrier.PointRHS{}, refused(SolveFailureFamilyRefresh, "acyclic-fold-begin"), false
	}
	active := true
	defer func() {
		if active {
			_ = epoch.work.AbortPointRHSFold()
		}
	}()
	boundary = refused(SolveFailureFamilyRefresh, "acyclic-fold-environment")
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
	boundary = refused(SolveFailureFamilyRefresh, "acyclic-fold-producer")
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
	boundary = refused(SolveFailureFamilyRefresh, "acyclic-fold-finish")
	result, ok = epoch.work.FinishPointRHSFold()
	active = false
	return result, boundary, ok && !epoch.canceled()
}
