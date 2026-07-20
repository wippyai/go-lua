package transformer

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// formalPathInvalidationStep is the checked binding from one existing
// relationCode EffectInvalidatePath node to the ProductDomain path-mutation
// laws. It owns no mutation semantics and no alternate Effect ordering.
type formalPathInvalidationStep struct {
	target   pathdom.PathKey
	scope    InvalidationScope
	demands  []formalQualifiedGuardDemand
	lanes    []state.ProductLane
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
	lanes := make([]state.ProductLane, 0, len(participants))
	seen := make(map[state.ProductLane]bool)
	for _, group := range span.groupDescriptors() {
		if group.kind == formalFiberGroupValues {
			continue
		}
		if !participantSet[group.lane] {
			continue
		}
		lanes = append(lanes, group.lane)
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
		func(span formalFiberDescriptorSpan, evaluator formalTupleLeafEvaluator, values state.ValueFactor[FormalSlot], factors []state.LaneFactor) (state.ValueFactor[FormalSlot], []state.LaneFactor, error) {
			return a.applyFormalPathInvalidationLeaf(span, evaluator, plan, values, factors)
		})
}

func (a *formalTupleAlgebra) applyFormalPathInvalidationLeaf(span formalFiberDescriptorSpan, evaluator formalTupleLeafEvaluator, plan *formalPathInvalidationStep, values state.ValueFactor[FormalSlot], factors []state.LaneFactor) (state.ValueFactor[FormalSlot], []state.LaneFactor, error) {
	if plan == nil || plan.variable != span.variable || !evaluator.valid() || evaluator.variable != span.variable || plan.target == "" {
		return state.ValueFactor[FormalSlot]{}, nil, errFormalComponentForeignOwner
	}
	domain := evaluator.authority.product
	position := func(lane state.ProductLane) (int, bool) {
		for index := range factors {
			if factors[index].Lane() == lane {
				return index, true
			}
		}
		return 0, false
	}
	ownerIndex, foundOwner := position(plan.owner.Lane())
	if !foundOwner {
		return state.ValueFactor[FormalSlot]{}, nil, errFormalComponentMalformed
	}
	ownerFactor := factors[ownerIndex]
	ownerSkeleton, ownerScalars, err := domain.DecomposeCoordinateFamily(ownerFactor, plan.owner, span.keys)
	if err != nil {
		return state.ValueFactor[FormalSlot]{}, nil, err
	}
	switch plan.scope {
	case InvalidationScopeSubtree:
		transaction, prepareErr := domain.PrepareCoordinatePathSubtreeMutation(ownerSkeleton, ownerScalars, plan.target)
		if prepareErr != nil {
			return state.ValueFactor[FormalSlot]{}, nil, prepareErr
		}
		bound, bindErr := domain.BindPathSubtreeMutationFactors(span.keys, func(lane state.ProductLane) (state.LaneFactor, bool) {
			index, present := position(lane)
			if !present {
				return state.LaneFactor{}, false
			}
			return factors[index], true
		})
		if bindErr != nil {
			return state.ValueFactor[FormalSlot]{}, nil, bindErr
		}
		mutated, applyErr := domain.ApplyPathSubtreeMutationFactors(transaction, bound)
		if applyErr != nil {
			return state.ValueFactor[FormalSlot]{}, nil, applyErr
		}
		for _, factor := range mutated.LaneFactors() {
			index, present := position(factor.Lane())
			if !present {
				return state.ValueFactor[FormalSlot]{}, nil, errFormalComponentMalformed
			}
			factors[index] = factor
		}
		for _, factor := range mutated.CoordinateFactors() {
			lane := factor.Family().Lane()
			index, present := position(lane)
			if !present {
				return state.ValueFactor[FormalSlot]{}, nil, errFormalComponentMalformed
			}
			base := factors[index]
			base, applyErr = domain.ReplaceCoordinateFamily(base, factor.Skeleton(), factor.Scalars())
			if applyErr != nil {
				return state.ValueFactor[FormalSlot]{}, nil, applyErr
			}
			factors[index] = base
		}
	case InvalidationScopeDescendants:
		transaction, prepareErr := domain.PrepareCoordinatePathDescendantMutation(ownerSkeleton, ownerScalars, plan.target)
		if prepareErr != nil {
			return state.ValueFactor[FormalSlot]{}, nil, prepareErr
		}
		for _, lane := range plan.lanes {
			index, present := position(lane)
			if !present {
				return state.ValueFactor[FormalSlot]{}, nil, errFormalComponentMalformed
			}
			before := factors[index]
			next, applyErr := domain.ApplyPathDescendantMutationLane(transaction, before)
			if applyErr != nil {
				return state.ValueFactor[FormalSlot]{}, nil, applyErr
			}
			factors[index] = next
		}
	default:
		return state.ValueFactor[FormalSlot]{}, nil, errFormalComponentMalformed
	}
	return values, factors, nil
}
