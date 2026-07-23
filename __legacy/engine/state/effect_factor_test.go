package state

import (
	"reflect"
	"testing"

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestDynamicIndexMembershipFactorMatchesCanonicalOrderedWrites(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	containerState := pathaddr.StateKey("sym950@1.table")
	keyState := pathaddr.StateKey("sym951@1.key")
	oldTable := pathaddr.StateKey("sym952@1.old")
	restoredTable := pathaddr.StateKey("sym953@1.restored")
	for _, key := range []pathaddr.StateKey{containerState, keyState, oldTable, restoredTable} {
		if _, ok := keys.InternStateKey(key); !ok {
			t.Fatalf("cannot intern %q", key)
		}
	}
	container, _ := keys.InternStateKey(containerState)
	dynamicKey := dynamicindex.Key{Table: container, Site: "effect-factor"}
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	fact := dynamicindex.NewFact(reg, dynamicindex.FactConfig{
		KeyValue: value, HasKeyValue: true, Value: value, HasValue: true,
		Admission: dynamicindex.AdmissionAdmitted,
	})
	restore := PendingDynamicAllValueRestore{Container: container, Table: restoredTable, Key: keyState}
	input := Reachable(State{}).
		AddPathKeyMembership(containerState, oldTable).
		AddDynamicIndexValueKeyMembership(container, "old-site", oldTable).
		AddDynamicIndexAllValuesKeyMembership(container, oldTable).
		AddPendingDynamicAllValueRestore(restore.Container, restore.Table, restore.Key)

	config := DynamicIndexMembershipFactorConfig{
		Key: dynamicKey, Fact: fact, TableStateKeys: []pathaddr.StateKey{containerState},
		AllValueTables: []pathaddr.StateKey{oldTable}, RestoreKeys: []pathaddr.StateKey{keyState},
		KeyStateKey: keyState, MembershipTable: containerState, TableSymbol: symbol.ID(950),
		HasKeyStateKey: true, DefinitelyAbsent: true, MayBeAbsent: true,
	}
	domain := RegisteredProductDomain(reg)
	plan, err := domain.PrepareDynamicIndexMembershipFactorPlan(keys, config)
	if err != nil {
		t.Fatal(err)
	}
	got, err := domain.ApplyDynamicIndexMembership(plan, input)
	if err != nil {
		t.Fatal(err)
	}

	want := input.ClearDynamicIndexValueKeyMembershipsForContainer(container)
	want = want.ClearKeyMembershipsForPath(containerState)
	want = want.ClearKeyMembershipsForTableSymbol(keys, symbol.ID(950))
	want = want.WriteDynamicIndexFact(reg, dynamicKey, fact)
	want = want.AddDynamicIndexAllValuesKeyMembership(container, oldTable)
	for _, pending := range want.PendingDynamicAllValueRestores(container, keyState) {
		want = want.AddDynamicIndexAllValuesKeyMembership(pending.Container, pending.Table)
		want = want.ClearPendingDynamicAllValueRestore(pending)
	}
	if !domain.Lattice().Equal(got, want) {
		t.Fatal("factor-native dynamic-index/membership program differs from canonical ordered writes")
	}
	if got.ReadDynamicIndexFact(reg, dynamicKey).Admission != dynamicindex.AdmissionAdmitted ||
		!effectStateKeyIn(got.DynamicIndexAllValuesKeyMembershipTables(container), restoredTable) ||
		len(got.PendingDynamicAllValueRestores(container, keyState)) != 0 {
		t.Fatal("factor did not publish the direct fact and restored all-values theorem")
	}
	if lanes := productLaneIDs(domain.DynamicIndexMembershipFactorLanes()); !reflect.DeepEqual(lanes, []LaneID{LaneDynamicIndex, LaneKeyMemberships}) {
		t.Fatalf("dynamic-index factor lanes = %v", lanes)
	}
}

func TestEffectDeltaFactorMatchesCanonicalWrite(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	target, ok := keys.InternStateKey(pathaddr.StateKey("sym960@1.target"))
	if !ok {
		t.Fatal("target key")
	}
	key := effectdelta.Key{Target: target, Site: "effect-factor", Kind: effectdelta.Mutation}
	domain := RegisteredProductDomain(reg)
	plan, err := domain.PrepareEffectDeltaFactorPlan(key, effectdelta.Top())
	if err != nil {
		t.Fatal(err)
	}
	input := Reachable(State{})
	got, err := domain.ApplyEffectDelta(plan, input)
	if err != nil {
		t.Fatal(err)
	}
	want := input.WriteEffectDelta(key, effectdelta.Top())
	if !domain.Lattice().Equal(got, want) {
		t.Fatal("factor-native effect delta differs from canonical write")
	}
	if lanes := productLaneIDs(domain.EffectDeltaFactorLanes()); !reflect.DeepEqual(lanes, []LaneID{LaneEffectDeltas}) {
		t.Fatalf("effect-delta lanes = %v", lanes)
	}
}
