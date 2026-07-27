package transformer

import (
	"fmt"
	"sort"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// formalEnvironmentWriteStep is a frozen adapter into the existing Values
// group. relationCode remains the sole syntax; this plan only makes target
// ownership and guard dependencies unrepresentably ambiguous at runtime.
type formalEnvironmentWriteStep struct {
	variable     relationVar
	code         *relationCode
	scope        loopMuTerm
	target       formalFiberGroupMember
	values       formalFiberGroupDescriptor
	valuesTop    formalFiberGroupMember
	value        formalQualifiedBinding
	readOrdinals []formalFiberOrdinal
	readGroups   []formalFiberGroupDescriptor
	demands      []formalQualifiedGuardDemand
	sealed       bool
}

func (p *formalEnvironmentWriteStep) valid(operator formalRelationOperatorRef) bool {
	return p != nil && p.sealed && p.variable != 0 && p.code != nil && p.code == operator.code &&
		p.scope == operator.scope && p.value.scope == p.scope && p.value.value.owner == p.variable &&
		p.value.value.arena == p.code.terms && p.target.group.variable == p.variable &&
		p.target.group.same(p.values) && p.valuesTop.group.same(p.values) && len(p.readOrdinals) != 0
}

func freezeFormalEnvironmentWriteStep(program *RelationProgram, variable relationVar, operator formalRelationOperatorRef) (*formalEnvironmentWriteStep, error) {
	if program == nil || variable == 0 || int(variable) > len(program.bodies) || operator.kind != formalRelationCellStep ||
		operator.code == nil || operator.root == 0 || operator.step == 0 || int(operator.root) >= len(operator.code.nodes) ||
		int(operator.step) > len(operator.code.nodes[operator.root].steps) {
		return nil, nil
	}
	step := operator.code.nodes[operator.root].steps[operator.step-1]
	if step.kind != boundaryStepEnvironmentWrite {
		return nil, nil
	}
	body := &program.bodies[variable-1]
	if program.formalSlots == nil || program.formalFibers == nil || body.variable != variable ||
		body.relation.code != operator.code || operator.code.terms == nil || !operator.code.terms.Sealed() ||
		step.slot == 0 || !operator.code.terms.validEnvironmentSlot(step.slot) || step.value == 0 || int(step.value) >= len(operator.code.terms.values) {
		return nil, fmt.Errorf("step has no sealed lexical slot/value ownership")
	}
	slot, ok := formalMiddleSlotForStateKey(program, body, step.slot)
	if !ok {
		return nil, fmt.Errorf("target slot has no formal identity")
	}
	span, ok := program.formalFibers.span(variable)
	if !ok {
		return nil, errFormalComponentForeignOwner
	}
	values, ok := span.valuesGroup()
	if !ok {
		return nil, fmt.Errorf("target body has no complete Values group")
	}
	target, ok := values.slot(slot)
	if !ok {
		return nil, fmt.Errorf("target slot is outside the Values group")
	}
	valuesTop, ok := values.top()
	if !ok {
		return nil, fmt.Errorf("target body has no Values Top fiber")
	}
	access, err := body.valueTermFactorAccess(step.value)
	if err != nil {
		return nil, err
	}
	readOrdinals := make([]formalFiberOrdinal, 0, len(access.Values)+2)
	for _, concrete := range access.Values {
		formal, present := formalMiddleSlotForStateKey(program, body, concrete)
		if !present {
			return nil, fmt.Errorf("EnvironmentWrite source slot %d has no formal identity", concrete)
		}
		member, present := values.slot(formal)
		if !present {
			return nil, fmt.Errorf("EnvironmentWrite source slot %d is outside formal Values", concrete)
		}
		ordinal, present := member.address(member.group)
		if !present {
			return nil, errFormalComponentMalformed
		}
		readOrdinals = append(readOrdinals, ordinal)
	}
	groups := make(map[state.LaneOrdinal]formalFiberGroupDescriptor)
	for _, group := range span.groupDescriptors() {
		if group.kind != formalFiberGroupValues {
			groups[group.lane.Ordinal()] = group
		}
	}
	readGroups := make([]formalFiberGroupDescriptor, 0, access.Lanes.Len())
	for _, id := range access.Lanes.IDs() {
		lane, present := body.productDomain.ProductLane(id)
		if !present {
			return nil, fmt.Errorf("EnvironmentWrite source lane %q is outside the product", id)
		}
		group, present := groups[lane.Ordinal()]
		if !present {
			return nil, fmt.Errorf("EnvironmentWrite source lane %q is outside formal fibers", id)
		}
		readGroups = append(readGroups, group)
		readOrdinals = append(readOrdinals, group.members...)
	}
	for _, member := range []formalFiberGroupMember{valuesTop, target} {
		ordinal, present := member.address(member.group)
		if !present {
			return nil, errFormalComponentMalformed
		}
		readOrdinals = append(readOrdinals, ordinal)
	}
	sort.Slice(readOrdinals, func(i, j int) bool { return readOrdinals[i] < readOrdinals[j] })
	write := 0
	for _, ordinal := range readOrdinals {
		if write == 0 || readOrdinals[write-1] != ordinal {
			readOrdinals[write] = ordinal
			write++
		}
	}
	readOrdinals = readOrdinals[:write]
	guards, err := reachableValueTermGuards(operator.code.terms, step.value)
	if err != nil {
		return nil, err
	}
	demands := make([]formalQualifiedGuardDemand, 0, len(guards))
	for _, guard := range guards {
		duplicate := false
		for _, prior := range demands {
			if prior.guard == guard {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		demands = append(demands, formalQualifiedGuardDemand{owner: variable, scope: operator.scope, arena: operator.code.terms, guard: guard})
		atoms := make(map[ValueTerm]struct{})
		if err := collectRelationGuardAtoms(operator.code.terms, guard, atoms, make(map[Guard]uint8)); err != nil {
			return nil, err
		}
		for atom := range atoms {
			if program.formalGuards == nil {
				return nil, fmt.Errorf("value guard has no formal vocabulary")
			}
			if _, ranked := program.formalGuards.lexicalRank(variable, operator.scope, operator.code.terms, atom); !ranked {
				return nil, fmt.Errorf("value guard atom is outside the operator's lexical scope")
			}
		}
	}
	value := formalQualifiedBinding{
		value: relationArenaValueRef{owner: variable, arena: operator.code.terms, term: step.value},
		scope: operator.scope,
	}
	return &formalEnvironmentWriteStep{
		variable: variable, code: operator.code, scope: operator.scope,
		target: target, values: values.descriptor, valuesTop: valuesTop,
		value: value, readOrdinals: readOrdinals, readGroups: readGroups, demands: demands, sealed: true,
	}, nil
}

func (l formalSparseLeafView) evaluateEnvironmentWrite(plan *formalEnvironmentWriteStep) (formalValue, error) {
	if plan == nil || plan.value.value.owner != l.variable || plan.value.value.arena != l.authority.terms {
		return formalValue{}, errFormalComponentForeignOwner
	}
	if symbolic, present, err := l.symbolicFactorOutput(plan.value, plan.readOrdinals); err != nil || present {
		return symbolic, err
	}
	arena := plan.value.value.arena
	var resolver valueNodeLeafResolver
	var factors []state.LaneFactor
	factorsReady := false
	resolver = valueNodeLeafResolver{
		guard: func(guard Guard) (bool, bool, bool) {
			return l.exactGuard(l.variable, arena, plan.value.scope, guard)
		},
		root: func(root Root) (product.Value, bool) {
			concrete, present := l.body.rootValueSlot(root)
			if !present {
				return product.Value{}, false
			}
			slot, present := formalMiddleSlotForStateKey(l.algebra.program, l.body, concrete)
			if !present {
				return product.Value{}, false
			}
			member, present := (formalValuesFiberGroup{descriptor: plan.values}).slot(slot)
			if !present {
				return product.Value{}, false
			}
			return l.value(member, plan.valuesTop)
		},
		slot: func(slot statekey.Value) (product.Value, bool) {
			formal, present := formalMiddleSlotForStateKey(l.algebra.program, l.body, slot)
			if !present {
				return product.Value{}, false
			}
			member, present := (formalValuesFiberGroup{descriptor: plan.values}).slot(formal)
			if !present {
				return product.Value{}, false
			}
			return l.value(member, plan.valuesTop)
		},
		dynamicRead: func(node valueNode, args []product.Value) (product.Value, bool) {
			if !factorsReady {
				factors = make([]state.LaneFactor, len(plan.readGroups))
				for index, group := range plan.readGroups {
					factor, err := l.laneFactor(group)
					if err != nil {
						return product.Value{}, false
					}
					factors[index] = factor
				}
				factorsReady = true
			}
			return resolveFormalDynamicValue(l.body, l.span, node, args, factors, func(child ValueTerm) (product.Value, bool) {
				return arena.evalValueCanonicalWithLeaves(child, resolver)
			})
		},
		allocationResult: func(candidate valueNode) (product.Value, bool) {
			return arena.allocationResult(candidate.allocation, candidate.resultIndex)
		},
		completeImpossibleConcat: func() (product.Value, bool) {
			return product.Bottom(l.authority.product.Registry()), true
		},
	}
	resolver.scope = func(current ValueTerm, inherited valueNodeLeafResolver) valueNodeLeafResolver {
		if current != plan.value.value.term {
			inherited.completeImpossibleConcat = nil
		}
		return inherited
	}
	value, exact := arena.evalValueCanonicalWithLeaves(plan.value.value.term, resolver)
	if !exact || !product.BelongsToRegistry(l.authority.product.Registry(), value) {
		return formalValue{}, fmt.Errorf("transformer: formal EnvironmentWrite value is unsupported")
	}
	return formalGroundValue(value), nil
}

func (a *formalTupleAlgebra) applyFormalEnvironmentWrite(operator formalRelationOperatorRef, predecessor formalRelationTuple) (formalRelationTuple, error) {
	if a == nil || operator.kind != formalRelationCellStep || operator.code == nil || !operator.environmentWrite.valid(operator) {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal EnvironmentWrite has no complete Values transaction")
	}
	if err := a.validateTuple(predecessor); err != nil {
		return formalRelationTuple{}, err
	}
	if predecessor.bottom() {
		return predecessor, nil
	}
	plan := operator.environmentWrite
	span, directory, authority, ok := a.span(predecessor.variable)
	if !ok || authority.code != operator.code || predecessor.root.owner != directory ||
		!plan.value.validForAuthority(authority) {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal EnvironmentWrite predecessor has foreign ownership")
	}
	group := plan.target.group
	ordinal, ok := plan.target.address(group)
	if !ok || group.variable != predecessor.variable || group.kind != formalFiberGroupValues || int(ordinal) >= span.count {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal EnvironmentWrite target is foreign")
	}
	top, ok := formalValuesFiberGroup{descriptor: group}.top()
	if !ok {
		return formalRelationTuple{}, errFormalComponentMalformed
	}
	topOrdinal, ok := top.address(group)
	if !ok {
		return formalRelationTuple{}, errFormalComponentMalformed
	}
	regions, err := a.partitionSparseLeafViewsUnderCare([]formalSparseTupleProjection{{
		tuple: predecessor, ordinals: plan.readOrdinals,
	}}, plan.demands)
	if err != nil {
		return formalRelationTuple{}, err
	}
	mark := a.decisions.checkpoint()
	fail := func(err error) (formalRelationTuple, error) {
		a.decisions.rollback(mark)
		return formalRelationTuple{}, err
	}
	targetRoot := decisionFalse
	bottom := product.Bottom(authority.product.Registry())
	for _, region := range regions {
		if len(region.views) != 1 {
			return fail(errDecisionMalformed)
		}
		view := region.views[0]
		var leaf decisionLeaf
		topLeaf, topPresent := view.leaf(topOrdinal)
		currentLeaf, currentPresent := view.leaf(ordinal)
		if !topPresent || !currentPresent || topLeaf > 1 {
			return fail(errFormalComponentMalformed)
		}
		if topLeaf == 1 {
			// The lifted Values Top has no finite per-slot exception.
			leaf = currentLeaf
		} else {
			actual, evalErr := view.evaluateEnvironmentWrite(plan)
			if evalErr != nil {
				return fail(evalErr)
			}
			ground, concrete := actual.concrete()
			if !concrete || !product.Equal(authority.product.Registry(), ground, bottom) {
				leaf, evalErr = authority.internFormalValue(actual)
				if evalErr != nil {
					return fail(evalErr)
				}
			}
		}
		targetRoot, err = a.decisions.condition(a.ctx, region.guard, a.decisions.terminal(leaf), targetRoot)
		if err != nil {
			return fail(err)
		}
	}
	descriptor := span.forest.descriptors[span.first+int(ordinal)]
	if err := a.validateDescriptorRoot(authority, descriptor, targetRoot); err != nil {
		return fail(err)
	}
	prior, err := directory.valueAt(predecessor.root, ordinal)
	if err != nil {
		return fail(err)
	}
	if prior == formalFiberValue(targetRoot) {
		return predecessor, nil
	}
	delta, err := directory.sealDelta([]formalFiberWrite{{ordinal: ordinal, value: formalFiberValue(targetRoot)}})
	if err != nil {
		return fail(err)
	}
	root, _, err := directory.applyDelta(predecessor.root, delta)
	if err != nil {
		return fail(err)
	}
	result := a.normalize(formalRelationTuple{variable: predecessor.variable, root: root})
	if err := a.validateTuple(result); err != nil {
		return fail(err)
	}
	return result, nil
}
