package state

import (
	"fmt"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// StaticMemberFactorPlan is the sealed persistent-path publication used by a
// static store. The primary and field-canonical coordinates are normalized
// once; both concrete and formal execution apply this exact plan.
type StaticMemberFactorPlan struct {
	seal      *productDomainSeal
	keys      *keyspace.KeySpace
	targets   []keyspace.Key
	value     product.Value
	authority CoordinatePathEvidenceAuthority[statekey.Value]
}

func (p StaticMemberFactorPlan) Valid() bool {
	return p.seal != nil && p.keys != nil && p.keys.Valid() && len(p.targets) != 0 && p.authority.seal != nil
}

// PrepareStaticMemberFactorPlan seals the exact primary/mirror publication.
func (d ProductDomain) PrepareStaticMemberFactorPlan(keys *keyspace.KeySpace, target keyspace.Key, value product.Value) (StaticMemberFactorPlan, error) {
	if !d.Valid() || keys == nil || !keys.Valid() || !product.BelongsToRegistry(d.reg, value) {
		return StaticMemberFactorPlan{}, fmt.Errorf("%w: invalid static-member publication", ErrInvalidLaneFactor)
	}
	if _, ok := keys.SegmentsView(target); !ok {
		return StaticMemberFactorPlan{}, fmt.Errorf("%w: foreign static-member target", ErrInvalidLaneFactor)
	}
	targets := []keyspace.Key{target}
	if canonical, mirrored := keys.FieldCanonical(target); mirrored && canonical != target {
		targets = append(targets, canonical)
	}
	return d.sealStaticMemberFactorPlan(keys, targets, value)
}

func (d ProductDomain) sealStaticMemberFactorPlan(keys *keyspace.KeySpace, targets []keyspace.Key, value product.Value) (StaticMemberFactorPlan, error) {
	plan := StaticMemberFactorPlan{seal: d.seal, keys: keys, targets: append([]keyspace.Key(nil), targets...), value: value}
	slots := make([]CoordinateSlot, len(targets))
	for index, target := range targets {
		var err error
		slots[index], err = d.PathStaticMemberCoordinateSlot(keys, target)
		if err != nil {
			return StaticMemberFactorPlan{}, err
		}
	}
	empty, err := d.SealCoordinateFactorInventory(keys, nil)
	if err != nil {
		return StaticMemberFactorPlan{}, err
	}
	writes, err := d.SealCoordinateFactorInventory(keys, slots)
	if err != nil {
		return StaticMemberFactorPlan{}, err
	}
	plan.authority, err = SealCoordinatePathEvidenceAuthority(
		d, keys, nil, nil, empty, writes, false, false,
		func(slot statekey.Value) bool { return slot != 0 },
	)
	if err != nil {
		return StaticMemberFactorPlan{}, err
	}
	return plan, nil
}

// StaticMemberFactorLane returns the unique registered path-evidence owner.
func (d ProductDomain) StaticMemberFactorLane(plan StaticMemberFactorPlan) (ProductLane, error) {
	family, ok := d.PathEvidenceCoordinateFamily()
	if !ok || !plan.Valid() || plan.seal != d.seal {
		return ProductLane{}, fmt.Errorf("%w: static-member owner unavailable", ErrInvalidLaneFactor)
	}
	return family.Lane(), nil
}

// StaticMemberFactorCoordinateWrites declares the complete immutable topology
// of one sealed publication, including any field-canonical mirror. Inventory
// owners consume this declaration instead of duplicating normalization rules.
func (d ProductDomain) StaticMemberFactorCoordinateWrites(plan StaticMemberFactorPlan) ([]CoordinateSlot, error) {
	if !d.Valid() || !plan.Valid() || plan.seal != d.seal {
		return nil, fmt.Errorf("%w: invalid static-member coordinate declaration", ErrInvalidLaneFactor)
	}
	slots := make([]CoordinateSlot, len(plan.targets))
	for index, target := range plan.targets {
		var err error
		slots[index], err = d.PathStaticMemberCoordinateSlot(plan.keys, target)
		if err != nil {
			return nil, err
		}
	}
	return slots, nil
}

// RekeyStaticMemberFactorPlanFormal transports the already-normalized target
// set through one sealed formal-root substitution. Canonical mirror admission
// is therefore decided once in the source address space and cannot change
// merely because a formal root uses a different structural key kind.
func (d ProductDomain) RekeyStaticMemberFactorPlanFormal(rekey CoordinateFormalRootRekey, plan StaticMemberFactorPlan) (StaticMemberFactorPlan, error) {
	if !d.Valid() || !rekey.validFor(d) || !plan.Valid() || plan.seal != d.seal || plan.keys != rekey.from {
		return StaticMemberFactorPlan{}, fmt.Errorf("%w: invalid static-member formal rekey", ErrInvalidLaneFactor)
	}
	targets := make([]keyspace.Key, len(plan.targets))
	for index, target := range plan.targets {
		var err error
		targets[index], err = d.RekeyStructuralKeyFormal(rekey, target)
		if err != nil {
			return StaticMemberFactorPlan{}, err
		}
	}
	return d.sealStaticMemberFactorPlan(rekey.to, targets, plan.value)
}

// BindStaticMemberFactorValue preserves a sealed plan's exact coordinate
// topology while binding the leaf-specific abstract value at execution time.
func (d ProductDomain) BindStaticMemberFactorValue(plan StaticMemberFactorPlan, value product.Value) (StaticMemberFactorPlan, error) {
	if !d.Valid() || !plan.Valid() || plan.seal != d.seal || !product.BelongsToRegistry(d.reg, value) {
		return StaticMemberFactorPlan{}, fmt.Errorf("%w: invalid static-member value binding", ErrInvalidLaneFactor)
	}
	plan.value = value
	return plan, nil
}

// ApplyStaticMemberFactor publishes the persistent structural coordinates in
// their registered family. It never reconstructs State or names a lane.
func (d ProductDomain) ApplyStaticMemberFactor(plan StaticMemberFactorPlan, current LaneFactor) (LaneFactor, error) {
	family, ok := d.PathEvidenceCoordinateFamily()
	if !ok || !plan.Valid() || plan.seal != d.seal || current.lane != family.Lane() {
		return LaneFactor{}, fmt.Errorf("%w: invalid static-member factor", ErrInvalidLaneFactor)
	}
	skeleton, scalars, err := d.DecomposeCoordinateFamily(current, family, plan.keys)
	if err != nil {
		return LaneFactor{}, err
	}
	carrier, err := d.OpenCoordinatePathEvidenceCarrier(
		skeleton, scalars, ValueLaneFactor{}, true,
		plan.authority, PathDescendantMutationFactors{},
	)
	if err != nil {
		return LaneFactor{}, err
	}
	for _, target := range plan.targets {
		if _, applied := carrier.WriteStaticMember(target, plan.value); !applied {
			return LaneFactor{}, fmt.Errorf("%w: static-member publication rejected", ErrInvalidLaneFactor)
		}
	}
	nextSkeleton, nextScalars, _, _, _, _, err := carrier.Freeze()
	if err != nil {
		return LaneFactor{}, err
	}
	return d.ReplaceCoordinateFamily(current, nextSkeleton, nextScalars)
}

// ApplyStaticMember applies the same factor law to concrete State, touching
// only the unique registered participant lane.
func (d ProductDomain) ApplyStaticMember(plan StaticMemberFactorPlan, input State) (State, error) {
	lane, err := d.StaticMemberFactorLane(plan)
	if err != nil {
		return State{}, err
	}
	factors, err := d.DecomposeLanes(input, []ProductLane{lane})
	if err != nil {
		return State{}, err
	}
	next, err := d.ApplyStaticMemberFactor(plan, factors[0])
	if err != nil {
		return State{}, err
	}
	return d.PatchLaneFactors(input, []LaneFactor{next})
}
