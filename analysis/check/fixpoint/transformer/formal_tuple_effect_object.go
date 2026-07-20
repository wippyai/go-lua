package transformer

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

type formalObjectMaterializationObject struct {
	identity    identity.Term
	root        ValueTerm
	members     []ValueTerm
	memberRoots []int
	suffixes    [][]segment.Segment
	stableShape bool
}

// formalObjectMaterializationStep is only a checked binding from the existing
// EffectObjectMaterialization node to ObjectConstructorPlan. It carries no
// mutation semantics and introduces no second Effect language.
type formalObjectMaterializationStep struct {
	objects     []formalObjectMaterializationObject
	demands     []formalQualifiedGuardDemand
	valueAccess state.TransferInputAccess
	valueGroups []formalFiberGroupDescriptor
	variable    relationVar
}

func freezeFormalObjectMaterializationStep(program *RelationProgram, variable relationVar, operator formalRelationOperatorRef) (*formalObjectMaterializationStep, error) {
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
	if node.kind != EffectObjectMaterialization {
		return nil, nil
	}
	if len(node.pathStoreObject.Heaps) == 0 || len(node.pathStoreObject.Entries) != 0 || node.pathStoreObject.ListFloor != 0 ||
		operator.code.terms == nil || operator.code.terms != operator.code.effects.terms {
		return nil, fmt.Errorf("transformer: malformed formal object materialization")
	}
	body := &program.bodies[variable-1]
	span, ok := program.formalFibers.span(variable)
	if !ok || span.keys == nil || !span.keys.Valid() || body.relation.code != operator.code {
		return nil, fmt.Errorf("transformer: object materialization has no formal owner")
	}
	objects := make([]formalObjectMaterializationObject, len(node.pathStoreObject.Heaps))
	templates, err := formalObjectMaterializationTemplates(body.relation, step.effect)
	if err != nil {
		return nil, fmt.Errorf("transformer: object materialization allocation schema: %w", err)
	}
	if len(templates) != len(objects) {
		return nil, fmt.Errorf("transformer: object materialization allocation schema width %d, want %d", len(templates), len(objects))
	}
	var guards []Guard
	rootObjects := make(map[ValueTerm]int, len(node.pathStoreObject.Heaps))
	for objectIndex, object := range node.pathStoreObject.Heaps {
		if object.Root == 0 {
			return nil, fmt.Errorf("transformer: object materialization root is absent")
		}
		rootObjects[object.Root] = objectIndex
		rootGuards, err := reachableValueTermGuards(operator.code.terms, object.Root)
		if err != nil {
			return nil, err
		}
		guards = append(guards, rootGuards...)
		frozen := formalObjectMaterializationObject{
			identity: identity.AllocationTerm(templates[objectIndex]), root: object.Root, members: make([]ValueTerm, len(object.Members)),
			memberRoots: make([]int, len(object.Members)), suffixes: make([][]segment.Segment, len(object.Members)), stableShape: object.StableShape,
		}
		for memberIndex := range frozen.memberRoots {
			frozen.memberRoots[memberIndex] = -1
		}
		for memberIndex, member := range object.Members {
			if member.Value == 0 || len(member.Suffix) == 0 {
				return nil, fmt.Errorf("transformer: object materialization member is incomplete")
			}
			frozen.members[memberIndex] = member.Value
			frozen.suffixes[memberIndex] = append([]segment.Segment(nil), member.Suffix...)
			memberGuards, err := reachableValueTermGuards(operator.code.terms, member.Value)
			if err != nil {
				return nil, err
			}
			guards = append(guards, memberGuards...)
		}
		objects[objectIndex] = frozen
	}
	for objectIndex := range objects {
		for memberIndex, member := range objects[objectIndex].members {
			if sourceObject, present := rootObjects[member]; present {
				objects[objectIndex].memberRoots[memberIndex] = sourceObject
			}
		}
	}
	valueTerms := make([]ValueTerm, 0)
	for _, object := range objects {
		valueTerms = append(valueTerms, object.root)
		valueTerms = append(valueTerms, object.members...)
	}
	valueAccess, valueGroups, err := freezeFormalValueFactorAccess(program, variable, valueTerms...)
	if err != nil {
		return nil, err
	}
	if step.guard != 0 {
		guards = append(guards, step.guard)
	}
	demands := make([]formalQualifiedGuardDemand, len(guards))
	for index, guard := range guards {
		demands[index] = formalQualifiedGuardDemand{owner: variable, scope: operator.scope, arena: operator.code.terms, guard: guard}
	}
	return &formalObjectMaterializationStep{
		objects: objects, demands: demands,
		valueAccess: valueAccess, valueGroups: valueGroups, variable: variable,
	}, nil
}

func (a *formalTupleAlgebra) applyFormalObjectMaterialization(operator formalRelationOperatorRef, predecessor formalRelationTuple) (formalRelationTuple, error) {
	plan := operator.objectMaterialization
	if plan == nil || plan.variable != predecessor.variable {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal object materialization is unbound")
	}
	return a.applyFormalEffectStep(operator, predecessor, plan.demands,
		func(span formalFiberDescriptorSpan, evaluator formalTupleLeafEvaluator, values state.ValueFactor[FormalSlot], factors []state.LaneFactor) (state.ValueFactor[FormalSlot], []state.LaneFactor, error) {
			return a.applyFormalObjectMaterializationLeaf(operator, span, evaluator, plan, values, factors)
		})
}

func (a *formalTupleAlgebra) applyFormalObjectMaterializationLeaf(
	operator formalRelationOperatorRef,
	span formalFiberDescriptorSpan,
	evaluator formalTupleLeafEvaluator,
	plan *formalObjectMaterializationStep,
	formalValues state.ValueFactor[FormalSlot],
	factors []state.LaneFactor,
) (state.ValueFactor[FormalSlot], []state.LaneFactor, error) {
	if plan == nil || plan.variable != span.variable || !evaluator.valid() || evaluator.variable != span.variable {
		return state.ValueFactor[FormalSlot]{}, nil, errFormalComponentForeignOwner
	}
	shapes := make([]state.ObjectConstructorShape, len(plan.objects))
	values := make([]state.ObjectConstructorValues, len(plan.objects))
	capability, err := evaluator.materializeValueFactorAccess(plan.valueAccess, plan.valueGroups)
	if err != nil {
		return state.ValueFactor[FormalSlot]{}, nil, err
	}
	for objectIndex, object := range plan.objects {
		root, exact := evaluator.evalArenaValueWithFactorAccess(span.variable, operator.code.terms, object.root, operator.scope, formalApplyTermView{}, capability)
		if !exact {
			return state.ValueFactor[FormalSlot]{}, nil, fmt.Errorf("transformer: formal object root is unresolved")
		}
		root = identityvalue.WithExactTerm(a.program.registry, root, object.identity)
		term := object.identity
		shapes[objectIndex] = state.ObjectConstructorShape{
			Identity: term, MemberSuffixes: object.suffixes, StableShape: object.stableShape,
		}
		values[objectIndex] = state.ObjectConstructorValues{Root: root, Members: make([]product.Value, len(object.members))}
	}
	for objectIndex, object := range plan.objects {
		for memberIndex, member := range object.members {
			if sourceObject := object.memberRoots[memberIndex]; sourceObject >= 0 {
				if sourceObject >= len(values) {
					return state.ValueFactor[FormalSlot]{}, nil, fmt.Errorf("transformer: formal object member root is outside allocation schema")
				}
				values[objectIndex].Members[memberIndex] = values[sourceObject].Root
				continue
			}
			value, resolved := evaluator.evalArenaValueWithFactorAccess(span.variable, operator.code.terms, member, operator.scope, formalApplyTermView{}, capability)
			if !resolved {
				return state.ValueFactor[FormalSlot]{}, nil, fmt.Errorf("transformer: formal object member is unresolved")
			}
			values[objectIndex].Members[memberIndex] = value
		}
	}
	domain := evaluator.authority.product
	transaction, err := domain.PrepareObjectConstructorPlan(span.keys, shapes)
	if err != nil {
		return state.ValueFactor[FormalSlot]{}, nil, err
	}
	for index, current := range factors {
		next, applyErr := domain.ApplyObjectConstructorFactor(transaction, values, current)
		if applyErr != nil {
			return state.ValueFactor[FormalSlot]{}, nil, applyErr
		}
		factors[index] = next
	}
	return formalValues, factors, nil
}
