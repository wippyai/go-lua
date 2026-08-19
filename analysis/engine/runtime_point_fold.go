// runtime_point_fold.go gathers producer inputs, evaluates rules and folds point terms.

package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// inputs reindexes the Group input vector the members below receive, together
// with the Group output target Scope.
//
// It transports only the inputs whose operand moved since this cache closed
// its input epoch. A retained entry is the same transported header the last
// admitted evaluation built from the same unmoved source, so re-transporting
// it would rebuild an identical value; an entry the operand plane cannot
// certify as unmoved, and every entry of a cache that has installed no
// candidate, is transported again.
func (epoch *executorEpoch) inputs(producer runtimeProducer, cache *producerEpoch) ([]carrier.PointState, bool) {
	if epoch == nil || epoch.work == nil || cache == nil || len(producer.inputs) != producer.group.InputCount() || len(cache.inputs) != producer.group.InputCount() {
		return nil, false
	}
	values := cache.inputs
	for index := range values {
		if cache.rememberAt != 0 && epoch.work.OwnsPointState(values[index]) && !epoch.producerInputChanged(producer.index, index, cache.rememberAt) {
			continue
		}
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

func (epoch *executorEpoch) evaluate(producer runtimeProducer, cache *producerEpoch) (result carrier.RuleContribution, reads []demand.Observation, ok bool) {
	if epoch != nil && epoch.diagnostics != nil && epoch.diagnostics.scheduleEnabled() {
		defer func() { epoch.diagnostics.recordEvaluate(&ok) }()
	}
	if epoch == nil || epoch.work == nil || cache == nil || epoch.canceled() || producer.span.count() == 0 || !producer.outputScope.Valid() || !producer.premise.Valid() || producer.premise.Manager() != epoch.runtime.carrier.Guards() {
		return carrier.RuleContribution{}, nil, false
	}
	rows := epoch.runtime.program.memberRows(producer.span)
	if len(rows) != producer.span.count() {
		return carrier.RuleContribution{}, nil, false
	}
	inputs, ok := epoch.inputs(producer, cache)
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
	for _, row := range rows {
		if epoch.canceled() {
			return carrier.RuleContribution{}, nil, false
		}
		result := row.exec(epoch.work, base, cache.inputStates, within)
		if !result.valid {
			// The failing member's identity is recovered from the Group the
			// producer already holds; the hot row carries only its position.
			member, identified := memberRowIdentity(producer.group, row)
			if !identified {
				return carrier.RuleContribution{}, nil, false
			}
			epoch.recordMemberFailure(SolveFailureReasonExecution, result.boundary, producer.group.Output(), producer.group, member)
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
			if !row.hasSlot {
				return carrier.RuleContribution{}, nil, false
			}
			patches = append(patches, result.patch)
			patchRows = append(patchRows, contributionPatch{slot: row.outputSlot, patch: result.patch})
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

// pointFoldEnvironmentEdge returns the exact owner-issued PointState that the
// fold admits.  Keeping transport separate from admission lets a Region's
// ingress order state inspect this same header without transporting the edge twice.
func (epoch *executorEpoch) pointFoldEnvironmentEdge(edgeIndex int) (carrier.PointState, bool) {
	if epoch == nil || epoch.runtime == nil || epoch.work == nil || edgeIndex < 0 || edgeIndex >= len(epoch.runtime.environments) {
		return carrier.PointState{}, false
	}
	edge := epoch.runtime.environments[edgeIndex]
	if edge.source < 0 || edge.source >= len(epoch.points) || edge.target < 0 || edge.target >= len(epoch.points) || !edge.input.valid() {
		return carrier.PointState{}, false
	}
	transported, ok := epoch.work.TransportPointState(epoch.points[edge.source], edge.input.pre, edge.input.plan, edge.input.post)
	if !ok || !epoch.work.OwnsPointState(transported) {
		return carrier.PointState{}, false
	}
	return transported, true
}

func (epoch *executorEpoch) addPointFoldEnvironmentEdge(edgeIndex int) bool {
	transported, ok := epoch.pointFoldEnvironmentEdge(edgeIndex)
	return ok && epoch.work.AddPointFoldEnvironment(transported) && !epoch.canceled()
}

// addPointFoldFactorEdgeWithBoundary preserves the existing projection,
// transport, and one transaction admission while returning only the failed
// boundary for the owning refresh diagnostic.
func (epoch *executorEpoch) pointFoldFactorEdgeWithBoundary(edgeIndex int) (carrier.PointState, solveBoundary, bool) {
	if epoch == nil || epoch.runtime == nil || epoch.work == nil || edgeIndex < 0 || edgeIndex >= len(epoch.runtime.factorEdges) {
		return carrier.PointState{}, refused(SolveFailureFamilyRefresh, "acyclic-fold-factor-validation"), false
	}
	edge := epoch.runtime.factorEdges[edgeIndex]
	if edge.source < 0 || edge.source >= len(epoch.points) || edge.target < 0 || edge.target >= len(epoch.points) || !edge.input.valid() {
		return carrier.PointState{}, refused(SolveFailureFamilyRefresh, "acyclic-fold-factor-validation"), false
	}
	// Factor projection commutes with the point boundary: support and guard
	// transport are shared by every plane, while typed reindex is factor-local.
	// Project first so a one-Factor edge never reconstructs the unrelated
	// Factor roots merely to discard them afterward.
	projected, ok := epoch.work.ProjectPointState(epoch.points[edge.source], edge.slot)
	if !ok || !epoch.work.OwnsPointState(projected) {
		return carrier.PointState{}, refused(SolveFailureFamilyRefresh, "acyclic-fold-factor-projection"), false
	}
	transported, transportBoundary, ok := epoch.work.TransportPointStateWithBoundary(projected, edge.input.pre, edge.input.plan, edge.input.post)
	if !ok || !epoch.work.OwnsPointState(transported) {
		return carrier.PointState{}, refused(SolveFailureFamilyRefresh, "acyclic-fold-factor-transport").withTransport(transportBoundary), false
	}
	return transported, boundaryNone, true
}

func (epoch *executorEpoch) addPointFoldFactorEdgeWithBoundary(edgeIndex int) (solveBoundary, bool) {
	transported, boundary, ok := epoch.pointFoldFactorEdgeWithBoundary(edgeIndex)
	if !ok {
		return boundary, false
	}
	if !epoch.work.AddPointFoldEnvironment(transported) {
		return refused(SolveFailureFamilyRefresh, "acyclic-fold-factor-admission"), false
	}
	return boundaryNone, !epoch.canceled()
}

// pointFoldGroupState closes one owner-issued RuleContribution at the single
// RuleContribution -> PointState boundary used by point folding.  Region
// ingress validation and fold admission both consume this exact PointState.
func (epoch *executorEpoch) pointFoldGroupState(group int) (carrier.PointState, bool) {
	if epoch == nil || epoch.work == nil || group < 0 || group >= len(epoch.producers) || !epoch.producers[group].hasValue {
		return carrier.PointState{}, false
	}
	return epoch.work.PointStateFromRuleContribution(epoch.producers[group].candidate)
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
	result, _, _, ok := epoch.foldPointWithBoundary(reference, base, point)
	return result, ok
}

// foldPointWithBoundary exposes whether the outer Point ownership check
// reached the existing canonical terms fold, and issues the ChangeSet that
// fold committed. The fold is a terminal operation -- its result reaches the
// published point plane without passing another emitting operation -- so it
// returns its evidence rather than discarding it. The boundary marker is
// consumed immediately by refresh diagnostics and retains no fold rows.
func (epoch *executorEpoch) foldPointWithBoundary(reference carrier.PointState, base carrier.PointRHS, point equation.Point) (carrier.PointRHS, carrier.ChangeSet, solveBoundary, bool) {
	if epoch == nil || epoch.runtime == nil || !point.Available() || !epoch.work.OwnsPointState(reference) || !epoch.work.OwnsPointRHS(base) {
		return carrier.PointRHS{}, carrier.ChangeSet{}, refused(SolveFailureFamilyRefresh, "acyclic-fold-point"), false
	}
	pointIndex, indexed := epoch.runtime.graph.PointIndex(point)
	if !indexed || pointIndex < 0 || pointIndex >= len(epoch.runtime.environmentIncoming) || pointIndex >= len(epoch.runtime.factorIncoming) {
		return carrier.PointRHS{}, carrier.ChangeSet{}, refused(SolveFailureFamilyRefresh, "acyclic-fold-point"), false
	}
	sets := pointFoldTermSets{
		first: pointFoldTermSet{environments: epoch.runtime.environmentIncoming[pointIndex], factors: epoch.runtime.factorIncoming[pointIndex]},
		count: 1,
	}
	return epoch.foldPointTermSetsWithBoundary(reference, base, sets, point)
}

func (epoch *executorEpoch) foldPointTerms(reference carrier.PointState, base carrier.PointRHS, environments, factors, groups []int, producerPoint equation.Point) (result carrier.PointRHS, ok bool) {
	result, _, ok = epoch.foldPointTermsWithBoundary(reference, base, environments, factors, groups, producerPoint)
	return result, ok
}

// pointFoldTermSet is one canonical input-order segment.  A Region uses two
// segments so external environment/factor/group terms are followed by the
// corresponding back terms without materializing or reordering a term list.
type pointFoldTermSet struct {
	environments []int
	factors      []int
	groups       []int
}

type pointFoldTermSets struct {
	first  pointFoldTermSet
	second pointFoldTermSet
	count  uint8
}

// foldPointTermsWithBoundary executes the same one-shot canonical fold as
// foldPointTerms while returning only its first failed transaction boundary.
// The marker is consumed immediately by refresh diagnostics and retains no
// Point, carrier state, or fold rows.
func (epoch *executorEpoch) foldPointTermsWithBoundary(reference carrier.PointState, base carrier.PointRHS, environments, factors, groups []int, producerPoint equation.Point) (result carrier.PointRHS, boundary solveBoundary, ok bool) {
	sets := pointFoldTermSets{
		first: pointFoldTermSet{environments: environments, factors: factors, groups: groups},
		count: 1,
	}
	result, _, boundary, ok = epoch.foldPointTermSetsWithBoundary(reference, base, sets, producerPoint)
	return result, boundary, ok
}

// foldPointTermSetsWithBoundary executes one canonical Point RHS transaction.
// Terms are transported/closed once and admitted in their sealed order.
func (epoch *executorEpoch) foldPointTermSetsWithBoundary(reference carrier.PointState, base carrier.PointRHS, sets pointFoldTermSets, producerPoint equation.Point) (result carrier.PointRHS, changes carrier.ChangeSet, boundary solveBoundary, ok bool) {
	if epoch != nil && epoch.diagnostics != nil {
		epoch.diagnostics.recordFold()
	}
	if epoch == nil || epoch.runtime == nil || epoch.work == nil || !epoch.work.OwnsPointState(reference) || !epoch.work.OwnsPointRHS(base) {
		return carrier.PointRHS{}, carrier.ChangeSet{}, refused(SolveFailureFamilyRefresh, "acyclic-fold-begin"), false
	}
	if sets.count < 1 || sets.count > 2 {
		return carrier.PointRHS{}, carrier.ChangeSet{}, refused(SolveFailureFamilyRefresh, "acyclic-fold-inputs"), false
	}
	producerCount := len(sets.first.groups)
	if sets.count > 1 {
		producerCount += len(sets.second.groups)
	}
	if producerPoint.Available() {
		producerCount = epoch.runtime.graph.ProducerCount(producerPoint)
	}
	termCount := len(sets.first.environments) + len(sets.first.factors)
	if sets.count > 1 {
		termCount += len(sets.second.environments) + len(sets.second.factors)
	}
	if termCount == 0 && producerCount == 0 {
		return base, carrier.ChangeSet{}, boundaryNone, true
	}
	if !epoch.work.BeginPointRHSFold(reference, base) {
		return carrier.PointRHS{}, carrier.ChangeSet{}, refused(SolveFailureFamilyRefresh, "acyclic-fold-begin"), false
	}
	active := true
	defer func() {
		if active {
			_ = epoch.work.AbortPointRHSFold()
		}
	}()
	for setIndex := 0; setIndex < int(sets.count); setIndex++ {
		set := sets.first
		if setIndex == 1 {
			set = sets.second
		}
		boundary = refused(SolveFailureFamilyRefresh, "acyclic-fold-environment")
		for _, edge := range set.environments {
			term, termOK := epoch.pointFoldEnvironmentEdge(edge)
			if !termOK {
				return carrier.PointRHS{}, carrier.ChangeSet{}, boundary, false
			}
			if !epoch.work.AddPointFoldEnvironment(term) || epoch.canceled() {
				return carrier.PointRHS{}, carrier.ChangeSet{}, boundary, false
			}
		}
		for _, edge := range set.factors {
			term, factorBoundary, termOK := epoch.pointFoldFactorEdgeWithBoundary(edge)
			if !termOK {
				boundary = factorBoundary
				return carrier.PointRHS{}, carrier.ChangeSet{}, boundary, false
			}
			if !epoch.work.AddPointFoldEnvironment(term) || epoch.canceled() {
				boundary = refused(SolveFailureFamilyRefresh, "acyclic-fold-factor-admission")
				return carrier.PointRHS{}, carrier.ChangeSet{}, boundary, false
			}
		}
		boundary = refused(SolveFailureFamilyRefresh, "acyclic-fold-producer")
		for _, group := range set.groups {
			term, termOK := epoch.pointFoldGroupState(group)
			if !termOK {
				return carrier.PointRHS{}, carrier.ChangeSet{}, boundary, false
			}
			if !epoch.work.AddPointFoldEnvironment(term) || epoch.canceled() {
				return carrier.PointRHS{}, carrier.ChangeSet{}, boundary, false
			}
		}
	}
	if producerPoint.Available() {
		boundary = refused(SolveFailureFamilyRefresh, "acyclic-fold-producer")
		for index := 0; index < epoch.runtime.graph.ProducerCount(producerPoint); index++ {
			group, present := epoch.runtime.graph.ProducerAt(producerPoint, index)
			groupIndex, indexed := epoch.runtime.graph.GroupIndex(group)
			if !present || !indexed || !epoch.addPointFoldGroup(groupIndex) {
				return carrier.PointRHS{}, carrier.ChangeSet{}, boundary, false
			}
		}
	}
	boundary = refused(SolveFailureFamilyRefresh, "acyclic-fold-finish")
	result, changes, ok = epoch.work.FinishPointRHSFold()
	active = false
	return result, changes, boundary, ok && !epoch.canceled()
}
