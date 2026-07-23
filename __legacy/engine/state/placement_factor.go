package state

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
)

// PlacementSkeletonFactor is the value-less quotient of the identity-indexed
// Placement lane.  A finite pointwise map has no structural payload: absent
// coordinates mean placement.Bottom.  Only map Top remains in the skeleton;
// every finite non-bottom value is owned by one independent PlacementFactor.
type PlacementSkeletonFactor struct {
	seal *productDomainSeal
	lane ProductLane
	top  bool
}

// PlacementFactor is one sealed (identity, placement) coordinate.
type PlacementFactor struct {
	seal  *productDomainSeal
	lane  ProductLane
	id    identity.Term
	value placement.Value
}

// PlacementSlot is a sealed identity coordinate with no value.
type PlacementSlot struct {
	seal *productDomainSeal
	lane ProductLane
	id   identity.Term
}

// Identity returns the placement coordinate.
func (f PlacementFactor) Identity() identity.ID {
	id, _ := f.id.Concrete()
	return id
}

// IdentityTerm returns the complete relational placement coordinate.
func (f PlacementFactor) IdentityTerm() identity.Term { return f.id }

// Value returns the coordinate's placement lattice value.
func (f PlacementFactor) Value() placement.Value { return f.value }

// Slot returns the factor's immutable coordinate.
func (f PlacementFactor) Slot() PlacementSlot {
	return PlacementSlot{seal: f.seal, lane: f.lane, id: f.id}
}

// Identity returns the placement coordinate.
func (s PlacementSlot) Identity() identity.ID {
	id, _ := s.id.Concrete()
	return id
}

// IdentityTerm returns the complete relational placement coordinate.
func (s PlacementSlot) IdentityTerm() identity.Term { return s.id }

// PlacementSlot seals one nonzero identity coordinate without assigning a
// scalar value.  It is the constructor authority for fresh object placement;
// publication still requires BindPlacementValue.
func (d ProductDomain) PlacementSlot(id identity.ID) (PlacementSlot, error) {
	return d.placementTermSlot(identity.ConcreteTerm(id))
}

func (d ProductDomain) placementTermSlot(id identity.Term) (PlacementSlot, error) {
	lane, ok := d.ProductLane(LanePlacement)
	if !ok || !id.Valid() {
		return PlacementSlot{}, fmt.Errorf("%w: placement slot requires enabled lane and nonzero identity", ErrInvalidLaneFactor)
	}
	return PlacementSlot{seal: d.seal, lane: lane, id: id}, nil
}

// DecomposePlacement transposes the monolithic pointwise map into its Top
// skeleton and sorted, independent non-bottom identity coordinates.
func (d ProductDomain) DecomposePlacement(factor LaneFactor) (PlacementSkeletonFactor, []PlacementFactor, error) {
	runtime, err := d.validatePlacementLaneFactor(factor)
	if err != nil {
		return PlacementSkeletonFactor{}, nil, err
	}
	lane := typedLaneFactorValue[placementLane](factor.payload)
	skeleton := PlacementSkeletonFactor{seal: d.seal, lane: runtime.lane, top: lane.top}
	if lane.top {
		return skeleton, nil, nil
	}
	factors := make([]PlacementFactor, 0, len(lane.values))
	for id, value := range lane.values {
		if !id.Valid() || value == placement.Bottom {
			return PlacementSkeletonFactor{}, nil, fmt.Errorf("%w: placement lane has a non-canonical finite coordinate", ErrInvalidLaneFactor)
		}
		factors = append(factors, PlacementFactor{seal: d.seal, lane: runtime.lane, id: id, value: value})
	}
	sort.Slice(factors, func(i, j int) bool { return identityTermLess(factors[i].id, factors[j].id) })
	return skeleton, factors, nil
}

// ReadPlacementTermFactor observes one symbolic or concrete identity directly
// from the registered placement factor.
func (d ProductDomain) ReadPlacementTermFactor(factor LaneFactor, id identity.Term) (placement.Value, error) {
	if !id.Valid() {
		return placement.Bottom, ErrInvalidLaneFactor
	}
	if _, err := d.validatePlacementLaneFactor(factor); err != nil {
		return placement.Bottom, err
	}
	return typedLaneFactorValue[placementLane](factor.payload).readTerm(id), nil
}

// ComposePlacement is the exact inverse of DecomposePlacement.  Top accepts
// no scalar coordinates; finite Bottom coordinates are represented only by
// omission, so duplicate, zero-identity, and explicit-Bottom inputs fail.
func (d ProductDomain) ComposePlacement(skeleton PlacementSkeletonFactor, factors []PlacementFactor) (LaneFactor, error) {
	runtime, err := d.validatePlacementSkeleton(skeleton)
	if err != nil {
		return LaneFactor{}, err
	}
	if skeleton.top {
		if len(factors) != 0 {
			return LaneFactor{}, fmt.Errorf("%w: placement Top cannot carry scalar coordinates", ErrInvalidLaneFactor)
		}
		lane := placementLane{mapLane: mapLane[identity.Term, placement.Value]{top: true}}
		return LaneFactor{lane: runtime.lane, payload: typedLaneFactorPayload[placementLane]{value: lane}}, nil
	}
	values := make(map[identity.Term]placement.Value, len(factors))
	for index, factor := range factors {
		if err := d.validatePlacementFactor(factor); err != nil {
			return LaneFactor{}, fmt.Errorf("%w: placement coordinate %d: %v", ErrInvalidLaneFactor, index, err)
		}
		if factor.value == placement.Bottom {
			return LaneFactor{}, fmt.Errorf("%w: placement coordinate %d explicitly stores Bottom", ErrInvalidLaneFactor, index)
		}
		if _, duplicate := values[factor.id]; duplicate {
			return LaneFactor{}, fmt.Errorf("%w: duplicate placement identity", ErrInvalidLaneFactor)
		}
		values[factor.id] = factor.value
	}
	lane := placementLaneFromMap(placementMapDomain(), values)
	return LaneFactor{lane: runtime.lane, payload: typedLaneFactorPayload[placementLane]{value: lane}}, nil
}

// ImportPlacementSkeleton re-seals the value-less placement carrier into this
// ProductDomain.  Placement is keyspace-free and its scalar lattice is
// registry-independent.
func (d ProductDomain) ImportPlacementSkeleton(source PlacementSkeletonFactor) (PlacementSkeletonFactor, error) {
	lane, ok := d.ProductLane(LanePlacement)
	if !ok || source.seal == nil || source.lane.seal != source.seal || source.lane.id != LanePlacement {
		return PlacementSkeletonFactor{}, fmt.Errorf("%w: incompatible placement skeleton import", ErrInvalidLaneFactor)
	}
	return PlacementSkeletonFactor{seal: d.seal, lane: lane, top: source.top}, nil
}

// ImportPlacementSlot re-seals one value-less identity coordinate.
func (d ProductDomain) ImportPlacementSlot(source PlacementSlot) (PlacementSlot, error) {
	lane, ok := d.ProductLane(LanePlacement)
	if !ok || source.seal == nil || source.lane.seal != source.seal || source.lane.id != LanePlacement || !source.id.Valid() {
		return PlacementSlot{}, fmt.Errorf("%w: incompatible placement slot import", ErrInvalidLaneFactor)
	}
	return PlacementSlot{seal: d.seal, lane: lane, id: source.id}, nil
}

// BindPlacementValue binds a non-bottom scalar value to a sealed coordinate.
// Bottom remains the implicit finite-map default and is never materialized.
func (d ProductDomain) BindPlacementValue(slot PlacementSlot, value placement.Value) (PlacementFactor, error) {
	if err := d.validatePlacementSlot(slot); err != nil {
		return PlacementFactor{}, err
	}
	if value <= placement.Bottom || value > placement.Unknown {
		return PlacementFactor{}, fmt.Errorf("%w: placement factor must be finite non-bottom", ErrInvalidLaneFactor)
	}
	return PlacementFactor{seal: slot.seal, lane: slot.lane, id: slot.id, value: value}, nil
}

// WithPlacementValue replaces only the scalar terminal while preserving its
// unforgeable ProductDomain and identity ownership.
func (d ProductDomain) WithPlacementValue(factor PlacementFactor, value placement.Value) (PlacementFactor, error) {
	if err := d.validatePlacementFactor(factor); err != nil {
		return PlacementFactor{}, err
	}
	return d.BindPlacementValue(factor.Slot(), value)
}

// PlacementDefault returns the semantic value of every coordinate omitted by
// the skeleton: Bottom for a finite map, Unknown for map Top.
func (d ProductDomain) PlacementDefault(skeleton PlacementSkeletonFactor) (placement.Value, error) {
	if _, err := d.validatePlacementSkeleton(skeleton); err != nil {
		return placement.Bottom, err
	}
	if skeleton.top {
		return placement.Unknown, nil
	}
	return placement.Bottom, nil
}

// PlacementSkeletonBottom returns the finite pointwise-map skeleton.
func (d ProductDomain) PlacementSkeletonBottom() (PlacementSkeletonFactor, error) {
	lane, ok := d.ProductLane(LanePlacement)
	if !ok {
		return PlacementSkeletonFactor{}, fmt.Errorf("%w: product has no placement lane", ErrInvalidLaneFactor)
	}
	return PlacementSkeletonFactor{seal: d.seal, lane: lane}, nil
}

// PlacementSkeletonEqual reports equality of the value-less carrier.
func (d ProductDomain) PlacementSkeletonEqual(left, right PlacementSkeletonFactor) (bool, error) {
	if err := d.validatePlacementSkeletonPair(left, right); err != nil {
		return false, err
	}
	return left.top == right.top, nil
}

// PlacementSkeletonJoin is the exact quotient join.
func (d ProductDomain) PlacementSkeletonJoin(left, right PlacementSkeletonFactor) (PlacementSkeletonFactor, error) {
	if err := d.validatePlacementSkeletonPair(left, right); err != nil {
		return PlacementSkeletonFactor{}, err
	}
	if left.top || right.top {
		left.top = true
	}
	return left, nil
}

// PlacementSkeletonWiden equals Join because the quotient has height two.
func (d ProductDomain) PlacementSkeletonWiden(previous, next PlacementSkeletonFactor) (PlacementSkeletonFactor, error) {
	return d.PlacementSkeletonJoin(previous, next)
}

// PlacementSkeletonMeet is the exact quotient meet.
func (d ProductDomain) PlacementSkeletonMeet(left, right PlacementSkeletonFactor) (PlacementSkeletonFactor, error) {
	if err := d.validatePlacementSkeletonPair(left, right); err != nil {
		return PlacementSkeletonFactor{}, err
	}
	if !right.top {
		return right, nil
	}
	return left, nil
}

// PlacementSkeletonNarrow preserves previous, matching the registered lane's
// absent narrowing operator.
func (d ProductDomain) PlacementSkeletonNarrow(previous, next PlacementSkeletonFactor) (PlacementSkeletonFactor, error) {
	if err := d.validatePlacementSkeletonPair(previous, next); err != nil {
		return PlacementSkeletonFactor{}, err
	}
	return previous, nil
}

func (d ProductDomain) validatePlacementLaneFactor(factor LaneFactor) (*productLaneRuntime, error) {
	runtime, err := d.validateFactor(factor)
	if err != nil {
		return nil, err
	}
	if runtime.lane.id != LanePlacement {
		return nil, fmt.Errorf("%w: got lane %q, want %q", ErrInvalidLaneFactor, runtime.lane.id, LanePlacement)
	}
	return runtime, nil
}

func (d ProductDomain) validatePlacementSkeleton(skeleton PlacementSkeletonFactor) (*productLaneRuntime, error) {
	if skeleton.seal == nil || skeleton.seal != d.seal {
		return nil, fmt.Errorf("%w: foreign placement skeleton domain", ErrInvalidLaneFactor)
	}
	runtime, err := d.validateLane(skeleton.lane)
	if err != nil || runtime.lane.id != LanePlacement {
		return nil, fmt.Errorf("%w: invalid placement skeleton lane", ErrInvalidLaneFactor)
	}
	return runtime, nil
}

func (d ProductDomain) validatePlacementSkeletonPair(left, right PlacementSkeletonFactor) error {
	if _, err := d.validatePlacementSkeleton(left); err != nil {
		return err
	}
	_, err := d.validatePlacementSkeleton(right)
	return err
}

func (d ProductDomain) validatePlacementSlot(slot PlacementSlot) error {
	if slot.seal == nil || slot.seal != d.seal || !slot.id.Valid() {
		return fmt.Errorf("%w: foreign or empty placement slot", ErrInvalidLaneFactor)
	}
	runtime, err := d.validateLane(slot.lane)
	if err != nil || runtime.lane.id != LanePlacement {
		return fmt.Errorf("%w: invalid placement slot lane", ErrInvalidLaneFactor)
	}
	return nil
}

func (d ProductDomain) validatePlacementFactor(factor PlacementFactor) error {
	if err := d.validatePlacementSlot(factor.Slot()); err != nil {
		return err
	}
	if factor.value <= placement.Bottom || factor.value > placement.Unknown {
		return fmt.Errorf("%w: invalid placement scalar", ErrInvalidLaneFactor)
	}
	return nil
}
