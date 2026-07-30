package state

import (
	"fmt"
	"sort"

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/escapeevent"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

// ApplyCallOutcomeKeyMembershipFactors publishes an already-resolved ordered
// batch into the registered key-membership carrier. Resolution and temporal
// clearing remain preparation concerns; this is the sole carrier mutation.
func (d ProductDomain) ApplyCallOutcomeKeyMembershipFactors(
	factor LaneFactor,
	facts []KeyMembership,
) (LaneFactor, error) {
	runtime, err := d.validateFactor(factor)
	if err != nil || runtime.lane.id != LaneKeyMemberships {
		return LaneFactor{}, fmt.Errorf("%w: invalid call-outcome key-membership factor", ErrInvalidLaneFactor)
	}
	lane := typedLaneFactorValue[keyMembershipLane](factor.payload)
	changed := false
	for _, fact := range facts {
		var step bool
		lane, step = lane.add(fact)
		changed = changed || step
	}
	if !changed {
		return factor, nil
	}
	return LaneFactor{lane: runtime.lane, payload: typedLaneFactorPayload[keyMembershipLane]{value: lane}}, nil
}

// ClearCallOutcomeDynamicIndexValueKeyMembershipFactors drops the complete
// provenance fiber for one dynamic-index container. It is the factor-native
// form of ClearDynamicIndexValueKeyMembershipsForContainer; callers do not
// receive a predicate or access to the membership inventory.
func (d ProductDomain) ClearCallOutcomeDynamicIndexValueKeyMembershipFactors(
	factor LaneFactor,
	container keyspace.Key,
) (LaneFactor, error) {
	runtime, err := d.validateFactor(factor)
	if err != nil || runtime.lane.id != LaneKeyMemberships || container.Kind == keyspace.KindInvalid {
		return LaneFactor{}, fmt.Errorf("%w: invalid call-outcome key-membership clear", ErrInvalidLaneFactor)
	}
	lane := typedLaneFactorValue[keyMembershipLane](factor.payload)
	next, changed := lane.clearMatching(func(m KeyMembership) bool {
		return (m.Kind == KeyMembershipDynamicIndexValue || m.Kind == KeyMembershipDynamicIndexAllValues) &&
			m.Container == container
	})
	if !changed {
		return factor, nil
	}
	return LaneFactor{lane: runtime.lane, payload: typedLaneFactorPayload[keyMembershipLane]{value: next}}, nil
}

// ApplyCallOutcomeFrozenTableFactor records one shallow must-frozen identity
// in the registered carrier without reconstructing State.
func (d ProductDomain) ApplyCallOutcomeFrozenTableFactor(
	factor LaneFactor,
	id identity.ID,
) (LaneFactor, error) {
	runtime, err := d.validateFactor(factor)
	if err != nil || runtime.lane.id != LaneFrozenTables || id == (identity.ID{}) {
		return LaneFactor{}, fmt.Errorf("%w: invalid call-outcome frozen-table factor", ErrInvalidLaneFactor)
	}
	lane := typedLaneFactorValue[frozenTableLane](factor.payload)
	next, changed := lane.freeze(id)
	if !changed {
		return factor, nil
	}
	return LaneFactor{lane: runtime.lane, payload: typedLaneFactorPayload[frozenTableLane]{value: next}}, nil
}

// CallOutcomeDynamicIndexMutation is one already-resolved normal-return map
// write. The batch API below is required because a single call may publish
// several sites for the same container; temporal membership clearing happens
// once outside this carrier, never once per fact.
type CallOutcomeDynamicIndexMutation struct {
	Key  dynamicindex.Key
	Fact dynamicindex.Fact
}

// ObserveCallOutcomeDynamicIndexFactors returns the finite facts for one
// container in stable site order. It is the closed candidate observation used
// when a structural read has several possible dynamic-index identities.
func (d ProductDomain) ObserveCallOutcomeDynamicIndexFactors(
	factor LaneFactor,
	container keyspace.Key,
) ([]CallOutcomeDynamicIndexMutation, error) {
	runtime, err := d.validateFactor(factor)
	if err != nil || runtime.lane.id != LaneDynamicIndex || container.Kind == keyspace.KindInvalid {
		return nil, fmt.Errorf("%w: invalid call-outcome dynamic-index observation", ErrInvalidLaneFactor)
	}
	lane := typedLaneFactorValue[dynamicIndexLane](factor.payload)
	if lane.isTop() {
		return nil, nil
	}
	out := make([]CallOutcomeDynamicIndexMutation, 0)
	for key, fact := range lane.values {
		if key.Table == container {
			out = append(out, CallOutcomeDynamicIndexMutation{Key: key, Fact: fact})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key.Site < out[j].Key.Site })
	return out, nil
}

// ApplyCallOutcomeDynamicIndexFactors applies one ordered batch to the unique
// dynamic-index carrier without reconstructing State.
func (d ProductDomain) ApplyCallOutcomeDynamicIndexFactors(
	factor LaneFactor,
	mutations []CallOutcomeDynamicIndexMutation,
) (LaneFactor, error) {
	runtime, err := d.validateFactor(factor)
	if err != nil || runtime.lane.id != LaneDynamicIndex {
		return LaneFactor{}, fmt.Errorf("%w: invalid call-outcome dynamic-index factor", ErrInvalidLaneFactor)
	}
	lane := typedLaneFactorValue[dynamicIndexLane](factor.payload)
	domain := dynamicindex.Domain(d.reg)
	changed := false
	for index, mutation := range mutations {
		if mutation.Key.Table.Kind == keyspace.KindInvalid || mutation.Key.Site == "" {
			return LaneFactor{}, fmt.Errorf("%w: invalid call-outcome dynamic-index mutation %d", ErrInvalidLaneFactor, index)
		}
		if domain.Equal(lane.read(d.reg, mutation.Key), mutation.Fact) {
			continue
		}
		if domain.Equal(mutation.Fact, domain.Bottom()) {
			var step bool
			lane, step = lane.without(mutation.Key)
			changed = changed || step
			continue
		}
		lane = lane.with(mutation.Key, mutation.Fact)
		changed = true
	}
	if !changed {
		return factor, nil
	}
	return LaneFactor{lane: runtime.lane, payload: typedLaneFactorPayload[dynamicIndexLane]{value: lane}}, nil
}

// CallOutcomeDynamicIndexMembershipSnapshot is a closed observation of the
// two provenance sets needed by normal-return batch preparation.
type CallOutcomeDynamicIndexMembershipSnapshot struct {
	ValueTables    []pathaddr.StateKey
	AllValueTables []pathaddr.StateKey
}

func (d ProductDomain) ObserveCallOutcomeDynamicIndexMembershipFactors(
	factor LaneFactor,
	container keyspace.Key,
	site dynamicindex.Site,
) (CallOutcomeDynamicIndexMembershipSnapshot, error) {
	runtime, err := d.validateFactor(factor)
	if err != nil || runtime.lane.id != LaneKeyMemberships || container.Kind == keyspace.KindInvalid {
		return CallOutcomeDynamicIndexMembershipSnapshot{}, fmt.Errorf("%w: invalid call-outcome membership observation", ErrInvalidLaneFactor)
	}
	lane := typedLaneFactorValue[keyMembershipLane](factor.payload)
	out := CallOutcomeDynamicIndexMembershipSnapshot{}
	if !lane.bottom {
		for membership := range lane.dynamicAll {
			if membership.Kind == KeyMembershipDynamicIndexAllValues && membership.Container == container {
				out.AllValueTables = append(out.AllValueTables, membership.Table)
			}
		}
		if !lane.dynamicTop && site != "" {
			for membership := range lane.dynamic {
				if membership.Kind == KeyMembershipDynamicIndexValue && membership.Container == container && membership.Site == site {
					out.ValueTables = append(out.ValueTables, membership.Table)
				}
			}
		}
	}
	return out, nil
}

// ApplyCallOutcomeStoreRelationFactors publishes resolved store relations
// without reconstructing State.
func (d ProductDomain) ApplyCallOutcomeStoreRelationFactors(
	factor LaneFactor,
	facts []StoreRelation,
) (LaneFactor, error) {
	runtime, err := d.validateFactor(factor)
	if err != nil || runtime.lane.id != LaneStoreRelations {
		return LaneFactor{}, fmt.Errorf("%w: invalid call-outcome store-relation factor", ErrInvalidLaneFactor)
	}
	lane := typedLaneFactorValue[storeRelationLane](factor.payload)
	changed := false
	for _, fact := range facts {
		var step bool
		lane, step = lane.add(fact)
		changed = changed || step
	}
	if !changed {
		return factor, nil
	}
	return LaneFactor{lane: runtime.lane, payload: typedLaneFactorPayload[storeRelationLane]{value: lane}}, nil
}

// ApplyCallOutcomeEscapeEventFactors publishes resolved escape events without
// reconstructing State.
func (d ProductDomain) ApplyCallOutcomeEscapeEventFactors(
	factor LaneFactor,
	facts []escapeevent.Fact,
) (LaneFactor, error) {
	runtime, err := d.validateFactor(factor)
	if err != nil || runtime.lane.id != LaneEscapeEvents {
		return LaneFactor{}, fmt.Errorf("%w: invalid call-outcome escape-event factor", ErrInvalidLaneFactor)
	}
	lane := typedLaneFactorValue[escapeevent.Lane](factor.payload)
	changed := false
	for _, fact := range facts {
		var step bool
		lane, step = lane.Add(fact)
		changed = changed || step
	}
	if !changed {
		return factor, nil
	}
	return LaneFactor{lane: runtime.lane, payload: typedLaneFactorPayload[escapeevent.Lane]{value: lane}}, nil
}

// ApplyCallOutcomeBranchProofFactors publishes resolved branch evidence into
// the unique path-evidence owner. Relation consequences are a separate
// prepared transaction because they affect more than this coordinate family.
func (d ProductDomain) ApplyCallOutcomeBranchProofFactors(
	factor LaneFactor,
	proofs []pathevidence.BranchProof,
) (LaneFactor, error) {
	runtime, err := d.validateFactor(factor)
	if err != nil || runtime.lane.id != LanePathEvidence {
		return LaneFactor{}, fmt.Errorf("%w: invalid call-outcome branch-proof factor", ErrInvalidLaneFactor)
	}
	lane := typedLaneFactorValue[pathevidence.Lane](factor.payload)
	next, changed := lane.AddBranchProofs(proofs)
	if !changed {
		return factor, nil
	}
	return LaneFactor{lane: runtime.lane, payload: typedLaneFactorPayload[pathevidence.Lane]{value: next}}, nil
}

// CallOutcomeLifecycleMutation is one resolved lifecycle operation. Resource
// canonicalization is frozen during preparation from the exact path-equality
// factor; Apply performs no query or path scan.
type CallOutcomeLifecycleKind uint8

const (
	CallOutcomeLifecycleInvalid CallOutcomeLifecycleKind = iota
	CallOutcomeLifecycleAcquire
	CallOutcomeLifecycleTransition
	CallOutcomeLifecycleEscape
)

func (k CallOutcomeLifecycleKind) Valid() bool {
	return k >= CallOutcomeLifecycleAcquire && k <= CallOutcomeLifecycleEscape
}

type CallOutcomeLifecycleMutation struct {
	Resource   typestate.Resource
	Kind       CallOutcomeLifecycleKind
	From       typestate.State
	To         typestate.State
	Obligation typestate.Obligation
	Site       uint32
}

// ApplyCallOutcomeLifecycleFactors applies a prepared lifecycle batch to the
// registered typestate carrier.
func (d ProductDomain) ApplyCallOutcomeLifecycleFactors(
	factor LaneFactor,
	mutations []CallOutcomeLifecycleMutation,
) (LaneFactor, error) {
	runtime, err := d.validateFactor(factor)
	if err != nil || runtime.lane.id != LaneTypestates {
		return LaneFactor{}, fmt.Errorf("%w: invalid call-outcome lifecycle factor", ErrInvalidLaneFactor)
	}
	store := typedLaneFactorValue[typestate.Store](factor.payload)
	next := store
	for index, mutation := range mutations {
		if mutation.Resource.ID == "" || mutation.Resource.Protocol == "" || !mutation.Kind.Valid() {
			return LaneFactor{}, fmt.Errorf("%w: invalid call-outcome lifecycle mutation %d", ErrInvalidLaneFactor, index)
		}
		switch mutation.Kind {
		case CallOutcomeLifecycleAcquire:
			next = next.Acquire(mutation.Resource, mutation.To, mutation.Obligation)
		case CallOutcomeLifecycleTransition:
			next = next.TransitionAt(mutation.Resource, mutation.From, mutation.To, mutation.Site)
		case CallOutcomeLifecycleEscape:
			next = next.Escape(mutation.Resource)
		default:
			return LaneFactor{}, fmt.Errorf("%w: unknown call-outcome lifecycle mutation %d", ErrInvalidLaneFactor, index)
		}
	}
	if typestate.Equal(store, next) {
		return factor, nil
	}
	return LaneFactor{lane: runtime.lane, payload: typedLaneFactorPayload[typestate.Store]{value: next}}, nil
}
