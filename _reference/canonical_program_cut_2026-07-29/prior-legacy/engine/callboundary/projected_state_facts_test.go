package callboundary

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/escapeevent"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestProjectedStatePathProjectorOwnsFormalBoundaryRootsAndDescendants(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))
	inputRoot, ok := keys.InternFormalRoot(formal.NewRoot(owner, 1, formal.Input))
	if !ok {
		t.Fatal("intern formal Input root")
	}
	outputRoot, ok := keys.InternFormalRoot(formal.NewRoot(owner, 1, formal.Output))
	if !ok {
		t.Fatal("intern formal Output root")
	}
	member := segment.Segment{Kind: segment.SegmentField, Name: "member"}
	inputChild, ok := keys.AppendSegment(inputRoot, member)
	if !ok {
		t.Fatal("append formal Input descendant")
	}
	outputChild, ok := keys.AppendSegment(outputRoot, member)
	if !ok {
		t.Fatal("append formal Output descendant")
	}
	projector, err := newProjectedStatePathProjector(reg, keys, state.BoundaryRoots{
		{Path: inputRoot, Value: product.Top()},
		{Path: outputRoot, Value: product.Top()},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string]struct {
		key  keyspace.Key
		want pathdom.Path
	}{
		"input-root":   {inputRoot, pathdom.NewPlaceholder(0)},
		"input-child":  {inputChild, pathdom.NewPlaceholder(0).Field("member")},
		"output-root":  {outputRoot, pathdom.Path{Root: "ret[0]"}},
		"output-child": {outputChild, pathdom.Path{Root: "ret[0]"}.Field("member")},
	} {
		got, exact := projector.key(source.key)
		if !exact || !got.Equal(source.want) {
			t.Fatalf("%s projection = %#v/%t, want %#v", name, got, exact, source.want)
		}
	}
	encoded := pathaddr.StateKey(keys.FormatReadOnly(inputChild))
	if got, exact := projector.stateKey(encoded); !exact || !got.Equal(pathdom.NewPlaceholder(0).Field("member")) {
		t.Fatalf("encoded formal StateKey projection = %#v/%t", got, exact)
	}
	value := typevalue.LiteralString(reg, "formal-descendant")
	world := state.Domain(reg).Bottom().WriteLocalPathKey(reg, inputChild, value)
	facts, err := NormalReturnFactsFromProjectedState(reg, keys, world, state.BoundaryRoots{
		{Path: inputRoot, Value: product.Top()},
		{Path: outputRoot, Value: product.Top()},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.PathRefinements) != 1 || !facts.PathRefinements[0].Path.Equal(pathdom.NewPlaceholder(0).Field("member")) ||
		!product.Equal(reg, facts.PathRefinements[0].Value, value) {
		t.Fatalf("formal descendant lane projection = %#v", facts.PathRefinements)
	}
	foreign := keyspace.New()
	foreignRoot, _ := foreign.InternFormalRoot(formal.NewRoot(owner, 1, formal.Input))
	if _, err := newProjectedStatePathProjector(reg, keys, state.BoundaryRoots{{Path: foreignRoot, Value: product.Top()}}, 1); err == nil {
		t.Fatal("isomorphic foreign formal root was admitted")
	}
}

func TestProjectedStateFactDescriptorClassificationIsComplete(t *testing.T) {
	want := map[NormalReturnFactLaneID]projectedStateFactOwnership{
		LanePathRefinements: projectedStateFactState, LanePersistentPathWrites: projectedStateFactSyntax,
		LanePathStaticMembers: projectedStateFactState, LanePathStaticMemberDeltas: projectedStateFactSyntax,
		LanePathPresenceImplications: projectedStateFactState, LanePathInvalidations: projectedStateFactMixed,
		LaneDynamicIndexFacts: projectedStateFactMixed, LaneKeyMemberships: projectedStateFactState,
		LaneDynamicValueKeys: projectedStateFactMixed, LaneDynamicAllValues: projectedStateFactState,
		LaneBranchProofs: projectedStateFactState, LaneChannelSelects: projectedStateFactState,
		LaneFrozenTables: projectedStateFactState, LaneEffectDeltas: projectedStateFactState,
		LaneEscapeEvents: projectedStateFactState, LaneStoreRelations: projectedStateFactMixed,
		LaneLifecycleFacts: projectedStateFactSyntax, LaneNumFloors: projectedStateFactState,
		LaneNumCeils: projectedStateFactState, LaneRelConstraints: projectedStateFactState,
	}
	if len(projectedStateFactHandlers) != len(NormalReturnFactDescriptors()) || len(want) != len(NormalReturnFactDescriptors()) {
		t.Fatalf("classification=%d want=%d descriptors=%d", len(projectedStateFactHandlers), len(want), len(NormalReturnFactDescriptors()))
	}
	for _, binding := range projectedStateFactHandlers {
		if want[binding.ID] != binding.Value.ownership {
			t.Fatalf("descriptor %q ownership=%d want=%d", binding.ID, binding.Value.ownership, want[binding.ID])
		}
		delete(want, binding.ID)
	}
	if len(want) != 0 {
		t.Fatalf("unclassified descriptors: %#v", want)
	}
}

func TestNormalReturnFactsFromProjectedStateCoversApplicableStateLanes(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	root := pathdom.Path{Root: "arg", Symbol: symbol.ID(801), Version: 1}
	child, other, static := root.Field("child"), root.Field("other"), root.Field("static")
	rootKey, childKey, otherKey := keys.FromPath(root), keys.FromPath(child), keys.FromPath(other)
	staticKey := keys.FromPath(static)
	rootState, childState, otherState := stateKey(t, root), stateKey(t, child), stateKey(t, other)
	id := identity.ID{Kind: "lua.table", Site: "projected-state", Index: 1}
	idValue := identityvalue.Present(reg, id)
	present := typevalue.LiteralString(reg, "present")
	dynamicSite := dynamicindex.Site("projected-dynamic")
	dynamicFact := dynamicindex.NewFact(reg, dynamicindex.FactConfig{KeyValue: present, HasKeyValue: true, Value: present, HasValue: true, Admission: dynamicindex.AdmissionAdmitted})
	effect := effectdelta.Value{Before: present, After: idValue, Change: effectdelta.ChangeChanged}

	world := state.Domain(reg).Bottom().
		WriteValue(reg, key.SymbolValue(root.Symbol), idValue).
		WriteLocalPathKey(reg, childKey, present).
		WriteLocalPathStaticMember(staticKey, present).
		AddPathPresenceImplication(pathevidence.NewPathPresenceImplication(childKey, presence.Present(), otherKey, presence.Present())).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: rootKey, Site: dynamicSite}, dynamicFact).
		AddPathKeyMembership(childState, rootState).
		AddDynamicIndexValueKeyMembership(rootKey, dynamicSite, rootState).
		AddDynamicIndexAllValuesKeyMembership(rootKey, rootState).
		AddBranchProof(pathevidence.BranchProof{Kind: pathevidence.BranchProofPathPresence, Path: childKey, Presence: presence.Present()}).
		AddChannelSelectFact(channelselectfact.Fact{Select: "projected-select", Kind: channelselectfact.FactSelect, Result: childState}).
		WriteHeapTableObject(reg, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: idValue})).
		FreezeTable(id).
		WriteEffectDelta(effectdelta.Key{Target: rootKey, Site: "projected-effect", Kind: effectdelta.Call}, effect).
		AddEscapeEvent(state.EscapeEvent{Target: rootState, Kind: escapeevent.KindSend, Recursive: true}).
		AddStoreRelation(state.StoreRelation{Source: childState, Into: rootState}).
		AcquireTypestate(state.TypestateResourceFromCanonicalKey(rootState, "projected-protocol"), "open", typestate.Obligation{Final: "closed"}).
		WritePlacement(id, placement.Stack).
		WriteLenFloor(keys, rootState, 1).
		WriteNumFloor(keys, childState, 2).
		WriteNumCeil(keys, childState, 9).
		WriteDiffConstraint(state.RelValueOperand(childState), state.RelValueOperand(otherState), 3)

	roots := state.BoundaryRoots{{Slot: key.SymbolValue(root.Symbol), Path: rootKey, Value: idValue}}
	artifact, err := state.ProjectBoundary(reg, keys, world, roots)
	if err != nil {
		t.Fatal(err)
	}
	projected, projectedRoots, err := artifact.ProjectedWorld(reg, keys)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := NormalReturnFactsFromProjectedState(reg, keys, projected, projectedRoots, 1)
	if err != nil {
		t.Fatal(err)
	}
	wantNonEmpty := map[NormalReturnFactLaneID]bool{
		LanePathRefinements: true, LanePathStaticMembers: true, LanePathPresenceImplications: true,
		LanePathInvalidations: true, LaneDynamicIndexFacts: true, LaneKeyMemberships: true,
		LaneDynamicValueKeys: true, LaneDynamicAllValues: true, LaneBranchProofs: true,
		LaneChannelSelects: true, LaneFrozenTables: true, LaneEffectDeltas: true,
		LaneEscapeEvents: true, LaneStoreRelations: true, LaneNumFloors: true, LaneNumCeils: true,
		LaneRelConstraints: true,
	}
	for _, lane := range NormalReturnFactLanes() {
		got := lane.Len(facts)
		if wantNonEmpty[lane.ID()] && got == 0 {
			t.Errorf("State-derived descriptor %q emitted no fact", lane.ID())
		}
		if !wantNonEmpty[lane.ID()] && got != 0 {
			t.Errorf("syntax-owned descriptor %q unexpectedly emitted %d facts", lane.ID(), got)
		}
	}
	for _, fact := range facts.PathRefinements {
		if fact.Path.PlaceholderIndex() != 0 {
			t.Fatalf("parameter path was not rebound exactly: %#v", fact.Path)
		}
	}
	wantNumCeilPath := pathdom.NewPlaceholder(0).Field("child")
	if len(facts.NumCeils) != 1 || !facts.NumCeils[0].Path.Equal(wantNumCeilPath) || facts.NumCeils[0].Ceil != 9 {
		t.Fatalf("projected NumCeils = %#v, want single $0.child <= 9 fact", facts.NumCeils)
	}
	factorFacts := projectedNormalReturnFactsFromFactorView(t, state.RegisteredProductDomain(reg), reg, keys, world, roots, projectedRoots, 1)
	if !reflect.DeepEqual(factorFacts, facts) {
		t.Fatalf("factor-view facts differ from canonical State emitter:\nfactor=%#v\nstate=%#v", factorFacts, facts)
	}
}

func TestNormalReturnFactsFactorViewMatchesReducedProductDomain(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	domain, err := state.TryRegisteredProductDomainWithLanes(reg, []state.LaneID{state.LaneValues, state.LanePathEvidence})
	if err != nil {
		t.Fatal(err)
	}
	root := pathdom.Path{Root: "arg", Symbol: symbol.ID(811), Version: 1}
	rootKey, childKey := keys.FromPath(root), keys.FromPath(root.Field("child"))
	value := typevalue.LiteralString(reg, "reduced")
	world := domain.Lattice().Bottom().WriteLocalPathKey(reg, childKey, value)
	roots := state.BoundaryRoots{{Slot: key.SymbolValue(root.Symbol), Path: rootKey, Value: product.Top()}}
	artifact, err := state.ProjectBoundary(reg, keys, world, roots)
	if err != nil {
		t.Fatal(err)
	}
	projected, projectedRoots, err := artifact.ProjectedWorld(reg, keys)
	if err != nil {
		t.Fatal(err)
	}
	want, err := NormalReturnFactsFromProjectedState(reg, keys, projected, projectedRoots, 1)
	if err != nil {
		t.Fatal(err)
	}
	got := projectedNormalReturnFactsFromFactorView(t, domain, reg, keys, world, roots, projectedRoots, 1)
	if !reflect.DeepEqual(got, want) || len(got.PathRefinements) != 1 {
		t.Fatalf("reduced factor-view parity:\nfactor=%#v\nstate=%#v", got, want)
	}
}

func projectedNormalReturnFactsFromFactorView(
	t *testing.T,
	domain state.ProductDomain,
	reg *axis.Registry,
	keys *keyspace.KeySpace,
	world state.State,
	roots state.BoundaryRoots,
	projectedRoots state.BoundaryRoots,
	paramCount int,
) NormalReturnFacts {
	t.Helper()
	allFactors, err := domain.Decompose(world)
	if err != nil {
		t.Fatal(err)
	}
	programs := make([]state.BoundaryReachabilityProgram, len(allFactors))
	for index := range allFactors {
		programs[index], err = domain.PrepareBoundaryFactorReachability(keys, allFactors[index])
		if err != nil {
			t.Fatal(err)
		}
	}
	programSet, err := state.SealBoundaryReachabilityProgramSet(programs...)
	if err != nil {
		t.Fatal(err)
	}
	factorRoots := make([]state.BoundaryFactorRoot, len(roots))
	seedValues := make([]product.Value, len(roots))
	for index := range roots {
		factorRoots[index] = state.BoundaryFactorRoot{Slot: roots[index].Slot, Path: roots[index].Path}
		seedValues[index] = roots[index].Value
	}
	selection, err := state.SealBoundaryFactorSelection(keys, factorRoots, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	selection, err = programSet.Close(selection, seedValues)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := domain.PrepareBoundaryFactorView(selection, NormalReturnFactSourceLanes())
	if err != nil {
		t.Fatal(err)
	}
	factors, err := domain.DecomposeLanes(world, plan.Lanes())
	if err != nil {
		t.Fatal(err)
	}
	view, err := plan.Project(factors)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := NormalReturnFactsFromProjectedSource(reg, keys, view, projectedRoots, paramCount)
	if err != nil {
		t.Fatal(err)
	}
	return facts
}

func TestProjectedStateFactsJoinAlternativesBeforeProjectionPreservesMustSemantics(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	root := pathdom.Path{Root: "arg", Symbol: symbol.ID(901), Version: 1}
	child := root.Field("child")
	rootKey, childKey := keys.FromPath(root), keys.FromPath(child)
	proof := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathPresence, Path: childKey, Presence: presence.Present()}
	left := state.Domain(reg).Bottom().AddBranchProof(proof).WriteLocalPathKey(reg, childKey, typevalue.LiteralString(reg, "left"))
	right := state.Domain(reg).Bottom().WriteLocalPathKey(reg, childKey, typevalue.LiteralString(reg, "right"))
	joined := state.Domain(reg).Join(left, right)
	roots := state.BoundaryRoots{{Slot: key.SymbolValue(root.Symbol), Path: rootKey, Value: product.Top()}}
	artifact, err := state.ProjectBoundary(reg, keys, joined, roots)
	if err != nil {
		t.Fatal(err)
	}
	projected, projectedRoots, err := artifact.ProjectedWorld(reg, keys)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := NormalReturnFactsFromProjectedState(reg, keys, projected, projectedRoots, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.BranchProofs) != 0 {
		t.Fatal("must proof from only one normal-return alternative escaped the joined State")
	}
}

func TestProjectedStateFactsRetainsOnlyExplicitConcreteBoundaryRoots(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	visible := pathdom.Path{Root: "captured", Symbol: symbol.ID(951), Version: 1}
	hidden := pathdom.Path{Root: "local", Symbol: symbol.ID(952), Version: 1}
	visibleChild, hiddenChild := visible.Field("child"), hidden.Field("child")
	value := typevalue.LiteralString(reg, "visible")
	world := state.Domain(reg).Bottom().
		WriteLocalPathKey(reg, keys.FromPath(visibleChild), value).
		WriteLocalPathKey(reg, keys.FromPath(hiddenChild), value)
	roots := state.BoundaryRoots{{Slot: key.SymbolValue(visible.Symbol), Path: keys.FromPath(visible), Value: product.Top()}}
	artifact, err := state.ProjectBoundary(reg, keys, world, roots)
	if err != nil {
		t.Fatal(err)
	}
	projected, projectedRoots, err := artifact.ProjectedWorld(reg, keys)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := NormalReturnFactsFromProjectedState(reg, keys, projected, projectedRoots, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.PathRefinements) != 1 || !facts.PathRefinements[0].Path.Equal(visibleChild) {
		t.Fatalf("concrete boundary refinements = %#v, want only explicit capture/global root", facts.PathRefinements)
	}
}

func TestProjectedStateFactsRetainCapturedResolverFactsThroughStableValueRoot(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	captured := pathdom.Path{Symbol: symbol.ID(971), Version: 1}
	capturedKey := keys.FromPath(captured)
	stableKey, ok := keys.FromStateKey(pathaddr.SymbolPathKey(captured.Symbol, nil))
	if !ok {
		t.Fatal("captured stable key missing")
	}
	value := typevalue.LiteralString(reg, "captured")
	site := dynamicindex.Site("captured-dynamic")
	dynamic := dynamicindex.NewFact(reg, dynamicindex.FactConfig{
		KeyValue: value, HasKeyValue: true,
		Value: value, HasValue: true,
		Admission: dynamicindex.AdmissionAdmitted,
	})
	world := state.Domain(reg).Bottom().
		WriteValue(reg, key.SymbolValue(captured.Symbol), product.Top()).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: capturedKey, Site: site}, dynamic)
	roots := state.BoundaryRoots{
		{Slot: key.SymbolValue(captured.Symbol), Path: stableKey, Value: product.Top()},
		{Slot: key.SymbolValue(captured.Symbol), Path: capturedKey, Value: product.Top()},
	}

	facts := projectedNormalReturnFacts(t, reg, keys, world, roots, 0)
	if len(facts.DynamicIndexFacts) != 1 ||
		!facts.DynamicIndexFacts[0].Table.Equal(captured) ||
		facts.DynamicIndexFacts[0].Site != site ||
		!dynamicindex.Domain(reg).Equal(facts.DynamicIndexFacts[0].Value, dynamic) {
		t.Fatalf("captured dynamic-index facts = %#v, want exact resolver-root fact", facts.DynamicIndexFacts)
	}
}

func TestProjectedStateFactsRebaseReturnedStaticMember(t *testing.T) {
	reg := standard.Registry()
	keys := keyspace.New()
	returned := pathdom.Path{Symbol: symbol.ID(972), Version: 1}
	member := returned.Field("get_x")
	memberValue := typevalue.LiteralString(reg, "member")
	world := state.Domain(reg).Bottom().WriteLocalPathStaticMember(keys.FromPath(member), memberValue)
	roots := state.BoundaryRoots{{Slot: key.ReturnSlot(0), Path: keys.FromPath(returned), Value: product.Top()}}

	facts := projectedNormalReturnFacts(t, reg, keys, world, roots, 0)
	want := pathdom.Path{Root: "ret[0]"}.Field("get_x")
	if len(facts.PathStaticMembers) != 1 ||
		!facts.PathStaticMembers[0].Path.Equal(want) ||
		!product.Equal(reg, facts.PathStaticMembers[0].Value, memberValue) {
		t.Fatalf("returned static members = %#v, want %s with exact value", facts.PathStaticMembers, want)
	}
}

func projectedNormalReturnFacts(
	t *testing.T,
	reg *axis.Registry,
	keys *keyspace.KeySpace,
	world state.State,
	roots state.BoundaryRoots,
	paramCount int,
) NormalReturnFacts {
	t.Helper()
	artifact, err := state.ProjectBoundary(reg, keys, world, roots)
	if err != nil {
		t.Fatal(err)
	}
	projected, projectedRoots, err := artifact.ProjectedWorld(reg, keys)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := NormalReturnFactsFromProjectedState(reg, keys, projected, projectedRoots, paramCount)
	if err != nil {
		t.Fatal(err)
	}
	return facts
}

func stateKey(t *testing.T, path pathdom.Path) pathaddr.StateKey {
	t.Helper()
	key, ok := pathaddr.StateKeyFromPathKey(path.Key())
	if !ok {
		t.Fatalf("invalid state path %q", path.Key())
	}
	return key
}
