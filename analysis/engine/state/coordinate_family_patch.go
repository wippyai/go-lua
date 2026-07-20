package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

// ReplaceCoordinateFamily installs one complete explicitly stored family
// image while preserving every sibling family in the same physical lane.
// Unlike PatchCoordinateFamily, omission is semantic: a coordinate absent
// from image is removed. Full-result evaluators must use this operation so a
// deletion cannot be mistaken for sparse structural carry.
func (d ProductDomain) ReplaceCoordinateFamily(
	current LaneFactor,
	skeleton CoordinateFamilySkeleton,
	image []CoordinateScalarFactor,
) (LaneFactor, error) {
	runtime, coordinate, err := d.validateCoordinateFamilyFactor(current, skeleton.family)
	if err != nil {
		return LaneFactor{}, err
	}
	if err := d.validateCoordinateSkeletonFor(coordinate, skeleton, skeleton.keys); err != nil {
		return LaneFactor{}, err
	}
	entries := make([]coordinateEntry, len(image))
	for index, scalar := range image {
		if err := d.validateCoordinateFactorFor(coordinate, scalar, skeleton.keys); err != nil {
			return LaneFactor{}, err
		}
		if index != 0 && !coordinate.ops.keyLess(image[index-1].slot.key, scalar.slot.key, skeleton.keys) {
			return LaneFactor{}, fmt.Errorf("%w: coordinate family replacement is not in strict key order", ErrInvalidLaneFactor)
		}
		support := coordinate.ops.scalarSupport(skeleton.payload, scalar.slot.key)
		if !support.valid() || support == CoordinateScalarForbidden {
			return LaneFactor{}, fmt.Errorf("%w: coordinate family replacement writes outside its skeleton", ErrInvalidLaneFactor)
		}
		entries[index] = coordinateEntry{key: scalar.slot.key, scalar: scalar.payload}
	}
	payload, err := coordinate.ops.replace(current.payload, skeleton.keys, skeleton.payload, entries)
	if err != nil || payload == nil {
		if err == nil {
			err = ErrInvalidLaneFactor
		}
		return LaneFactor{}, fmt.Errorf("%w: coordinate family replacement: %v", ErrInvalidLaneFactor, err)
	}
	return LaneFactor{lane: runtime.lane, payload: payload}, nil
}

// PatchCoordinateFamily installs one already-evaluated family skeleton and
// exact scalar writes into current. Every sibling family in the same lane is
// preserved physically. This is the publication inverse of sparse coordinate
// evaluation; it never scans the ProductDomain inventory or reconstructs a
// whole State.
func (d ProductDomain) PatchCoordinateFamily(
	current LaneFactor,
	skeleton CoordinateFamilySkeleton,
	writes []CoordinateScalarFactor,
) (LaneFactor, error) {
	runtime, coordinate, err := d.validateCoordinateFamilyFactor(current, skeleton.family)
	if err != nil {
		return LaneFactor{}, err
	}
	if err := d.validateCoordinateSkeletonFor(coordinate, skeleton, skeleton.keys); err != nil {
		return LaneFactor{}, err
	}
	_, entries, err := coordinate.ops.decompose(current.payload, skeleton.keys)
	if err != nil {
		return LaneFactor{}, fmt.Errorf("%w: coordinate family patch decomposition", ErrInvalidLaneFactor)
	}
	entries, err = d.patchCoordinateFamilyEntries(coordinate, skeleton, entries, writes, false)
	if err != nil {
		return LaneFactor{}, err
	}
	payload, err := coordinate.ops.replace(current.payload, skeleton.keys, skeleton.payload, entries)
	if err != nil || payload == nil {
		if err == nil {
			err = ErrInvalidLaneFactor
		}
		return LaneFactor{}, fmt.Errorf("%w: coordinate family patch composition: %v", ErrInvalidLaneFactor, err)
	}
	return LaneFactor{lane: runtime.lane, payload: payload}, nil
}

// ReconcileCoordinateFamily installs a changed family skeleton, preserves
// every still-supported sibling scalar, removes scalars forbidden by the new
// skeleton, and then applies the declared writes. It is the canonical
// publication law for a partial evaluator that owns the skeleton but not the
// family's complete scalar image.
func (d ProductDomain) ReconcileCoordinateFamily(
	current LaneFactor,
	skeleton CoordinateFamilySkeleton,
	writes []CoordinateScalarFactor,
) (LaneFactor, error) {
	runtime, coordinate, err := d.validateCoordinateFamilyFactor(current, skeleton.family)
	if err != nil {
		return LaneFactor{}, err
	}
	if err := d.validateCoordinateSkeletonFor(coordinate, skeleton, skeleton.keys); err != nil {
		return LaneFactor{}, err
	}
	_, prior, err := coordinate.ops.decompose(current.payload, skeleton.keys)
	if err != nil {
		return LaneFactor{}, fmt.Errorf("%w: coordinate family reconciliation decomposition", ErrInvalidLaneFactor)
	}
	entries, err := d.patchCoordinateFamilyEntries(coordinate, skeleton, prior, writes, true)
	if err != nil {
		return LaneFactor{}, err
	}
	payload, err := coordinate.ops.replace(current.payload, skeleton.keys, skeleton.payload, entries)
	if err != nil || payload == nil {
		if err == nil {
			err = ErrInvalidLaneFactor
		}
		return LaneFactor{}, fmt.Errorf("%w: coordinate family reconciliation: %v", ErrInvalidLaneFactor, err)
	}
	return LaneFactor{lane: runtime.lane, payload: payload}, nil
}

// PatchCoordinateFamilyFactor applies the same canonical sparse family law as
// PatchCoordinateFamily without materializing sibling families in the shared
// physical lane. It is the factor-native publication seam for tuple engines.
func (d ProductDomain) PatchCoordinateFamilyFactor(
	current CoordinateFamilyFactor,
	skeleton CoordinateFamilySkeleton,
	writes []CoordinateScalarFactor,
) (CoordinateFamilyFactor, error) {
	return d.patchExclusiveCoordinateFamilyFactor(current, skeleton, writes, false)
}

// ReconcileCoordinateFamilyFactor applies the same canonical topology-change
// law as ReconcileCoordinateFamily to one family-exclusive factor.
func (d ProductDomain) ReconcileCoordinateFamilyFactor(
	current CoordinateFamilyFactor,
	skeleton CoordinateFamilySkeleton,
	writes []CoordinateScalarFactor,
) (CoordinateFamilyFactor, error) {
	return d.patchExclusiveCoordinateFamilyFactor(current, skeleton, writes, true)
}

func (d ProductDomain) patchExclusiveCoordinateFamilyFactor(
	current CoordinateFamilyFactor,
	skeleton CoordinateFamilySkeleton,
	writes []CoordinateScalarFactor,
	reconcile bool,
) (CoordinateFamilyFactor, error) {
	coordinate, err := d.validateCoordinateSkeleton(current.skeleton)
	if err != nil || current.skeleton.family != skeleton.family || current.skeleton.keys != skeleton.keys {
		if err != nil {
			return CoordinateFamilyFactor{}, err
		}
		return CoordinateFamilyFactor{}, fmt.Errorf("%w: coordinate family factor patch ownership", ErrInvalidLaneFactor)
	}
	if err := d.validateCoordinateSkeletonFor(coordinate, skeleton, skeleton.keys); err != nil {
		return CoordinateFamilyFactor{}, err
	}
	entries, err := d.explicitCoordinateEntries(coordinate, current.skeleton, current.scalars)
	if err != nil {
		return CoordinateFamilyFactor{}, err
	}
	entries, err = d.patchCoordinateFamilyEntries(coordinate, skeleton, entries, writes, reconcile)
	if err != nil {
		return CoordinateFamilyFactor{}, err
	}
	scalars := make([]CoordinateScalarFactor, len(entries))
	for index, entry := range entries {
		scalars[index] = CoordinateScalarFactor{
			slot:    CoordinateSlot{family: coordinate.family, keys: skeleton.keys, key: entry.key},
			payload: entry.scalar,
		}
	}
	return CoordinateFamilyFactor{skeleton: skeleton, scalars: scalars}, nil
}

// patchCoordinateFamilyEntries is the single scalar-image law shared by
// physical LaneFactor publication and family-native tuple publication.
func (d ProductDomain) patchCoordinateFamilyEntries(
	coordinate *coordinateFamilyRuntime,
	skeleton CoordinateFamilySkeleton,
	prior []coordinateEntry,
	writes []CoordinateScalarFactor,
	reconcile bool,
) ([]coordinateEntry, error) {
	entries := append([]coordinateEntry(nil), prior...)
	if reconcile {
		entries = entries[:0]
		for _, entry := range prior {
			support := coordinate.ops.scalarSupport(skeleton.payload, entry.key)
			if !support.valid() {
				return nil, fmt.Errorf("%w: coordinate family reconciliation support", ErrInvalidLaneFactor)
			}
			if support != CoordinateScalarForbidden {
				entries = append(entries, entry)
			}
		}
	}
	for _, write := range writes {
		if err := d.validateCoordinateFactorFor(coordinate, write, skeleton.keys); err != nil {
			return nil, err
		}
		position, found := coordinateEntryPosition(coordinate, entries, write.slot.key, skeleton.keys)
		support := coordinate.ops.scalarSupport(skeleton.payload, write.slot.key)
		if !support.valid() || !reconcile && support == CoordinateScalarForbidden {
			return nil, fmt.Errorf("%w: coordinate family patch write support", ErrInvalidLaneFactor)
		}
		if support == CoordinateScalarForbidden {
			if found {
				entries = append(entries[:position], entries[position+1:]...)
			}
			continue
		}
		defaultScalar, defaultErr := coordinate.ops.defaultScalar(skeleton.payload, write.slot.key)
		if defaultErr != nil || defaultScalar == nil {
			return nil, fmt.Errorf("%w: coordinate family patch default", ErrInvalidLaneFactor)
		}
		omitted := support == CoordinateScalarOptional && coordinate.ops.scalarEqual(write.payload, defaultScalar)
		switch {
		case omitted && found:
			entries = append(entries[:position], entries[position+1:]...)
		case omitted:
		case found:
			entries[position].scalar = write.payload
		default:
			entries = append(entries, coordinateEntry{})
			copy(entries[position+1:], entries[position:])
			entries[position] = coordinateEntry{key: write.slot.key, scalar: write.payload}
		}
	}
	return entries, nil
}

func coordinateEntryPosition(runtime *coordinateFamilyRuntime, entries []coordinateEntry, key coordinateKeyPayload, keys *keyspace.KeySpace) (int, bool) {
	position := len(entries)
	for index, entry := range entries {
		if runtime.ops.keyLess(entry.key, key, keys) {
			continue
		}
		position = index
		return position, runtime.ops.keyEqual(entry.key, key)
	}
	return position, false
}
