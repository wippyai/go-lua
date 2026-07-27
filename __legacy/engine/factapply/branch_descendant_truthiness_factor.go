package factapply

import (
	"fmt"

	valuerefine "github.com/wippyai/go-lua/__legacy/analysis/domain/value/refinement"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
)

type branchDescendantTruthinessFactor struct {
	path       ResolvedStructuralPath
	root       symbol.ID
	wantTruthy bool
	typeValues *typevalue.Cache
	project    PathTypeProjector
}

type branchResolvedPathReader struct {
	domain state.ProductDomain
	keys   *keyspace.KeySpace
	root   symbol.ID
	value  product.Value
	lanes  map[state.LaneID]state.LaneFactor
}

func (r branchResolvedPathReader) ReadRootValue(root symbol.ID) (product.Value, bool) {
	return r.value, root != 0 && root == r.root
}

func (r branchResolvedPathReader) ReadLocalPathValue(path keyspace.Key) (product.Value, bool) {
	family, ok := r.domain.PathValueFamily()
	if !ok {
		return product.Value{}, false
	}
	factor, ok := r.lanes[family.Lane().ID()]
	if !ok {
		return product.Value{}, false
	}
	value, present, err := r.domain.ReadPathValueFactor(factor, r.keys, path)
	return value, present && err == nil
}

func (r branchResolvedPathReader) ReadDynamicIndexTable(table keyspace.Key) (state.DynamicIndexTableEvidence, bool) {
	factor, ok := r.lanes[state.LaneDynamicIndex]
	if !ok {
		return state.DynamicIndexTableEvidence{}, false
	}
	evidence, err := r.domain.ObserveDynamicIndexTableFactor(factor, table)
	return evidence, err == nil
}

func (r branchResolvedPathReader) ReadHeapObject(term identity.Term) (heapidentity.TableObject, bool) {
	factor, ok := r.lanes[state.LaneHeapTableIdentity]
	if !ok {
		return heapidentity.TableObject{}, false
	}
	object, err := r.domain.ReadHeapTableObjectTermFactor(factor, term)
	return object, err == nil
}

func branchDescendantTruthinessKernel(factor branchDescendantTruthinessFactor) branchAtomFactorKernel {
	return func(runtime branchAtomFactorRuntime, original, current BranchRelationFactorFrame) (BranchRelationFactorPatch, bool, error) {
		if original.plan == nil || current.plan != original.plan || len(original.values) != 1 {
			return BranchRelationFactorPatch{}, false, fmt.Errorf("factapply: descendant truthiness operands are incomplete")
		}
		reader := branchResolvedPathReader{
			domain: runtime.domain, keys: factor.path.keys, root: factor.root, value: original.values[0],
			lanes: make(map[state.LaneID]state.LaneFactor, len(original.lanes)),
		}
		for _, lane := range original.lanes {
			reader.lanes[lane.Lane().ID()] = lane
		}
		value, resolved := ResolveStructuralPathFactorValue(runtime.domain.Registry(), reader, factor.path)
		if !resolved && factor.project != nil {
			rootType, structural := typevalue.StructuralTypeOf(
				runtime.domain.Registry(), factor.typeValues, original.values[0],
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
				factor.typeValues, runtime.domain.Registry(), original.values[0],
				pathdom.Path{Symbol: factor.root, Segments: factor.path.segments}, factor.project,
			)
		}
		feasible := current.reachable
		if resolved && !product.Equal(runtime.domain.Registry(), value, product.Bottom(runtime.domain.Registry())) {
			if factor.wantTruthy {
				feasible = feasible && valuerefine.CanBeTruthy(runtime.domain.Registry(), value)
			} else {
				feasible = feasible && valuerefine.CanBeFalsy(runtime.domain.Registry(), value)
			}
		}
		return BranchRelationFactorPatch{plan: current.plan, reachable: feasible}, feasible, nil
	}
}
