package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

// LenFloorCoordinateFamily returns the registration-owned LenFloor family.
func (d ProductDomain) LenFloorCoordinateFamily() (CoordinateFamily, bool) {
	if !d.Valid() {
		return CoordinateFamily{}, false
	}
	var out CoordinateFamily
	found := false
	for _, lane := range d.factorLanes {
		for _, family := range lane.coordinates {
			if family.ops.rootAssignment.lenFloorValue != nil {
				if found {
					return CoordinateFamily{}, false
				}
				out, found = family.family, true
			}
		}
	}
	return out, found
}

// LenFloorCoordinateSlot seals one exact path in the canonical LenFloor
// coordinate family.
func (d ProductDomain) LenFloorCoordinateSlot(keys *keyspace.KeySpace, path keyspace.Key) (CoordinateSlot, error) {
	family, ok := d.LenFloorCoordinateFamily()
	if !ok {
		return CoordinateSlot{}, ErrInvalidLaneFactor
	}
	coordinate, err := d.validateCoordinateFamily(family)
	key := wrapLenFloorCoordinateKey(path)
	if err != nil || !coordinate.ops.keyValid(key, keys) {
		return CoordinateSlot{}, fmt.Errorf("%w: invalid LenFloor coordinate", ErrInvalidLaneFactor)
	}
	return CoordinateSlot{family: family, keys: keys, key: key}, nil
}

// LenFloorCoordinateValue observes positive length evidence. Top/no-evidence
// and Bottom defaults are not reported as facts.
func (d ProductDomain) LenFloorCoordinateValue(value CoordinateScalarFactor) (int64, bool, error) {
	family, ok := d.LenFloorCoordinateFamily()
	coordinate, err := d.validateCoordinateFamily(family)
	if !ok || err != nil || value.slot.family != family || d.validateCoordinateFactorFor(coordinate, value, value.slot.keys) != nil {
		return 0, false, ErrInvalidLaneFactor
	}
	floor, present := coordinate.ops.rootAssignment.lenFloorValue(value.payload)
	return floor, present, nil
}

func (d ProductDomain) pathEvidenceValueCoordinateSlot(keys *keyspace.KeySpace, path keyspace.Key, static bool) (CoordinateSlot, error) {
	family, ok := d.PathEvidenceCoordinateFamily()
	if !ok || keys == nil || !keys.Valid() {
		return CoordinateSlot{}, ErrInvalidLaneFactor
	}
	coordinate, err := d.validateCoordinateFamily(family)
	key := pathevidence.RefinementCoordinate(path)
	if static {
		key = pathevidence.StaticMemberCoordinate(path)
	}
	payload := typedCoordinateKeyPayload[pathevidence.CoordinateKey]{value: key}
	if err != nil || !coordinate.ops.keyValid(payload, keys) {
		return CoordinateSlot{}, ErrInvalidLaneFactor
	}
	return CoordinateSlot{family: family, keys: keys, key: payload}, nil
}

func (d ProductDomain) PathRefinementCoordinateSlot(keys *keyspace.KeySpace, path keyspace.Key) (CoordinateSlot, error) {
	return d.pathEvidenceValueCoordinateSlot(keys, path, false)
}

func (d ProductDomain) PathStaticMemberCoordinateSlot(keys *keyspace.KeySpace, path keyspace.Key) (CoordinateSlot, error) {
	return d.pathEvidenceValueCoordinateSlot(keys, path, true)
}

// PathEvidenceCoordinateValue observes an exact present refinement or static
// member scalar without composing the PathEvidence lane.
func (d ProductDomain) PathEvidenceCoordinateValue(value CoordinateScalarFactor) (product.Value, bool, error) {
	family, ok := d.PathEvidenceCoordinateFamily()
	coordinate, err := d.validateCoordinateFamily(family)
	if !ok || err != nil || value.slot.family != family || d.validateCoordinateFactorFor(coordinate, value, value.slot.keys) != nil {
		return product.Value{}, false, ErrInvalidLaneFactor
	}
	key := pathEvidenceCoordinateKey(value.slot.key)
	scalar := pathEvidenceCoordinateScalar(value.payload)
	result, present := pathevidence.CoordinateScalarValue(key, scalar)
	return result, present, nil
}
