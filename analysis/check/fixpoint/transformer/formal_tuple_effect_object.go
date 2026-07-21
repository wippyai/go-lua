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
	objects    []formalObjectMaterializationObject
	components []formalObjectMaterializationComponent
	variable   relationVar
}

type formalObjectMaterializationComponent struct {
	objectIndex  int
	memberIndex  int
	memberSource int
	constructor  state.ObjectConstructorPlan
	frame        formalProductFactorFrameBinding
	lift         formalClosedFactorLift
	valueAccess  state.TransferInputAccess
	valueGroups  []formalFiberGroupDescriptor
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
	if node.kind != EffectObjectMaterialization && node.kind != EffectPathStore {
		return nil, nil
	}
	if node.kind == EffectPathStore && len(node.pathStoreObject.Heaps) == 0 {
		return nil, nil
	}
	if len(node.pathStoreObject.Heaps) == 0 || node.pathStoreObject.ListFloor != 0 ||
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
	rootObjects := make(map[ValueTerm]int, len(node.pathStoreObject.Heaps))
	for objectIndex, object := range node.pathStoreObject.Heaps {
		if object.Root == 0 {
			return nil, fmt.Errorf("transformer: object materialization root is absent")
		}
		rootObjects[object.Root] = objectIndex
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
	sealComponents := func(objectIndex, memberIndex int) ([]formalObjectMaterializationComponent, error) {
		object := objects[objectIndex]
		terms := []ValueTerm{object.root}
		shape := state.ObjectConstructorShape{Identity: object.identity, StableShape: object.stableShape}
		memberSource := -1
		if memberIndex >= 0 {
			shape.MemberSuffixes = [][]segment.Segment{object.suffixes[memberIndex]}
			memberSource = object.memberRoots[memberIndex]
			memberTerm := object.members[memberIndex]
			if memberSource >= 0 {
				memberTerm = objects[memberSource].root
			}
			terms = append(terms, memberTerm)
		}
		valueAccess, valueGroups, componentErr := freezeFormalValueFactorAccess(program, variable, terms...)
		if componentErr != nil {
			return nil, componentErr
		}
		constructor, componentErr := body.productDomain.PrepareObjectConstructorPlan(span.keys, []state.ObjectConstructorShape{shape})
		if componentErr != nil {
			return nil, componentErr
		}
		coordinateWrites, componentErr := body.productDomain.ObjectConstructorCoordinateWrites(constructor)
		if componentErr != nil {
			return nil, componentErr
		}
		coordinates, componentErr := body.productDomain.SealCoordinateFactorInventory(span.keys, coordinateWrites)
		if componentErr != nil {
			return nil, componentErr
		}
		coordinates, componentErr = body.productDomain.CloseCoordinateFactorInventory(span.keys, coordinates)
		if componentErr != nil {
			return nil, componentErr
		}
		families := make([]state.CoordinateFamily, 0)
		for _, slot := range coordinates.Slots() {
			family := slot.Family()
			if len(families) == 0 || families[len(families)-1] != family {
				families = append(families, family)
			}
		}
		familyIndex := make(map[state.CoordinateFamily]int, len(families))
		parents := make([]int, len(families))
		for index, family := range families {
			familyIndex[family], parents[index] = index, index
		}
		var find func(int) int
		find = func(index int) int {
			if parents[index] != index {
				parents[index] = find(parents[index])
			}
			return parents[index]
		}
		union := func(left, right int) {
			left, right = find(left), find(right)
			if left != right {
				parents[right] = left
			}
		}
		for index, family := range families {
			slots, slotErr := coordinates.FamilySlots(family)
			if slotErr != nil || len(slots) == 0 {
				if slotErr == nil {
					slotErr = errFormalComponentMalformed
				}
				return nil, slotErr
			}
			seed, closeErr := body.productDomain.SealCoordinateFactorInventory(span.keys, slots)
			if closeErr == nil {
				seed, closeErr = body.productDomain.CloseCoordinateFactorInventory(span.keys, seed)
			}
			if closeErr != nil {
				return nil, closeErr
			}
			for _, selected := range seed.Slots() {
				other, present := familyIndex[selected.Family()]
				if !present {
					return nil, errFormalComponentMalformed
				}
				union(index, other)
			}
		}
		components := make([]formalObjectMaterializationComponent, 0, len(families))
		for index := range families {
			if find(index) != index {
				continue
			}
			var slots []state.CoordinateSlot
			for candidate, family := range families {
				if find(candidate) != index {
					continue
				}
				owned, slotErr := coordinates.FamilySlots(family)
				if slotErr != nil {
					return nil, slotErr
				}
				slots = append(slots, owned...)
			}
			familyCoordinates, familyErr := body.productDomain.SealCoordinateFactorInventory(span.keys, slots)
			if familyErr == nil {
				familyCoordinates, familyErr = body.productDomain.CloseCoordinateFactorInventory(span.keys, familyCoordinates)
			}
			if familyErr != nil {
				return nil, familyErr
			}
			selection, familyErr := body.productDomain.SealProductFactorSelection(nil, familyCoordinates, nil, false)
			if familyErr != nil {
				return nil, familyErr
			}
			frame, familyErr := sealFormalProductFactorFrameBinding(body.productDomain, span, selection, nil, false, true)
			if familyErr != nil {
				return nil, familyErr
			}
			lift, familyErr := sealFormalClosedFactorLift(span, [][]formalFiberOrdinal{frame.ordinals}, frame.ordinals)
			if familyErr != nil {
				return nil, familyErr
			}
			components = append(components, formalObjectMaterializationComponent{
				objectIndex: objectIndex, memberIndex: memberIndex, memberSource: memberSource,
				constructor: constructor, frame: frame, lift: lift,
				valueAccess: valueAccess, valueGroups: valueGroups,
			})
		}
		return components, nil
	}
	components := make([]formalObjectMaterializationComponent, 0, len(objects))
	for objectIndex := range objects {
		sealed, componentErr := sealComponents(objectIndex, -1)
		if componentErr != nil {
			return nil, componentErr
		}
		components = append(components, sealed...)
	}
	for objectIndex, object := range objects {
		for memberIndex := range object.members {
			sealed, componentErr := sealComponents(objectIndex, memberIndex)
			if componentErr != nil {
				return nil, componentErr
			}
			components = append(components, sealed...)
		}
	}
	return &formalObjectMaterializationStep{objects: objects, components: components, variable: variable}, nil
}

func (a *formalTupleAlgebra) applyFormalObjectMaterialization(operator formalRelationOperatorRef, predecessor formalRelationTuple) (formalRelationTuple, error) {
	plan := operator.objectMaterialization
	if plan == nil || plan.variable != predecessor.variable || len(plan.components) == 0 {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal object materialization is unbound")
	}
	mark := a.decisions.checkpoint()
	current := predecessor
	stable := make(map[ValueTerm]bool)
	stableDecisions := make(map[ValueTerm]decisionRef)
	isStable := func(term ValueTerm) (bool, error) {
		if value, present := stable[term]; present {
			return value, nil
		}
		access, err := a.program.bodies[predecessor.variable-1].valueTermLaneFactorAccess(term)
		if err != nil {
			return false, err
		}
		value := access.Lanes.Len() == 0
		stable[term] = value
		return value, nil
	}
	for componentIndex := range plan.components {
		component := &plan.components[componentIndex]
		object := plan.objects[component.objectIndex]
		terms := []ValueTerm{object.root}
		if component.memberIndex >= 0 {
			memberTerm := object.members[component.memberIndex]
			if component.memberSource >= 0 {
				memberTerm = plan.objects[component.memberSource].root
			}
			terms = append(terms, memberTerm)
		}
		derived := make([]decisionRef, len(terms))
		for index, term := range terms {
			stableTerm, err := isStable(term)
			if err != nil {
				a.decisions.rollback(mark)
				return formalRelationTuple{}, err
			}
			if stableTerm {
				if prior, present := stableDecisions[term]; present {
					derived[index] = prior
					continue
				}
			}
			compiled, err := a.compileFormalValueTermDecisions(current, operator.code.terms, operator.scope, term)
			if err != nil || len(compiled) != 1 {
				if err == nil {
					err = errFormalComponentMalformed
				}
				a.decisions.rollback(mark)
				return formalRelationTuple{}, err
			}
			derived[index] = compiled[0]
			if stableTerm {
				stableDecisions[term] = compiled[0]
			}
		}
		var err error
		current, err = a.applyFormalEffectDerivedLift(operator, current, nil, component.lift, derived,
			func(view formalSparseLeafView) ([]formalClosedFactorLeafWrite, error) {
				frame, err := a.materializeFormalProductFactorFrame(view, component.frame)
				if err != nil {
					return nil, err
				}
				frame, err = a.applyFormalObjectMaterializationLeaf(view, plan, component, frame)
				if err != nil {
					return nil, err
				}
				writes, err := a.factorFormalProductFactorFrame(view, component.frame, frame)
				if err != nil {
					return nil, err
				}
				changed := writes[:0]
				for _, write := range writes {
					prior, present := view.leaf(write.ordinal)
					if !present {
						return nil, errFormalComponentMalformed
					}
					if prior != write.leaf {
						changed = append(changed, write)
					}
				}
				return changed, nil
			})
		if err != nil {
			a.decisions.rollback(mark)
			return formalRelationTuple{}, err
		}
	}
	return current, nil
}

func (a *formalTupleAlgebra) applyFormalObjectMaterializationLeaf(
	view formalSparseLeafView,
	plan *formalObjectMaterializationStep,
	component *formalObjectMaterializationComponent,
	frame state.ProductFactorFrame,
) (state.ProductFactorFrame, error) {
	if plan == nil || component == nil || plan.variable != view.variable || view.authority == nil ||
		component.objectIndex < 0 || component.objectIndex >= len(plan.objects) {
		return state.ProductFactorFrame{}, errFormalComponentForeignOwner
	}
	object := plan.objects[component.objectIndex]
	if len(view.derived) != 1 && (component.memberIndex < 0 || len(view.derived) != 2) {
		return state.ProductFactorFrame{}, errFormalComponentMalformed
	}
	root, err := formalGroundValueDecisionLeaf(view.authority, view.derived[0])
	if err != nil {
		return state.ProductFactorFrame{}, err
	}
	root = identityvalue.WithExactTerm(a.program.registry, root, object.identity)
	row := state.ObjectConstructorValues{Root: root}
	if component.memberIndex >= 0 {
		if component.memberIndex >= len(object.members) {
			return state.ProductFactorFrame{}, errFormalComponentMalformed
		}
		member, memberErr := formalGroundValueDecisionLeaf(view.authority, view.derived[1])
		if memberErr != nil {
			return state.ProductFactorFrame{}, memberErr
		}
		if component.memberSource >= 0 {
			if component.memberSource >= len(plan.objects) {
				return state.ProductFactorFrame{}, errFormalComponentMalformed
			}
			member = identityvalue.WithExactTerm(a.program.registry, member, plan.objects[component.memberSource].identity)
		}
		row.Members = []product.Value{member}
	}
	return view.authority.product.ApplyObjectConstructorFrame(component.constructor, []state.ObjectConstructorValues{row}, component.frame.selection, frame)
}
