package transformer

import (
	"fmt"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// formalOperatorCoordinateFootprint is the one immutable scalar-coordinate
// declaration owned by a sealed relation operator. The body fiber schema and
// the WTO executable binding retain this same value; neither is permitted to
// rediscover a coordinate from solve-time factors.
type formalOperatorCoordinateFootprint struct {
	owner     *formalOperatorCoordinateFootprints
	cell      formalRelationCell
	inventory state.CoordinateFactorInventory
	// source is present only for Apply. It is the exact callee-coordinate cone
	// selected by backward reachability from normal terminals; execution and
	// destination footprint preparation consume this same immutable selector.
	source state.CoordinateFactorInventory
	// sourceSelector is the forest-owned, exact catalog reference for source.
	// It is assigned once after the coordinate dependency closure stabilizes;
	// execution keys never use a lossy inventory hash as semantic identity.
	sourceSelector formalApplyCoordinateSelectorRef
}

func (f formalOperatorCoordinateFootprint) validFor(body *relationProgramBody, cell formalRelationCell) bool {
	return body != nil && f.owner != nil && cell.valid() && f.cell == cell &&
		f.inventory.ValidFor(body.productDomain, f.inventory.KeySpace())
}

type formalOperatorCoordinateFootprints struct {
	byCell map[formalRelationCell]formalOperatorCoordinateFootprint
}

type formalDefinitionResourceTopology struct {
	definitions []formalRelationDefinition
	resources   []formalRelationResource
}

// freezeFormalDefinitionResourceTopology is the sole ordinal assignment for
// synthetic definition/resource cells. Region sealing and pre-schema
// coordinate declaration consume this same sorted topology.
func freezeFormalDefinitionResourceTopology(program *RelationProgram) (formalDefinitionResourceTopology, error) {
	if program == nil {
		return formalDefinitionResourceTopology{}, fmt.Errorf("transformer: formal definition/resource topology is unowned")
	}
	definitions := append([]relationProgramDefinition(nil), program.definitions...)
	sort.SliceStable(definitions, func(left, right int) bool {
		a, b := definitions[left], definitions[right]
		if a.owner != b.owner {
			return a.owner < b.owner
		}
		if a.point != b.point {
			return a.point < b.point
		}
		if a.frame != b.frame {
			return a.frame < b.frame
		}
		if a.target != b.target {
			return a.target < b.target
		}
		return !a.externallyReachable && b.externallyReachable
	})
	out := formalDefinitionResourceTopology{definitions: make([]formalRelationDefinition, len(definitions)+1)}
	resourceMembers := make(map[relationVar][]formalRelationDefinitionRef)
	for index, definition := range definitions {
		if definition.owner == 0 || int(definition.owner) > len(program.bodies) || definition.target == 0 || int(definition.target) > len(program.bodies) ||
			definition.owner == definition.target || definition.point == 0 || definition.frame == 0 {
			return formalDefinitionResourceTopology{}, fmt.Errorf("transformer: formal relation definition inventory is malformed")
		}
		ref := formalRelationDefinitionRef(index + 1)
		cell := formalRelationCell{Variable: definition.owner, Definition: ref, Kind: formalRelationCellDefinition}
		out.definitions[ref] = formalRelationDefinition{
			owner: definition.owner, target: definition.target, point: definition.point,
			frame: definition.frame, external: definition.externallyReachable, cell: cell,
		}
		if definition.externallyReachable {
			resourceMembers[definition.owner] = append(resourceMembers[definition.owner], ref)
		}
	}
	owners := make([]relationVar, 0, len(resourceMembers))
	for owner := range resourceMembers {
		owners = append(owners, owner)
	}
	sort.Slice(owners, func(left, right int) bool { return owners[left] < owners[right] })
	out.resources = make([]formalRelationResource, len(owners)+1)
	for index, owner := range owners {
		ref := formalRelationResourceRef(index + 1)
		cell := formalRelationCell{Variable: owner, Resource: ref, Kind: formalRelationCellResource}
		out.resources[ref] = formalRelationResource{owner: owner, members: append([]formalRelationDefinitionRef(nil), resourceMembers[owner]...), cell: cell}
	}
	return out, nil
}

func newFormalOperatorCoordinateFootprints() *formalOperatorCoordinateFootprints {
	return &formalOperatorCoordinateFootprints{byCell: make(map[formalRelationCell]formalOperatorCoordinateFootprint)}
}

func (f *formalOperatorCoordinateFootprints) declare(
	body *relationProgramBody,
	cell formalRelationCell,
	inventory state.CoordinateFactorInventory,
) error {
	if f == nil || f.byCell == nil || body == nil || cell.Variable != body.variable ||
		!inventory.ValidFor(body.productDomain, inventory.KeySpace()) {
		return fmt.Errorf("transformer: formal operator coordinate footprint is unowned")
	}
	declaration := formalOperatorCoordinateFootprint{owner: f, cell: cell, inventory: inventory}
	if prior, exists := f.byCell[cell]; exists {
		if prior.inventory.KeySpace() != inventory.KeySpace() || prior.inventory.Len() != inventory.Len() {
			return fmt.Errorf("transformer: formal operator has conflicting coordinate footprints")
		}
		for index, slot := range prior.inventory.Slots() {
			equal, err := body.productDomain.CoordinateSlotEqual(slot, inventory.Slots()[index])
			if err != nil || !equal {
				return fmt.Errorf("transformer: formal operator has conflicting coordinate footprints")
			}
		}
		return nil
	}
	f.byCell[cell] = declaration
	return nil
}

func (f *formalOperatorCoordinateFootprints) lookup(
	body *relationProgramBody,
	cell formalRelationCell,
) (formalOperatorCoordinateFootprint, bool) {
	if f == nil || body == nil {
		return formalOperatorCoordinateFootprint{}, false
	}
	value, ok := f.byCell[cell]
	return value, ok && value.validFor(body, cell)
}

func (f *formalOperatorCoordinateFootprints) bind(program *RelationProgram, cell formalRelationCell) (formalOperatorCoordinateFootprint, error) {
	if f == nil || program == nil || program.formalFibers == nil || cell.Variable == 0 || int(cell.Variable) > len(program.bodies) {
		return formalOperatorCoordinateFootprint{}, fmt.Errorf("transformer: formal operator has no coordinate footprint owner")
	}
	body := &program.bodies[cell.Variable-1]
	if footprint, ok := f.lookup(body, cell); ok {
		return footprint, nil
	}
	return formalOperatorCoordinateFootprint{}, fmt.Errorf("transformer: formal operator is outside the coordinate footprint declaration")
}

type formalFrameFootprintKey struct {
	variable relationVar
	frame    callFrameTerm
}

// freezeFormalFrameFootprintCells binds each sealed call frame once to the
// exact operator cells that execute its boundary image. Ordinary calls map to
// Apply steps; closure-analysis frames map to their Definition and, when
// externally reachable, Resource cell. The Apply closure performs no syntax
// scan while iterating.
func freezeFormalFrameFootprintCells(forest *formalFiberInventory) map[formalFrameFootprintKey][]formalRelationCell {
	out := make(map[formalFrameFootprintKey][]formalRelationCell)
	if forest == nil || forest.program == nil {
		return out
	}
	appendCell := func(key formalFrameFootprintKey, cell formalRelationCell) {
		for _, prior := range out[key] {
			if prior == cell {
				return
			}
		}
		out[key] = append(out[key], cell)
	}
	for bodyIndex := range forest.program.bodies {
		body := &forest.program.bodies[bodyIndex]
		if body.relation.code == nil {
			continue
		}
		for root := relationRootRef(1); int(root) < len(body.relation.code.nodes); root++ {
			for stepIndex, step := range body.relation.code.nodes[root].steps {
				if step.kind == boundaryStepApply && step.apply.frame != 0 {
					appendCell(formalFrameFootprintKey{variable: body.variable, frame: step.apply.frame}, formalRelationCell{
						Variable: body.variable, Root: root, Step: uint32(stepIndex + 1), Kind: formalRelationCellStep,
					})
				}
			}
		}
	}
	for ref := formalRelationDefinitionRef(1); int(ref) < len(forest.operatorTopology.definitions); ref++ {
		definition := forest.operatorTopology.definitions[ref]
		key := formalFrameFootprintKey{variable: definition.owner, frame: definition.frame}
		appendCell(key, definition.cell)
		if !definition.external {
			continue
		}
		for resourceRef := formalRelationResourceRef(1); int(resourceRef) < len(forest.operatorTopology.resources); resourceRef++ {
			resource := forest.operatorTopology.resources[resourceRef]
			if resource.owner == definition.owner && formalRelationResourceContains(resource, ref) {
				appendCell(key, resource.cell)
				break
			}
		}
	}
	return out
}

func formalCoordinateInventoriesEqual(domain state.ProductDomain, left, right state.CoordinateFactorInventory) (bool, error) {
	return domain.CoordinateFactorInventoriesEqual(left, right)
}

func freezeFormalStepCoordinateFootprint(
	forest *formalFiberInventory,
	body *relationProgramBody,
	variable relationVar,
	formalKeys *keyspace.KeySpace,
	rekey state.CoordinateFormalRootRekey,
	pointwise relationCoordinateFactorInventory,
	current state.CoordinateFactorInventory,
	normalReturnIdentities formalIdentitySupport,
	step boundaryStep,
) (state.CoordinateFactorInventory, error) {
	var slots []state.CoordinateSlot
	switch step.kind {
	case boundaryStepBranchRelations:
		atPoint, err := pointwise.At(step.branch.Point())
		if err != nil {
			return state.CoordinateFactorInventory{}, err
		}
		pointInventory, err := rekeyFormalCoordinateFactorInventory(body.productDomain, formalKeys, rekey, atPoint)
		if err != nil {
			return state.CoordinateFactorInventory{}, err
		}
		factors, err := body.pathSemantics.PrepareFormalBranchRelationFactors(body.productDomain, step.branch, pointInventory, rekey, formalKeys)
		if err != nil {
			return state.CoordinateFactorInventory{}, err
		}
		for index := 0; index < factors.Len(); index++ {
			factor, ok := factors.Factor(index)
			if !ok {
				return state.CoordinateFactorInventory{}, fmt.Errorf("formal BranchRelations factor %d is absent", index)
			}
			slots = append(slots, factor.CoordinateReads()...)
			slots = append(slots, factor.CoordinateWrites()...)
			slots = append(slots, factor.OriginalCoordinateReads()...)
		}
	case boundaryStepPresenceImplications:
		plan, err := body.pathSemantics.PreparePathValuePresenceImplications(body.productDomain.Registry(), step.presence)
		if err != nil {
			return state.CoordinateFactorInventory{}, err
		}
		plan, err = plan.RekeyFormal(body.productDomain, rekey)
		if err != nil {
			return state.CoordinateFactorInventory{}, err
		}
		dependency, err := plan.DependencyBlocks(body.productDomain, current)
		if err != nil {
			return state.CoordinateFactorInventory{}, err
		}
		slots = append(slots, dependency.Slots()...)
	case boundaryStepExternalCall:
		if site, ok := forest.externalCallSite(variable, step.point); ok {
			slots = append(slots, site.correlation.CoordinateSlots()...)
		}
		formalAuthority, authorityErr := body.pathSemantics.ProjectFormal(body.productDomain, rekey, formalKeys)
		if authorityErr != nil {
			return state.CoordinateFactorInventory{}, authorityErr
		}
		callSite, present := body.plan.Facts().CallSiteView(step.point)
		if !present {
			return state.CoordinateFactorInventory{}, fmt.Errorf("formal ExternalCall point %d has no call-site payload", step.point)
		}
		bindings, bindingErr := formalAuthority.CallBoundaryPathBindings(body.plan.Facts(), callSite)
		if bindingErr != nil {
			return state.CoordinateFactorInventory{}, bindingErr
		}
		normalLanes := state.NewLaneSet()
		if prepared, present := forest.externalCallSite(variable, step.point); present {
			normalLanes, authorityErr = factapply.ExternalCallTransactionLanes(body.productDomain, prepared.capability)
			if authorityErr != nil {
				return state.CoordinateFactorInventory{}, authorityErr
			}
		}
		normalCarrier, carrierErr := body.productDomain.SealCoordinateFactorInventory(formalKeys,
			append(current.Slots(), forest.externalCallCoordinateSlots(variable)...),
		)
		if carrierErr != nil {
			return state.CoordinateFactorInventory{}, carrierErr
		}
		normalSlots, normalErr := formalAuthority.NormalReturnCoordinateFootprint(body.productDomain, step.point, bindings, normalLanes, normalReturnIdentities, normalCarrier)
		if normalErr != nil {
			return state.CoordinateFactorInventory{}, normalErr
		}
		slots = append(slots, normalSlots...)
	case boundaryStepRootAssignment:
		more, err := freezeFormalRootAssignmentCoordinateSlots(body, formalKeys, rekey, current, step)
		if err != nil {
			return state.CoordinateFactorInventory{}, err
		}
		slots = append(slots, more...)
	case boundaryStepCovariantExposure:
		more, err := freezeFormalCovariantCoordinateSlots(body, formalKeys, rekey, current, step.covariant)
		if err != nil {
			return state.CoordinateFactorInventory{}, err
		}
		slots = append(slots, more...)
	case boundaryStepEffect:
		more, err := freezeFormalEffectCoordinateSlots(body, formalKeys, rekey, current, step)
		if err != nil {
			return state.CoordinateFactorInventory{}, err
		}
		slots = append(slots, more...)
	}
	owned, err := body.productDomain.SealCoordinateFactorInventory(formalKeys, slots)
	if err != nil {
		return state.CoordinateFactorInventory{}, err
	}
	return body.productDomain.CloseCoordinateFactorInventory(formalKeys, owned)
}

func freezeFormalEffectCoordinateSlots(body *relationProgramBody, formalKeys *keyspace.KeySpace, rekey state.CoordinateFormalRootRekey, current state.CoordinateFactorInventory, step boundaryStep) ([]state.CoordinateSlot, error) {
	if step.effect == 0 || body.relation.effects == nil || int(step.effect) >= len(body.relation.effects.nodes) {
		return nil, nil
	}
	effect := body.relation.effects.nodes[step.effect]
	switch effect.kind {
	case EffectPathStore:
		var slots []state.CoordinateSlot
		if effect.pathStoreHasAssignment {
			target, err := freezeFormalEffectPathKey(body, formalFiberDescriptorSpan{keys: formalKeys, rekey: rekey}, effect.pathStoreAssignment.Target)
			if err != nil {
				return nil, err
			}
			root, exact := formalKeys.StructuralRoot(target)
			owner, ownerExact := body.productDomain.PathValueFamily()
			if !exact || !ownerExact {
				return nil, fmt.Errorf("formal path replacement has no path dependency owner")
			}
			union, err := current.FamilySlots(owner)
			if err != nil {
				return nil, err
			}
			const dependencyID state.CoordinateDependencyID = 1
			dependencies, err := body.productDomain.PlanPathCoordinateDependencies(formalKeys, union, []state.CoordinateDependencySeed{{
				ID: dependencyID, ResolvePaths: []keyspace.Key{target}, WritePaths: []keyspace.Key{target}, DescendantMutationRoots: []keyspace.Key{root},
			}})
			if err != nil {
				return nil, err
			}
			dependency, present := dependencies.Dependency(dependencyID)
			if !present {
				return nil, fmt.Errorf("formal path replacement has no coordinate dependency certificate")
			}
			slots = append(slots, dependency.CoordinateReads()...)
			slots = append(slots, dependency.CoordinateWrites()...)
			for _, family := range body.productDomain.PathDescendantMutationCoordinateFamilies() {
				familySlots, familyErr := current.FamilySlots(family)
				if familyErr != nil {
					return nil, familyErr
				}
				selected, selectErr := body.productDomain.SelectPathMutationCoordinateSlots(family, familySlots, dependency.MutationRegions())
				if selectErr != nil {
					return nil, selectErr
				}
				slots = append(slots, selected...)
			}
		}
		if effect.pathStoreHasStatic {
			target, err := freezeFormalEffectPathKey(body, formalFiberDescriptorSpan{keys: formalKeys, rekey: rekey}, effect.pathStoreStatic.Target)
			if err != nil {
				return nil, err
			}
			plan, err := body.productDomain.PrepareStaticMemberFactorPlan(formalKeys, target, product.Bottom(body.productDomain.Registry()))
			if err != nil {
				return nil, err
			}
			writes, err := body.productDomain.StaticMemberFactorCoordinateWrites(plan)
			if err != nil {
				return nil, err
			}
			slots = append(slots, writes...)
		}
		if len(effect.pathStoreObject.Heaps) != 0 {
			templates, err := formalObjectMaterializationTemplates(body.relation, step.effect)
			if err != nil {
				return nil, err
			}
			if len(templates) != len(effect.pathStoreObject.Heaps) {
				return nil, fmt.Errorf("formal PathStore object allocation schema is incomplete")
			}
			shapes := make([]state.ObjectConstructorShape, len(effect.pathStoreObject.Heaps))
			for index, object := range effect.pathStoreObject.Heaps {
				shapes[index] = state.ObjectConstructorShape{Identity: identity.AllocationTerm(templates[index]), StableShape: object.StableShape}
				for _, member := range object.Members {
					shapes[index].MemberSuffixes = append(shapes[index].MemberSuffixes, member.Suffix)
				}
			}
			plan, err := body.productDomain.PrepareObjectConstructorPlan(body.keys, shapes)
			if err != nil {
				return nil, err
			}
			writes, err := body.productDomain.ObjectConstructorCoordinateWrites(plan)
			if err != nil {
				return nil, err
			}
			concrete, err := body.productDomain.SealCoordinateFactorInventory(body.keys, writes)
			if err != nil {
				return nil, err
			}
			formal, err := rekeyFormalCoordinateFactorInventory(body.productDomain, formalKeys, rekey, concrete)
			if err != nil {
				return nil, err
			}
			slots = append(slots, formal.Slots()...)
		}
		return slots, nil
	case EffectIndexMutation:
		// Dynamic keys remain in the canonical dynamic-index/object carriers.
		// Runtime singleton values never manufacture static coordinates.
		return nil, nil
	case EffectAllocationTemplate:
		graph, err := freezeFormalAllocationGraph(body, body.keys, effect.allocation)
		if err != nil {
			return nil, err
		}
		slots, err := body.productDomain.ObjectGraphMutationCoordinateWrites(graph)
		if err != nil {
			return nil, err
		}
		concrete, err := body.productDomain.SealCoordinateFactorInventory(body.keys, slots)
		if err != nil {
			return nil, err
		}
		formal, err := rekeyFormalCoordinateFactorInventory(body.productDomain, formalKeys, rekey, concrete)
		if err != nil {
			return nil, err
		}
		return formal.Slots(), nil
	case EffectObjectMaterialization:
		templates, err := formalObjectMaterializationTemplates(body.relation, step.effect)
		if err != nil {
			return nil, err
		}
		if len(templates) != len(effect.pathStoreObject.Heaps) {
			return nil, fmt.Errorf("formal ObjectMaterialization allocation schema is incomplete")
		}
		shapes := make([]state.ObjectConstructorShape, len(effect.pathStoreObject.Heaps))
		for index, object := range effect.pathStoreObject.Heaps {
			shapes[index] = state.ObjectConstructorShape{Identity: identity.AllocationTerm(templates[index]), StableShape: object.StableShape}
			for _, member := range object.Members {
				shapes[index].MemberSuffixes = append(shapes[index].MemberSuffixes, member.Suffix)
			}
		}
		plan, err := body.productDomain.PrepareObjectConstructorPlan(body.keys, shapes)
		if err != nil {
			return nil, err
		}
		slots, err := body.productDomain.ObjectConstructorCoordinateWrites(plan)
		if err != nil {
			return nil, err
		}
		concrete, err := body.productDomain.SealCoordinateFactorInventory(body.keys, slots)
		if err != nil {
			return nil, err
		}
		formal, err := rekeyFormalCoordinateFactorInventory(body.productDomain, formalKeys, rekey, concrete)
		if err != nil {
			return nil, err
		}
		return formal.Slots(), nil
	default:
		return nil, nil
	}
}

func freezeFormalRootAssignmentCoordinateSlots(body *relationProgramBody, formalKeys *keyspace.KeySpace, rekey state.CoordinateFormalRootRekey, current state.CoordinateFactorInventory, step boundaryStep) ([]state.CoordinateSlot, error) {
	if body.rootAssignments == nil || !body.rootAssignments.Valid() {
		return nil, nil
	}
	plan, err := body.rootAssignments.PrepareResolvedRootAssignmentPlan(step.rootAssignment.transaction)
	if err != nil {
		return nil, err
	}
	plan, err = bindLinkedCallResultSourcePath(body, step.rootAssignment, plan)
	if err != nil {
		return nil, err
	}
	plan, err = plan.RekeyFormal(body.productDomain, rekey)
	if err != nil {
		return nil, err
	}
	var slots []state.CoordinateSlot
	if object, ok := plan.ObjectLiteralSourcePlan(); ok {
		constructor, constructorErr := object.PrepareObjectConstructorPlan(body.productDomain, formalKeys)
		if constructorErr != nil {
			return nil, constructorErr
		}
		writes, writesErr := body.productDomain.ObjectConstructorCoordinateWrites(constructor)
		if writesErr != nil {
			return nil, writesErr
		}
		slots = append(slots, writes...)
	}
	if target, ok := plan.TargetPathKey(); ok {
		rootSlots, err := body.productDomain.BoundaryRootCoordinateSlots(formalKeys, []keyspace.Key{target})
		if err != nil {
			return nil, err
		}
		slots = append(slots, rootSlots...)
		completionSlots, completionErr := body.productDomain.RootAssignmentCompletionCoordinateTargetSlots(formalKeys, target)
		if completionErr != nil {
			return nil, completionErr
		}
		slots = append(slots, completionSlots...)
	}
	if proof, ok := plan.SourcePresenceProof(); ok {
		slot, err := body.productDomain.PathBranchProofCoordinateSlot(formalKeys, proof)
		if err != nil {
			return nil, err
		}
		slots = append(slots, slot)
	}
	if proof, ok := plan.PublishedEqualityProof(); ok {
		slot, err := body.productDomain.PathBranchProofCoordinateSlot(formalKeys, proof)
		if err != nil {
			return nil, err
		}
		slots = append(slots, slot)
	}
	transaction, hasScalars := plan.ScalarFactorTransaction()
	if hasScalars {
		for _, family := range body.productDomain.RootAssignmentScalarCoordinateFamilies() {
			inventory, err := current.FamilySlots(family)
			if err != nil {
				return nil, err
			}
			demands, err := body.productDomain.RootAssignmentScalarCoordinateDemands(transaction, family, formalKeys, inventory)
			if err != nil {
				return nil, err
			}
			for _, demand := range demands {
				slots = append(slots, demand.Target())
				if source, present := demand.PointSource(); present {
					slots = append(slots, source)
				}
			}
		}
	}
	return slots, nil
}

func freezeFormalOutcomeCoordinateFootprint(
	body *relationProgramBody,
	formalKeys *keyspace.KeySpace,
	rekey state.CoordinateFormalRootRekey,
	outcomeRef boundaryOutcomeRef,
	current state.CoordinateFactorInventory,
	identityTerms []identity.Term,
) (state.CoordinateFactorInventory, error) {
	if body == nil || outcomeRef == 0 || int(outcomeRef) >= len(body.relation.code.outcomes) {
		return state.CoordinateFactorInventory{}, fmt.Errorf("formal Outcome coordinate footprint is unowned")
	}
	if !current.ValidFor(body.productDomain, formalKeys) {
		return state.CoordinateFactorInventory{}, fmt.Errorf("formal Outcome coordinate footprint has no body inventory")
	}
	outcome := body.relation.code.outcomes[outcomeRef]
	transaction := outcome.returnTransaction.transaction
	var slots []state.CoordinateSlot
	concreteTargets := make([]factapply.CallReturnPresenceTarget, 0, transaction.ResultTargetCount())
	for index := 0; index < transaction.ResultTargetCount(); index++ {
		target, ok := transaction.ResultTarget(index)
		if !ok || target < 0 {
			return state.CoordinateFactorInventory{}, fmt.Errorf("formal Outcome result target is malformed")
		}
		concrete := body.keys.FromPath(pathdom.Path{Root: fmt.Sprintf("ret[%d]", target)})
		formalPath, err := body.productDomain.RekeyStructuralKeyFormal(rekey, concrete)
		if err != nil {
			return state.CoordinateFactorInventory{}, err
		}
		rootSlots, err := body.productDomain.BoundaryRootCoordinateSlots(formalKeys, []keyspace.Key{formalPath})
		if err != nil {
			return state.CoordinateFactorInventory{}, err
		}
		slots = append(slots, rootSlots...)
		concreteTargets = append(concreteTargets, factapply.CallReturnPresenceTarget{Index: target, Path: concrete})
	}
	if len(concreteTargets) > 1 && body.pathSemantics != nil && body.pathSemantics.Valid() {
		concrete, err := body.pathSemantics.CallReturnPresenceCoordinateInventory(body.productDomain, concreteTargets)
		if err != nil {
			return state.CoordinateFactorInventory{}, err
		}
		formal, err := rekeyFormalCoordinateFactorInventory(body.productDomain, formalKeys, rekey, concrete)
		if err != nil {
			return state.CoordinateFactorInventory{}, err
		}
		slots = append(slots, formal.Slots()...)
	}
	if outcome.covariant.HasStateSteps() {
		covariant, err := freezeFormalCovariantCoordinateSlots(body, formalKeys, rekey, current, outcome.covariant)
		if err != nil {
			return state.CoordinateFactorInventory{}, err
		}
		slots = append(slots, covariant...)
	}
	owned, err := body.productDomain.SealCoordinateFactorInventory(formalKeys, slots)
	if err != nil {
		return state.CoordinateFactorInventory{}, err
	}
	// N5 observes the existing registered identity graph, including heap
	// skeletons and member coordinates whose keys cannot be synthesized from
	// an identity term alone. Keep the complete frozen body inventory in the
	// carrier; the symbolic executor must avoid Cartesian materialization
	// without deleting semantic inputs.
	owned, err = body.productDomain.UnionCoordinateFactorInventories(formalKeys, current, owned)
	if err != nil {
		return state.CoordinateFactorInventory{}, err
	}
	return body.productDomain.CloseCoordinateFactorInventoryWithIdentityTerms(formalKeys, owned, identityTerms)
}

// freezeFormalCovariantCoordinateSlots derives the exact registered subtree
// cone of N6 from its structural source roots. The concrete and formal N6
// executors still use ApplyCovariantExposureFactors as their sole semantic
// law; this certificate only prevents unrelated coordinate fibers from being
// placed in the same symbolic product.
func freezeFormalCovariantCoordinateSlots(
	body *relationProgramBody,
	formalKeys *keyspace.KeySpace,
	rekey state.CoordinateFormalRootRekey,
	current state.CoordinateFactorInventory,
	transaction factapply.CovariantExposureTransaction,
) ([]state.CoordinateSlot, error) {
	if body == nil || formalKeys == nil || !formalKeys.Valid() || !current.ValidFor(body.productDomain, formalKeys) ||
		!transaction.Valid(body.productDomain.Registry()) {
		return nil, fmt.Errorf("formal CovariantExposure coordinate footprint is unowned")
	}
	if !transaction.HasStateSteps() {
		return nil, nil
	}
	owner, ok := body.productDomain.PathValueFamily()
	if !ok || body.pathSemantics == nil || !body.pathSemantics.Valid() {
		return nil, fmt.Errorf("formal CovariantExposure has no path dependency owner")
	}
	pathInventory, err := current.FamilySlots(owner)
	if err != nil {
		return nil, err
	}
	seeds := make([]state.CoordinateDependencySeed, 0, transaction.Len())
	for index := 0; index < transaction.Len(); index++ {
		step, present := transaction.Step(index)
		if !present {
			return nil, fmt.Errorf("formal CovariantExposure step %d is absent", index)
		}
		path := step.Exposure().SourcePath()
		if path.Symbol == 0 {
			continue
		}
		visible, present := body.pathSemantics.VisibleLocalPathKey(transaction.Point(), pathdom.NewPath(path.Symbol, ""))
		if !present {
			return nil, fmt.Errorf("formal CovariantExposure source %d has no visible root", index)
		}
		root, rekeyErr := body.productDomain.RekeyStructuralKeyFormal(rekey, visible)
		if rekeyErr != nil {
			return nil, rekeyErr
		}
		seeds = append(seeds, state.CoordinateDependencySeed{
			ID: state.CoordinateDependencyID(index + 1), SubtreeMutationRoots: []keyspace.Key{root},
		})
	}
	if len(seeds) == 0 {
		return nil, nil
	}
	dependencies, err := body.productDomain.PlanPathCoordinateDependencies(formalKeys, pathInventory, seeds)
	if err != nil {
		return nil, err
	}
	var slots []state.CoordinateSlot
	for _, seed := range seeds {
		dependency, present := dependencies.Dependency(seed.ID)
		if !present {
			return nil, fmt.Errorf("formal CovariantExposure dependency %d is absent", seed.ID)
		}
		slots = append(slots, dependency.CoordinateReads()...)
		slots = append(slots, dependency.CoordinateWrites()...)
		for _, family := range body.productDomain.PathDescendantMutationCoordinateFamilies() {
			familySlots, familyErr := current.FamilySlots(family)
			if familyErr != nil {
				return nil, familyErr
			}
			selected, selectErr := body.productDomain.SelectPathMutationCoordinateSlots(family, familySlots, dependency.MutationRegions())
			if selectErr != nil {
				return nil, selectErr
			}
			slots = append(slots, selected...)
		}
	}
	return slots, nil
}
