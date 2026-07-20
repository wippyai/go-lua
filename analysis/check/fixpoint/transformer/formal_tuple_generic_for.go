package transformer

import (
	"fmt"
	"sort"

	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
)

// formalGenericForStep is the frozen carrier binding for the one canonical
// factor-native generic-for transaction. relationCode owns the projection
// syntax; ProductDomain owns every residual factor law.
type formalGenericForStep struct {
	transaction       state.GenericForFactorTransaction
	projection        formalQualifiedBinding
	target            FormalSlot
	targetMember      formalFiberGroupMember
	valuesTop         formalFiberGroupMember
	projectionSlots   map[statekey.Value]formalFiberGroupMember
	projectionMembers []formalFiberGroupMember
	projectionAccess  state.TransferInputAccess
	projectionFactors []formalFiberGroupDescriptor
	currentOrdinals   []formalFiberOrdinal
	sourceOrdinals    []formalFiberOrdinal
	affectedOrdinals  []formalFiberOrdinal
	demands           []formalQualifiedGuardDemand
	values            formalFiberGroupDescriptor
	source            []formalFiberGroupDescriptor
	current           []formalFiberGroupDescriptor
	writes            []formalFiberGroupDescriptor
	sealed            bool
}

func (p *formalGenericForStep) valid(operator formalRelationOperatorRef) bool {
	return p != nil && p.sealed && p.transaction.Valid() && p.projection.value.owner != 0 &&
		p.projection.value.arena == operator.code.terms && p.values.valid() &&
		p.targetMember.group.same(p.values) && p.valuesTop.group.same(p.values) &&
		len(p.projectionMembers) == len(p.projectionSlots) &&
		len(p.projectionFactors) == p.projectionAccess.Lanes.Len() &&
		len(p.currentOrdinals) != 0 && len(p.sourceOrdinals) != 0 && len(p.affectedOrdinals) != 0 &&
		len(p.source) == len(p.transaction.SourceLanes()) &&
		len(p.current) == len(p.transaction.CurrentLanes()) &&
		len(p.writes) == len(p.transaction.WriteLanes())
}

func freezeFormalGenericForStep(program *RelationProgram, variable relationVar, operator formalRelationOperatorRef) (*formalGenericForStep, error) {
	if program == nil || variable == 0 || int(variable) > len(program.bodies) || operator.kind != formalRelationCellStep ||
		operator.code == nil || operator.root == 0 || operator.step == 0 || int(operator.root) >= len(operator.code.nodes) ||
		int(operator.step) > len(operator.code.nodes[operator.root].steps) {
		return nil, nil
	}
	step := operator.code.nodes[operator.root].steps[operator.step-1]
	if step.kind != boundaryStepGenericFor {
		return nil, nil
	}
	body := &program.bodies[variable-1]
	if len(step.access) != 1 || step.access[0].term == 0 || step.access[0].hasPoint ||
		!step.genericIdentity.valid(operator.code.terms, operator.code.shape) || step.genericIdentity.projection != step.access[0].term ||
		body.genericForMembership == nil || body.plan == nil || body.graph == nil || body.relation.code != operator.code {
		return nil, fmt.Errorf("GenericFor has no exact frozen projection")
	}
	op, ok := body.plan.GenericForOperation(step.point)
	if !ok {
		return nil, fmt.Errorf("GenericFor has no typed operation")
	}
	if step.genericIdentity.target != statekey.SymbolValue(op.Target()) {
		return nil, fmt.Errorf("GenericFor identity publication target drifted from its typed operation")
	}
	span, ok := program.formalFibers.span(variable)
	if !ok || span.keys == nil || !span.keys.Valid() {
		return nil, fmt.Errorf("GenericFor has no formal product ownership")
	}
	concrete, err := body.genericForMembership.PrepareGenericForFactorTransaction(transfer.NodeContext{
		Registry: program.registry, Graph: body.graph, Node: body.graph.Node(step.point), Point: step.point,
	}, op, body.productDomain)
	if err != nil {
		return nil, err
	}
	transaction, err := concrete.RekeyFormal(span.rekey)
	if err != nil {
		return nil, err
	}
	values, ok := span.valuesGroup()
	if !ok {
		return nil, fmt.Errorf("GenericFor has no complete Values group")
	}
	target, ok := formalMiddleSlotForStateKey(program, body, statekey.SymbolValue(op.Target()))
	if !ok {
		return nil, fmt.Errorf("GenericFor target has no formal Values slot")
	}
	if _, member := values.slot(target); !member {
		return nil, fmt.Errorf("GenericFor target is outside formal Values")
	}
	targetMember, _ := values.slot(target)
	valuesTop, ok := (formalValuesFiberGroup{descriptor: values.descriptor}).top()
	if !ok {
		return nil, fmt.Errorf("GenericFor Values group has no Top fiber")
	}
	readSlots, err := body.valueTermReadSlots(step.access[0].term)
	if err != nil {
		return nil, err
	}
	projectionSlots := make(map[statekey.Value]formalFiberGroupMember, len(readSlots))
	projectionMembers := make([]formalFiberGroupMember, 0, len(readSlots))
	for _, concreteSlot := range readSlots {
		formalSlot, present := formalMiddleSlotForStateKey(program, body, concreteSlot)
		if !present {
			return nil, fmt.Errorf("GenericFor projection slot %d has no formal identity", concreteSlot)
		}
		member, present := values.slot(formalSlot)
		if !present {
			return nil, fmt.Errorf("GenericFor projection slot %d is outside formal Values", concreteSlot)
		}
		projectionSlots[concreteSlot] = member
		projectionMembers = append(projectionMembers, member)
	}
	projectionAccess, projectionFactors, err := freezeFormalValueFactorAccess(program, variable, step.access[0].term)
	if err != nil {
		return nil, err
	}
	groups := make(map[state.ProductLane]formalFiberGroupDescriptor)
	for _, group := range span.groupDescriptors() {
		if group.kind != formalFiberGroupValues {
			groups[group.lane] = group
		}
	}
	bindGroups := func(lanes []state.ProductLane) ([]formalFiberGroupDescriptor, error) {
		out := make([]formalFiberGroupDescriptor, len(lanes))
		for index, lane := range lanes {
			group, present := groups[lane]
			if !present {
				return nil, fmt.Errorf("GenericFor lane %q is outside the frozen product", lane.ID())
			}
			out[index] = group
		}
		return out, nil
	}
	source, err := bindGroups(transaction.SourceLanes())
	if err != nil {
		return nil, err
	}
	current, err := bindGroups(transaction.CurrentLanes())
	if err != nil {
		return nil, err
	}
	writes, err := bindGroups(transaction.WriteLanes())
	if err != nil {
		return nil, err
	}
	guards, err := reachableValueTermGuards(operator.code.terms, step.access[0].term)
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
	for index := 0; index < len(demands); index++ {
		key := formalScopedGuardKey{variable: demands[index].owner, scope: demands[index].scope, arena: demands[index].arena, guard: demands[index].guard}
		for prior := 0; prior < index; prior++ {
			other := demands[prior]
			if key == (formalScopedGuardKey{variable: other.owner, scope: other.scope, arena: other.arena, guard: other.guard}) {
				demands = append(demands[:index], demands[index+1:]...)
				index--
				break
			}
		}
	}
	sealOrdinals := func(members []formalFiberGroupMember, groups []formalFiberGroupDescriptor) ([]formalFiberOrdinal, error) {
		ordinals := make([]formalFiberOrdinal, 0, len(members))
		for _, member := range members {
			ordinal, present := member.address(member.group)
			if !present {
				return nil, errFormalComponentMalformed
			}
			ordinals = append(ordinals, ordinal)
		}
		for _, group := range groups {
			ordinals = append(ordinals, group.members...)
		}
		sort.Slice(ordinals, func(i, j int) bool { return ordinals[i] < ordinals[j] })
		write := 0
		for _, ordinal := range ordinals {
			if write == 0 || ordinals[write-1] != ordinal {
				ordinals[write] = ordinal
				write++
			}
		}
		return ordinals[:write], nil
	}
	projectionSourceGroups := append(append([]formalFiberGroupDescriptor(nil), source...), projectionFactors...)
	sourceOrdinals, err := sealOrdinals(append([]formalFiberGroupMember{valuesTop}, projectionMembers...), projectionSourceGroups)
	if err != nil {
		return nil, err
	}
	currentGroups := append(append([]formalFiberGroupDescriptor(nil), current...), writes...)
	currentOrdinals, err := sealOrdinals([]formalFiberGroupMember{valuesTop, targetMember}, currentGroups)
	if err != nil {
		return nil, err
	}
	affectedOrdinals, err := sealOrdinals([]formalFiberGroupMember{targetMember}, writes)
	if err != nil {
		return nil, err
	}
	return &formalGenericForStep{
		transaction: transaction,
		projection: formalQualifiedBinding{
			value: relationArenaValueRef{owner: variable, arena: operator.code.terms, term: step.access[0].term},
			scope: operator.scope,
		},
		target: target, targetMember: targetMember, valuesTop: valuesTop,
		projectionSlots: projectionSlots, projectionMembers: projectionMembers,
		projectionAccess: projectionAccess, projectionFactors: projectionFactors,
		currentOrdinals: currentOrdinals, sourceOrdinals: sourceOrdinals, affectedOrdinals: affectedOrdinals,
		demands: demands, values: values.descriptor,
		source: source, current: current, writes: writes, sealed: true,
	}, nil
}

type formalGenericForLeafRegion struct {
	guard   decisionRef
	current formalSparseLeafView
	source  formalSparseLeafView
}

func (l formalSparseLeafView) evaluateGenericFor(plan *formalGenericForStep) (product.Value, error) {
	if plan == nil || plan.projection.value.owner != l.variable || plan.projection.value.arena != l.authority.terms {
		return product.Value{}, errFormalComponentForeignOwner
	}
	arena, term := plan.projection.value.arena, plan.projection.value.term
	var resolver valueNodeLeafResolver
	var factors []state.LaneFactor
	factorsReady := false
	resolver = valueNodeLeafResolver{
		guard: func(guard Guard) (bool, bool, bool) {
			return l.exactGuard(l.variable, arena, plan.projection.scope, guard)
		},
		root: func(root Root) (product.Value, bool) {
			slot, ok := l.algebra.program.formalSlots.Slot(l.body.body, root)
			if !ok {
				return product.Value{}, false
			}
			member, ok := (formalValuesFiberGroup{descriptor: plan.values}).slot(slot)
			if !ok {
				return product.Value{}, false
			}
			return l.value(member, plan.valuesTop)
		},
		slot: func(slot statekey.Value) (product.Value, bool) {
			member, ok := plan.projectionSlots[slot]
			if !ok {
				return product.Value{}, false
			}
			return l.value(member, plan.valuesTop)
		},
		dynamicRead: func(node valueNode, args []product.Value) (product.Value, bool) {
			if !factorsReady {
				factors = make([]state.LaneFactor, len(plan.projectionFactors))
				for index, group := range plan.projectionFactors {
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
	}
	value, exact := arena.evalValueCanonicalWithLeaves(term, resolver)
	if !exact || !product.BelongsToRegistry(l.authority.product.Registry(), value) {
		return product.Value{}, fmt.Errorf("transformer: formal GenericFor projection is unsupported")
	}
	return value, nil
}

func (a *formalTupleAlgebra) formalGenericForLeafRegions(current, source formalRelationTuple, plan *formalGenericForStep) ([]formalGenericForLeafRegion, error) {
	rows, err := a.partitionSparseLeafViewsUnderCare([]formalSparseTupleProjection{
		{tuple: current, ordinals: plan.currentOrdinals},
		{tuple: source, ordinals: plan.sourceOrdinals},
	}, plan.demands)
	if err != nil {
		return nil, err
	}
	out := make([]formalGenericForLeafRegion, len(rows))
	for index, row := range rows {
		if len(row.views) != 2 {
			return nil, errDecisionMalformed
		}
		out[index] = formalGenericForLeafRegion{guard: row.guard, current: row.views[0], source: row.views[1]}
	}
	return out, nil
}

func materializeFormalGenericForFactors(evaluator formalSparseLeafView, groups []formalFiberGroupDescriptor) ([]state.LaneFactor, error) {
	out := make([]state.LaneFactor, len(groups))
	for index, group := range groups {
		factor, err := evaluator.laneFactor(group)
		if err != nil {
			return nil, err
		}
		out[index] = factor
	}
	return out, nil
}

func (a *formalTupleAlgebra) applyFormalGenericFor(operator formalRelationOperatorRef, predecessor, nodeEntry formalRelationTuple) (formalRelationTuple, error) {
	plan := operator.genericFor
	if a == nil || !plan.valid(operator) {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal GenericFor has no complete factor transaction")
	}
	regions, err := a.formalGenericForLeafRegions(predecessor, nodeEntry, plan)
	if err != nil {
		return formalRelationTuple{}, err
	}
	span, directory, authority, ok := a.span(predecessor.variable)
	if !ok || authority.code != operator.code {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	mark := a.decisions.checkpoint()
	fail := func(err error) (formalRelationTuple, error) {
		a.decisions.rollback(mark)
		return formalRelationTuple{}, err
	}
	// GenericFor is a sparse product transaction. Omitted fibers are structural
	// carry in the immutable directory; only the target Values coordinate and
	// declared write groups may acquire new roots.
	type affectedRoot struct {
		ordinal formalFiberOrdinal
		root    decisionRef
	}
	affected := make([]affectedRoot, len(plan.affectedOrdinals))
	for index, ordinal := range plan.affectedOrdinals {
		affected[index].ordinal = ordinal
	}
	targetOrdinal, ok := plan.targetMember.address(plan.values)
	if !ok {
		return fail(errFormalComponentMalformed)
	}
	step, _ := formalRelationStepOperator(operator)
	execute := decisionTrue
	if step.guard != 0 {
		execute, err = a.decisionForGuard(predecessor.variable, operator.scope, operator.code.terms, step.guard)
		if err != nil {
			return fail(err)
		}
	}
	skip, err := formalDecisionBooleanNot(a, execute)
	if err != nil {
		return fail(err)
	}
	conditionLeaf := func(care decisionRef, ordinal formalFiberOrdinal, leaf decisionLeaf) error {
		if care == decisionFalse {
			return nil
		}
		index := sort.Search(len(affected), func(index int) bool { return affected[index].ordinal >= ordinal })
		if index >= len(affected) || affected[index].ordinal != ordinal {
			return errFormalComponentMalformed
		}
		affected[index].root, err = a.decisions.condition(a.ctx, care, a.decisions.terminal(leaf), affected[index].root)
		return err
	}
	bottom := product.Bottom(authority.product.Registry())
	for _, region := range regions {
		runCare, careErr := a.decisions.apply(a.ctx, uint8(decisionAnd), true, region.guard, execute, decisionLeafAnd)
		if careErr != nil {
			return fail(careErr)
		}
		skipCare, careErr := a.decisions.apply(a.ctx, uint8(decisionAnd), true, region.guard, skip, decisionLeafAnd)
		if careErr != nil {
			return fail(careErr)
		}
		if runCare != decisionFalse {
			actual, evalErr := region.source.evaluateGenericFor(plan)
			if evalErr != nil {
				return fail(evalErr)
			}
			sourceFactors, factorErr := materializeFormalGenericForFactors(region.source, plan.source)
			if factorErr != nil {
				return fail(factorErr)
			}
			currentFactors, factorErr := materializeFormalGenericForFactors(region.current, plan.current)
			if factorErr != nil {
				return fail(factorErr)
			}
			writes, factorErr := plan.transaction.Apply(sourceFactors, currentFactors)
			if factorErr != nil {
				return fail(factorErr)
			}
			topOrdinal, topOK := plan.valuesTop.address(plan.values)
			currentTop, topPresent := region.current.leaf(topOrdinal)
			currentTarget, targetPresent := region.current.leaf(targetOrdinal)
			if !topOK || !topPresent || !targetPresent || currentTop > 1 {
				return fail(errFormalComponentMalformed)
			}
			targetLeaf := currentTarget
			if currentTop == 0 {
				targetLeaf = 0
				if !product.Equal(authority.product.Registry(), actual, bottom) {
					targetLeaf, factorErr = authority.internGroundValue(actual)
					if factorErr != nil {
						return fail(factorErr)
					}
				}
			}
			if factorErr = conditionLeaf(runCare, targetOrdinal, targetLeaf); factorErr != nil {
				return fail(factorErr)
			}
			for index, group := range plan.writes {
				groupLeaves, factorErr := a.factorFormalSparseLane(authority, span, group, writes[index])
				if factorErr != nil || len(groupLeaves) != len(group.members) {
					if factorErr == nil {
						factorErr = errFormalComponentMalformed
					}
					return fail(factorErr)
				}
				for memberIndex, ordinal := range group.members {
					if factorErr = conditionLeaf(runCare, ordinal, groupLeaves[memberIndex]); factorErr != nil {
						return fail(factorErr)
					}
				}
			}
		}
		if skipCare != decisionFalse {
			for index := range affected {
				leaf, present := region.current.leaf(affected[index].ordinal)
				if !present {
					return fail(errFormalComponentMalformed)
				}
				if careErr = conditionLeaf(skipCare, affected[index].ordinal, leaf); careErr != nil {
					return fail(careErr)
				}
			}
		}
	}
	publication := make([]formalFiberWrite, 0, len(affected))
	for _, candidate := range affected {
		descriptor := span.forest.descriptors[span.first+int(candidate.ordinal)]
		if err := a.validateDescriptorRoot(authority, descriptor, candidate.root); err != nil {
			return fail(err)
		}
		prior, readErr := directory.valueAt(predecessor.root, candidate.ordinal)
		if readErr != nil {
			return fail(readErr)
		}
		if prior == formalFiberValue(candidate.root) {
			continue
		}
		publication = append(publication, formalFiberWrite{ordinal: candidate.ordinal, value: formalFiberValue(candidate.root)})
	}
	if len(publication) == 0 {
		return predecessor, nil
	}
	delta, err := directory.sealDelta(publication)
	if err != nil {
		return fail(err)
	}
	root, _, err := directory.applyDelta(predecessor.root, delta)
	if err != nil {
		return fail(err)
	}
	return a.normalize(formalRelationTuple{variable: predecessor.variable, root: root}), nil
}
