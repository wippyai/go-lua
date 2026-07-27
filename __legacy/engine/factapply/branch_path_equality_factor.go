package factapply

import (
	"fmt"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/symbol"
)

type branchPathEqualityDraft struct {
	left, right             keyspace.Key
	leftSymbol, rightSymbol symbol.ID
	persistent              bool
}

type branchPathEqualityFactor struct {
	program PathEqualityFactorProgram[BranchRelationValueRole]
}

func freezeBranchPathEqualityFactor(
	b *branchProgramBuilder,
	coordinateUniverse state.CoordinateFactorInventory,
	draft *branchPathEqualityDraft,
	seal *branchProgramSeal,
) (*branchPathEqualityFactor, error) {
	if b == nil || draft == nil || seal == nil || draft.left == draft.right {
		return nil, fmt.Errorf("factapply: invalid branch path-equality declaration")
	}
	family, ok := b.domain.PathValueFamily()
	if !ok {
		return nil, fmt.Errorf("factapply: branch path equality has no registered path family")
	}
	union, err := coordinateUniverse.FamilySlots(family)
	if err != nil {
		return nil, err
	}
	plan, err := b.domain.SealPathEqualityFactorPlan(b.authority.resolver.KeySpace(), draft.left, draft.right, union)
	if draft.persistent {
		plan, err = b.domain.SealPersistentPathEqualityFactorPlan(b.authority.resolver.KeySpace(), draft.left, draft.right, union)
	}
	if err != nil {
		return nil, err
	}
	if draft.leftSymbol == 0 || draft.rightSymbol == 0 {
		return nil, fmt.Errorf("factapply: branch path equality has unresolved lexical roots")
	}
	leftRole, leftOK := newBranchLexicalValueRole(seal, draft.leftSymbol)
	rightRole, rightOK := newBranchLexicalValueRole(seal, draft.rightSymbol)
	if !leftOK || !rightOK {
		return nil, fmt.Errorf("factapply: branch path equality has unbound Values roots")
	}
	program, err := preparePathEqualityFactorProgram(b.domain, plan, func(dependency statekey.ValueDependency) (BranchRelationValueRole, bool) {
		switch dependency {
		case plan.LeftValue():
			return leftRole, true
		case plan.RightValue():
			return rightRole, true
		default:
			return BranchRelationValueRole{}, false
		}
	}, b.authority.typeValues, draft.persistent)
	if err != nil {
		return nil, err
	}
	return &branchPathEqualityFactor{program: program}, nil
}

func bindBranchPathEqualityAccess(atom *branchAtom) error {
	if atom == nil || atom.equality == nil || !atom.equality.program.Valid() {
		return nil
	}
	program := atom.equality.program
	pathFamily, hasPathFamily := program.domain.PathEvidenceCoordinateFamily()
	if hasPathFamily {
		// The equality carrier freezes a potentially changed path-evidence
		// skeleton, so this family is reconciliation authority, not a scalar
		// patch merely because its selected proof slots are sparse.
		atom.access.coordinateFamilyReads = appendCoordinateFamilies(atom.access.coordinateFamilyReads, pathFamily)
		atom.access.coordinateFamilyWrites = appendCoordinateFamilies(atom.access.coordinateFamilyWrites, pathFamily)
	}
	topology, err := program.domain.SealPathDescendantMutationFactorTopology()
	if err != nil {
		return err
	}
	for _, family := range topology.Families() {
		atom.access.coordinateFamilyReads = appendCoordinateFamilies(atom.access.coordinateFamilyReads, family)
		atom.access.coordinateFamilyWrites = appendCoordinateFamilies(atom.access.coordinateFamilyWrites, family)
	}
	atom.access.coordinateReads = appendBranchCoordinateSlots(program.domain, atom.access.coordinateReads, program.CoordinateReads()...)
	atom.access.coordinateWrites = appendBranchCoordinateSlots(program.domain, atom.access.coordinateWrites, program.CoordinateWrites()...)
	for _, lane := range program.Lanes() {
		if hasPathFamily && lane == pathFamily.Lane() {
			continue
		}
		atom.access.laneReads = appendProductLanes(atom.access.laneReads, lane)
		atom.access.laneWrites = appendProductLanes(atom.access.laneWrites, lane)
	}
	return nil
}

func branchPathEqualityKernel(equality *branchPathEqualityFactor) branchAtomFactorKernel {
	return func(runtime branchAtomFactorRuntime, _ BranchRelationFactorFrame, current BranchRelationFactorFrame) (BranchRelationFactorPatch, bool, error) {
		if equality == nil || !equality.program.Valid() || current.plan == nil || !current.reachable {
			return BranchRelationFactorPatch{plan: current.plan}, false, nil
		}
		values := state.ValueFactor[BranchRelationValueRole]{Top: current.valuesTop, Values: make(map[BranchRelationValueRole]product.Value, len(current.values))}
		for index, role := range current.plan.layout.currentValues {
			if !product.Equal(runtime.domain.Registry(), current.values[index], product.Bottom(runtime.domain.Registry())) {
				values.Values[role] = current.values[index]
			}
		}
		workingLanes := append([]state.LaneFactor(nil), current.lanes...)
		programFactors := make([]state.LaneFactor, len(equality.program.Lanes()))
		programLaneIndices := make([]int, len(programFactors))
		pathCoordinateIndex := -1
		family, ok := runtime.domain.PathEvidenceCoordinateFamily()
		if !ok {
			return BranchRelationFactorPatch{}, false, fmt.Errorf("factapply: branch path equality has no path carrier")
		}
		for index, group := range current.plan.layout.currentCoordinates {
			if group.family == family {
				pathCoordinateIndex = index
				break
			}
		}
		if pathCoordinateIndex < 0 {
			return BranchRelationFactorPatch{}, false, fmt.Errorf("factapply: branch path equality coordinate carrier is absent")
		}
		for programIndex, lane := range equality.program.Lanes() {
			programLaneIndices[programIndex] = -1
			for index, candidate := range current.plan.layout.currentLanes {
				if candidate == lane {
					programFactors[programIndex], programLaneIndices[programIndex] = current.lanes[index], index
					break
				}
			}
			if programLaneIndices[programIndex] < 0 {
				return BranchRelationFactorPatch{}, false, fmt.Errorf("factapply: branch path-equality lane is absent")
			}
		}
		skeleton := current.coordinates[pathCoordinateIndex].Skeleton
		scalars := current.coordinates[pathCoordinateIndex].Scalars
		mutation, err := bindBranchPathDescendantMutationFactors(runtime.domain, current.plan.layout, current)
		if err != nil {
			return BranchRelationFactorPatch{}, false, err
		}
		carrier, err := state.OpenCoordinatePathEvidenceCarrier(
			runtime.domain, skeleton, scalars, state.ValueFactor[BranchRelationValueRole]{}, current.reachable,
			equality.program.PathEvidenceAuthority(), mutation,
		)
		if err != nil {
			return BranchRelationFactorPatch{}, false, err
		}
		out, err := equality.program.Apply(runtime.context.Context, tokenOf(runtime.context.Session), ValueRefinementFactorFrame[BranchRelationValueRole]{
			Values: values, Factors: programFactors, Carrier: carrier, Reachable: current.reachable,
		})
		if err != nil {
			return BranchRelationFactorPatch{}, false, err
		}
		for programIndex, index := range programLaneIndices {
			workingLanes[index] = out.Factors[programIndex]
		}
		coordinates := cloneBranchCoordinateOperands(current.coordinates)
		var carrierInvalidation []state.LaneFactor
		nextSkeleton, allScalars, _, carrierInvalidation, carrierCoordinates, _, err := out.Carrier.Freeze()
		if err != nil {
			return BranchRelationFactorPatch{}, false, err
		}
		coordinates[pathCoordinateIndex], err = bindBranchCoordinateLayout(
			runtime.domain, current.plan.layout.currentCoordinates[pathCoordinateIndex], nextSkeleton, allScalars,
		)
		if err != nil {
			return BranchRelationFactorPatch{}, false, err
		}
		for _, factor := range carrierInvalidation {
			for index, lane := range current.plan.layout.currentLanes {
				if factor.Lane() == lane {
					workingLanes[index] = factor
					break
				}
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
