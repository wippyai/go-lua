package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

type coordinatePathMutationOps struct {
	participates     bool
	affected         func(coordinateKeyPayload, *keyspace.KeySpace, []CoordinateDependencyLocation) bool
	applySubtree     func(coordinateSkeletonPayload, []coordinateEntry, pathSubtreeMutationRequest) (coordinateSkeletonPayload, []coordinateEntry, bool)
	applyDescendants func(coordinateSkeletonPayload, []coordinateEntry, pathDescendantMutationRequest) (coordinateSkeletonPayload, []coordinateEntry, bool)
}

func noCoordinatePathMutation() coordinatePathMutationOps { return coordinatePathMutationOps{} }
func coordinatePathSubtreeMutation(
	affected func(coordinateKeyPayload, *keyspace.KeySpace, []CoordinateDependencyLocation) bool,
	applySubtree func(coordinateSkeletonPayload, []coordinateEntry, pathSubtreeMutationRequest) (coordinateSkeletonPayload, []coordinateEntry, bool),
	applyDescendants func(coordinateSkeletonPayload, []coordinateEntry, pathDescendantMutationRequest) (coordinateSkeletonPayload, []coordinateEntry, bool),
) coordinatePathMutationOps {
	return coordinatePathMutationOps{
		participates:     true,
		affected:         affected,
		applySubtree:     applySubtree,
		applyDescendants: applyDescendants,
	}
}
func coordinatePathMutationOpsComplete(ops coordinatePathMutationOps) bool {
	return !ops.participates && ops.affected == nil && ops.applySubtree == nil && ops.applyDescendants == nil ||
		ops.participates && ops.affected != nil && ops.applySubtree != nil && ops.applyDescendants != nil
}

// PathDescendantMutationCoordinateFamilies returns every registered factored
// family whose topology/scalars are destructively invalidated by N4 path
// replacement. The path-evidence carrier remains handled by its coupled core.
func (d ProductDomain) PathDescendantMutationCoordinateFamilies() []CoordinateFamily {
	if !d.Valid() {
		return nil
	}
	out := make([]CoordinateFamily, 0, 1)
	pathOwner, hasPathOwner := d.PathEvidenceCoordinateFamily()
	for _, lane := range d.factorLanes {
		for _, family := range lane.coordinates {
			if family.ops.pathMutation.participates && (!hasPathOwner || family.family != pathOwner) {
				out = append(out, family.family)
			}
		}
	}
	return out
}

// SelectPathMutationCoordinateSlots asks one registered participant for the
// exact finite coordinates intersecting a family-owned semantic mutation
// region. A participating family without this law is rejected at domain seal;
// callers never default to the whole family inventory.
func (d ProductDomain) SelectPathMutationCoordinateSlots(
	family CoordinateFamily,
	slots []CoordinateSlot,
	locations []CoordinateDependencyLocation,
) ([]CoordinateSlot, error) {
	coordinate, err := d.validateCoordinateFamily(family)
	if err != nil || !coordinate.ops.pathMutation.participates || coordinate.ops.pathMutation.affected == nil {
		return nil, fmt.Errorf("state: coordinate family has no path-mutation selection law")
	}
	out := make([]CoordinateSlot, 0)
	for _, slot := range slots {
		if err := d.validateCoordinateSlotFor(coordinate, slot, slot.keys); err != nil || slot.family != family {
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("state: foreign path-mutation coordinate slot")
		}
		if coordinate.ops.pathMutation.affected(slot.key, slot.keys, locations) {
			out = append(out, slot)
		}
	}
	return out, nil
}

func (d ProductDomain) applyCoordinatePathSubtreeMutation(transaction PathSubtreeMutation, skeleton CoordinateFamilySkeleton, scalars []CoordinateScalarFactor) (CoordinateFamilySkeleton, []CoordinateScalarFactor, error) {
	coordinate, err := d.validateCoordinateSkeleton(skeleton)
	if err != nil || !coordinate.ops.pathMutation.participates {
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("state: coordinate family does not own path mutation")
	}
	entries, err := d.explicitCoordinateEntries(coordinate, skeleton, scalars)
	if err != nil {
		return CoordinateFamilySkeleton{}, nil, err
	}
	nextSkeleton, nextEntries, ok := coordinate.ops.pathMutation.applySubtree(skeleton.payload, entries, pathSubtreeMutationRequest{keys: transaction.keys, prefixes: transaction.prefixes, path: transaction.path})
	if !ok || nextSkeleton == nil {
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("state: coordinate family rejected path mutation")
	}
	out := make([]CoordinateScalarFactor, len(nextEntries))
	for index, entry := range nextEntries {
		if entry.key == nil || entry.scalar == nil || !coordinate.ops.keyValid(entry.key, skeleton.keys) || !coordinate.ops.scalarValid(entry.key, entry.scalar) || index != 0 && !coordinate.ops.keyLess(nextEntries[index-1].key, entry.key, skeleton.keys) {
			return CoordinateFamilySkeleton{}, nil, ErrInvalidLaneFactor
		}
		out[index] = CoordinateScalarFactor{slot: CoordinateSlot{family: skeleton.family, keys: skeleton.keys, key: entry.key}, payload: entry.scalar}
	}
	return CoordinateFamilySkeleton{family: skeleton.family, keys: skeleton.keys, payload: nextSkeleton}, out, nil
}

func (d ProductDomain) applyCoordinatePathDescendantMutation(transaction PathDescendantMutation, skeleton CoordinateFamilySkeleton, scalars []CoordinateScalarFactor) (CoordinateFamilySkeleton, []CoordinateScalarFactor, error) {
	coordinate, err := d.validateCoordinateSkeleton(skeleton)
	if err != nil || !coordinate.ops.pathMutation.participates || coordinate.ops.pathMutation.applyDescendants == nil {
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("state: coordinate family does not own path descendant mutation")
	}
	entries, err := d.explicitCoordinateEntries(coordinate, skeleton, scalars)
	if err != nil {
		return CoordinateFamilySkeleton{}, nil, err
	}
	nextSkeleton, nextEntries, ok := coordinate.ops.pathMutation.applyDescendants(
		skeleton.payload,
		entries,
		pathDescendantMutationRequest{keys: transaction.keys, prefixes: transaction.prefixes, path: transaction.path},
	)
	if !ok || nextSkeleton == nil {
		return CoordinateFamilySkeleton{}, nil, fmt.Errorf("state: coordinate family rejected path descendant mutation")
	}
	out := make([]CoordinateScalarFactor, len(nextEntries))
	for index, entry := range nextEntries {
		if entry.key == nil || entry.scalar == nil || !coordinate.ops.keyValid(entry.key, skeleton.keys) || !coordinate.ops.scalarValid(entry.key, entry.scalar) || index != 0 && !coordinate.ops.keyLess(nextEntries[index-1].key, entry.key, skeleton.keys) {
			return CoordinateFamilySkeleton{}, nil, ErrInvalidLaneFactor
		}
		out[index] = CoordinateScalarFactor{slot: CoordinateSlot{family: skeleton.family, keys: skeleton.keys, key: entry.key}, payload: entry.scalar}
	}
	return CoordinateFamilySkeleton{family: skeleton.family, keys: skeleton.keys, payload: nextSkeleton}, out, nil
}
