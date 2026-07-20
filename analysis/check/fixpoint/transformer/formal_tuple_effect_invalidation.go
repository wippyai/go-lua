package transformer

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

type formalPathInvalidationLane struct {
	group formalFiberGroupDescriptor
}

// formalPathInvalidationStep is the checked binding from one existing
// relationCode EffectInvalidatePath node to the ProductDomain path-mutation
// laws. It owns no mutation semantics and no alternate Effect ordering.
type formalPathInvalidationStep struct {
	target   pathdom.PathKey
	scope    InvalidationScope
	demands  []formalQualifiedGuardDemand
	lanes    []formalPathInvalidationLane
	owner    state.CoordinateFamily
	variable relationVar
}

func freezeFormalPathInvalidationStep(program *RelationProgram, variable relationVar, operator formalRelationOperatorRef) (*formalPathInvalidationStep, error) {
	if program == nil || variable == 0 || int(variable) > len(program.bodies) || operator.kind != formalRelationCellStep ||
		operator.code == nil || operator.root == 0 || operator.step == 0 || int(operator.root) >= len(operator.code.nodes) ||
		int(operator.step) > len(operator.code.nodes[operator.root].steps) {
		return nil, nil
	}
	step := operator.code.nodes[operator.root].steps[operator.step-1]
	if step.kind != boundaryStepEffect || step.effect == 0 || operator.code.effects == nil || int(step.effect) >= len(operator.code.effects.nodes) {
		return nil, nil
	}
	node := operator.code.effects.nodes[step.effect]
	if node.kind != EffectInvalidatePath {
		return nil, nil
	}
	config := node.invalidation
	if config.Target.kind != effectTargetPath || config.Target.path == 0 || config.Precise != nil ||
		config.PreserveStructuralWitness || config.PreserveDynamicValueMemberships {
		return nil, fmt.Errorf("transformer: formal path invalidation requires an exact unqualified path target")
	}
	if config.Scope != InvalidationScopeSubtree && config.Scope != InvalidationScopeDescendants {
		return nil, fmt.Errorf("transformer: formal path invalidation has invalid scope")
	}
	body := &program.bodies[variable-1]
	span, ok := program.formalFibers.span(variable)
	if !ok || span.keys == nil || !span.keys.Valid() || body.relation.code != operator.code {
		return nil, fmt.Errorf("transformer: formal path invalidation has no formal owner")
	}
	target, err := freezeFormalEffectPathKey(body, span, config.Target.path)
	if err != nil {
		return nil, fmt.Errorf("transformer: formal path invalidation target: %w", err)
	}
	owner, ok := body.productDomain.PathValueFamily()
	if !ok {
		return nil, fmt.Errorf("transformer: formal path invalidation has no path-evidence owner")
	}
	participants := body.productDomain.PathDescendantMutationParticipantLanes()
	if config.Scope == InvalidationScopeSubtree {
		topology, topologyErr := body.productDomain.SealPathSubtreeMutationFactorTopology()
		if topologyErr != nil {
			return nil, topologyErr
		}
		participants = topology.Lanes()
		for _, family := range topology.Families() {
			found := false
			for _, lane := range participants {
				found = found || lane == family.Lane()
			}
			if !found {
				participants = append(participants, family.Lane())
			}
		}
	}
	participantSet := make(map[state.ProductLane]bool, len(participants))
	for _, lane := range participants {
		participantSet[lane] = true
	}
	lanes := make([]formalPathInvalidationLane, 0, len(participants))
	seen := make(map[state.ProductLane]bool)
	for _, group := range span.groupDescriptors() {
		if group.kind == formalFiberGroupValues {
			continue
		}
		if !participantSet[group.lane] {
			continue
		}
		lanes = append(lanes, formalPathInvalidationLane{group: group})
		seen[group.lane] = true
	}
	for lane := range participantSet {
		if !seen[lane] {
			return nil, fmt.Errorf("transformer: path invalidation lane %s is outside the frozen product", lane.ID())
		}
	}
	demands := make([]formalQualifiedGuardDemand, 0, 1)
	if step.guard != 0 {
		demands = append(demands, formalQualifiedGuardDemand{owner: variable, scope: operator.scope, arena: operator.code.terms, guard: step.guard})
	}
	return &formalPathInvalidationStep{
		target: span.keys.FormatReadOnly(target), scope: config.Scope, demands: demands,
		lanes: lanes, owner: owner, variable: variable,
	}, nil
}

func (a *formalTupleAlgebra) applyFormalPathInvalidation(operator formalRelationOperatorRef, predecessor formalRelationTuple) (formalRelationTuple, error) {
	plan := operator.pathInvalidation
	if plan == nil || plan.variable != predecessor.variable {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal path invalidation is unbound")
	}
	return a.applyFormalEffectStep(operator, predecessor, plan.demands,
		func(span formalFiberDescriptorSpan, evaluator formalTupleLeafEvaluator) ([]decisionLeaf, error) {
			return a.applyFormalPathInvalidationLeaf(span, evaluator, plan)
		})
}

func (a *formalTupleAlgebra) applyFormalPathInvalidationLeaf(span formalFiberDescriptorSpan, evaluator formalTupleLeafEvaluator, plan *formalPathInvalidationStep) ([]decisionLeaf, error) {
	if plan == nil || plan.variable != span.variable || !evaluator.valid() || evaluator.variable != span.variable || plan.target == "" {
		return nil, errFormalComponentForeignOwner
	}
	domain := evaluator.authority.product
	complete, err := evaluator.completeLeaves()
	if err != nil {
		return nil, err
	}
	current := make(map[state.LaneOrdinal]state.LaneFactor, len(plan.lanes))
	for _, lane := range plan.lanes {
		factor, err := a.materializeFormalEffectLane(evaluator.authority, span, lane.group, complete)
		if err != nil {
			return nil, err
		}
		current[lane.group.lane.Ordinal()] = factor
	}
	ownerFactor, foundOwner := current[plan.owner.Lane().Ordinal()]
	if !foundOwner || ownerFactor.Lane() != plan.owner.Lane() {
		return nil, errFormalComponentMalformed
	}
	ownerSkeleton, ownerScalars, err := domain.DecomposeCoordinateFamily(ownerFactor, plan.owner, span.keys)
	if err != nil {
		return nil, err
	}
	out := append([]decisionLeaf(nil), complete...)
	switch plan.scope {
	case InvalidationScopeSubtree:
		transaction, prepareErr := domain.PrepareCoordinatePathSubtreeMutation(ownerSkeleton, ownerScalars, plan.target)
		if prepareErr != nil {
			return nil, prepareErr
		}
		factors, bindErr := domain.BindPathSubtreeMutationFactors(span.keys, func(lane state.ProductLane) (state.LaneFactor, bool) {
			factor, present := current[lane.Ordinal()]
			return factor, present && factor.Lane() == lane
		})
		if bindErr != nil {
			return nil, bindErr
		}
		factors, applyErr := domain.ApplyPathSubtreeMutationFactors(transaction, factors)
		if applyErr != nil {
			return nil, applyErr
		}
		for _, factor := range factors.LaneFactors() {
			current[factor.Lane().Ordinal()] = factor
		}
		for _, factor := range factors.CoordinateFactors() {
			lane := factor.Family().Lane()
			base, present := current[lane.Ordinal()]
			if !present || base.Lane() != lane {
				return nil, errFormalComponentMalformed
			}
			base, applyErr = domain.ReplaceCoordinateFamily(base, factor.Skeleton(), factor.Scalars())
			if applyErr != nil {
				return nil, applyErr
			}
			current[lane.Ordinal()] = base
		}
		for _, lane := range plan.lanes {
			next, present := current[lane.group.lane.Ordinal()]
			if !present || next.Lane() != lane.group.lane {
				return nil, errFormalComponentMalformed
			}
			if factorErr := a.factorFormalEffectGroup(evaluator.authority, span, lane.group, state.ValueFactor[FormalSlot]{}, next, out); factorErr != nil {
				return nil, factorErr
			}
		}
	case InvalidationScopeDescendants:
		transaction, prepareErr := domain.PrepareCoordinatePathDescendantMutation(ownerSkeleton, ownerScalars, plan.target)
		if prepareErr != nil {
			return nil, prepareErr
		}
		for _, lane := range plan.lanes {
			before, present := current[lane.group.lane.Ordinal()]
			if !present || before.Lane() != lane.group.lane {
				return nil, errFormalComponentMalformed
			}
			next, applyErr := domain.ApplyPathDescendantMutationLane(transaction, before)
			if applyErr != nil {
				return nil, applyErr
			}
			if err := a.factorFormalEffectGroup(evaluator.authority, span, lane.group, state.ValueFactor[FormalSlot]{}, next, out); err != nil {
				return nil, err
			}
		}
	default:
		return nil, errFormalComponentMalformed
	}
	return out, nil
}
