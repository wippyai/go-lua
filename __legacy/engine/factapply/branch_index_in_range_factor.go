package factapply

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// branchIndexInRangeFactor combines the two effects of a canonical in-range
// branch proof: publication of the proof itself and, when the array has a
// statically exact sequence length, a numeric ceiling on its index.  The
// bound comes solely from branchIndexInRangeStaticBound; this carrier adapter
// only resolves its declared array operand and publishes that result.
type branchIndexInRangeFactor struct {
	proof       pathevidence.BranchProof
	index       keyspace.Key
	array       ResolvedStructuralPath
	arrayRoot   BranchRelationValueRole
	arraySymbol symbol.ID
	typeValues  *typevalue.Cache
	project     PathTypeProjector
}

type branchIndexInRangeDraft struct {
	proof        pathevidence.BranchProof
	index, array keyspace.Key
	arraySymbol  symbol.ID
}

func freezeBranchIndexInRangeFactor(b *branchProgramBuilder, draft *branchIndexInRangeDraft, seal *branchProgramSeal) (*branchIndexInRangeFactor, error) {
	if b == nil || draft == nil || seal == nil || draft.index == (keyspace.Key{}) || draft.array == (keyspace.Key{}) || draft.arraySymbol == 0 {
		return nil, fmt.Errorf("factapply: invalid index-in-range factor declaration")
	}
	array, err := FreezeResolvedStructuralPath(b.authority.resolver.KeySpace(), draft.array, draft.arraySymbol)
	if err != nil {
		return nil, fmt.Errorf("factapply: index-in-range array operand: %w", err)
	}
	arrayRoot, ok := newBranchLexicalValueRole(seal, draft.arraySymbol)
	if !ok {
		return nil, fmt.Errorf("factapply: index-in-range array Values root is absent")
	}
	return &branchIndexInRangeFactor{
		proof: draft.proof, index: draft.index, array: array, arrayRoot: arrayRoot, arraySymbol: draft.arraySymbol,
		typeValues: b.authority.typeValues, project: b.authority.projectPath,
	}, nil
}

func branchIndexInRangeKernel(factor branchIndexInRangeFactor) branchAtomFactorKernel {
	proofKernel := branchPathProofKernel(factor.proof)
	return func(runtime branchAtomFactorRuntime, original, current BranchRelationFactorFrame) (BranchRelationFactorPatch, bool, error) {
		if current.plan == nil || !current.reachable {
			return BranchRelationFactorPatch{plan: current.plan}, false, nil
		}
		next := current
		if array, ok := factor.resolveArrayValue(runtime, current); ok {
			if bound, exact := branchIndexInRangeStaticBound(factor.typeValues, runtime.domain.Registry(), array); exact {
				mutation, err := runtime.domain.PrepareCoordinateBranchBound(
					state.CoordinateBoundValue, state.CoordinateBoundUpper,
					factor.array.keys, factor.index, bound,
				)
				if err != nil {
					return BranchRelationFactorPatch{}, false, err
				}
				patch, feasible, err := applyBranchCoordinateMutationFactor(runtime, next, mutation)
				if err != nil || !feasible {
					return patch, feasible, err
				}
				next = branchRelationFrameFromPatch(next, patch)
			}
		}
		return proofKernel(runtime, original, next)
	}
}

func (f branchIndexInRangeFactor) resolveArrayValue(runtime branchAtomFactorRuntime, frame BranchRelationFactorFrame) (product.Value, bool) {
	rootIndex := -1
	for index, role := range frame.plan.layout.currentValues {
		if role == f.arrayRoot {
			rootIndex = index
			break
		}
	}
	if rootIndex < 0 || rootIndex >= len(frame.values) {
		return product.Value{}, false
	}
	reader := branchResolvedPathReader{
		domain: runtime.domain, keys: f.array.keys, root: f.arraySymbol, value: frame.values[rootIndex],
		lanes: make(map[state.LaneID]state.LaneFactor, len(frame.lanes)),
	}
	for _, lane := range frame.lanes {
		reader.lanes[lane.Lane().ID()] = lane
	}
	if value, resolved := ResolveStructuralPathFactorValue(runtime.domain.Registry(), reader, f.array); resolved {
		return value, true
	}
	if f.project != nil {
		if rootType, structural := typevalue.StructuralTypeOf(runtime.domain.Registry(), f.typeValues, frame.values[rootIndex], typevalue.StructuralTypeOptions{ApplyPresence: true}); structural {
			if projected, ok := f.project(rootType, pathdom.Path{Symbol: f.arraySymbol, Segments: f.array.segments}); ok {
				return projectedPathValue(runtime.domain.Registry(), f.typeValues, projected), true
			}
		}
	}
	return projectPathOriginFromRoot(f.typeValues, runtime.domain.Registry(), frame.values[rootIndex], pathdom.Path{Symbol: f.arraySymbol, Segments: f.array.segments}, f.project)
}

func applyBranchCoordinateMutationFactor(runtime branchAtomFactorRuntime, current BranchRelationFactorFrame, mutation state.CoordinateBranchMutation) (BranchRelationFactorPatch, bool, error) {
	groupIndex := -1
	scalarIndex := -1
	for index, group := range current.plan.layout.currentCoordinates {
		for scalar, slot := range group.slots {
			equal, err := runtime.domain.CoordinateSlotEqual(slot, mutation.Slot())
			if err != nil {
				return BranchRelationFactorPatch{}, false, err
			}
			if equal {
				groupIndex, scalarIndex = index, scalar
				break
			}
		}
	}
	if groupIndex < 0 || scalarIndex < 0 {
		return BranchRelationFactorPatch{}, false, fmt.Errorf("factapply: index-in-range ceiling coordinate is absent")
	}
	group := current.coordinates[groupIndex]
	nextSkeleton, nextScalar, err := runtime.domain.ApplyCoordinateBranchMutation(mutation, group.Skeleton, group.Scalars[scalarIndex])
	if err != nil {
		return BranchRelationFactorPatch{}, false, err
	}
	coordinates := cloneBranchCoordinateOperands(current.coordinates)
	coordinates[groupIndex].Skeleton = nextSkeleton
	coordinates[groupIndex].Scalars[scalarIndex] = nextScalar
	return BranchRelationFactorPatch{plan: current.plan, coordinates: coordinates, reachable: current.reachable}, current.reachable, nil
}
