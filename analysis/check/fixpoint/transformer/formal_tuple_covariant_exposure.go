package transformer

import (
	"fmt"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/state"
	typecovariant "github.com/wippyai/go-lua/analysis/type/covariant"
)

// formalCovariantExposureStep is the frozen address vocabulary for the one
// canonical N6 factor law. It contains no State adapter and no transfer
// semantics: ApplyCovariantExposureFactors remains the sole evaluator.
type formalCovariantExposureStep struct {
	variable relationVar
	code     *relationCode
	scope    loopMuTerm

	transaction factapply.CovariantExposureTransaction
	bindings    []factapply.CovariantFactorBinding[FormalSlot]
	topology    state.CovariantFactorTopology
	values      formalFiberGroupDescriptor
	valuesTop   formalFiberGroupMember
	valueSlots  []FormalSlot
	valueFibers []formalFiberGroupMember
	lanes       []formalFiberGroupDescriptor

	// Both projections are deliberately identical. The first is the evolving
	// N0..N5 point value; the second is the lexical point-entry authority which
	// owns whole-node rollback and reachability. No other product fiber enters
	// the N6 correlation cone.
	currentOrdinals []formalFiberOrdinal
	entryOrdinals   []formalFiberOrdinal
	writeOrdinals   []formalFiberOrdinal
	demands         []formalQualifiedGuardDemand
	sealed          bool
}

// freezeFormalCovariantBindings is the sole formal address adapter for N6,
// shared by standalone CovariantExposure Steps and Outcome finalization. It
// freezes the same point-visible SSA root chosen by the concrete authority,
// then performs the one certified concrete-to-formal rekey.
func freezeFormalCovariantBindings(
	program *RelationProgram,
	variable relationVar,
	span formalFiberDescriptorSpan,
	transaction factapply.CovariantExposureTransaction,
) ([]factapply.CovariantFactorBinding[FormalSlot], error) {
	if program == nil || variable == 0 || int(variable) > len(program.bodies) ||
		!transaction.Valid(program.registry) {
		return nil, fmt.Errorf("CovariantExposure has no formal binding authority")
	}
	body := &program.bodies[variable-1]
	bindings := make([]factapply.CovariantFactorBinding[FormalSlot], transaction.Len())
	for index := range bindings {
		item, present := transaction.Step(index)
		if !present {
			return nil, fmt.Errorf("CovariantExposure step %d is absent", index)
		}
		path := item.Exposure().SourcePath()
		if path.Symbol == 0 {
			bindings[index] = factapply.CovariantFactorBinding[FormalSlot]{Kind: factapply.CovariantFactorBindingNoop}
			continue
		}
		if program.formalSlots == nil || body.pathSemantics == nil || !body.pathSemantics.Valid() ||
			span.keys == nil || !span.keys.Valid() {
			return nil, fmt.Errorf("CovariantExposure source %d has no point-visible path authority", index)
		}
		slot, present := formalMiddleSlotForStateKey(program, body, statekey.SymbolValue(path.Symbol))
		if !present {
			return nil, fmt.Errorf("CovariantExposure source %d has no formal Values slot", index)
		}
		visible, present := body.pathSemantics.VisibleLocalPathKey(transaction.Point(), pathdom.NewPath(path.Symbol, ""))
		if !present {
			return nil, fmt.Errorf("CovariantExposure source %d has no point-visible structural root", index)
		}
		root, rekeyErr := body.productDomain.RekeyStructuralKeyFormal(span.rekey, visible)
		if rekeyErr != nil {
			return nil, fmt.Errorf("CovariantExposure source %d structural root: %w", index, rekeyErr)
		}
		bindings[index] = factapply.CovariantFactorBinding[FormalSlot]{
			Kind: factapply.CovariantFactorBindingStructural, Source: slot, Root: root,
		}
	}
	return bindings, nil
}

func (p *formalCovariantExposureStep) valid(operator formalRelationOperatorRef) bool {
	return p != nil && p.sealed && p.variable != 0 && p.code == operator.code &&
		p.scope == operator.scope && p.transaction.HasStateSteps() &&
		p.values.valid() && p.values.variable == p.variable &&
		p.valuesTop.group.same(p.values) && len(p.valueSlots) == len(p.valueFibers) &&
		len(p.bindings) == p.transaction.Len() && len(p.lanes) == p.topology.Len() &&
		len(p.currentOrdinals) != 0 && len(p.entryOrdinals) != 0
}

func freezeFormalCovariantExposureStep(program *RelationProgram, variable relationVar, operator formalRelationOperatorRef) (*formalCovariantExposureStep, error) {
	if program == nil || variable == 0 || int(variable) > len(program.bodies) ||
		operator.kind != formalRelationCellStep || operator.code == nil || operator.root == 0 ||
		operator.step == 0 || int(operator.root) >= len(operator.code.nodes) ||
		int(operator.step) > len(operator.code.nodes[operator.root].steps) {
		return nil, nil
	}
	step := operator.code.nodes[operator.root].steps[operator.step-1]
	if step.kind != boundaryStepCovariantExposure {
		return nil, nil
	}
	body := &program.bodies[variable-1]
	span, ok := program.formalFibers.span(variable)
	if !ok || span.keys == nil || !span.keys.Valid() || body.keys == nil || !body.keys.Valid() ||
		body.relation.code != operator.code || !step.covariant.HasStateSteps() ||
		!step.covariant.Valid(program.registry) {
		return nil, fmt.Errorf("CovariantExposure has no formal N6 ownership")
	}
	values, ok := span.valuesGroup()
	if !ok {
		return nil, fmt.Errorf("CovariantExposure has no complete Values group")
	}
	valuesTop, ok := values.top()
	if !ok {
		return nil, fmt.Errorf("CovariantExposure Values group has no Top fiber")
	}
	topology, err := body.productDomain.SealCovariantFactorTopology()
	if err != nil {
		return nil, fmt.Errorf("CovariantExposure factor topology: %w", err)
	}
	groups := span.groupDescriptors()
	lanes := make([]formalFiberGroupDescriptor, topology.Len())
	for index := range lanes {
		lane, present := topology.Lane(index)
		if !present {
			return nil, fmt.Errorf("CovariantExposure factor topology is incomplete")
		}
		found := false
		for _, group := range groups {
			if group.kind != formalFiberGroupValues && group.lane == lane {
				lanes[index], found = group, true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("CovariantExposure lane %q is outside the formal product", lane.ID())
		}
	}

	bindings, err := freezeFormalCovariantBindings(program, variable, span, step.covariant)
	if err != nil {
		return nil, err
	}
	valueMembers := make(map[FormalSlot]formalFiberGroupMember)
	for index := range bindings {
		binding := bindings[index]
		if binding.Kind == factapply.CovariantFactorBindingNoop {
			continue
		}
		member, present := values.slot(binding.Source)
		if !present {
			return nil, fmt.Errorf("CovariantExposure source %d is outside formal Values", index)
		}
		valueMembers[binding.Source] = member
	}
	valueSlots := make([]FormalSlot, 0, len(valueMembers))
	for slot := range valueMembers {
		valueSlots = append(valueSlots, slot)
	}
	sort.Slice(valueSlots, func(i, j int) bool { return valueSlots[i].root.Less(valueSlots[j].root) })
	valueFibers := make([]formalFiberGroupMember, len(valueSlots))
	for index, slot := range valueSlots {
		valueFibers[index] = valueMembers[slot]
	}
	sealOrdinals := func(includeTop bool, members []formalFiberGroupMember, groups []formalFiberGroupDescriptor) ([]formalFiberOrdinal, error) {
		capacity := len(members)
		if includeTop {
			capacity++
		}
		for _, group := range groups {
			capacity += len(group.members)
		}
		ordinals := make([]formalFiberOrdinal, 0, capacity)
		if includeTop {
			ordinal, present := valuesTop.address(valuesTop.group)
			if !present {
				return nil, errFormalComponentMalformed
			}
			ordinals = append(ordinals, ordinal)
		}
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
	currentOrdinals, err := sealOrdinals(true, valueFibers, lanes)
	if err != nil {
		return nil, err
	}
	writeOrdinals, err := sealOrdinals(false, valueFibers, lanes)
	if err != nil {
		return nil, err
	}
	demands := make([]formalQualifiedGuardDemand, 0, 1)
	if step.guard != 0 {
		demands = append(demands, formalQualifiedGuardDemand{
			owner: variable, scope: operator.scope, arena: operator.code.terms, guard: step.guard,
		})
	}
	return &formalCovariantExposureStep{
		variable: variable, code: operator.code, scope: operator.scope,
		transaction: step.covariant.Clone(), bindings: bindings, topology: topology,
		values: values.descriptor, valuesTop: valuesTop, valueSlots: valueSlots, valueFibers: valueFibers, lanes: lanes,
		currentOrdinals: currentOrdinals, entryOrdinals: append([]formalFiberOrdinal(nil), currentOrdinals...),
		writeOrdinals: writeOrdinals, demands: demands, sealed: true,
	}, nil
}

func (l formalSparseLeafView) materializeCovariantValues(plan *formalCovariantExposureStep) (state.ValueFactor[FormalSlot], error) {
	topOrdinal, ok := plan.valuesTop.address(plan.values)
	if !ok {
		return state.ValueFactor[FormalSlot]{}, errFormalComponentMalformed
	}
	topLeaf, present := l.leaf(topOrdinal)
	if !present || topLeaf > 1 {
		return state.ValueFactor[FormalSlot]{}, errFormalComponentMalformed
	}
	if topLeaf == 1 {
		return state.ValueFactor[FormalSlot]{Top: true}, nil
	}
	bottom := product.Bottom(l.authority.product.Registry())
	var values map[FormalSlot]product.Value
	for index, slot := range plan.valueSlots {
		value, exact := l.value(plan.valueFibers[index], plan.valuesTop)
		if !exact {
			return state.ValueFactor[FormalSlot]{}, errFormalComponentMalformed
		}
		if product.Equal(l.authority.product.Registry(), value, bottom) {
			continue
		}
		if values == nil {
			values = make(map[FormalSlot]product.Value, len(plan.valueSlots))
		}
		values[slot] = value
	}
	return state.ValueFactor[FormalSlot]{Values: values}, nil
}

func (a *formalTupleAlgebra) applyFormalCovariantExposure(
	operator formalRelationOperatorRef,
	plan *formalCovariantExposureStep,
	predecessor, pointEntry formalRelationTuple,
) (formalRelationTuple, error) {
	if a == nil || !plan.valid(operator) {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal CovariantExposure has no complete N6 transaction")
	}
	if err := a.validateTuple(predecessor); err != nil {
		return formalRelationTuple{}, err
	}
	if err := a.validateTuple(pointEntry); err != nil {
		return formalRelationTuple{}, err
	}
	if predecessor.bottom() {
		return predecessor, nil
	}
	if pointEntry.bottom() || predecessor.variable != pointEntry.variable {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	span, directory, authority, ok := a.span(predecessor.variable)
	if !ok || predecessor.root.owner != directory || pointEntry.root.owner != directory || authority.code != operator.code {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	regions, err := a.partitionSparseLeafViewsUnderCare([]formalSparseTupleProjection{
		{tuple: predecessor, ordinals: plan.currentOrdinals},
		{tuple: pointEntry, ordinals: plan.entryOrdinals},
	}, plan.demands)
	if err != nil {
		return formalRelationTuple{}, err
	}
	mark := a.decisions.checkpoint()
	fail := func(err error) (formalRelationTuple, error) {
		a.decisions.rollback(mark)
		return formalRelationTuple{}, err
	}
	type affectedRoot struct {
		ordinal formalFiberOrdinal
		root    decisionRef
	}
	affected := make([]affectedRoot, len(plan.writeOrdinals))
	for index, ordinal := range plan.writeOrdinals {
		affected[index].ordinal = ordinal
	}
	step, ok := formalRelationStepOperator(operator)
	if !ok {
		return fail(errFormalComponentMalformed)
	}
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
	for _, region := range regions {
		if len(region.views) != 2 {
			return fail(errDecisionMalformed)
		}
		current := region.views[0]
		runCare, careErr := a.decisions.apply(a.ctx, uint8(decisionAnd), true, region.guard, execute, decisionLeafAnd)
		if careErr != nil {
			return fail(careErr)
		}
		skipCare, careErr := a.decisions.apply(a.ctx, uint8(decisionAnd), true, region.guard, skip, decisionLeafAnd)
		if careErr != nil {
			return fail(careErr)
		}
		if runCare != decisionFalse {
			values, factorErr := current.materializeCovariantValues(plan)
			if factorErr != nil {
				return fail(factorErr)
			}
			factors := make([]state.LaneFactor, len(plan.lanes))
			for index, group := range plan.lanes {
				factors[index], factorErr = current.laneFactor(group)
				if factorErr != nil {
					return fail(factorErr)
				}
			}
			factored, factorErr := factapply.ApplyCovariantExposureFactors(a.ctx, typecovariant.WidenRecord,
				factapply.CovariantFactorTransaction[FormalSlot]{
					Transaction: plan.transaction, Bindings: plan.bindings, Values: values, Factors: factors,
					Domain: authority.product, Keys: span.keys, Topology: plan.topology,
					Token: cancellation.FromContext(a.ctx).Token(),
				})
			if factorErr != nil {
				return fail(factorErr)
			}
			if factored.Values.Top != values.Top || len(factored.Factors) != len(plan.lanes) {
				return fail(errFormalComponentMalformed)
			}
			bottom := product.Bottom(authority.product.Registry())
			for index, member := range plan.valueFibers {
				ordinal, present := member.address(plan.values)
				if !present {
					return fail(errFormalComponentMalformed)
				}
				leaf, present := current.leaf(ordinal)
				if !present {
					return fail(errFormalComponentMalformed)
				}
				if !factored.Values.Top {
					value := bottom
					if candidate, found := factored.Values.Values[plan.valueSlots[index]]; found {
						value = candidate
					}
					leaf = 0
					if !product.Equal(authority.product.Registry(), value, bottom) {
						leaf, factorErr = authority.internGroundValue(value)
						if factorErr != nil {
							return fail(factorErr)
						}
					}
				}
				if factorErr = conditionLeaf(runCare, ordinal, leaf); factorErr != nil {
					return fail(factorErr)
				}
			}
			for index, group := range plan.lanes {
				leaves, factorErr := a.factorFormalSparseLane(authority, span, group, factored.Factors[index])
				if factorErr != nil || len(leaves) != len(group.members) {
					if factorErr == nil {
						factorErr = errFormalComponentMalformed
					}
					return fail(factorErr)
				}
				for memberIndex, ordinal := range group.members {
					if factorErr = conditionLeaf(runCare, ordinal, leaves[memberIndex]); factorErr != nil {
						return fail(factorErr)
					}
				}
			}
		}
		if skipCare != decisionFalse {
			for _, ordinal := range plan.writeOrdinals {
				leaf, present := current.leaf(ordinal)
				if !present {
					return fail(errFormalComponentMalformed)
				}
				if careErr = conditionLeaf(skipCare, ordinal, leaf); careErr != nil {
					return fail(careErr)
				}
			}
		}
	}
	if len(affected) == 0 {
		return predecessor, nil
	}
	writes := make([]formalFiberWrite, 0, len(affected))
	for _, candidate := range affected {
		descriptor := span.forest.descriptors[span.first+int(candidate.ordinal)]
		if err := a.validateDescriptorRoot(authority, descriptor, candidate.root); err != nil {
			return fail(err)
		}
		prior, readErr := directory.valueAt(predecessor.root, candidate.ordinal)
		if readErr != nil {
			return fail(readErr)
		}
		if prior != formalFiberValue(candidate.root) {
			writes = append(writes, formalFiberWrite{ordinal: candidate.ordinal, value: formalFiberValue(candidate.root)})
		}
	}
	if len(writes) == 0 {
		return predecessor, nil
	}
	delta, err := directory.sealDelta(writes)
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
