package state

import (
	"reflect"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

func TestDynamicIndexMembershipEvidenceFactorMatchesRegisteredCarrier(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	stateKey := func(raw pathaddr.StateKey) pathaddr.StateKey {
		t.Helper()
		if _, ok := keys.InternStateKey(raw); !ok {
			t.Fatalf("intern %q", raw)
		}
		return raw
	}
	containerState := stateKey("sym970@1.table")
	container, _ := keys.InternStateKey(containerState)
	originContainerState := stateKey("sym971@1.origin")
	originContainer, _ := keys.InternStateKey(originContainerState)
	keyState := stateKey("sym972@1.key")
	sourceState := stateKey("sym973@1.source")
	allValueTable := stateKey("sym974@1.all")
	sourceTable := stateKey("sym975@1.source_table")
	originKey := stateKey("sym976@1.origin_key")

	before := Reachable(State{}).
		AddPathKeyMembership(sourceState, sourceTable).
		AddDynamicIndexReadOrigin(keyState, originContainer, originKey)
	after := Reachable(State{}).
		AddDynamicIndexAllValuesKeyMembership(container, allValueTable).
		AddDynamicIndexAllValuesKeyMembership(originContainer, allValueTable)
	lane, ok := domain.ProductLane(LaneKeyMemberships)
	if !ok {
		t.Fatal("missing membership lane")
	}
	inputFactors, err := domain.DecomposeLanes(before, []ProductLane{lane})
	if err != nil {
		t.Fatal(err)
	}
	outputFactors, err := domain.DecomposeLanes(after, []ProductLane{lane})
	if err != nil {
		t.Fatal(err)
	}
	got, err := domain.ObserveDynamicIndexMutationEvidence(inputFactors[0], outputFactors[0], DynamicIndexMembershipEvidenceQuery{
		Container: container, KeyStateKey: keyState, SourceStateKeys: []pathaddr.StateKey{sourceState},
		TableStateKeys: []pathaddr.StateKey{allValueTable},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertStateKeySetEqual(t, got.AllValueTables, after.DynamicIndexAllValuesKeyMembershipTables(container))
	assertStateKeySetEqual(t, got.SourceMemberships, before.PathKeyMembershipTables(sourceState))
	wantRestore := PendingDynamicAllValueRestore{Container: originContainer, Table: allValueTable, Key: originKey}
	if len(got.PendingRestores) != 1 || got.PendingRestores[0] != wantRestore {
		t.Fatalf("pending restores = %#v, want %#v", got.PendingRestores, wantRestore)
	}
}

func TestFactorNativeIndexEvidenceQueriesMatchConcreteState(t *testing.T) {
	reg := standard.Registry()
	domain := RegisteredProductDomain(reg)
	keys := keyspace.New()
	start := pathdom.PathKey("sym980@1.child")
	other := pathdom.PathKey("sym981@1.child")
	startState := testStateKey(t, start)
	otherState := testStateKey(t, other)
	startKey := mustStateKey(t, keys, start)
	otherKey := mustStateKey(t, keys, other)
	id := identity.ID{Kind: "table", Site: "factor-query", Index: 1}
	wantObject := heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root: product.NewWithPresence(reg, product.ShapeTop, presence.Present()),
	})
	input := Reachable(State{}).
		AddBranchProof(pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: startKey, Other: otherKey}).
		WriteLenFloor(keys, startState, 7).
		WritePlacement(id, placement.OwnedHeap).
		WriteHeapTableObject(reg, id, wantObject)

	pathFamily, ok := domain.PathValueFamily()
	if !ok {
		t.Fatal("missing path-value family")
	}
	pathFactors, err := domain.DecomposeLanes(input, []ProductLane{pathFamily.Lane()})
	if err != nil {
		t.Fatal(err)
	}
	equivalent, err := domain.EquivalentPathStateKeysFactor(pathFactors[0], keys, startKey)
	if err != nil {
		t.Fatal(err)
	}
	assertStateKeySetEqual(t, equivalent, input.EquivalentStateKeys(keys, startState))

	lengthFactor := onlyLenFloorFactor(t, domain, input)
	floor, present, err := domain.ReadLengthFloorFactor(lengthFactor, keys, startKey)
	if err != nil || !present || floor != 7 {
		t.Fatalf("length factor = (%d,%t,%v), want (7,true,nil)", floor, present, err)
	}

	placementFactor := onlyPlacementLaneFactor(t, domain, input)
	gotPlacement, err := domain.ReadPlacementTermFactor(placementFactor, identity.ConcreteTerm(id))
	if err != nil || gotPlacement != input.ReadPlacement(id) {
		t.Fatalf("placement factor = (%v,%v), want %v", gotPlacement, err, input.ReadPlacement(id))
	}

	heapFactor := onlyHeapTableIdentityFactor(t, domain, input)
	gotObject, err := domain.ReadHeapTableObjectTermFactor(heapFactor, identity.ConcreteTerm(id))
	if err != nil || !heapidentity.ObjectDomain(reg).Equal(gotObject, input.ReadHeapTableObject(reg, id)) {
		t.Fatalf("heap factor differs from concrete state: err=%v", err)
	}
	_ = otherState
}

func assertStateKeySetEqual(t *testing.T, got, want []pathaddr.StateKey) {
	t.Helper()
	got = append([]pathaddr.StateKey(nil), got...)
	want = append([]pathaddr.StateKey(nil), want...)
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("state-key sets differ: got=%#v want=%#v", got, want)
	}
}
