package state

import (
	"fmt"

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
)

// TypestateQueryCapability is the registered composite read law for lifecycle
// observations. Resource identity is a quotient by path equality, so reading a
// typestate slot is mathematically Typestate x PathEquality rather than a
// typestate-only operation.
type TypestateQueryCapability struct {
	seal      *productDomainSeal
	keys      *keyspace.KeySpace
	typestate ProductLane
	path      ProductLane
}

func (c TypestateQueryCapability) ValidFor(d ProductDomain) bool {
	return d.Valid() && c.seal == d.seal && c.keys != nil && c.keys.Valid() &&
		c.typestate.ID() == LaneTypestates && c.path.ID() == LanePathEvidence
}

// Lanes is the exact registered product footprint of the composite query.
func (c TypestateQueryCapability) Lanes() LaneSet {
	if c.seal == nil {
		return LaneSet{}
	}
	return NewLaneSet(c.typestate.ID(), c.path.ID())
}

func (c TypestateQueryCapability) TypestateLane() ProductLane { return c.typestate }
func (c TypestateQueryCapability) PathEqualityLane() ProductLane {
	return c.path
}

// SealTypestateQueryCapability derives the composite from lane registration;
// callers cannot spell or omit either factor manually.
func (d ProductDomain) SealTypestateQueryCapability(keys *keyspace.KeySpace) (TypestateQueryCapability, error) {
	if !d.Valid() || keys == nil || !keys.Valid() {
		return TypestateQueryCapability{}, fmt.Errorf("%w: typestate query is unowned", ErrInvalidLaneFactor)
	}
	typestateLane, hasTypestate := d.ProductLane(LaneTypestates)
	pathFamily, hasPath := d.PathValueFamily()
	if !hasTypestate || !hasPath || pathFamily.Lane().ID() != LanePathEvidence {
		return TypestateQueryCapability{}, fmt.Errorf("%w: typestate query factors are not registered", ErrInvalidLaneFactor)
	}
	return TypestateQueryCapability{seal: d.seal, keys: keys, typestate: typestateLane, path: pathFamily.Lane()}, nil
}

func (d ProductDomain) validateTypestateQueryFactors(
	capability TypestateQueryCapability,
	typestateFactor, pathFactor LaneFactor,
) (typestate.Store, error) {
	if !capability.ValidFor(d) {
		return typestate.Store{}, fmt.Errorf("%w: foreign typestate query capability", ErrInvalidLaneFactor)
	}
	typestateRuntime, err := d.validateFactor(typestateFactor)
	if err != nil || typestateRuntime.lane != capability.typestate {
		return typestate.Store{}, fmt.Errorf("%w: invalid typestate query factor", ErrInvalidLaneFactor)
	}
	pathRuntime, err := d.validateFactor(pathFactor)
	if err != nil || pathRuntime.lane != capability.path {
		return typestate.Store{}, fmt.Errorf("%w: invalid typestate path-equality factor", ErrInvalidLaneFactor)
	}
	return typedLaneFactorValue[typestate.Store](typestateFactor.payload), nil
}

// CanonicalTypestateResourceFactor resolves one resource through the exact
// registered path-equality factor, then observes its typestate slot.
func (d ProductDomain) CanonicalTypestateResourceFactor(
	capability TypestateQueryCapability,
	typestateFactor, pathFactor LaneFactor,
	target pathaddr.StateKey,
	protocol typestate.Protocol,
) (typestate.Resource, typestate.Slot, bool, error) {
	store, err := d.validateTypestateQueryFactors(capability, typestateFactor, pathFactor)
	if err != nil {
		return typestate.Resource{}, typestate.Slot{}, false, err
	}
	canonical := fieldCanonicalTypestatePathKey(capability.keys, target.PathKey())
	path, ok := capability.keys.FromStateKey(target.PathKey())
	if !ok {
		return typestate.Resource{}, typestate.Slot{}, false, fmt.Errorf("%w: unresolved typestate target", ErrInvalidLaneFactor)
	}
	equivalent, err := d.EquivalentPathStateKeysFactor(pathFactor, capability.keys, path)
	if err != nil {
		return typestate.Resource{}, typestate.Slot{}, false, err
	}
	for _, candidate := range equivalent {
		formatted := fieldCanonicalTypestatePathKey(capability.keys, candidate.PathKey())
		if formatted != "" && (canonical == "" || formatted < canonical) {
			canonical = formatted
		}
	}
	canonicalKey, ok := pathaddr.StateKeyFromPathKey(canonical)
	if !ok {
		return typestate.Resource{}, typestate.Slot{}, false, fmt.Errorf("%w: malformed canonical typestate target", ErrInvalidLaneFactor)
	}
	resource := TypestateResourceFromCanonicalKey(canonicalKey, protocol)
	slot, found := store.Lookup(resource)
	return resource, slot, found, nil
}

// OpenTypestateObligationsFactor observes the typestate half of the same
// capability. Requiring the composite capability prevents a caller from later
// matching those resources through an undeclared path-equality read.
func (d ProductDomain) OpenTypestateObligationsFactor(
	capability TypestateQueryCapability,
	typestateFactor, pathFactor LaneFactor,
) ([]typestate.OpenObligation, error) {
	store, err := d.validateTypestateQueryFactors(capability, typestateFactor, pathFactor)
	if err != nil {
		return nil, err
	}
	return store.OpenObligations(), nil
}
