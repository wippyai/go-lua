package transformer

import (
	"fmt"
	"sort"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// formalPresenceImplicationStep is only the formal root binding and product
// layout for canonical N2. Publication order, descendant barriers, dependency
// SCCs and closure all remain owned by factapply.PresenceImplicationPlan.
type formalPresenceImplicationStep struct {
	plan               factapply.PresenceImplicationDependencyPlan
	roots              factapply.PresenceImplicationRootBinding[FormalSlot]
	values             formalFiberGroupDescriptor
	path               formalFiberGroupDescriptor
	positions          map[state.ProductLane]int
	valuesTop          formalFiberOrdinal
	valueRoots         []formalPresenceValueRoot
	pathFamily         formalCoordinateFamilyFiberGroup
	pathSlots          []formalPresenceCoordinateSlot
	mutationLanes      []formalFiberGroupDescriptor
	mutationFamilies   []formalCoordinateFamilyFiberGroup
	readOrdinals       []formalFiberOrdinal
	writeOrdinals      []formalFiberOrdinal
	projectionOrdinals []formalFiberOrdinal
	demands            []formalQualifiedGuardDemand
	variable           relationVar
}

type formalPresenceValueRoot struct {
	root    FormalSlot
	ordinal formalFiberOrdinal
}

type formalPresenceCoordinateSlot struct {
	slot    state.CoordinateSlot
	ordinal formalFiberOrdinal
}

func sealFormalPresenceOrdinals(span formalFiberDescriptorSpan, ordinals []formalFiberOrdinal) ([]formalFiberOrdinal, error) {
	sort.Slice(ordinals, func(i, j int) bool { return ordinals[i] < ordinals[j] })
	unique := ordinals[:0]
	for _, ordinal := range ordinals {
		if int(ordinal) < 0 || int(ordinal) >= span.count {
			return nil, fmt.Errorf("transformer: formal N2 ordinal is outside the product")
		}
		if len(unique) == 0 || unique[len(unique)-1] != ordinal {
			unique = append(unique, ordinal)
		}
	}
	return unique, nil
}

func freezeFormalPresenceImplicationStep(program *RelationProgram, variable relationVar, operator formalRelationOperatorRef) (*formalPresenceImplicationStep, error) {
	if program == nil || variable == 0 || int(variable) > len(program.bodies) || operator.kind != formalRelationCellStep ||
		operator.code == nil || operator.root == 0 || operator.step == 0 || int(operator.root) >= len(operator.code.nodes) ||
		int(operator.step) > len(operator.code.nodes[operator.root].steps) {
		return nil, nil
	}
	step := operator.code.nodes[operator.root].steps[operator.step-1]
	if step.kind != boundaryStepPresenceImplications {
		return nil, nil
	}
	body := &program.bodies[variable-1]
	span, ok := program.formalFibers.span(variable)
	if !ok || body.pathSemantics == nil || !body.pathSemantics.Valid() || program.formalSlots == nil || body.relation.code != operator.code {
		return nil, fmt.Errorf("transformer: formal N2 has no path authority")
	}
	concrete, err := body.pathSemantics.PreparePathValuePresenceImplications(body.productDomain.Registry(), step.presence)
	if err != nil {
		return nil, err
	}
	formalPlan, err := concrete.RekeyFormal(body.productDomain, span.rekey)
	if err != nil {
		return nil, err
	}
	inventory := span.coordinates
	if !inventory.ValidFor(body.productDomain, span.keys) {
		return nil, fmt.Errorf("PresenceImplications N2 has no frozen coordinate inventory")
	}
	dependency, err := formalPlan.DependencyBlocks(body.productDomain, inventory)
	if err != nil {
		return nil, err
	}
	var demands []formalQualifiedGuardDemand
	if step.guard != 0 {
		demands = []formalQualifiedGuardDemand{{owner: variable, scope: operator.scope, arena: operator.code.terms, guard: step.guard}}
	}
	return freezeFormalPresenceImplicationDependency(program, body, span, dependency, demands)
}

func freezeFormalPresenceImplicationDependency(
	program *RelationProgram,
	body *relationProgramBody,
	span formalFiberDescriptorSpan,
	dependency factapply.PresenceImplicationDependencyPlan,
	demands []formalQualifiedGuardDemand,
) (*formalPresenceImplicationStep, error) {
	if program == nil || body == nil || body.variable == 0 || span.variable != body.variable {
		return nil, fmt.Errorf("transformer: formal N2 dependency has no product ownership")
	}
	binding, err := factapply.SealPresenceImplicationRootBinding(dependency, func(dependency statekey.ValueDependency) (FormalSlot, bool) {
		return formalLiveValueSlotForDependency(program, body, dependency)
	}, func(slot FormalSlot) bool { return slot.Valid() })
	if err != nil {
		return nil, err
	}
	values, ok := span.valuesGroup()
	if !ok {
		return nil, fmt.Errorf("transformer: formal N2 has no Values factor")
	}
	valuesTop, ok := values.top()
	if !ok {
		return nil, fmt.Errorf("transformer: formal N2 has no Values-top fiber")
	}
	valuesTopOrdinal, ok := valuesTop.address(values.descriptor)
	if !ok {
		return nil, fmt.Errorf("transformer: formal N2 Values-top fiber is malformed")
	}
	family, ok := body.productDomain.PathEvidenceCoordinateFamily()
	if !ok {
		return nil, fmt.Errorf("transformer: formal N2 has no path-evidence factor")
	}
	path, ok := span.coordinateLaneGroup(family.Lane())
	if !ok {
		return nil, fmt.Errorf("transformer: formal N2 path-evidence factor is outside the tuple")
	}
	var pathFamily formalCoordinateFamilyFiberGroup
	for _, candidate := range path.families() {
		if coordinateFamilySame(candidate.family, family) {
			pathFamily = candidate
			break
		}
	}
	if pathFamily.family.ID() == "" {
		return nil, fmt.Errorf("transformer: formal N2 path-evidence family is outside the tuple")
	}
	lanes := body.productDomain.NonValuesLaneInventory()
	positions := make(map[state.ProductLane]int, len(lanes))
	for index, lane := range lanes {
		positions[lane] = index
	}
	valueRootOrdinals := make(map[FormalSlot]formalFiberOrdinal)
	pathSlotOrdinals := make(map[formalFiberOrdinal]state.CoordinateSlot)
	// The path-evidence carrier is materialized and republished through its
	// registered lane spelling.  Its semantic reducer may touch only selected
	// family slots, but the physical carrier contract requires every member of
	// that lane to be present in the formal projection.
	readOrdinals := append([]formalFiberOrdinal{valuesTopOrdinal}, path.descriptor.members...)
	writeOrdinals := make([]formalFiberOrdinal, 0)
	bindValue := func(root FormalSlot, write bool) error {
		member, present := values.slot(root)
		if !present {
			return fmt.Errorf("transformer: formal N2 Values root is outside the tuple")
		}
		ordinal, present := member.address(values.descriptor)
		if !present {
			return errFormalComponentMalformed
		}
		valueRootOrdinals[root] = ordinal
		readOrdinals = append(readOrdinals, ordinal)
		if write {
			writeOrdinals = append(writeOrdinals, ordinal)
		}
		return nil
	}
	bindCoordinate := func(slot state.CoordinateSlot, write bool) error {
		position, present := formalCoordinatePosition(body.productDomain, span, pathFamily, slot)
		if !present || position < 0 || position >= len(pathFamily.scalars) {
			return fmt.Errorf("transformer: formal N2 coordinate is outside the path family")
		}
		ordinal := pathFamily.scalars[position]
		pathSlotOrdinals[ordinal] = slot
		readOrdinals = append(readOrdinals, pathFamily.skeleton, ordinal)
		if write {
			writeOrdinals = append(writeOrdinals, ordinal)
		}
		return nil
	}
	mayContradict := false
	pathMutation := false
	for _, stage := range dependency.Stages() {
		for _, slot := range stage.ReducerWrites() {
			if err := bindCoordinate(slot, true); err != nil {
				return nil, err
			}
		}
		if stage.ReducerWritesSkeleton() {
			readOrdinals = append(readOrdinals, pathFamily.skeleton)
			writeOrdinals = append(writeOrdinals, pathFamily.skeleton)
			for _, ordinal := range pathFamily.scalars {
				descriptor := span.forest.descriptors[span.first+int(ordinal)]
				pathSlotOrdinals[ordinal] = descriptor.coordinate
				readOrdinals = append(readOrdinals, ordinal)
				writeOrdinals = append(writeOrdinals, ordinal)
			}
		}
		for _, block := range stage.Blocks() {
			reads, readOK := binding.BlockRoots(block.ValueReadDependencies())
			writes, writeOK := binding.BlockRoots(block.ValueWriteDependencies())
			if !readOK || !writeOK {
				return nil, fmt.Errorf("transformer: formal N2 root binding is incomplete")
			}
			for _, root := range reads {
				if err := bindValue(root, false); err != nil {
					return nil, err
				}
			}
			for _, root := range writes {
				if err := bindValue(root, true); err != nil {
					return nil, err
				}
			}
			for _, slot := range block.CoordinateReads() {
				if err := bindCoordinate(slot, false); err != nil {
					return nil, err
				}
			}
			for _, slot := range block.CoordinateWrites() {
				if err := bindCoordinate(slot, true); err != nil {
					return nil, err
				}
			}
			pathMutation = pathMutation || block.PathMutation()
			mayContradict = mayContradict || block.MayContradict()
		}
	}
	mutationLanes := make([]formalFiberGroupDescriptor, 0)
	mutationFamilies := make([]formalCoordinateFamilyFiberGroup, 0)
	if pathMutation {
		topology, topologyErr := body.productDomain.SealPathDescendantMutationFactorTopology()
		if topologyErr != nil {
			return nil, topologyErr
		}
		for _, lane := range topology.Lanes() {
			group, present := span.ordinaryLaneGroup(lane)
			if !present {
				return nil, fmt.Errorf("transformer: formal N2 mutation lane %q is outside the tuple", lane.ID())
			}
			mutationLanes = append(mutationLanes, group.descriptor)
			readOrdinals = append(readOrdinals, group.descriptor.members...)
			writeOrdinals = append(writeOrdinals, group.descriptor.members...)
		}
		families := topology.Families()
		for _, mutationFamily := range families[1:] {
			group, present := span.coordinateLaneGroup(mutationFamily.Lane())
			if !present {
				return nil, fmt.Errorf("transformer: formal N2 mutation coordinate lane %q is outside the tuple", mutationFamily.Lane().ID())
			}
			var selected formalCoordinateFamilyFiberGroup
			for _, candidate := range group.families() {
				if coordinateFamilySame(candidate.family, mutationFamily) {
					selected = candidate
					break
				}
			}
			if selected.family.ID() == "" {
				return nil, fmt.Errorf("transformer: formal N2 mutation family is outside the tuple")
			}
			mutationFamilies = append(mutationFamilies, selected)
			readOrdinals = append(readOrdinals, selected.skeleton)
			readOrdinals = append(readOrdinals, selected.scalars...)
			writeOrdinals = append(writeOrdinals, selected.skeleton)
			writeOrdinals = append(writeOrdinals, selected.scalars...)
		}
	}
	if mayContradict {
		writeOrdinals = append(writeOrdinals, 0)
	}
	readOrdinals, err = sealFormalPresenceOrdinals(span, readOrdinals)
	if err != nil {
		return nil, err
	}
	writeOrdinals, err = sealFormalPresenceOrdinals(span, writeOrdinals)
	if err != nil {
		return nil, err
	}
	projectionOrdinals := append(append([]formalFiberOrdinal(nil), readOrdinals...), writeOrdinals...)
	projectionOrdinals, err = sealFormalPresenceOrdinals(span, projectionOrdinals)
	if err != nil {
		return nil, err
	}
	if len(projectionOrdinals) != 0 && projectionOrdinals[0] == 0 {
		projectionOrdinals = projectionOrdinals[1:]
	}
	valueRoots := make([]formalPresenceValueRoot, 0, len(valueRootOrdinals))
	for root, ordinal := range valueRootOrdinals {
		valueRoots = append(valueRoots, formalPresenceValueRoot{root: root, ordinal: ordinal})
	}
	sort.Slice(valueRoots, func(i, j int) bool { return valueRoots[i].ordinal < valueRoots[j].ordinal })
	pathSlots := make([]formalPresenceCoordinateSlot, 0, len(pathSlotOrdinals))
	for ordinal, slot := range pathSlotOrdinals {
		pathSlots = append(pathSlots, formalPresenceCoordinateSlot{slot: slot, ordinal: ordinal})
	}
	sort.Slice(pathSlots, func(i, j int) bool { return pathSlots[i].ordinal < pathSlots[j].ordinal })
	return &formalPresenceImplicationStep{
		plan: dependency, roots: binding, values: values.descriptor, path: path.descriptor,
		positions: positions, valuesTop: valuesTopOrdinal, valueRoots: valueRoots, pathFamily: pathFamily, pathSlots: pathSlots,
		mutationLanes: mutationLanes, mutationFamilies: mutationFamilies,
		readOrdinals: readOrdinals, writeOrdinals: writeOrdinals, projectionOrdinals: projectionOrdinals,
		demands: demands, variable: body.variable,
	}, nil
}

type formalPresenceLeafWrite struct {
	ordinal formalFiberOrdinal
	leaf    decisionLeaf
}

func (a *formalTupleAlgebra) materializeFormalCoordinateFamily(
	view formalSparseLeafView,
	family formalCoordinateFamilyFiberGroup,
	selected []formalPresenceCoordinateSlot,
) (state.CoordinateFamilyFactor, error) {
	leaf, present := view.leaf(family.skeleton)
	if !present {
		return state.CoordinateFamilyFactor{}, errFormalComponentMalformed
	}
	var skeleton state.CoordinateFamilySkeleton
	var err error
	if leaf == 0 {
		skeleton, err = view.authority.product.CoordinateSkeletonBottom(family.family, view.span.keys)
	} else {
		terminal, terminalErr := view.authority.terminal(leaf)
		if terminalErr != nil || terminal.kind != formalComponentCoordinateSkeleton || !coordinateFamilySame(terminal.skeleton.Family(), family.family) {
			return state.CoordinateFamilyFactor{}, errFormalComponentMalformed
		}
		skeleton = terminal.skeleton
	}
	if err != nil {
		return state.CoordinateFamilyFactor{}, err
	}
	bindings := selected
	if bindings == nil {
		bindings = make([]formalPresenceCoordinateSlot, len(family.scalars))
		for index, ordinal := range family.scalars {
			bindings[index] = formalPresenceCoordinateSlot{
				slot: view.span.forest.descriptors[view.span.first+int(ordinal)].coordinate, ordinal: ordinal,
			}
		}
	}
	scalars := make([]state.CoordinateScalarFactor, 0, len(bindings))
	for _, binding := range bindings {
		if !coordinateFamilySame(binding.slot.Family(), family.family) {
			continue
		}
		leaf, present = view.leaf(binding.ordinal)
		if !present {
			return state.CoordinateFamilyFactor{}, errFormalComponentMalformed
		}
		if leaf == 0 {
			continue
		}
		terminal, terminalErr := view.authority.terminal(leaf)
		if terminalErr != nil || terminal.kind != formalComponentCoordinateScalar {
			return state.CoordinateFamilyFactor{}, errFormalComponentMalformed
		}
		equal, equalErr := view.authority.product.CoordinateSlotEqual(terminal.scalar.Slot(), binding.slot)
		if equalErr != nil || !equal {
			return state.CoordinateFamilyFactor{}, errFormalComponentMalformed
		}
		scalars = append(scalars, terminal.scalar)
	}
	return view.authority.product.SealCoordinateFamilyFactor(skeleton, scalars)
}

func (a *formalTupleAlgebra) factorFormalCoordinateFamily(
	authority *formalComponentTerminalAuthority,
	span formalFiberDescriptorSpan,
	family formalCoordinateFamilyFiberGroup,
	factor state.CoordinateFamilyFactor,
) ([]formalPresenceLeafWrite, error) {
	if authority == nil || !coordinateFamilySame(factor.Family(), family.family) {
		return nil, errFormalComponentMalformed
	}
	out := make([]formalPresenceLeafWrite, 0, len(family.scalars)+1)
	bottom, err := authority.product.CoordinateSkeletonBottom(family.family, span.keys)
	if err != nil {
		return nil, err
	}
	sameBottom, err := authority.product.CoordinateSkeletonRepresentationEqual(factor.Skeleton(), bottom)
	if err != nil {
		return nil, err
	}
	skeletonLeaf := decisionLeaf(0)
	if !sameBottom {
		skeletonLeaf, err = authority.internCoordinateSkeleton(factor.Skeleton())
		if err != nil {
			return nil, err
		}
	}
	out = append(out, formalPresenceLeafWrite{ordinal: family.skeleton, leaf: skeletonLeaf})
	scalars := factor.Scalars()
	scalarIndex := 0
	for _, ordinal := range family.scalars {
		descriptor := span.forest.descriptors[span.first+int(ordinal)]
		leaf := decisionLeaf(0)
		if scalarIndex < len(scalars) {
			equal, equalErr := authority.product.CoordinateSlotEqual(descriptor.coordinate, scalars[scalarIndex].Slot())
			if equalErr != nil {
				return nil, equalErr
			}
			if equal {
				omitted, omitErr := authority.product.CoordinateScalarIsOmitted(factor.Skeleton(), scalars[scalarIndex])
				if omitErr != nil {
					return nil, omitErr
				}
				if !omitted {
					leaf, err = authority.internCoordinateScalar(scalars[scalarIndex])
					if err != nil {
						return nil, err
					}
				}
				scalarIndex++
			}
		}
		out = append(out, formalPresenceLeafWrite{ordinal: ordinal, leaf: leaf})
	}
	if scalarIndex != len(scalars) {
		return nil, errFormalComponentMalformed
	}
	return out, nil
}

func (a *formalTupleAlgebra) applyFormalPresenceImplications(operator formalRelationOperatorRef, predecessor formalRelationTuple) (formalRelationTuple, error) {
	plan := operator.presenceImplications
	if a == nil || plan == nil || plan.variable != predecessor.variable || operator.kind != formalRelationCellStep || operator.code == nil {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal N2 is unbound")
	}
	if err := a.validateTuple(predecessor); err != nil || predecessor.bottom() {
		return predecessor, err
	}
	span, directory, authority, ok := a.span(predecessor.variable)
	if !ok || predecessor.root.owner != directory || authority.code != operator.code {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	regions, err := a.partitionSparseLeafViewsUnderCare([]formalSparseTupleProjection{{
		tuple: predecessor, ordinals: plan.projectionOrdinals,
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
	guardDecision := decisionTrue
	if step.guard != 0 {
		guardDecision, err = a.decisionForGuard(predecessor.variable, operator.scope, operator.code.terms, step.guard)
		if err != nil {
			return fail(err)
		}
	}
	notGuard, err := formalDecisionBooleanNot(a, guardDecision)
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
	publish := func(index int, care decisionRef, leaf decisionLeaf) error {
		if care == decisionFalse {
			return nil
		}
		var publishErr error
		affected[index].root, publishErr = a.decisions.condition(a.ctx, care, a.decisions.terminal(leaf), affected[index].root)
		return publishErr
	}
	for _, region := range regions {
		if len(region.views) != 1 {
			return fail(errDecisionMalformed)
		}
		view := region.views[0]
		trueCare, careErr := a.decisions.apply(a.ctx, uint8(decisionAnd), true, region.guard, guardDecision, decisionLeafAnd)
		if careErr != nil {
			return fail(careErr)
		}
		falseCare, careErr := a.decisions.apply(a.ctx, uint8(decisionAnd), true, region.guard, notGuard, decisionLeafAnd)
		if careErr != nil {
			return fail(careErr)
		}
		if trueCare != decisionFalse {
			// N2 owns a concrete coordinate-path-evidence transaction.  Its
			// Values inputs may still be entry-dependent, so preserve the sealed
			// formal image until entry substitution supplies concrete operands
			// instead of materializing a symbolic leaf as product.Value here.
			symbolic, symbolicErr := view.symbolicValuesIn(plan.projectionOrdinals)
			if symbolicErr != nil {
				return fail(symbolicErr)
			}
			if symbolic {
				for index, ordinal := range plan.writeOrdinals {
					leaf := decisionLeaf(1)
					if ordinal != 0 {
						var present bool
						leaf, present = view.leaf(ordinal)
						if !present {
							return fail(errFormalComponentMalformed)
						}
					}
					if publishErr := publish(index, trueCare, leaf); publishErr != nil {
						return fail(publishErr)
					}
				}
			} else {
				leaves, reachable, leafErr := a.applyFormalPresenceImplicationsLeaf(view, plan)
				if leafErr != nil {
					return fail(leafErr)
				}
				if reachable {
					for index, leaf := range leaves {
						if leafErr = publish(index, trueCare, leaf); leafErr != nil {
							return fail(leafErr)
						}
					}
				}
			}
		}
		for index, ordinal := range plan.writeOrdinals {
			leaf := decisionLeaf(1)
			if ordinal != 0 {
				var present bool
				leaf, present = view.leaf(ordinal)
				if !present {
					return fail(errFormalComponentMalformed)
				}
			}
			if careErr = publish(index, falseCare, leaf); careErr != nil {
				return fail(careErr)
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

func (a *formalTupleAlgebra) applyFormalPresenceImplicationsLeaf(
	view formalSparseLeafView,
	plan *formalPresenceImplicationStep,
) ([]decisionLeaf, bool, error) {
	if plan == nil || view.algebra != a || view.authority == nil || plan.variable != view.variable {
		return nil, false, errFormalComponentForeignOwner
	}
	if err := a.ctx.Err(); err != nil {
		return nil, false, err
	}
	domain, span := view.authority.product, view.span
	topLeaf, present := view.leaf(plan.valuesTop)
	if !present || topLeaf > 1 {
		return nil, false, errFormalComponentMalformed
	}
	values := state.ValueFactor[FormalSlot]{Top: topLeaf == 1}
	if !values.Top {
		values.Values = make(map[FormalSlot]product.Value, len(plan.valueRoots))
		for _, binding := range plan.valueRoots {
			leaf, found := view.leaf(binding.ordinal)
			if !found {
				return nil, false, errFormalComponentMalformed
			}
			if leaf == 0 {
				continue
			}
			terminal, terminalErr := view.authority.terminal(leaf)
			if terminalErr != nil || terminal.kind != formalComponentGroundValue {
				return nil, false, errFormalComponentMalformed
			}
			values.Values[binding.root] = terminal.ground
		}
	}
	pathFactor, err := a.materializeFormalCoordinateFamily(view, plan.pathFamily, plan.pathSlots)
	if err != nil {
		return nil, false, err
	}
	mutationLanes := make([]state.LaneFactor, len(plan.mutationLanes))
	for index, group := range plan.mutationLanes {
		mutationLanes[index], err = view.laneFactor(group)
		if err != nil {
			return nil, false, err
		}
	}
	mutationCoordinates := make([]state.CoordinateFamilyFactor, len(plan.mutationFamilies))
	for index, family := range plan.mutationFamilies {
		mutationCoordinates[index], err = a.materializeFormalCoordinateFamily(view, family, nil)
		if err != nil {
			return nil, false, err
		}
	}
	var mutation state.PathDescendantMutationFactors
	if len(mutationLanes) != 0 || len(mutationCoordinates) != 0 {
		mutation, err = domain.SealPathDescendantMutationFactors(mutationLanes, mutationCoordinates)
		if err != nil {
			return nil, false, err
		}
	}
	open := func(reads, writes []FormalSlot, authority state.CoordinatePathEvidenceAuthority[FormalSlot], pathMutation bool) (*state.CoordinatePathEvidenceCarrier[FormalSlot], error) {
		selected := state.ValueFactor[FormalSlot]{Top: values.Top}
		if !selected.Top {
			selected.Values = make(map[FormalSlot]product.Value, len(reads)+len(writes))
			for _, root := range append(append([]FormalSlot(nil), reads...), writes...) {
				if value, present := values.Values[root]; present {
					selected.Values[root] = value
				}
			}
		}
		activeMutation := state.PathDescendantMutationFactors{}
		if pathMutation {
			activeMutation = mutation
		}
		return state.OpenCoordinatePathEvidenceCarrier(
			domain, pathFactor.Skeleton(), pathFactor.Scalars(), selected, true,
			authority, activeMutation,
		)
	}
	freeze := func(carrier *state.CoordinatePathEvidenceCarrier[FormalSlot], valueWrites []FormalSlot, pathMutation bool) (bool, error) {
		skeleton, scalars, selected, invalidation, coordinateInvalidation, reachable, freezeErr := carrier.Freeze()
		if freezeErr != nil {
			return false, freezeErr
		}
		if !reachable {
			return false, nil
		}
		pathFactor, freezeErr = domain.SealCoordinateFamilyFactor(skeleton, scalars)
		if freezeErr != nil {
			return false, freezeErr
		}
		if !values.Top {
			bottom := product.Bottom(domain.Registry())
			for _, root := range valueWrites {
				value := bottom
				if current, present := selected.Values[root]; present {
					value = current
				}
				if product.Equal(domain.Registry(), value, bottom) {
					delete(values.Values, root)
				} else {
					values.Values[root] = value
				}
			}
		}
		if pathMutation {
			mutation, freezeErr = domain.SealPathDescendantMutationFactors(invalidation, coordinateInvalidation)
			if freezeErr != nil {
				return false, freezeErr
			}
		}
		return true, nil
	}
	reachable := true
stageLoop:
	for _, stage := range plan.plan.Stages() {
		if err := a.ctx.Err(); err != nil {
			return nil, false, err
		}
		stageAuthority, authorityOK := plan.roots.StageAuthority(stage)
		if !authorityOK {
			return nil, false, fmt.Errorf("transformer: formal N2 stage authority is incomplete")
		}
		carrier, openErr := open(nil, nil, stageAuthority, false)
		if openErr != nil {
			return nil, false, openErr
		}
		if err := factapply.ApplyPresenceImplicationCoordinateReducer(plan.plan, carrier, stage, stageAuthority); err != nil {
			return nil, false, err
		}
		if stillReachable, freezeErr := freeze(carrier, nil, false); freezeErr != nil {
			return nil, false, freezeErr
		} else if !stillReachable {
			reachable = false
			break stageLoop
		}
		for _, block := range stage.Blocks() {
			reads, readOK := plan.roots.BlockRoots(block.ValueReadDependencies())
			writes, writeOK := plan.roots.BlockRoots(block.ValueWriteDependencies())
			if !readOK || !writeOK {
				return nil, false, fmt.Errorf("transformer: formal N2 root binding is incomplete")
			}
			blockAuthority, authorityOK := plan.roots.BlockAuthority(block)
			if !authorityOK {
				return nil, false, fmt.Errorf("transformer: formal N2 block authority is incomplete")
			}
			carrier, openErr = open(reads, writes, blockAuthority, block.PathMutation())
			if openErr != nil {
				return nil, false, openErr
			}
			feasible, applyErr := factapply.ApplyPresenceImplicationCoordinateBlock(plan.plan, a.ctx, carrier, block, plan.roots)
			if applyErr != nil {
				return nil, false, applyErr
			}
			if !feasible {
				reachable = false
				break stageLoop
			}
			if stillReachable, freezeErr := freeze(carrier, writes, block.PathMutation()); freezeErr != nil {
				return nil, false, freezeErr
			} else if !stillReachable {
				reachable = false
				break stageLoop
			}
		}
	}
	if !reachable {
		return nil, false, nil
	}
	out := make([]decisionLeaf, len(plan.writeOrdinals))
	for index, ordinal := range plan.writeOrdinals {
		if ordinal == 0 {
			out[index] = 1
			continue
		}
		leaf, found := view.leaf(ordinal)
		if !found {
			return nil, false, errFormalComponentMalformed
		}
		out[index] = leaf
	}
	write := func(ordinal formalFiberOrdinal, leaf decisionLeaf) error {
		index := sort.Search(len(plan.writeOrdinals), func(i int) bool { return plan.writeOrdinals[i] >= ordinal })
		if index >= len(plan.writeOrdinals) || plan.writeOrdinals[index] != ordinal {
			return nil
		}
		out[index] = leaf
		return nil
	}
	if !values.Top {
		bottom := product.Bottom(domain.Registry())
		for _, binding := range plan.valueRoots {
			if sort.Search(len(plan.writeOrdinals), func(i int) bool { return plan.writeOrdinals[i] >= binding.ordinal }) >= len(plan.writeOrdinals) {
				continue
			}
			value := bottom
			if current, found := values.Values[binding.root]; found {
				value = current
			}
			leaf := decisionLeaf(0)
			if !product.Equal(domain.Registry(), value, bottom) {
				leaf, err = view.authority.internGroundValue(value)
				if err != nil {
					return nil, false, err
				}
			}
			_ = write(binding.ordinal, leaf)
		}
	}
	basePath, err := view.laneFactor(plan.path)
	if err != nil {
		return nil, false, err
	}
	pathScalars := pathFactor.Scalars()
	pathWrites := make([]state.CoordinateScalarFactor, 0, len(plan.pathSlots))
	for _, binding := range plan.pathSlots {
		position := sort.Search(len(plan.writeOrdinals), func(i int) bool { return plan.writeOrdinals[i] >= binding.ordinal })
		if position >= len(plan.writeOrdinals) || plan.writeOrdinals[position] != binding.ordinal {
			continue
		}
		var scalar state.CoordinateScalarFactor
		found := false
		for _, candidate := range pathScalars {
			equal, equalErr := domain.CoordinateSlotEqual(candidate.Slot(), binding.slot)
			if equalErr != nil {
				return nil, false, equalErr
			}
			if equal {
				scalar, found = candidate, true
				break
			}
		}
		if !found {
			scalar, err = domain.CoordinateDefault(pathFactor.Skeleton(), binding.slot)
			if err != nil {
				return nil, false, err
			}
		}
		pathWrites = append(pathWrites, scalar)
	}
	pathLane, err := domain.PatchCoordinateFamily(basePath, pathFactor.Skeleton(), pathWrites)
	if err != nil {
		return nil, false, err
	}
	pathLeaves, err := a.factorCoordinateGroup(view.authority, span, plan.path, pathLane)
	if err != nil {
		return nil, false, err
	}
	for index, ordinal := range plan.path.members {
		_ = write(ordinal, pathLeaves[index])
	}
	if len(plan.mutationLanes) != len(mutation.LaneFactors()) || len(plan.mutationFamilies) != len(mutation.CoordinateFactors()) {
		return nil, false, errFormalComponentMalformed
	}
	for index, group := range plan.mutationLanes {
		leaves, factorErr := a.factorFormalSparseLane(view.authority, span, group, mutation.LaneFactors()[index])
		if factorErr != nil {
			return nil, false, factorErr
		}
		for memberIndex, ordinal := range group.members {
			_ = write(ordinal, leaves[memberIndex])
		}
	}
	for index, family := range plan.mutationFamilies {
		leaves, factorErr := a.factorFormalCoordinateFamily(view.authority, span, family, mutation.CoordinateFactors()[index])
		if factorErr != nil {
			return nil, false, factorErr
		}
		for _, item := range leaves {
			_ = write(item.ordinal, item.leaf)
		}
	}
	return out, true, nil
}
