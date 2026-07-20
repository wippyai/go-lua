package transformer

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// formalPathReplacementStep is only a frozen edge adapter into
// state.PathReplacementTransaction. It is not an effect language: relationCode
// remains the sole ordered syntax and ProductDomain remains the sole mutation
// algebra. Paths are resolved and rekeyed while the template is frozen; solve
// time observes only an exact formal key and the symbolic value term.
type formalPathReplacementStep struct {
	hasAssignment bool
	target        keyspace.Key
	source        keyspace.Key
	hasSource     bool
	value         ValueTerm
	hasStatic     bool
	staticValue   ValueTerm
	staticPlan    state.StaticMemberFactorPlan
	staticGroup   formalFiberGroupDescriptor
	demands       []formalQualifiedGuardDemand
	values        formalFiberGroupDescriptor
	reads         []formalFiberGroupDescriptor
	writes        []formalFiberGroupDescriptor
}

func freezeFormalPathReplacementStep(program *RelationProgram, variable relationVar, operator formalRelationOperatorRef) (*formalPathReplacementStep, error) {
	if program == nil || variable == 0 || int(variable) > len(program.bodies) || operator.kind != formalRelationCellStep || operator.code == nil || operator.root == 0 || operator.step == 0 ||
		int(operator.root) >= len(operator.code.nodes) || int(operator.step) > len(operator.code.nodes[operator.root].steps) {
		return nil, nil
	}
	step := operator.code.nodes[operator.root].steps[operator.step-1]
	if step.kind != boundaryStepEffect {
		return nil, nil
	}
	if step.effect == 0 || operator.code.effects == nil || int(step.effect) >= len(operator.code.effects.nodes) {
		return nil, fmt.Errorf("effect has no frozen syntax")
	}
	node := operator.code.effects.nodes[step.effect]
	if node.kind != EffectPathStore {
		return nil, nil
	}
	// Object construction owns a distinct graph mutation. The two ordered
	// PathStore writes, however, are one lexical transaction: destructive path
	// replacement followed by persistent static-member publication.
	if !node.pathStoreHasAssignment && !node.pathStoreHasStatic || len(node.pathStoreObject.Heaps) != 0 ||
		len(node.pathStoreObject.Entries) != 0 || node.pathStoreObject.ListFloor != 0 {
		return nil, nil
	}
	if operator.code.terms == nil || operator.code.terms != operator.code.effects.terms ||
		program.bodies[variable-1].relation.code != operator.code {
		return nil, fmt.Errorf("path replacement has no lexical term owner")
	}
	body := &program.bodies[variable-1]
	span, ok := program.formalFibers.span(variable)
	if !ok || span.keys == nil || !span.keys.Valid() {
		return nil, fmt.Errorf("path replacement has no formal keyspace")
	}
	var err error
	var guards []Guard
	collectGuards := func(value ValueTerm) error {
		found, guardErr := reachableValueTermGuards(operator.code.terms, value)
		if guardErr != nil {
			return guardErr
		}
		guards = append(guards, found...)
		return nil
	}
	plan := &formalPathReplacementStep{hasAssignment: node.pathStoreHasAssignment, hasStatic: node.pathStoreHasStatic}
	if plan.hasAssignment {
		write := node.pathStoreAssignment
		plan.target, err = freezeFormalEffectPathKey(body, span, write.Target)
		if err != nil {
			return nil, fmt.Errorf("path replacement target: %w", err)
		}
		plan.value = write.Value
		if err := collectGuards(write.Value); err != nil {
			return nil, err
		}
		if write.HasSourcePath && !write.SuppressProof {
			plan.source, err = freezeFormalEffectPathKey(body, span, write.SourcePath)
			if err != nil {
				return nil, fmt.Errorf("path replacement source: %w", err)
			}
			plan.hasSource = true
		}
	}
	if plan.hasStatic {
		write := node.pathStoreStatic
		staticTarget, targetErr := freezeFormalEffectPathKey(body, span, write.Target)
		if targetErr != nil {
			return nil, fmt.Errorf("static-member target: %w", targetErr)
		}
		plan.staticValue = write.Value
		plan.staticPlan, err = body.productDomain.PrepareStaticMemberFactorPlan(span.keys, staticTarget, product.Bottom(program.registry))
		if err != nil {
			return nil, fmt.Errorf("static-member plan: %w", err)
		}
		if err := collectGuards(write.Value); err != nil {
			return nil, err
		}
	}
	if step.guard != 0 {
		guards = append(guards, step.guard)
	}
	demands := make([]formalQualifiedGuardDemand, len(guards))
	for index, guard := range guards {
		demands[index] = formalQualifiedGuardDemand{owner: variable, scope: operator.scope, arena: operator.code.terms, guard: guard}
	}
	groups := span.groupDescriptors()
	byLane := make(map[state.ProductLane]formalFiberGroupDescriptor, len(groups))
	var values formalFiberGroupDescriptor
	for _, group := range groups {
		if group.kind == formalFiberGroupValues {
			values = group
		} else {
			byLane[group.lane] = group
		}
	}
	if !values.valid() {
		return nil, fmt.Errorf("path replacement has no complete Values group")
	}
	var readLanes, writeLanes []state.ProductLane
	if plan.hasAssignment {
		readLanes = body.productDomain.PathReplacementReadLanes()
		writeLanes = body.productDomain.PathReplacementWriteLanes()
	}
	readGroups := make([]formalFiberGroupDescriptor, len(readLanes))
	writeGroups := make([]formalFiberGroupDescriptor, len(writeLanes))
	for index, lane := range readLanes {
		group, present := byLane[lane]
		if !present {
			return nil, fmt.Errorf("path replacement read lane %s is outside the frozen product", lane.ID())
		}
		readGroups[index] = group
	}
	for index, lane := range writeLanes {
		group, present := byLane[lane]
		if !present {
			return nil, fmt.Errorf("path replacement write lane %s is outside the frozen product", lane.ID())
		}
		writeGroups[index] = group
	}
	plan.demands, plan.values, plan.reads, plan.writes = demands, values, readGroups, writeGroups
	if plan.hasStatic {
		staticLane, laneErr := body.productDomain.StaticMemberFactorLane(plan.staticPlan)
		if laneErr != nil {
			return nil, laneErr
		}
		group, present := byLane[staticLane]
		if !present {
			return nil, fmt.Errorf("static-member lane %s is outside the frozen product", staticLane.ID())
		}
		plan.staticGroup = group
	}
	if plan.hasAssignment && (plan.value == 0 || plan.target.Kind == keyspace.KindInvalid || plan.hasSource != (plan.source.Kind != keyspace.KindInvalid)) ||
		plan.hasStatic && (plan.staticValue == 0 || !plan.staticPlan.Valid() || !plan.staticGroup.valid()) {
		return nil, fmt.Errorf("path replacement adapter is incomplete")
	}
	return plan, nil
}

func freezeEffectStructuralPath(body *relationProgramBody, term PathTerm) (pathdom.Path, error) {
	if body == nil || body.relation.arena == nil || body.keys == nil || !body.keys.Valid() || term == 0 {
		return pathdom.Path{}, fmt.Errorf("path term is unowned")
	}
	path, exact := body.relation.arena.evalPathCanonicalWithRoot(term, func(root Root) (pathdom.Path, bool) {
		key, ok := body.rootPathKey(root)
		if !ok {
			return pathdom.Path{}, false
		}
		return body.keys.StatePath(key)
	})
	if !exact {
		return pathdom.Path{}, fmt.Errorf("path term is not an exact frozen structural path")
	}
	return path, nil
}

func freezeFormalEffectPath(body *relationProgramBody, span formalFiberDescriptorSpan, term PathTerm) (keyspace.Key, pathdom.Path, error) {
	if body == nil || body.relation.arena == nil || body.keys == nil || !body.keys.Valid() || term == 0 || span.keys == nil || !span.keys.Valid() {
		return keyspace.Key{}, pathdom.Path{}, fmt.Errorf("path term is unowned")
	}
	path, err := freezeEffectStructuralPath(body, term)
	if err != nil {
		return keyspace.Key{}, pathdom.Path{}, err
	}
	concrete := body.keys.FromPath(path)
	if concrete.Kind == keyspace.KindInvalid || body.keys.FormatReadOnly(concrete) == "" {
		return keyspace.Key{}, pathdom.Path{}, fmt.Errorf("path term has no sealed concrete key")
	}
	formal, err := body.productDomain.RekeyStructuralKeyFormal(span.rekey, concrete)
	if err != nil {
		return keyspace.Key{}, pathdom.Path{}, err
	}
	return formal, path, nil
}

func freezeFormalEffectPathKey(body *relationProgramBody, span formalFiberDescriptorSpan, term PathTerm) (keyspace.Key, error) {
	formal, _, err := freezeFormalEffectPath(body, span, term)
	return formal, err
}

type formalPathReplacementValues struct {
	program *RelationProgram
	body    *relationProgramBody
	factor  state.ValueFactor[FormalSlot]
}

func (v formalPathReplacementValues) slot(dependency statekey.ValueDependency) (FormalSlot, bool) {
	return formalLiveValueSlotForDependency(v.program, v.body, dependency)
}

func (v formalPathReplacementValues) ReadPathReplacementValue(dependency statekey.ValueDependency) (product.Value, bool) {
	slot, ok := v.slot(dependency)
	if !ok {
		return product.Value{}, false
	}
	if v.factor.Top {
		return product.Top(), true
	}
	value, ok := v.factor.Values[slot]
	return value, ok
}

func (a *formalTupleAlgebra) applyFormalPathReplacement(operator formalRelationOperatorRef, predecessor formalRelationTuple) (formalRelationTuple, error) {
	if a == nil || operator.kind != formalRelationCellStep || operator.code == nil || operator.pathReplacement == nil {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal Effect has no complete factor transaction")
	}
	return a.applyFormalEffectStep(operator, predecessor, operator.pathReplacement.demands,
		func(span formalFiberDescriptorSpan, evaluator formalTupleLeafEvaluator) ([]decisionLeaf, error) {
			return a.applyFormalPathReplacementLeaf(operator, span, evaluator, operator.pathReplacement)
		})
}

// applyFormalEffectStep owns the guard-exact relationCode composition shared
// by every factor-native Effect adapter. Individual effects only supply their
// registered leaf transaction; none may duplicate decision partitioning,
// rollback, false-guard carry, or tuple publication.
func (a *formalTupleAlgebra) applyFormalEffectStep(
	operator formalRelationOperatorRef,
	predecessor formalRelationTuple,
	demands []formalQualifiedGuardDemand,
	applyLeaf func(formalFiberDescriptorSpan, formalTupleLeafEvaluator) ([]decisionLeaf, error),
	declaredWrites ...formalFiberOrdinal,
) (formalRelationTuple, error) {
	if a == nil || applyLeaf == nil || operator.kind != formalRelationCellStep || operator.code == nil {
		return formalRelationTuple{}, fmt.Errorf("transformer: invalid formal Effect step")
	}
	if err := a.validateTuple(predecessor); err != nil {
		return formalRelationTuple{}, err
	}
	if predecessor.bottom() {
		return predecessor, nil
	}
	span, directory, authority, ok := a.span(predecessor.variable)
	if !ok || authority.code != operator.code || predecessor.root.owner != directory {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal Effect predecessor has foreign ownership")
	}
	mark := a.decisions.checkpoint()
	fail := func(err error) (formalRelationTuple, error) {
		a.decisions.rollback(mark)
		return formalRelationTuple{}, err
	}
	regions, err := a.tupleLeafRegionsWithGuardDemands(predecessor, demands)
	if err != nil {
		return fail(err)
	}
	step := operator.code.nodes[operator.root].steps[operator.step-1]
	guardDecision := decisionTrue
	if step.guard != 0 {
		if a.program.formalGuards == nil || !a.program.formalGuards.valid() {
			return fail(fmt.Errorf("transformer: formal Effect guard has no frozen vocabulary"))
		}
		guardDecision, err = a.decisionForGuard(predecessor.variable, operator.scope, operator.code.terms, step.guard)
		if err != nil {
			return fail(err)
		}
	}
	writeOrdinals := declaredWrites
	if len(writeOrdinals) == 0 {
		writeOrdinals = make([]formalFiberOrdinal, span.count)
		for ordinal := range writeOrdinals {
			writeOrdinals[ordinal] = formalFiberOrdinal(ordinal)
		}
	}
	type affectedRoot struct {
		ordinal formalFiberOrdinal
		prior   formalFiberValue
		root    decisionRef
	}
	affected := make([]affectedRoot, len(writeOrdinals))
	for index, ordinal := range writeOrdinals {
		if index != 0 && writeOrdinals[index-1] >= ordinal {
			return fail(fmt.Errorf("transformer: formal Effect write footprint is not sealed"))
		}
		prior, readErr := directory.valueAt(predecessor.root, ordinal)
		if readErr != nil {
			return fail(readErr)
		}
		affected[index] = affectedRoot{ordinal: ordinal, prior: prior, root: decisionRef(prior)}
	}
	for _, region := range regions {
		trueCare, careErr := a.decisions.apply(a.ctx, uint8(decisionAnd), true, region.guard, guardDecision, decisionLeafAnd)
		if careErr != nil {
			return fail(careErr)
		}
		if trueCare != decisionFalse {
			leaves, effectErr := applyLeaf(span, region.evaluator)
			if effectErr != nil {
				return fail(effectErr)
			}
			if len(leaves) != span.count {
				return fail(errFormalComponentMalformed)
			}
			for index := range affected {
				ordinal := affected[index].ordinal
				leaf := leaves[int(ordinal)]
				priorLeaf, present := region.evaluator.leaves.leaf(ordinal)
				if !present {
					return fail(errFormalComponentMalformed)
				}
				if leaf == priorLeaf {
					continue
				}
				affected[index].root, effectErr = a.decisions.condition(
					a.ctx, trueCare, a.decisions.terminal(leaf), affected[index].root,
				)
				if effectErr != nil {
					return fail(effectErr)
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
		if candidate.prior != formalFiberValue(candidate.root) {
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

func (a *formalTupleAlgebra) applyFormalPathReplacementLeaf(operator formalRelationOperatorRef, span formalFiberDescriptorSpan, evaluator formalTupleLeafEvaluator, plan *formalPathReplacementStep) ([]decisionLeaf, error) {
	if !evaluator.valid() || evaluator.variable != span.variable || plan == nil || !plan.values.valid() ||
		plan.values.variable != span.variable {
		return nil, errFormalComponentForeignOwner
	}
	valueLeaves, err := evaluator.leaves.group(plan.values)
	if err != nil {
		return nil, err
	}
	values, err := a.materializeValuesGroup(evaluator.authority, plan.values, valueLeaves)
	if err != nil {
		return nil, err
	}
	domain := evaluator.authority.product
	complete, err := evaluator.completeLeaves()
	if err != nil {
		return nil, err
	}
	out := append([]decisionLeaf(nil), complete...)
	if plan.hasAssignment {
		value, exact := evaluator.evalArenaValue(span.variable, operator.code.terms, plan.value, operator.scope, formalApplyTermView{})
		if !exact {
			return nil, fmt.Errorf("transformer: formal Effect assignment value source is unsupported")
		}
		readLanes := domain.PathReplacementReadLanes()
		if len(plan.reads) != len(readLanes) || len(plan.writes) != len(domain.PathReplacementWriteLanes()) {
			return nil, errFormalComponentMalformed
		}
		readFactors := make([]state.LaneFactor, len(readLanes))
		for index := range readLanes {
			group := plan.reads[index]
			readFactors[index], err = a.materializeFormalEffectLane(evaluator.authority, span, group, complete)
			if err != nil {
				return nil, err
			}
		}
		reader := formalPathReplacementValues{program: a.program, body: &a.program.bodies[span.variable-1], factor: values}
		transaction, transactionErr := domain.PreparePathReplacement(state.PathReplacementConfig{
			Keys: span.keys, Target: plan.target, Source: plan.source, HasSource: plan.hasSource, Value: value,
		}, reader, readFactors)
		if transactionErr != nil {
			return nil, transactionErr
		}
		nextValues, valuesErr := state.ApplyPathReplacementValues(domain, transaction, values, reader.slot)
		if valuesErr != nil {
			return nil, valuesErr
		}
		if err := a.factorFormalEffectGroup(evaluator.authority, span, plan.values, nextValues, state.LaneFactor{}, out); err != nil {
			return nil, err
		}
		for _, group := range plan.writes {
			current, laneErr := a.materializeFormalEffectLane(evaluator.authority, span, group, out)
			if laneErr != nil {
				return nil, laneErr
			}
			next, laneErr := domain.ApplyPathReplacementFactor(transaction, current, current)
			if laneErr != nil {
				return nil, laneErr
			}
			if err := a.factorFormalEffectGroup(evaluator.authority, span, group, state.ValueFactor[FormalSlot]{}, next, out); err != nil {
				return nil, err
			}
		}
	}
	if plan.hasStatic {
		value, exact := evaluator.evalArenaValue(span.variable, operator.code.terms, plan.staticValue, operator.scope, formalApplyTermView{})
		if !exact {
			return nil, fmt.Errorf("transformer: formal Effect static-member value source is unsupported")
		}
		bound, bindErr := domain.BindStaticMemberFactorValue(plan.staticPlan, value)
		if bindErr != nil {
			return nil, bindErr
		}
		current, laneErr := a.materializeFormalEffectLane(evaluator.authority, span, plan.staticGroup, out)
		if laneErr != nil {
			return nil, laneErr
		}
		next, laneErr := domain.ApplyStaticMemberFactor(bound, current)
		if laneErr != nil {
			return nil, laneErr
		}
		if err := a.factorFormalEffectGroup(evaluator.authority, span, plan.staticGroup, state.ValueFactor[FormalSlot]{}, next, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func formalEffectGroupLeaves(group formalFiberGroupDescriptor, all []decisionLeaf) ([]decisionLeaf, error) {
	out := make([]decisionLeaf, len(group.members))
	for index, ordinal := range group.members {
		if int(ordinal) < 0 || int(ordinal) >= len(all) {
			return nil, fmt.Errorf("transformer: formal effect group ordinal %d is absent", ordinal)
		}
		out[index] = all[ordinal]
	}
	return out, nil
}

func (a *formalTupleAlgebra) materializeFormalEffectLane(authority *formalComponentTerminalAuthority, span formalFiberDescriptorSpan, group formalFiberGroupDescriptor, all []decisionLeaf) (state.LaneFactor, error) {
	leaves, err := formalEffectGroupLeaves(group, all)
	if err != nil {
		return state.LaneFactor{}, err
	}
	switch group.kind {
	case formalFiberGroupOrdinaryLane:
		return a.materializeOrdinaryGroup(authority, group, leaves)
	case formalFiberGroupCoordinateLane:
		return a.materializeCoordinateGroup(authority, span, group, leaves)
	default:
		return state.LaneFactor{}, errFormalComponentMalformed
	}
}

func (a *formalTupleAlgebra) factorFormalEffectGroup(authority *formalComponentTerminalAuthority, span formalFiberDescriptorSpan, group formalFiberGroupDescriptor, values state.ValueFactor[FormalSlot], lane state.LaneFactor, out []decisionLeaf) error {
	var leaves []decisionLeaf
	var err error
	switch group.kind {
	case formalFiberGroupValues:
		leaves, err = a.factorValuesGroup(authority, group, values)
	case formalFiberGroupOrdinaryLane:
		leaves, err = a.factorOrdinaryGroup(authority, group, lane)
	case formalFiberGroupCoordinateLane:
		leaves, err = a.factorCoordinateGroup(authority, span, group, lane)
	default:
		err = errFormalComponentMalformed
	}
	if err != nil || len(leaves) != len(group.members) {
		if err == nil {
			err = errFormalComponentMalformed
		}
		return err
	}
	for index, ordinal := range group.members {
		if int(ordinal) < 0 || int(ordinal) >= len(out) {
			return errFormalComponentMalformed
		}
		out[ordinal] = leaves[index]
	}
	return nil
}
