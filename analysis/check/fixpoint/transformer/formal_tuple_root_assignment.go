package transformer

import (
	"fmt"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// formalRootAssignmentStep binds the one canonical factor-native N4 program
// to exact physical fibers. It contains no State reconstruction and no N4
// semantics: correlation/publication live here, while every transfer law is
// owned by RootAssignmentFactorProgram.
type formalRootAssignmentStep struct {
	variable relationVar
	code     *relationCode
	scope    loopMuTerm
	plan     factapply.ResolvedRootAssignmentPlan
	factor   factapply.RootAssignmentFactorProgram
	domain   state.ProductDomain
	term     rootAssignmentTerm
	guard    Guard

	sources       []formalQualifiedBinding
	sourceSlots   map[statekey.Value]formalFiberGroupMember
	sourceMembers []formalFiberGroupMember
	demands       []formalQualifiedGuardDemand

	values       formalFiberGroupDescriptor
	valuesTop    formalFiberGroupMember
	target       FormalSlot
	targetMember formalFiberGroupMember
	fresh        []formalRootAssignmentFreshQuery

	current []formalRootAssignmentLaneBinding
	point   []formalRootAssignmentLaneBinding
	writes  []formalRootAssignmentLaneBinding
	lift    formalClosedFactorLift

	currentOrdinals    []formalFiberOrdinal
	pointOrdinals      []formalFiberOrdinal
	affectedOrdinals   []formalFiberOrdinal
	pathReadAuthority  state.CoordinatePathEvidenceAuthority[statekey.Value]
	pathWriteAuthority state.CoordinatePathEvidenceAuthority[statekey.Value]
	sealed             bool
}

type formalRootAssignmentLaneBinding struct {
	lane  state.ProductLane
	group formalFiberGroupDescriptor
}

type formalRootAssignmentFreshQuery struct {
	path   pathdom.Path
	member formalFiberGroupMember
}

func (p *formalRootAssignmentStep) valid(operator formalRelationOperatorRef) bool {
	return p != nil && p.sealed && p.variable != 0 && p.code == operator.code &&
		p.scope == operator.scope && p.plan.Valid() && p.factor.Valid() && len(p.sources) != 0 &&
		p.values.valid() && len(p.currentOrdinals) != 0 && len(p.affectedOrdinals) != 0 && p.lift.sealed
}

func freezeFormalRootAssignmentStep(program *RelationProgram, variable relationVar, operator formalRelationOperatorRef) (*formalRootAssignmentStep, error) {
	if program == nil || variable == 0 || int(variable) > len(program.bodies) || operator.kind != formalRelationCellStep ||
		operator.code == nil || operator.root == 0 || operator.step == 0 || int(operator.root) >= len(operator.code.nodes) ||
		int(operator.step) > len(operator.code.nodes[operator.root].steps) {
		return nil, nil
	}
	step := operator.code.nodes[operator.root].steps[operator.step-1]
	if step.kind != boundaryStepRootAssignment {
		return nil, nil
	}
	body := &program.bodies[variable-1]
	if body.variable != variable || body.relation.code != operator.code || body.rootAssignments == nil ||
		!body.rootAssignments.Valid() || !body.productDomain.Valid() || operator.code.terms == nil || !operator.code.terms.Sealed() {
		return nil, fmt.Errorf("RootAssignment has no frozen product authority")
	}
	plan, err := body.rootAssignments.PrepareResolvedRootAssignmentPlan(step.rootAssignment.transaction)
	if err != nil {
		return nil, err
	}
	plan, err = bindLinkedCallResultSourcePath(body, step.rootAssignment, plan)
	if err != nil || !plan.Valid() {
		if err == nil {
			err = fmt.Errorf("RootAssignment has no complete resolved plan")
		}
		return nil, err
	}
	span, ok := program.formalFibers.span(variable)
	if !ok {
		return nil, errFormalComponentForeignOwner
	}
	if keys, owned := plan.PathKeySpace(); !owned || keys != body.keys {
		return nil, fmt.Errorf("RootAssignment path authority is outside the body keyspace")
	}
	plan, err = plan.RekeyFormal(body.productDomain, span.rekey)
	if err != nil {
		return nil, fmt.Errorf("RootAssignment formal rekey: %w", err)
	}
	factor, err := plan.FactorProgram()
	if err != nil {
		return nil, err
	}
	values, ok := span.valuesGroup()
	if !ok {
		return nil, fmt.Errorf("RootAssignment has no Values carrier")
	}
	valuesTop, ok := values.top()
	if !ok {
		return nil, fmt.Errorf("RootAssignment Values carrier has no Top fiber")
	}
	targetKey, ok := plan.TargetValueSlot()
	if !ok {
		return nil, fmt.Errorf("RootAssignment target has no Values slot")
	}
	target, ok := formalMiddleSlotForStateKey(program, body, targetKey)
	if !ok {
		return nil, fmt.Errorf("RootAssignment target is outside formal Middle")
	}
	targetMember, ok := values.slot(target)
	if !ok {
		return nil, fmt.Errorf("RootAssignment target is outside Values carrier")
	}

	sources := make([]formalQualifiedBinding, len(step.rootAssignment.sources))
	sourceSlots := make(map[statekey.Value]formalFiberGroupMember)
	sourceMembers := make([]formalFiberGroupMember, 0)
	var demands []formalQualifiedGuardDemand
	seenGuards := make(map[Guard]struct{})
	bindValueSlot := func(concrete statekey.Value) (formalFiberGroupMember, error) {
		if member, present := sourceSlots[concrete]; present {
			return member, nil
		}
		formalSlot, present := formalMiddleSlotForStateKey(program, body, concrete)
		if !present {
			return formalFiberGroupMember{}, fmt.Errorf("RootAssignment source slot %d has no formal identity", concrete)
		}
		member, present := values.slot(formalSlot)
		if !present {
			return formalFiberGroupMember{}, fmt.Errorf("RootAssignment source slot %d is outside Values", concrete)
		}
		sourceSlots[concrete] = member
		sourceMembers = append(sourceMembers, member)
		return member, nil
	}
	for index, source := range step.rootAssignment.sources {
		sources[index] = formalQualifiedBinding{
			value: relationArenaValueRef{owner: variable, arena: operator.code.terms, term: source}, scope: operator.scope,
		}
		readSlots, readErr := body.valueTermReadSlots(source)
		if readErr != nil {
			return nil, readErr
		}
		for _, slot := range readSlots {
			if _, bindErr := bindValueSlot(slot); bindErr != nil {
				return nil, bindErr
			}
		}
		guards, guardErr := reachableValueTermGuards(operator.code.terms, source)
		if guardErr != nil {
			return nil, guardErr
		}
		for _, guard := range guards {
			if _, duplicate := seenGuards[guard]; duplicate {
				continue
			}
			seenGuards[guard] = struct{}{}
			demands = append(demands, formalQualifiedGuardDemand{owner: variable, scope: operator.scope, arena: operator.code.terms, guard: guard})
		}
	}
	if step.guard != 0 {
		if _, duplicate := seenGuards[step.guard]; !duplicate {
			demands = append(demands, formalQualifiedGuardDemand{owner: variable, scope: operator.scope, arena: operator.code.terms, guard: step.guard})
		}
	}

	queries, err := plan.FactorCompletionFreshEmptyPaths()
	if err != nil {
		return nil, err
	}
	fresh := make([]formalRootAssignmentFreshQuery, len(queries))
	for index, query := range queries {
		if query.Symbol == 0 || len(query.Segments) != 0 {
			return nil, fmt.Errorf("RootAssignment fresh-empty query is not a root")
		}
		member, bindErr := bindValueSlot(statekey.SymbolValue(query.Symbol))
		if bindErr != nil {
			return nil, bindErr
		}
		fresh[index] = formalRootAssignmentFreshQuery{path: query.Clone(), member: member}
	}
	if dynamicPlan, present := plan.DynamicSourcePlan(); present {
		if query, hasQuery, queryErr := dynamicPlan.TableNonEmptyQuery(); queryErr != nil {
			return nil, queryErr
		} else if hasQuery {
			if rootSlot, hasRoot := query.RootValueSlot(); hasRoot {
				if _, bindErr := bindValueSlot(rootSlot); bindErr != nil {
					return nil, bindErr
				}
			}
		}
	}

	groups := make(map[state.LaneOrdinal]formalFiberGroupDescriptor)
	for _, group := range span.groupDescriptors() {
		if group.kind != formalFiberGroupValues {
			groups[group.lane.Ordinal()] = group
		}
	}
	valuesLane, hasValuesLane := body.productDomain.SlotFactoredCarrier()
	bindLanes := func(lanes []state.ProductLane) ([]formalRootAssignmentLaneBinding, error) {
		out := make([]formalRootAssignmentLaneBinding, 0, len(lanes))
		for _, lane := range lanes {
			if hasValuesLane && lane.Ordinal() == valuesLane.Ordinal() {
				continue
			}
			group, present := groups[lane.Ordinal()]
			if !present {
				return nil, fmt.Errorf("RootAssignment lane %q is outside the frozen product", lane.ID())
			}
			out = append(out, formalRootAssignmentLaneBinding{lane: lane, group: group})
		}
		return out, nil
	}
	current, err := bindLanes(factor.CurrentLanes())
	if err != nil {
		return nil, err
	}
	point, err := bindLanes(factor.PointEntryLanes())
	if err != nil {
		return nil, err
	}
	writes, err := bindLanes(factor.CurrentWriteLanes())
	if err != nil {
		return nil, err
	}
	sealOrdinals := func(members []formalFiberGroupMember, bindings []formalRootAssignmentLaneBinding) ([]formalFiberOrdinal, error) {
		ordinals := make([]formalFiberOrdinal, 0, len(members))
		for _, member := range members {
			ordinal, present := member.address(member.group)
			if !present {
				return nil, errFormalComponentMalformed
			}
			ordinals = append(ordinals, ordinal)
		}
		for _, binding := range bindings {
			ordinals = append(ordinals, binding.group.members...)
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
	currentMembers := append([]formalFiberGroupMember{valuesTop, targetMember}, sourceMembers...)
	currentOrdinals, err := sealOrdinals(currentMembers, current)
	if err != nil {
		return nil, err
	}
	pointOrdinals, err := sealOrdinals(nil, point)
	if err != nil {
		return nil, err
	}
	affectedOrdinals, err := sealOrdinals([]formalFiberGroupMember{targetMember}, writes)
	if err != nil {
		return nil, err
	}
	lift, err := sealFormalClosedFactorLift(span, [][]formalFiberOrdinal{currentOrdinals, pointOrdinals}, affectedOrdinals)
	if err != nil {
		return nil, fmt.Errorf("RootAssignment closed factor lift: %w", err)
	}
	emptyCoordinates, err := body.productDomain.SealCoordinateFactorInventory(span.keys, nil)
	if err != nil {
		return nil, err
	}
	pathReadAuthority, err := state.SealCoordinatePathEvidenceAuthority(
		body.productDomain, span.keys, nil, nil, span.coordinates, emptyCoordinates,
		false, false, func(statekey.Value) bool { return false },
	)
	if err != nil {
		return nil, err
	}
	pathWriteAuthority, err := state.SealCoordinatePathEvidenceAuthority(
		body.productDomain, span.keys, nil, nil, span.coordinates, span.coordinates,
		false, true, func(statekey.Value) bool { return false },
	)
	if err != nil {
		return nil, err
	}
	return &formalRootAssignmentStep{
		variable: variable, code: operator.code, scope: operator.scope, plan: plan, factor: factor, domain: body.productDomain,
		term: step.rootAssignment, guard: step.guard, sources: sources, sourceSlots: sourceSlots,
		sourceMembers: sourceMembers, demands: demands, values: values.descriptor, valuesTop: valuesTop,
		target: target, targetMember: targetMember, fresh: fresh,
		current: current, point: point, writes: writes,
		lift: lift, currentOrdinals: currentOrdinals, pointOrdinals: pointOrdinals,
		affectedOrdinals:  affectedOrdinals,
		pathReadAuthority: pathReadAuthority, pathWriteAuthority: pathWriteAuthority,
		sealed: true,
	}, nil
}

func (l formalSparseLeafView) evaluateRootAssignment(plan *formalRootAssignmentStep, binding formalQualifiedBinding, factors *formalRootAssignmentFactors) (product.Value, error) {
	if plan == nil || binding.value.owner != l.variable || binding.value.arena != l.authority.terms {
		return product.Value{}, errFormalComponentForeignOwner
	}
	arena, term := binding.value.arena, binding.value.term
	var resolver valueNodeLeafResolver
	resolver = valueNodeLeafResolver{
		guard: func(guard Guard) (bool, bool, bool) { return l.exactGuard(l.variable, arena, binding.scope, guard) },
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
			member, ok := plan.sourceSlots[slot]
			if !ok {
				return product.Value{}, false
			}
			return l.value(member, plan.valuesTop)
		},
		dynamicRead: func(node valueNode, args []product.Value) (product.Value, bool) {
			if factors == nil {
				return product.Value{}, false
			}
			return resolveFormalDynamicValue(l.body, l.span, node, args, factors.values, func(child ValueTerm) (product.Value, bool) {
				return arena.evalValueCanonicalWithLeaves(child, resolver)
			})
		},
		allocationResult: func(candidate valueNode) (product.Value, bool) {
			return arena.allocationResult(candidate.allocation, candidate.resultIndex)
		},
	}
	value, exact := arena.evalValueCanonicalWithLeaves(term, resolver)
	if !exact || !product.BelongsToRegistry(l.authority.product.Registry(), value) {
		return product.Value{}, fmt.Errorf("transformer: formal RootAssignment source is unsupported")
	}
	return value, nil
}

type formalRootAssignmentFactors struct {
	bindings []formalRootAssignmentLaneBinding
	values   []state.LaneFactor
}

func materializeFormalRootAssignmentFactors(view formalSparseLeafView, bindings []formalRootAssignmentLaneBinding) (formalRootAssignmentFactors, error) {
	out := formalRootAssignmentFactors{bindings: bindings, values: make([]state.LaneFactor, len(bindings))}
	for index, binding := range bindings {
		factor, err := view.laneFactor(binding.group)
		if err != nil {
			return formalRootAssignmentFactors{}, err
		}
		out.values[index] = factor
	}
	return out, nil
}

func (f *formalRootAssignmentFactors) at(lane state.ProductLane) (*state.LaneFactor, bool) {
	for index := range f.bindings {
		if f.bindings[index].lane.Ordinal() == lane.Ordinal() {
			return &f.values[index], true
		}
	}
	return nil, false
}

func (f formalRootAssignmentFactors) value(lane state.ProductLane) (state.LaneFactor, bool) {
	value, ok := (&f).at(lane)
	if !ok {
		return state.LaneFactor{}, false
	}
	return *value, true
}

func (a *formalTupleAlgebra) applyFormalRootAssignmentPlan(operator formalRelationOperatorRef, predecessor, pointEntry formalRelationTuple) (formalRelationTuple, error) {
	plan := operator.rootAssignment
	if a == nil || !plan.valid(operator) {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal RootAssignment has no complete factor transaction")
	}
	if err := a.validateTuple(predecessor); err != nil {
		return formalRelationTuple{}, err
	}
	if err := a.validateTuple(pointEntry); err != nil {
		return formalRelationTuple{}, err
	}
	if predecessor.bottom() || pointEntry.bottom() || predecessor.variable != pointEntry.variable {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	_, directory, authority, ok := a.span(predecessor.variable)
	if !ok || predecessor.root.owner != directory || pointEntry.root.owner != directory || authority.code != operator.code {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	mark := a.decisions.checkpoint()
	fail := func(err error) (formalRelationTuple, error) {
		a.decisions.rollback(mark)
		return formalRelationTuple{}, err
	}
	execute := decisionTrue
	if plan.guard != 0 {
		var err error
		execute, err = a.decisionForGuard(plan.variable, plan.scope, plan.code.terms, plan.guard)
		if err != nil {
			return fail(err)
		}
	}
	result, err := a.applyFormalClosedFactorLift(
		plan.lift,
		[]formalRelationTuple{predecessor, pointEntry},
		plan.demands,
		execute,
		func(_ decisionRef, views []formalSparseLeafView) ([]formalClosedFactorLeafWrite, error) {
			if len(views) != 2 {
				return nil, errFormalComponentMalformed
			}
			leaves, leafErr := a.applyFormalRootAssignmentLeaf(plan, views[0], views[1])
			if leafErr != nil || len(leaves) != len(plan.affectedOrdinals) {
				if leafErr == nil {
					leafErr = errFormalComponentMalformed
				}
				return nil, leafErr
			}
			writes := make([]formalClosedFactorLeafWrite, 0, len(leaves))
			for index, ordinal := range plan.affectedOrdinals {
				prior, present := views[0].leaf(ordinal)
				if !present {
					return nil, errFormalComponentMalformed
				}
				if prior != leaves[index] {
					writes = append(writes, formalClosedFactorLeafWrite{ordinal: ordinal, leaf: leaves[index]})
				}
			}
			return writes, nil
		},
	)
	if err != nil {
		return fail(err)
	}
	if result.root == predecessor.root {
		a.decisions.rollback(mark)
		return predecessor, nil
	}
	return result, nil
}
func (a *formalTupleAlgebra) applyFormalRootAssignmentLeaf(plan *formalRootAssignmentStep, currentView, pointView formalSparseLeafView) ([]decisionLeaf, error) {
	current, err := materializeFormalRootAssignmentFactors(currentView, plan.current)
	if err != nil {
		return nil, err
	}
	point, err := materializeFormalRootAssignmentFactors(pointView, plan.point)
	if err != nil {
		return nil, err
	}
	sources := make([]product.Value, len(plan.sources))
	for index, binding := range plan.sources {
		sources[index], err = currentView.evaluateRootAssignment(plan, binding, &current)
		if err != nil {
			return nil, fmt.Errorf("transformer: formal RootAssignment source %d: %w", index, err)
		}
	}
	if len(sources) == 0 {
		return nil, errFormalComponentMalformed
	}
	primary := sources[0]
	present, err := a.formalRootAssignmentPointPresence(plan, pointView, &point)
	if err != nil {
		return nil, err
	}
	var dynamic factapply.RootAssignmentDynamicSourceTransaction
	hasDynamic := false
	composed := product.Bottom(currentView.authority.product.Registry())
	productive := false
	published := product.Bottom(currentView.authority.product.Registry())
	hasPublished := false
	var pathResult factapply.RootAssignmentPathFactorResult
	var freshPredicates []factapply.FreshEmptyPredicate
	topOrdinal, topOK := plan.valuesTop.address(plan.values)
	targetOrdinal, targetOK := plan.targetMember.address(plan.values)
	topLeaf, topPresent := currentView.leaf(topOrdinal)
	if !topOK || !targetOK || !topPresent || topLeaf > 1 {
		return nil, errFormalComponentMalformed
	}
	for _, stage := range plan.factor.Stages() {
		switch stage {
		case factapply.RootAssignmentFactorStageObjectMaterialization:
			primary, err = a.applyFormalRootAssignmentObject(plan, currentView, sources, &current)
		case factapply.RootAssignmentFactorStageSourceComposition:
			if _, dynamicShape := plan.plan.DynamicSourcePlan(); dynamicShape {
				dynamic, err = a.resolveFormalRootAssignmentDynamic(plan, currentView, &current, sources)
				hasDynamic = err == nil
				if err == nil {
					present = present || dynamic.DefinitelyPresent()
				}
			}
			if err != nil {
				break
			}
			composed, productive, err = plan.factor.ComposeSource(primary, present)
		case factapply.RootAssignmentFactorStageValuePublication:
			published, hasPublished, err = plan.factor.ApplyValuePublication(composed, productive, topLeaf == 1)
		case factapply.RootAssignmentFactorStageDynamicSource:
			if !hasDynamic {
				err = errFormalComponentMalformed
			} else {
				for _, lane := range currentView.authority.product.RootAssignmentDynamicSourceLanes() {
					factor, found := current.at(lane)
					if !found {
						return nil, errFormalComponentMalformed
					}
					*factor, err = plan.factor.ApplyDynamicSource(dynamic, *factor)
					if err != nil {
						break
					}
				}
			}
		case factapply.RootAssignmentFactorStagePathMutation:
			if productive {
				pathResult, err = a.applyFormalRootAssignmentPath(plan, currentView, composed, dynamic, hasDynamic, &current)
			}
		case factapply.RootAssignmentFactorStageEqualityQuotient:
			if productive {
				err = a.applyFormalRootAssignmentEqualities(plan, pathResult.Equalities, &current)
			}
		case factapply.RootAssignmentFactorStageScalarTransfer:
			if productive {
				err = a.applyFormalRootAssignmentScalar(plan, currentView, &current, &point)
			}
		case factapply.RootAssignmentFactorStageFreshEmpty:
			if productive {
				freshPredicates, err = a.formalRootAssignmentFreshPredicates(plan, currentView, &current)
			}
		case factapply.RootAssignmentFactorStageCompletion:
			if productive {
				err = a.applyFormalRootAssignmentCompletion(plan, primary, freshPredicates, &current)
			}
		default:
			err = errFormalComponentMalformed
		}
		if err != nil {
			return nil, fmt.Errorf("transformer: formal RootAssignment stage %d: %w", stage, err)
		}
	}

	leaves := make([]decisionLeaf, len(plan.affectedOrdinals))
	for index, ordinal := range plan.affectedOrdinals {
		leaf, present := currentView.leaf(ordinal)
		if !present {
			return nil, errFormalComponentMalformed
		}
		leaves[index] = leaf
	}
	// Registered sidecar stages are not all value-productivity dependent. In
	// particular DynamicSource publishes read origins and key memberships even
	// when the abstract scalar read is unresolved. Values publication remains
	// gated by productive; every registered write factor below carries its own
	// canonical stage result (often the unchanged physical factor).
	if productive && topLeaf == 0 {
		if !hasPublished {
			return nil, errFormalComponentMalformed
		}
		targetLeaf := decisionLeaf(0)
		if !product.Equal(currentView.authority.product.Registry(), published, product.Bottom(currentView.authority.product.Registry())) {
			targetLeaf, err = currentView.authority.internGroundValue(published)
			if err != nil {
				return nil, err
			}
		}
		index := sort.Search(len(plan.affectedOrdinals), func(index int) bool { return plan.affectedOrdinals[index] >= targetOrdinal })
		if index >= len(leaves) || plan.affectedOrdinals[index] != targetOrdinal {
			return nil, errFormalComponentMalformed
		}
		leaves[index] = targetLeaf
	}
	span := currentView.span
	for _, binding := range plan.writes {
		factor, found := current.value(binding.lane)
		if !found {
			return nil, errFormalComponentMalformed
		}
		groupLeaves, factorErr := a.factorFormalSparseLane(currentView.authority, span, binding.group, factor)
		if factorErr != nil || len(groupLeaves) != len(binding.group.members) {
			if factorErr == nil {
				factorErr = errFormalComponentMalformed
			}
			return nil, factorErr
		}
		for memberIndex, ordinal := range binding.group.members {
			index := sort.Search(len(plan.affectedOrdinals), func(index int) bool { return plan.affectedOrdinals[index] >= ordinal })
			if index >= len(leaves) || plan.affectedOrdinals[index] != ordinal {
				return nil, errFormalComponentMalformed
			}
			leaves[index] = groupLeaves[memberIndex]
		}
	}
	return leaves, nil
}

func (a *formalTupleAlgebra) applyFormalRootAssignmentObject(plan *formalRootAssignmentStep, view formalSparseLeafView, sources []product.Value, current *formalRootAssignmentFactors) (product.Value, error) {
	objectPlan, ok := plan.plan.ObjectLiteralSourcePlan()
	if !ok {
		return product.Value{}, errFormalComponentMalformed
	}
	prepared, err := objectPlan.PrepareGuardedObjectConstructor(view.authority.product, view.span.keys, sources)
	if err != nil {
		return product.Value{}, err
	}
	constructor, rows, ok := prepared.ObjectConstructor()
	if !ok {
		return product.Value{}, errFormalComponentMalformed
	}
	lanes, err := plan.factor.ObjectConstructorLanes(constructor)
	if err != nil {
		return product.Value{}, err
	}
	for _, lane := range lanes {
		factor, present := current.at(lane)
		if !present {
			return product.Value{}, errFormalComponentMalformed
		}
		*factor, err = plan.factor.ApplyObjectMaterialization(constructor, rows, *factor)
		if err != nil {
			return product.Value{}, err
		}
	}
	primary, ok := prepared.RootSourceValue()
	if !ok {
		return product.Value{}, errFormalComponentMalformed
	}
	return primary, nil
}

func (a *formalTupleAlgebra) formalRootAssignmentPointPresence(plan *formalRootAssignmentStep, pointView formalSparseLeafView, point *formalRootAssignmentFactors) (bool, error) {
	proof, present := plan.plan.SourcePresenceProof()
	if !present {
		return false, nil
	}
	domain := pointView.authority.product
	family, ok := domain.PathEvidenceCoordinateFamily()
	if !ok {
		return false, errFormalComponentMalformed
	}
	factor, ok := point.value(family.Lane())
	if !ok {
		return false, errFormalComponentMalformed
	}
	skeleton, scalars, err := domain.DecomposeCoordinateFamily(factor, family, pointView.span.keys)
	if err != nil {
		return false, err
	}
	carrier, err := domain.OpenCoordinatePathEvidenceCarrier(
		skeleton, scalars, state.ValueLaneFactor{}, false,
		plan.pathReadAuthority, state.PathDescendantMutationFactors{},
	)
	if err != nil {
		return false, err
	}
	return carrier.HasProof(proof), nil
}

type formalRootAssignmentCoordinateLane struct {
	lane      state.ProductLane
	families  []state.CoordinateFamily
	skeletons []state.CoordinateFamilySkeleton
	scalars   [][]state.CoordinateScalarFactor
}

func openFormalRootAssignmentCoordinateLane(domain state.ProductDomain, factor state.LaneFactor, keys *keyspace.KeySpace) (formalRootAssignmentCoordinateLane, error) {
	families, err := domain.CoordinateFamilies(factor.Lane())
	if err != nil || len(families) == 0 {
		return formalRootAssignmentCoordinateLane{}, err
	}
	out := formalRootAssignmentCoordinateLane{
		lane: factor.Lane(), families: families,
		skeletons: make([]state.CoordinateFamilySkeleton, len(families)),
		scalars:   make([][]state.CoordinateScalarFactor, len(families)),
	}
	for index, family := range families {
		out.skeletons[index], out.scalars[index], err = domain.DecomposeCoordinateFamily(factor, family, keys)
		if err != nil {
			return formalRootAssignmentCoordinateLane{}, err
		}
	}
	return out, nil
}

func (l *formalRootAssignmentCoordinateLane) familyIndex(family state.CoordinateFamily) (int, bool) {
	index := int(family.Ordinal())
	return index, index >= 0 && index < len(l.families) &&
		l.families[index].Lane().Ordinal() == family.Lane().Ordinal() && l.families[index].ID() == family.ID()
}

func (l *formalRootAssignmentCoordinateLane) resolve(domain state.ProductDomain, family state.CoordinateFamily, slot state.CoordinateSlot) (state.CoordinateFamilySkeleton, state.CoordinateScalarFactor, error) {
	index, ok := l.familyIndex(family)
	if !ok {
		return state.CoordinateFamilySkeleton{}, state.CoordinateScalarFactor{}, errFormalComponentMalformed
	}
	for _, scalar := range l.scalars[index] {
		equal, err := domain.CoordinateSlotEqual(scalar.Slot(), slot)
		if err != nil {
			return state.CoordinateFamilySkeleton{}, state.CoordinateScalarFactor{}, err
		}
		if equal {
			return l.skeletons[index], scalar, nil
		}
	}
	scalar, err := domain.CoordinateDefault(l.skeletons[index], slot)
	return l.skeletons[index], scalar, err
}

func (l *formalRootAssignmentCoordinateLane) replace(domain state.ProductDomain, family state.CoordinateFamily, skeleton state.CoordinateFamilySkeleton, scalar state.CoordinateScalarFactor) error {
	index, ok := l.familyIndex(family)
	if !ok {
		return errFormalComponentMalformed
	}
	l.skeletons[index] = skeleton
	next := make([]state.CoordinateScalarFactor, 0, len(l.scalars[index])+1)
	replaced := false
	for _, candidate := range l.scalars[index] {
		equal, err := domain.CoordinateSlotEqual(candidate.Slot(), scalar.Slot())
		if err != nil {
			return err
		}
		if equal {
			candidate, replaced = scalar, true
		}
		omitted, err := domain.CoordinateScalarIsOmitted(skeleton, candidate)
		if err != nil {
			return err
		}
		if !omitted {
			next = append(next, candidate)
		}
	}
	if !replaced {
		omitted, err := domain.CoordinateScalarIsOmitted(skeleton, scalar)
		if err != nil {
			return err
		}
		if !omitted {
			next = append(next, scalar)
			if err := formalRootAssignmentSortedScalars(domain, next); err != nil {
				return err
			}
		}
	}
	l.scalars[index] = next
	return nil
}

func (l formalRootAssignmentCoordinateLane) compose(domain state.ProductDomain, keys *keyspace.KeySpace) (state.LaneFactor, error) {
	return domain.ComposeCoordinateFamilies(l.lane, keys, l.skeletons, l.scalars)
}

func (a *formalTupleAlgebra) resolveFormalRootAssignmentDynamic(plan *formalRootAssignmentStep, view formalSparseLeafView, current *formalRootAssignmentFactors, sources []product.Value) (factapply.RootAssignmentDynamicSourceTransaction, error) {
	dynamicPlan, ok := plan.plan.DynamicSourcePlan()
	if !ok {
		return factapply.RootAssignmentDynamicSourceTransaction{}, errFormalComponentMalformed
	}
	dependencies, ok, err := plan.plan.DynamicSourceDependencies()
	if err != nil || !ok {
		if err == nil {
			err = errFormalComponentMalformed
		}
		return factapply.RootAssignmentDynamicSourceTransaction{}, err
	}
	lanes := dependencies.InputLanes()
	factors := make([]state.LaneFactor, len(lanes))
	for index, lane := range lanes {
		factor, present := current.value(lane)
		if !present {
			return factapply.RootAssignmentDynamicSourceTransaction{}, errFormalComponentMalformed
		}
		factors[index] = factor
	}
	bound, err := view.authority.product.BindRootAssignmentDynamicSourceInputs(dependencies, factors)
	if err != nil {
		return factapply.RootAssignmentDynamicSourceTransaction{}, err
	}
	dynamicFacts, factsOK := bound.DynamicIndexFactor()
	memberships, membershipsOK := bound.KeyMembershipFactor()
	if !factsOK || !membershipsOK {
		return factapply.RootAssignmentDynamicSourceTransaction{}, errFormalComponentMalformed
	}
	keySource, ok := dynamicPlan.KeyValueInput()
	if !ok {
		return factapply.RootAssignmentDynamicSourceTransaction{}, errFormalComponentMalformed
	}
	keyOrdinal, ok := plan.term.transaction.SourceOrdinal(keySource)
	if !ok || keyOrdinal < 0 || keyOrdinal >= len(sources) {
		return factapply.RootAssignmentDynamicSourceTransaction{}, errFormalComponentMalformed
	}
	reg := view.authority.product.Registry()
	inputs := factapply.RootAssignmentDynamicSourceInputs{
		KeyValue: sources[keyOrdinal], HasKeyValue: !product.Equal(reg, sources[keyOrdinal], product.Bottom(reg)),
		DynamicIndexFactor: dynamicFacts, KeyMembershipFactor: memberships,
	}
	if _, base, hasModulo := dynamicPlan.ModuloLengthPresenceInput(); hasModulo {
		ordinal, found := plan.term.transaction.SourceOrdinal(base)
		if !found || ordinal < 0 || ordinal >= len(sources) {
			return factapply.RootAssignmentDynamicSourceTransaction{}, errFormalComponentMalformed
		}
		inputs.ModuloBaseValue = sources[ordinal]
		inputs.HasModuloBaseValue = !product.Equal(reg, sources[ordinal], product.Bottom(reg))
	}
	if query, present, queryErr := dynamicPlan.TableNonEmptyQuery(); queryErr != nil {
		return factapply.RootAssignmentDynamicSourceTransaction{}, queryErr
	} else if present {
		resolve := func(get func() (state.CoordinateSlot, bool)) (state.CoordinateScalarFactor, error) {
			slot, ok := get()
			if !ok {
				return state.CoordinateScalarFactor{}, errFormalComponentMalformed
			}
			factor, ok := current.value(slot.Family().Lane())
			if !ok {
				return state.CoordinateScalarFactor{}, errFormalComponentMalformed
			}
			lane, openErr := openFormalRootAssignmentCoordinateLane(view.authority.product, factor, view.span.keys)
			if openErr != nil {
				return state.CoordinateScalarFactor{}, openErr
			}
			_, scalar, resolveErr := lane.resolve(view.authority.product, slot.Family(), slot)
			return scalar, resolveErr
		}
		length, queryErr := resolve(query.LenFloorSlot)
		if queryErr != nil {
			return factapply.RootAssignmentDynamicSourceTransaction{}, queryErr
		}
		refinement, queryErr := resolve(query.RefinementSlot)
		if queryErr != nil {
			return factapply.RootAssignmentDynamicSourceTransaction{}, queryErr
		}
		member, queryErr := resolve(query.StaticMemberSlot)
		if queryErr != nil {
			return factapply.RootAssignmentDynamicSourceTransaction{}, queryErr
		}
		queryInputs := factapply.RootAssignmentTableNonEmptyInputs{LenFloor: length, Refinement: refinement, StaticMember: member}
		if rootSlot, hasRoot := query.RootValueSlot(); hasRoot {
			formalSlot, found := formalMiddleSlotForStateKey(view.algebra.program, view.body, rootSlot)
			if !found {
				return factapply.RootAssignmentDynamicSourceTransaction{}, errFormalComponentMalformed
			}
			rootMember, found := (formalValuesFiberGroup{descriptor: plan.values}).slot(formalSlot)
			if !found {
				return factapply.RootAssignmentDynamicSourceTransaction{}, errFormalComponentMalformed
			}
			rootValue, found := view.value(rootMember, plan.valuesTop)
			if !found {
				return factapply.RootAssignmentDynamicSourceTransaction{}, errFormalComponentMalformed
			}
			queryInputs.HasRootValue, queryInputs.RootValue = true, rootValue
		}
		inputs.TableDefinitelyNonEmpty, queryErr = query.DefinitelyNonEmpty(plan.code.terms.typeValues, queryInputs)
		if queryErr != nil {
			return factapply.RootAssignmentDynamicSourceTransaction{}, queryErr
		}
	}
	return plan.factor.ResolveDynamicSource(a.ctx, inputs)
}

func (a *formalTupleAlgebra) applyFormalRootAssignmentPath(plan *formalRootAssignmentStep, view formalSparseLeafView, composed product.Value, dynamic factapply.RootAssignmentDynamicSourceTransaction, hasDynamic bool, current *formalRootAssignmentFactors) (factapply.RootAssignmentPathFactorResult, error) {
	domain, keys := view.authority.product, view.span.keys
	family, ok := domain.PathEvidenceCoordinateFamily()
	if !ok {
		return factapply.RootAssignmentPathFactorResult{}, errFormalComponentMalformed
	}
	factor, ok := current.at(family.Lane())
	if !ok {
		return factapply.RootAssignmentPathFactorResult{}, errFormalComponentMalformed
	}
	lane, err := openFormalRootAssignmentCoordinateLane(domain, *factor, keys)
	if err != nil {
		return factapply.RootAssignmentPathFactorResult{}, err
	}
	_, ok = lane.familyIndex(family)
	if !ok {
		return factapply.RootAssignmentPathFactorResult{}, errFormalComponentMalformed
	}
	oldValue, present := view.value(plan.targetMember, plan.valuesTop)
	if !present {
		return factapply.RootAssignmentPathFactorResult{}, errFormalComponentMalformed
	}
	factors, err := domain.BindPathSubtreeMutationFactors(keys, current.value)
	if err != nil {
		return factapply.RootAssignmentPathFactorResult{}, err
	}
	result, err := plan.factor.ApplyPathMutation(factapply.RootAssignmentPathFactorInput{
		Factors: factors, Authority: plan.pathWriteAuthority,
		OldValue: oldValue, Composed: composed, Dynamic: dynamic, HasDynamic: hasDynamic,
	})
	if err != nil {
		return factapply.RootAssignmentPathFactorResult{}, err
	}
	for _, next := range result.Factors.LaneFactors() {
		factor, ok := current.at(next.Lane())
		if !ok {
			return factapply.RootAssignmentPathFactorResult{}, errFormalComponentMalformed
		}
		*factor = next
	}
	for _, next := range result.Factors.CoordinateFactors() {
		factor, ok := current.at(next.Family().Lane())
		if !ok {
			return factapply.RootAssignmentPathFactorResult{}, errFormalComponentMalformed
		}
		*factor, err = domain.ReplaceCoordinateFamily(*factor, next.Skeleton(), next.Scalars())
		if err != nil {
			return factapply.RootAssignmentPathFactorResult{}, err
		}
	}
	return result, nil
}

func (a *formalTupleAlgebra) applyFormalRootAssignmentEqualities(plan *formalRootAssignmentStep, equalities []state.PathEqualityTransaction, current *formalRootAssignmentFactors) error {
	if len(equalities) == 0 {
		return nil
	}
	for _, equality := range equalities {
		for _, lane := range plan.domain.PathEqualityQuotientLanes() {
			factor, ok := current.at(lane)
			if !ok {
				return errFormalComponentMalformed
			}
			next, err := plan.factor.ApplyEqualityFactor(equality, *factor)
			if err != nil {
				return err
			}
			*factor = next
		}
	}
	return nil
}

func (a *formalTupleAlgebra) applyFormalRootAssignmentScalar(plan *formalRootAssignmentStep, view formalSparseLeafView, current, point *formalRootAssignmentFactors) error {
	domain, keys := plan.domain, view.span.keys
	for _, lane := range domain.RootAssignmentScalarLanes() {
		currentFactor, currentOK := current.at(lane)
		pointFactor, pointOK := point.value(lane)
		if !currentOK || !pointOK {
			return fmt.Errorf("scalar lane %q operands current=%t point=%t: %w", lane.ID(), currentOK, pointOK, errFormalComponentMalformed)
		}
		next, err := plan.factor.ApplyScalarFactor(pointFactor, *currentFactor)
		if err != nil {
			return err
		}
		*currentFactor = next
	}
	transaction, present := plan.plan.ScalarFactorTransaction()
	if !present {
		return fmt.Errorf("scalar transaction absent: %w", errFormalComponentMalformed)
	}
	for _, family := range domain.RootAssignmentScalarCoordinateFamilies() {
		currentFactor, currentOK := current.at(family.Lane())
		pointFactor, pointOK := point.value(family.Lane())
		if !currentOK || !pointOK {
			return fmt.Errorf("scalar coordinate %q operands current=%t point=%t: %w", family.ID(), currentOK, pointOK, errFormalComponentMalformed)
		}
		currentLane, err := openFormalRootAssignmentCoordinateLane(domain, *currentFactor, keys)
		if err != nil {
			return err
		}
		pointLane, err := openFormalRootAssignmentCoordinateLane(domain, pointFactor, keys)
		if err != nil {
			return err
		}
		familyIndex, ok := currentLane.familyIndex(family)
		if !ok {
			return fmt.Errorf("scalar coordinate family %q absent: %w", family.ID(), errFormalComponentMalformed)
		}
		inventory := make([]state.CoordinateSlot, len(currentLane.scalars[familyIndex]))
		for index := range inventory {
			inventory[index] = currentLane.scalars[familyIndex][index].Slot()
		}
		demands, err := domain.RootAssignmentScalarCoordinateDemands(transaction, family, keys, inventory)
		if err != nil {
			return err
		}
		for _, demand := range demands {
			skeleton, target, err := currentLane.resolve(domain, family, demand.Target())
			if err != nil {
				return err
			}
			var source state.CoordinateScalarFactor
			sourceSlot, hasSource := demand.PointSource()
			if hasSource {
				_, source, err = pointLane.resolve(domain, sourceSlot.Family(), sourceSlot)
				if err != nil {
					return err
				}
			}
			skeleton, target, err = plan.factor.ApplyScalarCoordinate(skeleton, target, source, hasSource)
			if err != nil {
				return err
			}
			if err := currentLane.replace(domain, family, skeleton, target); err != nil {
				return err
			}
		}
		*currentFactor, err = currentLane.compose(domain, keys)
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *formalTupleAlgebra) formalRootAssignmentFreshPredicates(plan *formalRootAssignmentStep, view formalSparseLeafView, current *formalRootAssignmentFactors) ([]factapply.FreshEmptyPredicate, error) {
	domain, keys := plan.domain, view.span.keys
	predicates := make([]factapply.FreshEmptyPredicate, len(plan.fresh))
	if len(plan.fresh) == 0 {
		return predicates, nil
	}
	family, ok := domain.RootAssignmentCoordinateFamily()
	if !ok {
		return nil, errFormalComponentMalformed
	}
	factor, ok := current.value(family.Lane())
	if !ok {
		return nil, errFormalComponentMalformed
	}
	lane, err := openFormalRootAssignmentCoordinateLane(domain, factor, keys)
	if err != nil {
		return nil, err
	}
	familyIndex, ok := lane.familyIndex(family)
	if !ok {
		return nil, errFormalComponentMalformed
	}
	for index, query := range plan.fresh {
		value, present := view.value(query.member, plan.valuesTop)
		if !present {
			return nil, errFormalComponentMalformed
		}
		fresh, err := plan.factor.EvaluateFreshEmpty(lane.skeletons[familyIndex], value)
		if err != nil {
			return nil, err
		}
		predicates[index] = factapply.FreshEmptyPredicate{Path: query.path.Clone(), Fresh: fresh}
	}
	return predicates, nil
}

func (a *formalTupleAlgebra) applyFormalRootAssignmentCompletion(plan *formalRootAssignmentStep, primary product.Value, predicates []factapply.FreshEmptyPredicate, current *formalRootAssignmentFactors) error {
	domain := plan.domain
	keys, ok := plan.plan.PathKeySpace()
	if !ok {
		return errFormalComponentMalformed
	}
	completion, err := plan.factor.PrepareCompletion(domain.Registry(), primary, predicates)
	if err != nil {
		return err
	}
	for _, lane := range domain.RootAssignmentCompletionLanes() {
		factor, ok := current.at(lane)
		if !ok {
			return errFormalComponentMalformed
		}
		*factor, err = plan.factor.ApplyCompletionFactor(completion, *factor)
		if err != nil {
			return err
		}
	}
	for _, family := range domain.RootAssignmentCompletionCoordinateFamilies() {
		factor, ok := current.at(family.Lane())
		if !ok {
			return errFormalComponentMalformed
		}
		lane, err := openFormalRootAssignmentCoordinateLane(domain, *factor, keys)
		if err != nil {
			return err
		}
		slot, present, err := domain.RootAssignmentCompletionCoordinateSlot(completion, family, keys)
		if err != nil {
			return err
		}
		if !present {
			continue
		}
		skeleton, scalar, err := lane.resolve(domain, family, slot)
		if err != nil {
			return err
		}
		skeleton, scalar, err = plan.factor.ApplyCompletionCoordinate(completion, skeleton, scalar)
		if err != nil {
			return err
		}
		if err := lane.replace(domain, family, skeleton, scalar); err != nil {
			return err
		}
		*factor, err = lane.compose(domain, keys)
		if err != nil {
			return err
		}
	}
	return nil
}

func appendFormalRootAssignmentCoordinateSlot(domain state.ProductDomain, slots []state.CoordinateSlot, slot state.CoordinateSlot) []state.CoordinateSlot {
	for _, existing := range slots {
		equal, err := domain.CoordinateSlotEqual(existing, slot)
		if err == nil && equal {
			return slots
		}
	}
	return append(slots, slot)
}

func formalRootAssignmentSortedScalars(domain state.ProductDomain, scalars []state.CoordinateScalarFactor) error {
	var err error
	sort.SliceStable(scalars, func(i, j int) bool {
		if err != nil {
			return false
		}
		var less bool
		less, err = domain.CoordinateSlotLess(scalars[i].Slot(), scalars[j].Slot())
		return less
	})
	return err
}
