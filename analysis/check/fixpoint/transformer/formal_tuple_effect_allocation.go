package transformer

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// formalAllocationTemplateStep binds one relationCode allocation occurrence
// directly to the shared identity.Term object-graph join law.
type formalAllocationTemplateStep struct {
	graph         state.ObjectGraphMutationPlan
	demands       []formalQualifiedGuardDemand
	groups        []formalFiberGroupDescriptor
	writeOrdinals []formalFiberOrdinal
	variable      relationVar
}

func freezeFormalAllocationGraph(body *relationProgramBody, keys *keyspace.KeySpace, allocation AllocationTemplateTerm) (state.ObjectGraphMutationPlan, error) {
	if body == nil || body.relation.arena == nil || !body.relation.arena.validAllocation(allocation) || keys == nil || !keys.Valid() {
		return state.ObjectGraphMutationPlan{}, fmt.Errorf("transformer: formal allocation graph is unowned")
	}
	node := body.relation.arena.allocations[allocation]
	if len(node.identities) == 0 || len(node.identities) != len(node.op.Template().Objects) {
		return state.ObjectGraphMutationPlan{}, fmt.Errorf("transformer: formal allocation graph has no complete identity map")
	}
	materialized, ok := effectlowering.MaterializeSymbolicStaticAllocation(
		body.productDomain.Registry(), nil, keys, cfg.Point(node.op.Site().Ordinal), node.op.Template(), node.identities,
	)
	if !ok {
		return state.ObjectGraphMutationPlan{}, fmt.Errorf("transformer: formal allocation graph failed symbolic lowering")
	}
	return body.productDomain.PrepareObjectGraphJoinPlan(keys, materialized.Mutations)
}

func freezeFormalAllocationTemplateStep(program *RelationProgram, variable relationVar, operator formalRelationOperatorRef) (*formalAllocationTemplateStep, error) {
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
	if node.kind != EffectAllocationTemplate {
		return nil, nil
	}
	body := &program.bodies[variable-1]
	span, ok := program.formalFibers.span(variable)
	if !ok || body.relation.code != operator.code {
		return nil, fmt.Errorf("transformer: formal allocation has no formal owner")
	}
	graph, err := freezeFormalAllocationGraph(body, span.keys, node.allocation)
	if err != nil {
		return nil, err
	}
	groupsByLane := make(map[state.ProductLane]formalFiberGroupDescriptor)
	for _, group := range span.groupDescriptors() {
		if group.kind != formalFiberGroupValues {
			groupsByLane[group.lane] = group
		}
	}
	lanes := body.productDomain.ObjectMutationParticipantLanes()
	groups := make([]formalFiberGroupDescriptor, len(lanes))
	var writeOrdinals []formalFiberOrdinal
	for index, lane := range lanes {
		group, present := groupsByLane[lane]
		if !present {
			return nil, fmt.Errorf("transformer: formal allocation lane %s is outside the frozen product", lane.ID())
		}
		groups[index] = group
		writeOrdinals = append(writeOrdinals, group.members...)
	}
	sort.Slice(writeOrdinals, func(i, j int) bool { return writeOrdinals[i] < writeOrdinals[j] })
	for index := 1; index < len(writeOrdinals); index++ {
		if writeOrdinals[index-1] == writeOrdinals[index] {
			return nil, fmt.Errorf("transformer: formal allocation write footprint overlaps at fiber %d", writeOrdinals[index])
		}
	}
	demands := make([]formalQualifiedGuardDemand, 0, 1)
	if step.guard != 0 {
		demands = append(demands, formalQualifiedGuardDemand{owner: variable, scope: operator.scope, arena: operator.code.terms, guard: step.guard})
	}
	return &formalAllocationTemplateStep{
		graph: graph, demands: demands, groups: groups,
		writeOrdinals: writeOrdinals, variable: variable,
	}, nil
}

func (a *formalTupleAlgebra) applyFormalAllocationTemplate(operator formalRelationOperatorRef, predecessor formalRelationTuple) (formalRelationTuple, error) {
	plan := operator.allocationTemplate
	if plan == nil || plan.variable != predecessor.variable {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal allocation template is unbound")
	}
	return a.applyFormalEffectStep(operator, predecessor, plan.demands,
		func(span formalFiberDescriptorSpan, evaluator formalTupleLeafEvaluator) ([]decisionLeaf, error) {
			complete, err := evaluator.completeLeaves()
			if err != nil {
				return nil, err
			}
			out := append([]decisionLeaf(nil), complete...)
			for _, group := range plan.groups {
				current, err := a.materializeFormalEffectLane(evaluator.authority, span, group, complete)
				if err != nil {
					return nil, err
				}
				next, err := evaluator.authority.product.ApplyObjectGraphMutationFactor(plan.graph, current)
				if err != nil {
					return nil, err
				}
				if err := a.factorFormalEffectGroup(evaluator.authority, span, group, state.ValueFactor[FormalSlot]{}, next, out); err != nil {
					return nil, err
				}
			}
			return out, nil
		}, plan.writeOrdinals...)
}
