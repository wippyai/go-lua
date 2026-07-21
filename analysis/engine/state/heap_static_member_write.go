package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

// HeapStaticMemberWritePlan is the additive registered heap transaction for
// one exact owner term and one rootless member suffix.  Unlike constructor and
// object-graph replacement plans, it retains the complete existing object and
// changes only the written member.
type HeapStaticMemberWritePlan struct {
	seal   *productDomainSeal
	keys   *keyspace.KeySpace
	owner  identity.Term
	suffix []segment.Segment
	value  product.Value
}

func (p HeapStaticMemberWritePlan) Valid() bool {
	return p.seal != nil && p.keys != nil && p.keys.Valid() && p.owner.Valid() && len(p.suffix) != 0
}

// PrepareHeapStaticMemberWritePlan seals one owner-exact member update.  The
// owner is supplied by a frozen identity certificate; this API never infers it
// from a heap coordinate, member value, or runtime state.
func (d ProductDomain) PrepareHeapStaticMemberWritePlan(
	keys *keyspace.KeySpace,
	owner identity.Term,
	suffix []segment.Segment,
	value product.Value,
) (HeapStaticMemberWritePlan, error) {
	if !d.Valid() || keys == nil || !keys.Valid() || !owner.Valid() || len(suffix) == 0 ||
		!product.BelongsToRegistry(d.reg, value) {
		return HeapStaticMemberWritePlan{}, fmt.Errorf("%w: invalid heap static-member write", ErrInvalidLaneFactor)
	}
	if _, ok := heapidentity.StaticMemberSuffixKey(keys, suffix); !ok {
		return HeapStaticMemberWritePlan{}, fmt.Errorf("%w: invalid heap static-member suffix", ErrInvalidLaneFactor)
	}
	return HeapStaticMemberWritePlan{
		seal: d.seal, keys: keys, owner: owner,
		suffix: append([]segment.Segment(nil), suffix...), value: value,
	}, nil
}

// BindHeapStaticMemberWriteValue retains topology while binding a leaf-local
// scalar.  Bottom is intentionally accepted only as a value argument; formal
// execution chooses not to invoke this plan for unresolved owners.
func (d ProductDomain) BindHeapStaticMemberWriteValue(plan HeapStaticMemberWritePlan, value product.Value) (HeapStaticMemberWritePlan, error) {
	if !plan.Valid() || plan.seal != d.seal || !product.BelongsToRegistry(d.reg, value) {
		return HeapStaticMemberWritePlan{}, fmt.Errorf("%w: invalid heap static-member value", ErrInvalidLaneFactor)
	}
	plan.value = value
	return plan, nil
}

// HeapStaticMemberWriteLane returns the sole registered heap participant.
func (d ProductDomain) HeapStaticMemberWriteLane(plan HeapStaticMemberWritePlan) (ProductLane, error) {
	lane, ok := d.ProductLane(LaneHeapTableIdentity)
	if !ok || !plan.Valid() || plan.seal != d.seal {
		return ProductLane{}, fmt.Errorf("%w: heap static-member owner unavailable", ErrInvalidLaneFactor)
	}
	return lane, nil
}

// HeapStaticMemberWriteCoordinateWrites declares the member and its required
// root in the registered heap family.  Coordinate admission is topology only;
// it is never a positive member publication.
func (d ProductDomain) HeapStaticMemberWriteCoordinateWrites(plan HeapStaticMemberWritePlan) ([]CoordinateSlot, error) {
	lane, err := d.HeapStaticMemberWriteLane(plan)
	if err != nil {
		return nil, err
	}
	families, err := d.CoordinateFamilies(lane)
	if err != nil || len(families) != 1 {
		return nil, fmt.Errorf("%w: heap static-member coordinate family", ErrInvalidLaneFactor)
	}
	member, ok := heapidentity.StaticMemberSuffixKey(plan.keys, plan.suffix)
	if !ok {
		return nil, fmt.Errorf("%w: heap static-member coordinate suffix", ErrInvalidLaneFactor)
	}
	return []CoordinateSlot{
		{family: families[0], keys: plan.keys, key: wrapHeapCoordinateKey(heapCoordinateRootKey(plan.owner))},
		{family: families[0], keys: plan.keys, key: wrapHeapCoordinateKey(heapCoordinateKey{kind: heapCoordinateMember, id: plan.owner, key: member})},
	}, nil
}

// HeapStaticMemberWriteCoordinateReads returns every currently admitted
// coordinate of plan's certified owner.  A member write is a read-modify
// transaction: selecting only its destination member would reconstruct a
// fragment and discard siblings from the input object.  The exact owner term
// remains the sole authority for this selection; unrelated heap objects and
// members are never admitted.
//
// An absent owner is represented by an empty selection.  The write coordinates
// still authorize the first materialization, and ApplyHeapStaticMemberWriteFactor
// will seed it only after observing Bottom at execution.
func (d ProductDomain) HeapStaticMemberWriteCoordinateReads(
	plan HeapStaticMemberWritePlan,
	available CoordinateFactorInventory,
) ([]CoordinateSlot, error) {
	lane, err := d.HeapStaticMemberWriteLane(plan)
	if err != nil || !available.ValidFor(d, plan.keys) {
		return nil, fmt.Errorf("%w: invalid heap static-member read selection", ErrInvalidLaneFactor)
	}
	families, err := d.CoordinateFamilies(lane)
	if err != nil || len(families) != 1 {
		return nil, fmt.Errorf("%w: heap static-member read family", ErrInvalidLaneFactor)
	}
	slots, err := available.FamilySlots(families[0])
	if err != nil {
		return nil, err
	}
	out := make([]CoordinateSlot, 0)
	for _, slot := range slots {
		coordinate := heapCoordinateKeyValue(slot.key)
		if coordinate.id == plan.owner {
			out = append(out, slot)
		}
	}
	return out, nil
}

// ApplyHeapStaticMemberWriteFactor performs the additive mutation.  A missing
// or Top owner object receives no positive publication: fabricating a partial
// object would lose its owned root/member schema and would be unsound.
func (d ProductDomain) ApplyHeapStaticMemberWriteFactor(plan HeapStaticMemberWritePlan, current LaneFactor) (LaneFactor, error) {
	runtime, err := d.validateHeapTableIdentityFactor(current)
	if err != nil || !plan.Valid() || plan.seal != d.seal || current.lane.id != LaneHeapTableIdentity {
		return LaneFactor{}, fmt.Errorf("%w: invalid heap static-member factor", ErrInvalidLaneFactor)
	}
	lane := typedLaneFactorValue[heapTableIdentityLane](current.payload)
	if lane.top {
		return current, nil
	}
	object := lane.readTerm(d.reg, plan.owner)
	if object.IsBottom() {
		// A formal input can own a member write without carrying a local heap
		// fragment yet. Seed only that owner's unknown root, then add the one
		// declared member; boundary composition joins this fragment with any
		// caller-owned skeleton/members under the same certified identity.
		object = heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: identityvalue.PresentTerm(d.reg, plan.owner),
		})
	}
	next, written := object.WithStaticMember(d.reg, plan.keys, plan.suffix, plan.value)
	if !written {
		return LaneFactor{}, fmt.Errorf("%w: heap static-member write rejected", ErrInvalidLaneFactor)
	}
	lane = lane.withTerm(plan.owner, next)
	return LaneFactor{lane: runtime.lane, payload: typedLaneFactorPayload[heapTableIdentityLane]{value: lane}}, nil
}
