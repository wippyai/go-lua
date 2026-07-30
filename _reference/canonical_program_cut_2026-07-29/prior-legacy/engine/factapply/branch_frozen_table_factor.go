package factapply

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// branchFrozenTableFactor is the carrier-neutral frozen-table evidence law.
// Path resolution is the same registered query composition used by other
// branch factors; publication touches only the registered must-set lane.
type branchFrozenTableFactor struct {
	path       ResolvedStructuralPath
	root       symbol.ID
	typeValues *typevalue.Cache
	project    PathTypeProjector
}

func branchFrozenTableKernel(factor branchFrozenTableFactor) branchAtomFactorKernel {
	return func(runtime branchAtomFactorRuntime, _ BranchRelationFactorFrame, current BranchRelationFactorFrame) (BranchRelationFactorPatch, bool, error) {
		if current.plan == nil || len(current.values) != 1 || !current.reachable {
			return BranchRelationFactorPatch{plan: current.plan}, false, nil
		}
		reader := branchResolvedPathReader{
			domain: runtime.domain, keys: factor.path.keys, root: factor.root, value: current.values[0],
			lanes: make(map[state.LaneID]state.LaneFactor, len(current.lanes)),
		}
		for _, lane := range current.lanes {
			reader.lanes[lane.Lane().ID()] = lane
		}
		value, resolved := ResolveStructuralPathFactorValue(runtime.domain.Registry(), reader, factor.path)
		if !resolved && factor.project != nil {
			rootType, structural := typevalue.StructuralTypeOf(
				runtime.domain.Registry(), factor.typeValues, current.values[0],
				typevalue.StructuralTypeOptions{ApplyPresence: true},
			)
			if structural {
				if projected, ok := factor.project(rootType, pathdom.Path{Symbol: factor.root, Segments: factor.path.segments}); ok {
					value, resolved = projectedPathValue(runtime.domain.Registry(), factor.typeValues, projected), true
				}
			}
		}
		if !resolved {
			value, resolved = projectPathOriginFromRoot(
				factor.typeValues, runtime.domain.Registry(), current.values[0],
				pathdom.Path{Symbol: factor.root, Segments: factor.path.segments}, factor.project,
			)
		}
		id, hasIdentity := product.Get(runtime.domain.Registry(), value, identity.Key).ID()
		lanes := append([]state.LaneFactor(nil), current.lanes...)
		if resolved && hasIdentity {
			found := false
			for index, lane := range lanes {
				if lane.Lane().ID() != state.LaneFrozenTables {
					continue
				}
				var err error
				lanes[index], err = runtime.domain.ApplyCallOutcomeFrozenTableFactor(lane, id)
				if err != nil {
					return BranchRelationFactorPatch{}, false, err
				}
				found = true
				break
			}
			if !found {
				return BranchRelationFactorPatch{}, false, fmt.Errorf("factapply: frozen-table lane is absent")
			}
		}
		return BranchRelationFactorPatch{
			plan: current.plan, lanes: lanes,
			coordinates: cloneBranchCoordinateOperands(current.coordinates), reachable: current.reachable,
		}, current.reachable, nil
	}
}
