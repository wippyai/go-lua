package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/state/escapeevent"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

// BoundaryFactorViewPlan freezes the enabled source inventory and one closed
// boundary selection for a sparse publication codec. Requested optional lanes
// which are absent from the ProductDomain are deliberately omitted: their
// observations have the same canonical empty/bottom spelling as State.
type BoundaryFactorViewPlan struct {
	domain    ProductDomain
	selection BoundaryFactorSelection
	runtimes  []*productLaneRuntime
	ordinals  map[LaneID]int
}

// PrepareBoundaryFactorView seals requested into domain-owned positional
// descriptors once. The returned order is the first-use order in requested,
// filtered to enabled non-Values lanes. Duplicate declarations and an enabled
// Values request are rejected at preparation rather than in the hot path.
func (d ProductDomain) PrepareBoundaryFactorView(
	selection BoundaryFactorSelection,
	requested []LaneID,
) (BoundaryFactorViewPlan, error) {
	if !d.Valid() || !selection.valid() {
		return BoundaryFactorViewPlan{}, fmt.Errorf("%w: boundary factor view is unowned", ErrInvalidLaneFactor)
	}
	plan := BoundaryFactorViewPlan{
		domain: d, selection: selection,
		runtimes: make([]*productLaneRuntime, 0, len(requested)),
		ordinals: make(map[LaneID]int, len(requested)),
	}
	declared := make(map[LaneID]struct{}, len(requested))
	for _, id := range requested {
		if _, duplicate := declared[id]; duplicate {
			return BoundaryFactorViewPlan{}, fmt.Errorf("%w: duplicate boundary factor view lane %q", ErrInvalidLaneFactor, id)
		}
		declared[id] = struct{}{}
		lane, enabled := d.ProductLane(id)
		if !enabled {
			continue
		}
		if lane.slotFactored {
			return BoundaryFactorViewPlan{}, fmt.Errorf("%w: Values cannot be observed as a boundary factor view lane", ErrInvalidLaneFactor)
		}
		runtime := &d.factorLanes[lane.ordinal]
		plan.ordinals[id] = len(plan.runtimes)
		plan.runtimes = append(plan.runtimes, runtime)
	}
	return plan, nil
}

// Lanes returns the exact enabled positional inventory expected by Project.
// The returned slice is caller-owned and is suitable for DecomposeLanes.
func (p BoundaryFactorViewPlan) Lanes() []ProductLane {
	lanes := make([]ProductLane, len(p.runtimes))
	for i := range p.runtimes {
		lanes[i] = p.runtimes[i].lane
	}
	return lanes
}

// RootSchemas returns the exact ordered scalar-free root tuple sealed by the
// selection. The copy is caller-owned; publication plans may retain it so the
// hot path can supply values only and cannot rewrite structural roles.
func (p BoundaryFactorViewPlan) RootSchemas() []BoundaryFactorRoot {
	return append([]BoundaryFactorRoot(nil), p.selection.roots...)
}

// KeySpace returns the immutable structural authority sealed into this view.
// It is exposed only so a prepared publication codec can invoke the canonical
// call-boundary emitter without retaining the wider selection internals.
func (p BoundaryFactorViewPlan) KeySpace() *keyspace.KeySpace {
	return p.selection.keys
}

// BoundaryFactorView is a read-only sparse product view over one exact tuple
// of boundary-projected factors. It composes no State and discovers no lanes.
type BoundaryFactorView struct {
	plan    BoundaryFactorViewPlan
	factors []LaneFactor
}

// Project validates the caller-owned tuple positionally and projects it using
// the operations frozen in the plan. There are no per-call maps, inventory
// scans, or catalog lookups; the one result slice is the retained sparse view.
func (p BoundaryFactorViewPlan) Project(supplied []LaneFactor) (BoundaryFactorView, error) {
	if !p.domain.Valid() || !p.selection.valid() || len(supplied) != len(p.runtimes) {
		return BoundaryFactorView{}, fmt.Errorf("%w: incomplete boundary factor view tuple", ErrInvalidLaneFactor)
	}
	view := BoundaryFactorView{plan: p, factors: make([]LaneFactor, len(supplied))}
	ctx := boundaryProjectContext{reg: p.domain.reg, keys: p.selection.keys, closure: p.selection.closure}
	for index := range p.runtimes {
		runtime := p.runtimes[index]
		factor := supplied[index]
		if factor.lane != runtime.lane || factor.payload == nil {
			return BoundaryFactorView{}, fmt.Errorf("%w: boundary factor view position %d is not lane %q", ErrInvalidLaneFactor, index, runtime.lane.id)
		}
		payload, ok := runtime.ops.boundaryProject(&ctx, factor.payload)
		if !ok {
			return BoundaryFactorView{}, fmt.Errorf("state: boundary factor projection failed in lane %q", runtime.lane.id)
		}
		view.factors[index] = LaneFactor{lane: runtime.lane, payload: payload}
	}
	return view, nil
}

func boundaryFactorViewLane[T any](view BoundaryFactorView, id LaneID) (T, bool) {
	var zero T
	ordinal, present := view.plan.ordinals[id]
	if !present || ordinal < 0 || ordinal >= len(view.factors) {
		return zero, false
	}
	payload, exact := view.factors[ordinal].payload.(typedLaneFactorPayload[T])
	if !exact {
		return zero, false
	}
	return payload.value, true
}

func (v BoundaryFactorView) ForEachPathRefinement(ks *keyspace.KeySpace, visit func(keyspace.Key, product.Value) bool) {
	lane, present := boundaryFactorViewLane[pathevidence.Lane](v, LanePathEvidence)
	if present {
		lane.ForEachPathRefinement(ks, visit)
	}
}

func (v BoundaryFactorView) ForEachPathStaticMember(ks *keyspace.KeySpace, visit func(keyspace.Key, product.Value) bool) {
	lane, present := boundaryFactorViewLane[pathevidence.Lane](v, LanePathEvidence)
	if present {
		lane.ForEachPathStaticMember(ks, visit)
	}
}

func (v BoundaryFactorView) PathPresenceImplicationsSnapshot(keys *keyspace.KeySpace) pathevidence.PathPresenceImplicationsSnapshot {
	lane, present := boundaryFactorViewLane[pathevidence.Lane](v, LanePathEvidence)
	if !present || keys != v.plan.selection.keys {
		return pathevidence.PathPresenceImplicationsSnapshot{Bottom: true}
	}
	return lane.PathPresenceImplicationsSnapshot(keys)
}

func (v BoundaryFactorView) BranchProofsSnapshot(keys *keyspace.KeySpace) pathevidence.BranchProofsSnapshot {
	lane, present := boundaryFactorViewLane[pathevidence.Lane](v, LanePathEvidence)
	if !present || keys != v.plan.selection.keys {
		return pathevidence.BranchProofsSnapshot{Bottom: true}
	}
	return lane.BranchProofsSnapshot(keys)
}

func (v BoundaryFactorView) ReadLocalPathKey(reg *axis.Registry, path keyspace.Key) product.Value {
	lane, present := boundaryFactorViewLane[pathevidence.Lane](v, LanePathEvidence)
	if !present || reg != v.plan.domain.reg || v.plan.selection.keys.FormatReadOnly(path) == "" {
		return product.Bottom(reg)
	}
	return lane.ReadPathKey(reg, path)
}

func (v BoundaryFactorView) DynamicIndexFactsSnapshot() DynamicIndexFactsSnapshot {
	lane, present := boundaryFactorViewLane[dynamicIndexLane](v, LaneDynamicIndex)
	if !present {
		return DynamicIndexFactsSnapshot{}
	}
	return dynamicIndexFactsSnapshot(lane)
}

func (v BoundaryFactorView) KeyMembershipsSnapshot() KeyMembershipsSnapshot {
	lane, present := boundaryFactorViewLane[keyMembershipLane](v, LaneKeyMemberships)
	if !present {
		return KeyMembershipsSnapshot{Bottom: true}
	}
	return lane.snapshot()
}

func (v BoundaryFactorView) ChannelSelectFactsSnapshot() ChannelSelectFactsSnapshot {
	lane, present := boundaryFactorViewLane[channelselectfact.Lane](v, LaneChannelSelect)
	if !present {
		return ChannelSelectFactsSnapshot{Bottom: true}
	}
	return channelSelectFactsSnapshot(lane)
}

func (v BoundaryFactorView) FrozenTablesSnapshot() FrozenTablesSnapshot {
	lane, present := boundaryFactorViewLane[frozenTableLane](v, LaneFrozenTables)
	if !present {
		return FrozenTablesSnapshot{Bottom: true}
	}
	return frozenTablesSnapshot(lane)
}

func (v BoundaryFactorView) EffectDeltasSnapshot() EffectDeltasSnapshot {
	lane, present := boundaryFactorViewLane[effectDeltaLane](v, LaneEffectDeltas)
	if !present {
		return EffectDeltasSnapshot{}
	}
	return effectDeltasSnapshot(lane)
}

func (v BoundaryFactorView) EscapeEventsSnapshot() EscapeEventsSnapshot {
	lane, present := boundaryFactorViewLane[escapeevent.Lane](v, LaneEscapeEvents)
	if !present {
		return EscapeEventsSnapshot{Bottom: true}
	}
	return escapeEventsSnapshot(lane)
}

func (v BoundaryFactorView) StoreRelationsSnapshot() StoreRelationsSnapshot {
	lane, present := boundaryFactorViewLane[storeRelationLane](v, LaneStoreRelations)
	if !present {
		return StoreRelationsSnapshot{Bottom: true}
	}
	return storeRelationsSnapshot(lane)
}

func (v BoundaryFactorView) NumFloorsSnapshot(keys *keyspace.KeySpace) NumFloorsSnapshot {
	lane, present := boundaryFactorViewLane[numBoundLane](v, LaneNumFloors)
	if !present || keys != v.plan.selection.keys {
		return NumFloorsSnapshot{Bottom: true}
	}
	return numFloorsSnapshot(lane, keys)
}

func (v BoundaryFactorView) NumCeilsSnapshot(keys *keyspace.KeySpace) NumCeilsSnapshot {
	lane, present := boundaryFactorViewLane[numBoundLane](v, LaneNumCeils)
	if !present || keys != v.plan.selection.keys {
		return NumCeilsSnapshot{Bottom: true}
	}
	return numCeilsSnapshot(lane, keys)
}

func (v BoundaryFactorView) RelConstraints() RelConstraintsSnapshot {
	lane, present := boundaryFactorViewLane[diffRelationLane](v, LaneDiffRelations)
	if !present {
		return RelConstraintsSnapshot{Bottom: true}
	}
	return relConstraintsSnapshot(lane)
}

func (v BoundaryFactorView) HeapTableObjectsSnapshot() HeapTableObjectsSnapshot {
	lane, present := boundaryFactorViewLane[heapTableIdentityLane](v, LaneHeapTableIdentity)
	if !present {
		return HeapTableObjectsSnapshot{}
	}
	snapshot, _ := checkedHeapTableObjectsSnapshot(lane)
	return snapshot
}

// FiniteHeapTableObjectsSnapshot crosses the concrete call-outcome fence.
// Top and unresolved relational identities are typed failures, never a panic
// or a silently weakened finite map.
func (v BoundaryFactorView) FiniteHeapTableObjectsSnapshot() (HeapTableObjectsSnapshot, error) {
	lane, present := boundaryFactorViewLane[heapTableIdentityLane](v, LaneHeapTableIdentity)
	if !present {
		return HeapTableObjectsSnapshot{}, nil
	}
	snapshot, err := checkedHeapTableObjectsSnapshot(lane)
	if err != nil {
		return HeapTableObjectsSnapshot{}, err
	}
	if snapshot.Top {
		return HeapTableObjectsSnapshot{}, fmt.Errorf("state: top heap identity cannot cross concrete call boundary")
	}
	return snapshot, nil
}

// FinitePlacementsSnapshot observes the registered placement lane through the
// same snapshot law used by State. Disabled optional lanes publish empty;
// Top and unresolved identities fail transactionally.
func (v BoundaryFactorView) FinitePlacementsSnapshot() (PlacementsSnapshot, error) {
	lane, present := boundaryFactorViewLane[placementLane](v, LanePlacement)
	if !present {
		return PlacementsSnapshot{}, nil
	}
	snapshot, err := checkedPlacementsSnapshot(lane)
	if err != nil {
		return PlacementsSnapshot{}, err
	}
	if snapshot.Top {
		return PlacementsSnapshot{}, fmt.Errorf("state: top placement cannot cross concrete call boundary")
	}
	return snapshot, nil
}
