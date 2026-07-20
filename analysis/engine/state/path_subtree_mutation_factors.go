package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

// PathSubtreeMutationFactorTopology is the unique factor spelling of a
// destructive subtree mutation. Coordinate-backed participants are named by
// family; only participants without a coordinate implementation are named by
// whole lane. The two inventories are therefore disjoint by construction.
type PathSubtreeMutationFactorTopology struct {
	seal     *productDomainSeal
	lanes    []ProductLane
	families []CoordinateFamily
}

// SealPathSubtreeMutationFactorTopology derives the complete partition from
// ProductDomain registration. Adding a family or lane changes this topology
// without changing any mutation consumer.
func (d ProductDomain) SealPathSubtreeMutationFactorTopology() (PathSubtreeMutationFactorTopology, error) {
	if !d.Valid() {
		return PathSubtreeMutationFactorTopology{}, fmt.Errorf("%w: invalid path-subtree topology domain", ErrInvalidProductLane)
	}
	owner, hasOwner := d.PathValueFamily()
	out := PathSubtreeMutationFactorTopology{seal: d.seal}
	for laneIndex := range d.factorLanes {
		runtime := &d.factorLanes[laneIndex]
		factored := false
		for familyIndex := range runtime.coordinates {
			family := &runtime.coordinates[familyIndex]
			if !family.ops.pathMutation.participates && (!hasOwner || family.family != owner) {
				continue
			}
			out.families = append(out.families, family.family)
			factored = true
		}
		law, declared := findLaneSemanticLaw(runtime.semanticLaws, laneSemanticPathSubtreeMutation)
		if declared && law.participates && !factored {
			out.lanes = append(out.lanes, runtime.lane)
		}
	}
	if !hasOwner {
		return PathSubtreeMutationFactorTopology{}, fmt.Errorf("%w: path-subtree topology has no path-value owner", ErrInvalidLaneFactor)
	}
	return out, nil
}

func (t PathSubtreeMutationFactorTopology) ValidFor(d ProductDomain) bool {
	return d.Valid() && t.seal != nil && t.seal == d.seal
}

func (t PathSubtreeMutationFactorTopology) Lanes() []ProductLane {
	return append([]ProductLane(nil), t.lanes...)
}

func (t PathSubtreeMutationFactorTopology) Families() []CoordinateFamily {
	return append([]CoordinateFamily(nil), t.families...)
}

// PathSubtreeMutationFactors is one topology-ordered product factor. A
// participating semantic axis occurs in exactly one of lanes or coordinates.
type PathSubtreeMutationFactors struct {
	seal        *productDomainSeal
	lanes       []LaneFactor
	coordinates []CoordinateFamilyFactor
}

// SealPathSubtreeMutationFactors binds the exact topology-derived tuple.
func (d ProductDomain) SealPathSubtreeMutationFactors(lanes []LaneFactor, coordinates []CoordinateFamilyFactor) (PathSubtreeMutationFactors, error) {
	topology, err := d.SealPathSubtreeMutationFactorTopology()
	if err != nil {
		return PathSubtreeMutationFactors{}, err
	}
	requiredLanes, requiredFamilies := topology.Lanes(), topology.Families()
	if len(lanes) != len(requiredLanes) || len(coordinates) != len(requiredFamilies) {
		return PathSubtreeMutationFactors{}, fmt.Errorf("%w: incomplete path-subtree mutation tuple", ErrIncompleteLaneFactors)
	}
	out := PathSubtreeMutationFactors{
		seal: d.seal, lanes: make([]LaneFactor, len(lanes)), coordinates: make([]CoordinateFamilyFactor, len(coordinates)),
	}
	for index, lane := range requiredLanes {
		factor := lanes[index]
		runtime, factorErr := d.validateFactor(factor)
		if factorErr != nil || factor.lane != lane || runtime.lane != lane {
			return PathSubtreeMutationFactors{}, fmt.Errorf("%w: path-subtree mutation lane %d", ErrInvalidLaneFactor, index)
		}
		out.lanes[index] = factor
	}
	for index, family := range requiredFamilies {
		factor := coordinates[index]
		if factor.Family() != family {
			return PathSubtreeMutationFactors{}, fmt.Errorf("%w: path-subtree mutation coordinate %d", ErrInvalidLaneFactor, index)
		}
		sealed, factorErr := d.SealCoordinateFamilyFactor(factor.skeleton, factor.scalars)
		if factorErr != nil {
			return PathSubtreeMutationFactors{}, factorErr
		}
		out.coordinates[index] = sealed
	}
	return out, nil
}

// BindPathSubtreeMutationFactors transposes a caller-owned lane inventory at
// the edge. The retained tuple contains no whole-lane spelling for a factored
// family and execution never reconstructs State.
func (d ProductDomain) BindPathSubtreeMutationFactors(keys *keyspace.KeySpace, lookup func(ProductLane) (LaneFactor, bool)) (PathSubtreeMutationFactors, error) {
	if keys == nil || !keys.Valid() || lookup == nil {
		return PathSubtreeMutationFactors{}, fmt.Errorf("%w: invalid path-subtree mutation binding", ErrInvalidLaneFactor)
	}
	topology, err := d.SealPathSubtreeMutationFactorTopology()
	if err != nil {
		return PathSubtreeMutationFactors{}, err
	}
	lanes := make([]LaneFactor, len(topology.lanes))
	for index, lane := range topology.lanes {
		factor, present := lookup(lane)
		if !present {
			return PathSubtreeMutationFactors{}, fmt.Errorf("%w: path-subtree lane %s", ErrIncompleteLaneFactors, lane.ID())
		}
		lanes[index] = factor
	}
	coordinates := make([]CoordinateFamilyFactor, len(topology.families))
	for index, family := range topology.families {
		factor, present := lookup(family.Lane())
		if !present {
			return PathSubtreeMutationFactors{}, fmt.Errorf("%w: path-subtree coordinate family %s", ErrIncompleteLaneFactors, family.ID())
		}
		skeleton, scalars, factorErr := d.DecomposeCoordinateFamily(factor, family, keys)
		if factorErr != nil {
			return PathSubtreeMutationFactors{}, factorErr
		}
		coordinates[index], factorErr = d.SealCoordinateFamilyFactor(skeleton, scalars)
		if factorErr != nil {
			return PathSubtreeMutationFactors{}, factorErr
		}
	}
	return d.SealPathSubtreeMutationFactors(lanes, coordinates)
}

// ApplyPathSubtreeMutationFactors applies the registry-derived product law in
// one transaction. Failure returns no partially updated tuple.
func (d ProductDomain) ApplyPathSubtreeMutationFactors(transaction PathSubtreeMutation, current PathSubtreeMutationFactors) (PathSubtreeMutationFactors, error) {
	if !d.ownsPathSubtreeMutation(transaction) || current.seal == nil || current.seal != d.seal {
		return PathSubtreeMutationFactors{}, fmt.Errorf("state: foreign path-subtree mutation factors")
	}
	topology, err := d.SealPathSubtreeMutationFactorTopology()
	if err != nil || len(current.lanes) != len(topology.lanes) || len(current.coordinates) != len(topology.families) {
		return PathSubtreeMutationFactors{}, fmt.Errorf("%w: malformed path-subtree mutation factors", ErrIncompleteLaneFactors)
	}
	out := PathSubtreeMutationFactors{
		seal: d.seal, lanes: append([]LaneFactor(nil), current.lanes...), coordinates: cloneCoordinateFamilyFactors(current.coordinates),
	}
	for index, lane := range topology.lanes {
		if out.lanes[index].Lane() != lane {
			return PathSubtreeMutationFactors{}, fmt.Errorf("%w: path-subtree lane %d", ErrInvalidLaneFactor, index)
		}
		out.lanes[index], err = d.applyPathSubtreeMutationFactor(transaction, out.lanes[index])
		if err != nil {
			return PathSubtreeMutationFactors{}, err
		}
	}
	for index, family := range topology.families {
		factor := out.coordinates[index]
		if factor.Family() != family {
			return PathSubtreeMutationFactors{}, fmt.Errorf("%w: path-subtree coordinate %d", ErrInvalidLaneFactor, index)
		}
		skeleton, scalars, applyErr := d.applyCoordinatePathSubtreeMutationFactor(transaction, factor.skeleton, factor.scalars)
		if applyErr != nil {
			return PathSubtreeMutationFactors{}, applyErr
		}
		out.coordinates[index], err = d.SealCoordinateFamilyFactor(skeleton, scalars)
		if err != nil {
			return PathSubtreeMutationFactors{}, err
		}
	}
	return out, nil
}

func (f PathSubtreeMutationFactors) LaneFactors() []LaneFactor {
	return append([]LaneFactor(nil), f.lanes...)
}

func (f PathSubtreeMutationFactors) CoordinateFactors() []CoordinateFamilyFactor {
	return cloneCoordinateFamilyFactors(f.coordinates)
}
