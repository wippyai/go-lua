package factapply

import (
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

// observeIndexMutationEvidence is the concrete edge into the canonical
// factor-native temporal query. State is only decomposed here; all semantic
// observation and cross-snapshot composition belongs to ProductDomain.
func observeIndexMutationEvidence(
	reg *axis.Registry,
	input, output state.State,
	query state.DynamicIndexMembershipEvidenceQuery,
) (state.DynamicIndexMembershipEvidence, bool) {
	if reg == nil {
		return state.DynamicIndexMembershipEvidence{}, false
	}
	domain := state.RegisteredProductDomain(reg)
	lane, ok := domain.ProductLane(state.LaneKeyMemberships)
	if !ok {
		return state.DynamicIndexMembershipEvidence{}, false
	}
	before, err := domain.DecomposeLanes(input, []state.ProductLane{lane})
	if err != nil {
		return state.DynamicIndexMembershipEvidence{}, false
	}
	after, err := domain.DecomposeLanes(output, []state.ProductLane{lane})
	if err != nil {
		return state.DynamicIndexMembershipEvidence{}, false
	}
	evidence, err := domain.ObserveDynamicIndexMutationEvidence(before[0], after[0], query)
	return evidence, err == nil
}

func observeIndexMutationEquivalentKeys(reg *axis.Registry, keys *keyspace.KeySpace, output state.State, path keyspace.Key) ([]pathaddr.StateKey, bool) {
	domain := state.RegisteredProductDomain(reg)
	family, ok := domain.PathValueFamily()
	if !ok {
		return nil, false
	}
	factors, err := domain.DecomposeLanes(output, []state.ProductLane{family.Lane()})
	if err != nil {
		return nil, false
	}
	values, err := domain.EquivalentPathStateKeysFactor(factors[0], keys, path)
	return values, err == nil
}

func observeIndexMutationLengthFloor(reg *axis.Registry, keys *keyspace.KeySpace, input state.State, path keyspace.Key) (int64, bool) {
	domain := state.RegisteredProductDomain(reg)
	family, ok := domain.LenFloorCoordinateFamily()
	if !ok {
		return 0, false
	}
	factors, err := domain.DecomposeLanes(input, []state.ProductLane{family.Lane()})
	if err != nil {
		return 0, false
	}
	floor, present, err := domain.ReadLengthFloorFactor(factors[0], keys, path)
	return floor, present && err == nil
}

func observeIndexMutationPlacement(reg *axis.Registry, output state.State, term identity.Term) (placement.Value, bool) {
	domain := state.RegisteredProductDomain(reg)
	lane, ok := domain.ProductLane(state.LanePlacement)
	if !ok {
		return placement.Bottom, false
	}
	factors, err := domain.DecomposeLanes(output, []state.ProductLane{lane})
	if err != nil {
		return placement.Bottom, false
	}
	value, err := domain.ReadPlacementTermFactor(factors[0], term)
	return value, err == nil
}

func observeIndexMutationHeapObject(reg *axis.Registry, output state.State, term identity.Term) (heapidentity.TableObject, bool) {
	domain := state.RegisteredProductDomain(reg)
	lane, ok := domain.ProductLane(state.LaneHeapTableIdentity)
	if !ok {
		return heapidentity.BottomObject(reg), false
	}
	factors, err := domain.DecomposeLanes(output, []state.ProductLane{lane})
	if err != nil {
		return heapidentity.BottomObject(reg), false
	}
	object, err := domain.ReadHeapTableObjectTermFactor(factors[0], term)
	return object, err == nil
}
