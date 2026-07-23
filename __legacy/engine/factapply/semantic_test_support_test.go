package factapply

import (
	"strings"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/escape"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/escapeevent"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

const testObjectLiteralGraphID uint64 = 9001

func testTableLiteralID(expr factflow.ExprRef) identity.ID {
	return testTableIdentity(testObjectLiteralGraphID, uint64(expr))
}

func dottedSuffixSegments(suffix string) []segment.Segment {
	trimmed := strings.TrimPrefix(suffix, ".")
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, ".")
	segments := make([]segment.Segment, 0, len(parts))
	for _, part := range parts {
		segments = append(segments, segment.Segment{Kind: segment.SegmentField, Name: part})
	}
	return segments
}

func staticSuffixKey(t *testing.T, ks *keyspace.KeySpace, suffix string) keyspace.Key {
	t.Helper()
	k, ok := heapidentity.StaticMemberSuffixKey(ks, dottedSuffixSegments(suffix))
	if !ok {
		t.Fatalf("StaticMemberSuffixKey(%q) failed", suffix)
	}
	return k
}

func assertHeapStaticMember(t *testing.T, reg *axis.Registry, ks *keyspace.KeySpace, gotState state.State, expr factflow.ExprRef, suffix string, want product.Value) {
	t.Helper()
	id := testTableLiteralID(expr)
	object := gotState.ReadHeapTableObject(reg, id)
	got, ok := object.StaticMember(staticSuffixKey(t, ks, suffix))
	if !ok || !product.Equal(reg, got, want) {
		t.Fatalf("heap object %v static %s = %s/%v, want %s", id, suffix, formatValue(reg, got), ok, formatValue(reg, want))
	}
	if rootID, ok := product.Get(reg, object.Root(), identity.Key).ID(); !ok || rootID != id {
		t.Fatalf("heap object %v root identity = %v/%v, want %v", id, rootID, ok, id)
	}
	if !heapidentity.ObjectDomain(reg).LessOrEq(object, heapidentity.TopObject()) {
		t.Fatalf("heap object %v not in domain", id)
	}
}

func assertPlacement(t *testing.T, gotState state.State, id identity.ID, want placement.Value) {
	t.Helper()
	if got := gotState.ReadPlacement(id); got != want {
		t.Fatalf("placement[%v] = %s, want %s", id, got, want)
	}
}

func testTaintSpec(id userlattice.AxisID) userlattice.Spec {
	return userlattice.Spec{
		ID: id, Elements: []userlattice.ElementID{"Untainted", "Sanitized", "Tainted", "Unknown"},
		Bottom: "Untainted", Top: "Unknown",
		Order: []userlattice.OrderPair{
			{Lower: "Untainted", Upper: "Sanitized"},
			{Lower: "Untainted", Upper: "Tainted"},
			{Lower: "Sanitized", Upper: "Unknown"},
			{Lower: "Tainted", Upper: "Unknown"},
		},
		Hooks: userlattice.Hooks{
			OnAssign:       userlattice.AssignHook{Mode: userlattice.AssignPropagate},
			OnCallBoundary: userlattice.CallBoundaryHook{Mode: userlattice.CallBoundaryKeep},
			OnClaim: []userlattice.ClaimHook{
				{Claim: "tainted", Element: "Tainted"},
				{Claim: "sanitized", Element: "Sanitized"},
			},
		},
	}
}

func concreteRootTransactionLaneSeeds(t *testing.T, axisReg *axis.Registry, ks *keyspace.KeySpace, userAxis userlattice.AxisID) map[state.LaneID]state.State {
	t.Helper()
	present := presentValue(axisReg)
	rootKey := concreteTransactionStateKey(t, "sym501@1.root")
	otherKey := concreteTransactionStateKey(t, "sym502@1.other")
	localRoot, ok := ks.FromStateKey(rootKey.PathKey())
	if !ok {
		t.Fatal("root key not interned")
	}
	localOther, ok := ks.FromStateKey(otherKey.PathKey())
	if !ok {
		t.Fatal("other key not interned")
	}
	tableID := identity.ID{Kind: "table", Site: "root-transaction", Index: 1}
	dynKey := dynamicindex.Key{Table: localRoot, Site: "root-transaction"}
	dynFact := dynamicindex.Fact{KeyPresence: presence.Present(), KeyValue: present, Value: present, Admission: dynamicindex.AdmissionAdmitted}
	effectKey := effectdelta.Key{Target: localRoot, Site: "root-transaction", Kind: effectdelta.Mutation}
	effectValue := effectdelta.Value{Before: present, After: present, Change: effectdelta.ChangeChanged}
	escapeFact := state.EscapeEvent{Target: rootKey, Kind: escapeevent.KindStore}
	channel := channelselectfact.Fact{Select: "root-transaction", Kind: channelselectfact.FactSelect, Result: rootKey}
	proof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: localRoot, Other: localOther}

	return map[state.LaneID]state.State{
		state.LaneValues:            state.State{}.WriteValue(axisReg, statekey.SymbolValue(501), present),
		state.LanePathEvidence:      state.State{}.WriteLocalPathKey(axisReg, localRoot, present).AddBranchProof(proof),
		state.LaneDynamicIndex:      state.State{}.WriteDynamicIndexFact(axisReg, dynKey, dynFact),
		state.LaneHeapTableIdentity: state.State{}.WriteHeapTableObject(axisReg, tableID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: present})),
		state.LaneFrozenTables:      state.State{}.FreezeTable(tableID),
		state.LaneEffectDeltas:      state.State{}.WriteEffectDelta(effectKey, effectValue),
		state.LaneEscapeEvents:      state.State{}.AddEscapeEvent(escapeFact),
		state.LaneChannelSelect:     state.State{}.AddChannelSelectFact(channel),
		state.LaneStoreRelations:    state.State{}.AddStoreRelation(state.StoreRelation{Source: rootKey, Into: otherKey}),
		state.LaneKeyMemberships:    state.State{}.AddPathKeyMembership(rootKey, otherKey),
		state.LaneTypestates: state.State{}.AcquireTypestate(
			state.TypestateResourceFromCanonicalKey(rootKey, typestate.Protocol("root-transaction")),
			typestate.State("open"),
			typestate.Obligation{Final: typestate.State("closed")},
		),
		state.LanePlacement:     state.State{}.WritePlacement(tableID, placement.Stack),
		state.LaneLenFloors:     state.State{}.WriteLenFloor(ks, rootKey, 2),
		state.LaneNumFloors:     state.State{}.WriteNumFloor(ks, rootKey, 3),
		state.LaneNumCeils:      state.State{}.WriteNumCeil(ks, rootKey, 7),
		state.LaneDiffRelations: state.State{}.WriteDiffConstraint(state.RelValueOperand(rootKey), state.RelValueOperand(otherKey), -1),
		state.LaneUserLattices:  state.State{}.WriteUserElement(axisReg, ks, userAxis, rootKey, "Tainted"),
	}
}

func concreteRootTransactionRegistry(t *testing.T, userAxis userlattice.AxisID) *axis.Registry {
	t.Helper()
	reg := axis.NewRegistry()
	axis.Register(reg, variantorigin.Spec())
	axis.Register(reg, identity.Spec())
	axis.Register(reg, runtimekind.Spec())
	axis.Register(reg, typewitness.Spec())
	axis.Register(reg, escape.Spec())
	axis.Register(reg, evidence.Spec())
	axis.Register(reg, assertion.Spec())
	if _, err := userlattice.Register(reg, testTaintSpec(userAxis)); err != nil {
		t.Fatalf("register user state lane: %v", err)
	}
	return reg.Freeze()
}

func concreteTransactionStateKey(t *testing.T, raw string) pathaddr.StateKey {
	t.Helper()
	got, ok := pathaddr.StateKeyFromPathKey(pathdom.PathKey(raw))
	if !ok {
		t.Fatalf("StateKeyFromPathKey(%q) failed", raw)
	}
	return got
}

func branchEdgeTransactionResolver(point cfg.Point, symbols ...symbol.ID) *visibility.Resolver {
	builder := visibility.NewBuilder()
	for _, sym := range symbols {
		builder.Define(point, sym, "branch-edge")
	}
	return visibility.NewResolver(builder.Build())
}

func mustCoordinateFactorInventory(
	t testing.TB,
	authority *PathSemanticAuthority,
	domain state.ProductDomain,
	slots []state.CoordinateSlot,
) state.CoordinateFactorInventory {
	t.Helper()
	inventory, err := authority.SealCoordinateFactorInventory(domain, slots)
	if err != nil {
		t.Fatal(err)
	}
	return inventory
}

func mustPresenceCoordinateFactorInventory(
	t testing.TB,
	plan PresenceImplicationPlan,
	domain state.ProductDomain,
	slots []state.CoordinateSlot,
) state.CoordinateFactorInventory {
	t.Helper()
	inventory, err := plan.SealCoordinateFactorInventory(domain, slots)
	if err != nil {
		t.Fatal(err)
	}
	return inventory
}
