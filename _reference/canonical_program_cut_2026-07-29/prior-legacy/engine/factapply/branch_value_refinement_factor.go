package factapply

import (
	"fmt"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
)

// branchValueRefinementDraft is syntax retained only until the transaction's
// registered coordinate inventory is sealed. Execution uses only the generic
// ValueRefinementFactorProgram below.
type branchValueRefinementDraft struct {
	path              pathdom.Path
	refinement        factflow.ValueRefinement
	invalidatePrefix  bool
	nonBooleanTrigger pathdom.Path
}

// branchValueRefinementFactor is the one canonical guard-refinement law used
// by concrete and formal BranchRelations adapters. The adapter supplies a
// dense product fiber; no State, resolver, or inventory survives preparation.
type branchValueRefinementFactor struct {
	program           ValueRefinementFactorProgram[BranchRelationValueRole]
	root              BranchRelationValueRole
	target            pathdom.PathKey
	invalidatePrefix  bool
	nonBooleanTrigger ResolvedStructuralPath
	triggerRoot       BranchRelationValueRole
}

func freezeBranchValueRefinementFactor(
	b *branchProgramBuilder,
	coordinateUniverse state.CoordinateFactorInventory,
	draft *branchValueRefinementDraft,
	seal *branchProgramSeal,
) (*branchValueRefinementFactor, error) {
	if b == nil || draft == nil || seal == nil {
		return nil, fmt.Errorf("factapply: invalid branch value-refinement declaration")
	}
	target, ok := visibility.AddressAt(b.authority.resolver, b.transaction.point, draft.path).VisibleKeyspaceKey()
	if !ok {
		target, ok = b.key(draft.path)
	}
	if !ok {
		return nil, fmt.Errorf("factapply: branch value-refinement target is not visible")
	}
	root, ok := newBranchLexicalValueRole(seal, draft.path.Symbol)
	if !ok {
		return nil, fmt.Errorf("factapply: branch value-refinement root is not lexical")
	}
	plan, err := b.domain.SealValueRefinementPlan(
		b.authority.resolver.KeySpace(), target, coordinateUniverse,
	)
	if err != nil {
		return nil, fmt.Errorf("factapply: branch value-refinement coordinate plan: %w", err)
	}
	program, err := PrepareGuardValueRefinementFactorProgram(
		b.domain, plan, draft.refinement,
		func(dependency statekey.ValueDependency) (BranchRelationValueRole, bool) {
			return root, dependency == plan.RootValue()
		},
		b.authority.typeValues, b.authority.projectPath,
	)
	if err != nil {
		return nil, fmt.Errorf("factapply: branch value-refinement factor program: %w", err)
	}
	out := &branchValueRefinementFactor{
		program: program, root: root,
		target:           pathdom.PathKey(b.authority.resolver.KeySpace().FormatReadOnly(target)),
		invalidatePrefix: draft.invalidatePrefix,
	}
	if !draft.nonBooleanTrigger.IsEmpty() {
		trigger, triggerOK := b.key(draft.nonBooleanTrigger)
		if !triggerOK {
			return nil, fmt.Errorf("factapply: branch conditional refinement trigger is not visible")
		}
		out.nonBooleanTrigger, err = FreezeResolvedStructuralPath(
			b.authority.resolver.KeySpace(), trigger, draft.nonBooleanTrigger.Symbol,
		)
		if err != nil {
			return nil, fmt.Errorf("factapply: branch conditional refinement trigger: %w", err)
		}
		out.triggerRoot, triggerOK = newBranchLexicalValueRole(seal, draft.nonBooleanTrigger.Symbol)
		if !triggerOK {
			return nil, fmt.Errorf("factapply: branch conditional refinement trigger root is not lexical")
		}
	}
	return out, nil
}

func bindBranchValueRefinementAccess(atom *branchAtom) error {
	if atom == nil || atom.refinement == nil {
		return nil
	}
	program := atom.refinement.program
	if family, present := program.domain.PathEvidenceCoordinateFamily(); present {
		atom.access.coordinateFamilyReads = appendCoordinateFamilies(atom.access.coordinateFamilyReads, family)
		if program.WritesCoordinateSkeleton() || program.NeedsDescendantInvalidation() {
			atom.access.coordinateFamilyWrites = appendCoordinateFamilies(atom.access.coordinateFamilyWrites, family)
		}
	}
	if program.NeedsDescendantInvalidation() {
		topology, err := program.domain.SealPathDescendantMutationFactorTopology()
		if err != nil {
			return err
		}
		for _, family := range topology.Families() {
			atom.access.coordinateFamilyReads = appendCoordinateFamilies(atom.access.coordinateFamilyReads, family)
			atom.access.coordinateFamilyWrites = appendCoordinateFamilies(atom.access.coordinateFamilyWrites, family)
		}
	}
	for _, slot := range program.CoordinateReads() {
		atom.access.coordinateReads = appendUniqueCoordinateSlot(atom.refinement.program.domain, atom.access.coordinateReads, slot)
	}
	for _, slot := range program.plan.FactorCoordinateReads() {
		atom.access.coordinateReads = appendUniqueCoordinateSlot(atom.refinement.program.domain, atom.access.coordinateReads, slot)
	}
	for _, slot := range program.CoordinateWrites() {
		atom.access.coordinateWrites = appendUniqueCoordinateSlot(atom.refinement.program.domain, atom.access.coordinateWrites, slot)
	}
	atom.access.laneReads = append([]state.ProductLane(nil), program.Lanes()...)
	if atom.refinement.invalidatePrefix {
		topology, err := program.domain.SealPathSubtreeMutationFactorTopology()
		if err != nil {
			return err
		}
		atom.access.laneReads = appendProductLanes(atom.access.laneReads, topology.Lanes()...)
		atom.access.laneWrites = appendProductLanes(atom.access.laneWrites, topology.Lanes()...)
		for _, family := range topology.Families() {
			atom.access.coordinateFamilyReads = appendCoordinateFamilies(atom.access.coordinateFamilyReads, family)
			atom.access.coordinateFamilyWrites = appendCoordinateFamilies(atom.access.coordinateFamilyWrites, family)
		}
	}
	return nil
}

func bindBranchPathSubtreeMutationFactors(
	domain state.ProductDomain,
	layout BranchRelationFactorLayout,
	lanes []state.LaneFactor,
	coordinates []BranchRelationCoordinateOperands,
) (state.PathSubtreeMutationFactors, error) {
	topology, err := domain.SealPathSubtreeMutationFactorTopology()
	if err != nil {
		return state.PathSubtreeMutationFactors{}, err
	}
	laneFactors := make([]state.LaneFactor, len(topology.Lanes()))
	for index, wanted := range topology.Lanes() {
		found := false
		for currentIndex, lane := range layout.currentLanes {
			if lane == wanted {
				laneFactors[index], found = lanes[currentIndex], true
				break
			}
		}
		if !found {
			return state.PathSubtreeMutationFactors{}, fmt.Errorf("factapply: path-subtree mutation lane is absent")
		}
	}
	coordinateFactors := make([]state.CoordinateFamilyFactor, len(topology.Families()))
	for index, wanted := range topology.Families() {
		found := false
		for currentIndex, group := range layout.currentCoordinates {
			if group.family != wanted {
				continue
			}
			coordinateFactors[index], err = domain.SealCoordinateFamilyFactor(
				coordinates[currentIndex].Skeleton, coordinates[currentIndex].Scalars,
			)
			if err != nil {
				return state.PathSubtreeMutationFactors{}, err
			}
			found = true
			break
		}
		if !found {
			return state.PathSubtreeMutationFactors{}, fmt.Errorf("factapply: path-subtree mutation coordinate family is absent")
		}
	}
	return domain.SealPathSubtreeMutationFactors(laneFactors, coordinateFactors)
}

func applyBranchPathSubtreeMutationFactors(
	domain state.ProductDomain,
	layout BranchRelationFactorLayout,
	lanes []state.LaneFactor,
	coordinates []BranchRelationCoordinateOperands,
	factors state.PathSubtreeMutationFactors,
) ([]state.LaneFactor, []BranchRelationCoordinateOperands, error) {
	outLanes := append([]state.LaneFactor(nil), lanes...)
	for _, factor := range factors.LaneFactors() {
		found := false
		for index, lane := range layout.currentLanes {
			if lane == factor.Lane() {
				outLanes[index], found = factor, true
				break
			}
		}
		if !found {
			return nil, nil, fmt.Errorf("factapply: path-subtree mutation lane result is absent")
		}
	}
	outCoordinates := cloneBranchCoordinateOperands(coordinates)
	for _, factor := range factors.CoordinateFactors() {
		found := false
		for index, group := range layout.currentCoordinates {
			if group.family != factor.Family() {
				continue
			}
			var err error
			outCoordinates[index], err = bindBranchCoordinateLayout(domain, group, factor.Skeleton(), factor.Scalars())
			if err != nil {
				return nil, nil, err
			}
			found = true
			break
		}
		if !found {
			return nil, nil, fmt.Errorf("factapply: path-subtree mutation coordinate result is absent")
		}
	}
	return outLanes, outCoordinates, nil
}

func branchValueRefinementKernel(refinement *branchValueRefinementFactor) branchAtomFactorKernel {
	return func(runtime branchAtomFactorRuntime, _ BranchRelationFactorFrame, current BranchRelationFactorFrame) (BranchRelationFactorPatch, bool, error) {
		if refinement == nil || !refinement.program.Valid() || current.plan == nil || !current.reachable {
			return BranchRelationFactorPatch{plan: current.plan}, false, nil
		}
		workingLanes := append([]state.LaneFactor(nil), current.lanes...)
		values := state.ValueFactor[BranchRelationValueRole]{Top: current.valuesTop, Values: make(map[BranchRelationValueRole]product.Value, len(current.values))}
		for index, role := range current.plan.layout.currentValues {
			if !product.Equal(runtime.domain.Registry(), current.values[index], product.Bottom(runtime.domain.Registry())) {
				values.Values[role] = current.values[index]
			}
		}
		if refinement.nonBooleanTrigger.keys != nil {
			triggerValue, triggerBound := values.Values[refinement.triggerRoot]
			if values.Top {
				triggerValue, triggerBound = product.Top(), true
			}
			if !triggerBound {
				triggerValue, triggerBound = product.Bottom(runtime.domain.Registry()), true
			}
			reader := branchResolvedPathReader{
				domain: runtime.domain, keys: refinement.nonBooleanTrigger.keys,
				root: refinement.nonBooleanTrigger.root, value: triggerValue,
				lanes: make(map[state.LaneID]state.LaneFactor, len(current.lanes)),
			}
			for _, lane := range current.lanes {
				reader.lanes[lane.Lane().ID()] = lane
			}
			resolved, exact := ResolveStructuralPathFactorValue(runtime.domain.Registry(), reader, refinement.nonBooleanTrigger)
			if !exact && refinement.program.projectPath != nil {
				rootType, structural := typevalue.StructuralTypeOf(
					runtime.domain.Registry(), refinement.program.typeValues, triggerValue,
					typevalue.StructuralTypeOptions{ApplyPresence: true},
				)
				if structural {
					projected, projectedOK := projectStructuralSegments(
						refinement.program.projectPath, rootType, refinement.nonBooleanTrigger.segments,
					)
					if projectedOK {
						resolved, exact = projectedPathValue(runtime.domain.Registry(), refinement.program.typeValues, projected), true
					}
				}
			}
			if !exact {
				resolved, exact = projectPathOriginFromRootSegments(
					refinement.program.typeValues, runtime.domain.Registry(), triggerValue,
					refinement.nonBooleanTrigger.segments, refinement.program.projectPath,
				)
			}
			if !exact {
				return branchValueRefinementIdentityPatch(current), current.reachable, nil
			}
			kinds := product.Get(runtime.domain.Registry(), resolved, runtimekind.Key)
			if kinds.IsBottom() || kinds.IsTop() || kinds.Contains(runtimekind.Boolean) {
				return branchValueRefinementIdentityPatch(current), current.reachable, nil
			}
		}
		pathFamily, ok := runtime.domain.PathEvidenceCoordinateFamily()
		if !ok {
			return BranchRelationFactorPatch{}, false, fmt.Errorf("factapply: branch refinement has no path-evidence family")
		}
		pathCoordinateIndex := -1
		for index, group := range current.plan.layout.currentCoordinates {
			if group.family == pathFamily {
				pathCoordinateIndex = index
				break
			}
		}
		if pathCoordinateIndex < 0 {
			return BranchRelationFactorPatch{}, false, fmt.Errorf("factapply: branch refinement path carrier is absent")
		}
		coordinates := cloneBranchCoordinateOperands(current.coordinates)
		skeleton := coordinates[pathCoordinateIndex].Skeleton
		scalars := coordinates[pathCoordinateIndex].Scalars
		var err error
		if refinement.invalidatePrefix {
			mutation, present, mutationErr := runtime.domain.PrepareCoordinatePathSubtreeMutationIfPresent(skeleton, scalars, refinement.target)
			if mutationErr != nil {
				return BranchRelationFactorPatch{}, false, mutationErr
			}
			if present {
				factors, bindErr := bindBranchPathSubtreeMutationFactors(runtime.domain, current.plan.layout, workingLanes, coordinates)
				if bindErr != nil {
					return BranchRelationFactorPatch{}, false, bindErr
				}
				factors, err = runtime.domain.ApplyPathSubtreeMutationFactors(mutation, factors)
				if err != nil {
					return BranchRelationFactorPatch{}, false, err
				}
				workingLanes, coordinates, err = applyBranchPathSubtreeMutationFactors(
					runtime.domain, current.plan.layout, workingLanes, coordinates, factors,
				)
				if err != nil {
					return BranchRelationFactorPatch{}, false, err
				}
				skeleton, scalars = coordinates[pathCoordinateIndex].Skeleton, coordinates[pathCoordinateIndex].Scalars
			}
		}
		programFactors := make([]state.LaneFactor, len(refinement.program.Lanes()))
		programLaneIndices := make([]int, len(programFactors))
		for programIndex, lane := range refinement.program.Lanes() {
			programLaneIndices[programIndex] = -1
			for index, candidate := range current.plan.layout.currentLanes {
				if candidate == lane {
					programFactors[programIndex], programLaneIndices[programIndex] = workingLanes[index], index
					break
				}
			}
			if programLaneIndices[programIndex] < 0 {
				return BranchRelationFactorPatch{}, false, fmt.Errorf("factapply: branch refinement factor lane is absent")
			}
		}
		mutation, err := bindBranchPathDescendantMutationFactors(runtime.domain, current.plan.layout, current)
		if err != nil {
			return BranchRelationFactorPatch{}, false, err
		}
		carrier, err := state.OpenCoordinatePathEvidenceCarrier(
			runtime.domain, skeleton, scalars, state.ValueFactor[BranchRelationValueRole]{Values: map[BranchRelationValueRole]product.Value{}}, current.reachable,
			refinement.program.PathEvidenceAuthority(), mutation,
		)
		if err != nil {
			return BranchRelationFactorPatch{}, false, err
		}
		out, err := refinement.program.Apply(runtime.context.Context, tokenOf(runtime.context.Session), ValueRefinementFactorFrame[BranchRelationValueRole]{
			Values: values, Factors: programFactors, Carrier: carrier, Reachable: current.reachable,
		})
		if err != nil {
			return BranchRelationFactorPatch{}, false, err
		}
		for programIndex, index := range programLaneIndices {
			workingLanes[index] = out.Factors[programIndex]
		}
		nextSkeleton, nextScalars, _, carrierLanes, carrierCoordinates, _, err := out.Carrier.Freeze()
		if err != nil {
			return BranchRelationFactorPatch{}, false, err
		}
		for _, factor := range carrierLanes {
			for index, lane := range current.plan.layout.currentLanes {
				if factor.Lane() == lane {
					workingLanes[index] = factor
					break
				}
			}
		}
		for index, group := range current.plan.layout.currentCoordinates {
			if group.family == skeleton.Family() {
				coordinates[index], err = bindBranchCoordinateLayout(runtime.domain, group, nextSkeleton, nextScalars)
				if err != nil {
					return BranchRelationFactorPatch{}, false, err
				}
				break
			}
		}
		coordinates, err = applyBranchPathDescendantCoordinateFactors(runtime.domain, current.plan.layout, coordinates, carrierCoordinates)
		if err != nil {
			return BranchRelationFactorPatch{}, false, err
		}
		patch := BranchRelationFactorPatch{
			plan: current.plan, values: make([]product.Value, len(current.plan.layout.currentValues)), valuesTop: out.Values.Top,
			lanes: workingLanes, coordinates: coordinates, reachable: out.Reachable,
		}
		for index, role := range current.plan.layout.currentValues {
			patch.values[index] = product.Bottom(runtime.domain.Registry())
			if out.Values.Top {
				patch.values[index] = product.Top()
			} else if value, present := out.Values.Values[role]; present {
				patch.values[index] = value
			}
		}
		return patch, out.Reachable, nil
	}
}

func branchValueRefinementIdentityPatch(current BranchRelationFactorFrame) BranchRelationFactorPatch {
	return BranchRelationFactorPatch{
		plan: current.plan, values: append([]product.Value(nil), current.values...), valuesTop: current.valuesTop,
		lanes: append([]state.LaneFactor(nil), current.lanes...), reachable: current.reachable,
	}
}
