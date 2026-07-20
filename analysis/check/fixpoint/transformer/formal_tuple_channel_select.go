package transformer

import (
	"fmt"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// formalChannelSelectStep is a frozen adapter into the canonical ordered N3
// syntax and evaluator. It owns no State and defines no second fact language.
type formalChannelSelectStep struct {
	transaction   factapply.PreparedChannelSelectTransaction[FormalSlot]
	demands       []formalQualifiedGuardDemand
	values        formalFiberGroupDescriptor
	channel       formalFiberGroupDescriptor
	pathValues    formalFiberGroupDescriptor
	hasPathValues bool
	readOrdinals  []formalFiberOrdinal
	writeOrdinals []formalFiberOrdinal
}

func freezeFormalChannelSelectStep(program *RelationProgram, variable relationVar, operator formalRelationOperatorRef) (*formalChannelSelectStep, error) {
	if program == nil || variable == 0 || int(variable) > len(program.bodies) || operator.kind != formalRelationCellStep ||
		operator.code == nil || operator.root == 0 || operator.step == 0 || int(operator.root) >= len(operator.code.nodes) ||
		int(operator.step) > len(operator.code.nodes[operator.root].steps) {
		return nil, nil
	}
	step := operator.code.nodes[operator.root].steps[operator.step-1]
	if step.kind != boundaryStepChannelSelect {
		return nil, nil
	}
	body := &program.bodies[variable-1]
	span, ok := program.formalFibers.span(variable)
	if !ok || span.keys == nil || !span.keys.Valid() || body.keys == nil || !body.keys.Valid() || body.relation.code != operator.code {
		return nil, fmt.Errorf("ChannelSelect has no formal product ownership")
	}
	values, ok := span.valuesGroup()
	if !ok {
		return nil, fmt.Errorf("ChannelSelect has no complete Values group")
	}
	channelLane, ok := body.productDomain.ProductLane(state.LaneChannelSelect)
	if !ok {
		return nil, fmt.Errorf("ChannelSelect lane is disabled")
	}
	groups := span.groupDescriptors()
	groupForLane := func(lane state.ProductLane) (formalFiberGroupDescriptor, bool) {
		for _, group := range groups {
			if group.kind != formalFiberGroupValues && group.lane == lane {
				return group, true
			}
		}
		return formalFiberGroupDescriptor{}, false
	}
	channel, ok := groupForLane(channelLane)
	if !ok {
		return nil, fmt.Errorf("ChannelSelect lane is outside the frozen product")
	}
	pathGroup := formalFiberGroupDescriptor{}
	hasPathGroup := false
	if family, present := body.productDomain.PathValueFamily(); present {
		pathGroup, hasPathGroup = groupForLane(family.Lane())
		if !hasPathGroup {
			return nil, fmt.Errorf("path-value family is outside the frozen product")
		}
	}
	prepared, err := factapply.PrepareChannelSelectTransaction(program.registry, step.channel,
		func(path pathdom.Path) (pathaddr.StateKey, bool) {
			concrete := body.keys.FromPath(path)
			formalKey, rekeyErr := body.productDomain.RekeyStructuralKeyFormal(span.rekey, concrete)
			if rekeyErr != nil {
				return "", false
			}
			return pathaddr.StateKeyFromPathKey(span.keys.FormatReadOnly(formalKey))
		},
		func(point cfg.Point, index int) (FormalSlot, bool) {
			if index < 0 {
				return FormalSlot{}, false
			}
			slot, slotOK := formalMiddleSlotForStateKey(program, body, statekey.CallResult(uint32(point), uint32(index)))
			if !slotOK {
				return FormalSlot{}, false
			}
			_, member := values.slot(slot)
			return slot, member
		},
	)
	if err != nil || !prepared.Complete() {
		if err == nil {
			err = fmt.Errorf("ChannelSelect has an unbound formal path")
		}
		return nil, err
	}
	var demands []formalQualifiedGuardDemand
	if step.guard != 0 {
		demands = []formalQualifiedGuardDemand{{owner: variable, scope: operator.scope, arena: operator.code.terms, guard: step.guard}}
	}
	sealGroups := func(groups ...formalFiberGroupDescriptor) []formalFiberOrdinal {
		width := 0
		for _, group := range groups {
			if group.valid() {
				width += len(group.members)
			}
		}
		ordinals := make([]formalFiberOrdinal, 0, width)
		for _, group := range groups {
			if group.valid() {
				ordinals = append(ordinals, group.members...)
			}
		}
		sort.Slice(ordinals, func(i, j int) bool { return ordinals[i] < ordinals[j] })
		write := 0
		for _, ordinal := range ordinals {
			if write == 0 || ordinals[write-1] != ordinal {
				ordinals[write] = ordinal
				write++
			}
		}
		return ordinals[:write]
	}
	readGroups := []formalFiberGroupDescriptor{values.descriptor, channel}
	if hasPathGroup {
		readGroups = append(readGroups, pathGroup)
	}
	return &formalChannelSelectStep{
		transaction: prepared, demands: demands, values: values.descriptor, channel: channel,
		pathValues: pathGroup, hasPathValues: hasPathGroup,
		readOrdinals: sealGroups(readGroups...), writeOrdinals: sealGroups(values.descriptor, channel),
	}, nil
}

func (a *formalTupleAlgebra) applyFormalChannelSelect(operator formalRelationOperatorRef, predecessor formalRelationTuple) (formalRelationTuple, error) {
	if a == nil || operator.channelSelect == nil || operator.kind != formalRelationCellStep || operator.code == nil {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal ChannelSelect has no complete factor transaction")
	}
	if err := a.validateTuple(predecessor); err != nil || predecessor.bottom() {
		return predecessor, err
	}
	span, directory, authority, ok := a.span(predecessor.variable)
	if !ok || predecessor.root.owner != directory || authority.code != operator.code {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	plan := operator.channelSelect
	if len(plan.readOrdinals) == 0 || len(plan.writeOrdinals) == 0 {
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
	step := operator.code.nodes[operator.root].steps[operator.step-1]
	guard := decisionTrue
	if step.guard != 0 {
		guard, err = a.decisionForGuard(predecessor.variable, operator.scope, operator.code.terms, step.guard)
		if err != nil {
			return fail(err)
		}
	}
	notGuard, err := formalDecisionBooleanNot(a, guard)
	if err != nil {
		return fail(err)
	}
	type affectedRoot struct {
		ordinal formalFiberOrdinal
		root    decisionRef
	}
	affected := make([]affectedRoot, len(plan.writeOrdinals))
	for index, ordinal := range plan.writeOrdinals {
		affected[index].ordinal = ordinal
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
		if len(region.views) != 1 {
			return fail(errDecisionMalformed)
		}
		view := region.views[0]
		trueCare, careErr := a.decisions.apply(a.ctx, uint8(decisionAnd), true, region.guard, guard, decisionLeafAnd)
		if careErr != nil {
			return fail(careErr)
		}
		falseCare, careErr := a.decisions.apply(a.ctx, uint8(decisionAnd), true, region.guard, notGuard, decisionLeafAnd)
		if careErr != nil {
			return fail(careErr)
		}
		if trueCare != decisionFalse {
			valuesLeaves, channelLeaves, leafErr := a.applyFormalChannelSelectLeaf(operator, span, view, plan)
			if leafErr != nil {
				return fail(leafErr)
			}
			for index, ordinal := range plan.values.members {
				if leafErr = conditionLeaf(trueCare, ordinal, valuesLeaves[index]); leafErr != nil {
					return fail(leafErr)
				}
			}
			for index, ordinal := range plan.channel.members {
				if leafErr = conditionLeaf(trueCare, ordinal, channelLeaves[index]); leafErr != nil {
					return fail(leafErr)
				}
			}
		}
		if falseCare != decisionFalse {
			for _, ordinal := range plan.writeOrdinals {
				leaf, present := view.leaf(ordinal)
				if !present {
					return fail(errFormalComponentMalformed)
				}
				if careErr = conditionLeaf(falseCare, ordinal, leaf); careErr != nil {
					return fail(careErr)
				}
			}
		}
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
	return a.normalize(formalRelationTuple{variable: predecessor.variable, root: root}), nil
}

func (a *formalTupleAlgebra) applyFormalChannelSelectLeaf(operator formalRelationOperatorRef, span formalFiberDescriptorSpan, evaluator formalSparseLeafView, plan *formalChannelSelectStep) ([]decisionLeaf, []decisionLeaf, error) {
	valuesLeaves := make([]decisionLeaf, len(plan.values.members))
	for index, ordinal := range plan.values.members {
		leaf, present := evaluator.leaf(ordinal)
		if !present {
			return nil, nil, errFormalComponentMalformed
		}
		valuesLeaves[index] = leaf
	}
	values, err := a.materializeValuesGroup(evaluator.authority, plan.values, valuesLeaves)
	if err != nil {
		return nil, nil, err
	}
	channel, err := evaluator.laneFactor(plan.channel)
	if err != nil {
		return nil, nil, err
	}
	var pathValues state.LaneFactor
	if plan.hasPathValues {
		pathValues, err = evaluator.laneFactor(plan.pathValues)
		if err != nil {
			return nil, nil, err
		}
	}
	read := func(path factapply.PreparedChannelSelectPath) (product.Value, bool) {
		key, ok := span.keys.FromStateKey(path.StateKey().PathKey())
		if !ok {
			return product.Value{}, false
		}
		if plan.hasPathValues {
			value, present, readErr := evaluator.authority.product.ReadPathValueFactor(pathValues, span.keys, key)
			if readErr == nil && present && !product.Equal(evaluator.authority.product.Registry(), value, product.Bottom(evaluator.authority.product.Registry())) {
				return value, true
			}
		}
		root, formalRoot := span.keys.DescribeFormalRoot(key)
		if !formalRoot {
			return product.Value{}, false
		}
		slot, ok := a.program.formalSlots.SlotAt(root.Owner(), root.Ordinal(), root.Vocabulary())
		if !ok {
			return product.Value{}, false
		}
		value := product.Bottom(evaluator.authority.product.Registry())
		if values.Top {
			value = product.Top()
		} else if current, present := values.Values[slot]; present {
			value = current
		}
		segments := span.keys.Segments(key)
		if len(segments) == 0 {
			return value, true
		}
		return sourcevalue.ProjectDynamicTableTypePath(evaluator.authority.product.Registry(), operator.code.terms.typeValues, value, segments)
	}
	evaluated, err := factapply.EvaluatePreparedChannelSelect(a.ctx, evaluator.authority.product.Registry(), operator.code.terms.typeValues, plan.transaction, read)
	if err != nil {
		return nil, nil, err
	}
	if !values.Top {
		next := make(map[FormalSlot]product.Value, len(values.Values)+len(evaluated.ResultWrites()))
		for slot, value := range values.Values {
			next[slot] = value
		}
		bottom := product.Bottom(evaluator.authority.product.Registry())
		for _, write := range evaluated.ResultWrites() {
			if product.Equal(evaluator.authority.product.Registry(), write.Value, bottom) {
				delete(next, write.Target)
			} else {
				next[write.Target] = write.Value
			}
		}
		values.Values = next
	}
	channel, err = evaluator.authority.product.ApplyChannelSelectFactsFactor(channel, evaluated.Facts())
	if err != nil {
		return nil, nil, err
	}
	valuesLeaves, err = a.factorValuesGroup(evaluator.authority, plan.values, values)
	if err != nil {
		return nil, nil, err
	}
	var channelLeaves []decisionLeaf
	switch plan.channel.kind {
	case formalFiberGroupOrdinaryLane:
		channelLeaves, err = a.factorOrdinaryGroup(evaluator.authority, plan.channel, channel)
	case formalFiberGroupCoordinateLane:
		channelLeaves, err = a.factorCoordinateGroup(evaluator.authority, span, plan.channel, channel)
	default:
		err = errFormalComponentMalformed
	}
	if err != nil || len(valuesLeaves) != len(plan.values.members) || len(channelLeaves) != len(plan.channel.members) {
		if err == nil {
			err = errFormalComponentMalformed
		}
		return nil, nil, err
	}
	return valuesLeaves, channelLeaves, nil
}
