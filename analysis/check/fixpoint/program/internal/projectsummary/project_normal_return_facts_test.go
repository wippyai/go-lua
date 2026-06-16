package projectsummary

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestFromResultProjectsNormalReturnFactsFromExitSnapshots(t *testing.T) {
	reg := standard.Registry()
	param0 := symbol.ID(901)
	param1 := symbol.ID(902)
	value0 := presentProduct(reg)
	value1 := absentProduct(reg)

	refineKey := normalReturnFactProjectTestKey(param0, ".refined")
	staticKey := normalReturnFactProjectTestKey(param0, ".member")
	dynAdmittedKey := normalReturnFactProjectTestKey(param0, `.items["admitted"]`)
	dynRejectedKey := normalReturnFactProjectTestKey(param0, `.items["rejected"]`)
	dynUnknownKey := normalReturnFactProjectTestKey(param1, ".items")
	branchPresenceKey := normalReturnFactProjectTestKey(param0, ".ready")
	branchEqualLeftKey := normalReturnFactProjectTestKey(param0, ".left")
	branchEqualRightKey := normalReturnFactProjectTestKey(param1, ".right")
	branchNotEqualLeftKey := normalReturnFactProjectTestKey(param0, ".a")
	branchNotEqualRightKey := normalReturnFactProjectTestKey(param1, ".b")
	selectResultKey := normalReturnFactProjectTestKey(param0, ".selectResult")
	receiveResultKey := normalReturnFactProjectTestKey(param0, ".receiveResult")
	receiveCaseKey := normalReturnFactProjectTestKey(param1, ".receiveCase")
	casePathKey := normalReturnFactProjectTestKey(param1, ".casePath")
	mutationKey := normalReturnFactProjectTestKey(param0, ".mutation")
	invalidationKey := normalReturnFactProjectTestKey(param1, ".invalidated")
	escapeKey := normalReturnFactProjectTestKey(param0, ".escape")
	sendEventKey := normalReturnFactProjectTestKey(param0, ".sent")
	freezeEventKey := normalReturnFactProjectTestKey(param1, ".pathFrozen")
	callKey := normalReturnFactProjectTestKey(param1, ".call")
	frozenID := identity.ID{Kind: "lua.table", Site: "project-freeze", Index: 1}
	frozenValue := product.Set(reg, value0, identity.Key, identity.Singleton(frozenID))
	staticFrozenKey := normalReturnFactProjectTestKey(param0, ".frozenMember")
	staticFrozenID := identity.ID{Kind: "lua.table", Site: "project-freeze-static", Index: 1}
	staticFrozenValue := product.Set(reg, value0, identity.Key, identity.Singleton(staticFrozenID))
	heapFrozenID := identity.ID{Kind: "lua.table", Site: "project-freeze-heap", Index: 1}
	heapFrozenValue := product.Set(reg, value0, identity.Key, identity.Singleton(heapFrozenID))

	exit := state.State{}.
		WriteValue(reg, key.SymbolValue(param0), frozenValue).
		WritePathKey(reg, refineKey, value0).
		WritePathStaticMember(staticKey, product.Top()).
		WritePathStaticMember(staticFrozenKey, staticFrozenValue).
		WriteHeapTableObject(reg, frozenID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: frozenValue,
			StaticMembers: map[pathdom.PathKey]product.Value{
				pathdom.PathKey(".heapChild"): heapFrozenValue,
				pathdom.PathKey(".self"):      frozenValue,
			},
		})).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: dynAdmittedKey, Site: "dyn-admitted"}, dynamicindex.Fact{
			KeyPresence: presence.Present(),
			KeyValue:    value0,
			Value:       value1,
			Admission:   dynamicindex.AdmissionAdmitted,
		}).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: dynRejectedKey, Site: "dyn-rejected"}, dynamicindex.Fact{
			KeyPresence: presence.Absent(),
			KeyValue:    value1,
			Value:       value0,
			Admission:   dynamicindex.AdmissionRejected,
		}).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: dynUnknownKey, Site: "dyn-unknown"}, dynamicindex.Fact{
			KeyPresence: presence.Maybe(),
			KeyValue:    value0,
			Value:       value1,
			Admission:   dynamicindex.AdmissionUnknown,
		}).
		AddBranchProof(pathevidence.BranchProof{
			Kind:     pathevidence.BranchProofPathPresence,
			Path:     branchPresenceKey,
			Presence: presence.Present(),
		}).
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathEqual,
			Path:  branchEqualLeftKey,
			Other: branchEqualRightKey,
		}).
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathNotEqual,
			Path:  branchNotEqualLeftKey,
			Other: branchNotEqualRightKey,
		}).
		AddChannelSelectFact(channelselectfact.Fact{
			Select:     "select-kind",
			Kind:       channelselectfact.FactSelect,
			Result:     selectResultKey,
			Index:      0,
			HasDefault: true,
		}).
		AddChannelSelectFact(channelselectfact.Fact{
			Select: "receive-kind",
			Kind:   channelselectfact.FactReceive,
			Result: receiveResultKey,
			Case:   receiveCaseKey,
			Index:  1,
		}).
		AddChannelSelectFact(channelselectfact.Fact{
			Select: "case-kind",
			Kind:   channelselectfact.FactCase,
			Case:   casePathKey,
			Index:  2,
		}).
		WriteEffectDelta(effectdelta.Key{
			Target: mutationKey,
			Site:   "effect-mutation",
			Kind:   effectdelta.Mutation,
		}, effectdelta.Value{
			Before: value0,
			After:  value1,
			Change: effectdelta.ChangeChanged,
		}).
		WriteEffectDelta(effectdelta.Key{
			Target: invalidationKey,
			Site:   callboundary.PathInvalidationEffectSite(),
			Kind:   effectdelta.Mutation,
		}, effectdelta.Top()).
		WriteEffectDelta(effectdelta.Key{
			Target: escapeKey,
			Site:   "effect-escape",
			Kind:   effectdelta.Escape,
		}, effectdelta.Value{
			Before: value0,
			After:  value0,
			Change: effectdelta.ChangeNone,
		}).
		WriteEffectDelta(effectdelta.Key{
			Target: sendEventKey,
			Site:   callboundary.EscapeEventEffectSite(callboundary.EscapeEventSend, true),
			Kind:   effectdelta.Escape,
		}, effectdelta.Top()).
		WriteEffectDelta(effectdelta.Key{
			Target: freezeEventKey,
			Site:   callboundary.FrozenTableEffectSite(),
			Kind:   effectdelta.Freeze,
		}, effectdelta.Top()).
		WriteEffectDelta(effectdelta.Key{
			Target: callKey,
			Site:   "effect-call",
			Kind:   effectdelta.Call,
		}, effectdelta.Value{
			Before: value1,
			After:  value0,
			Change: effectdelta.ChangeUnknown,
		}).
		FreezeTable(frozenID).
		FreezeTable(staticFrozenID).
		FreezeTable(heapFrozenID)

	got := FromResult(normalReturnFactProjectTestResult(reg, exit, param0, param1)).NormalReturnFacts

	if len(got.PathRefinements) != 1 ||
		!got.PathRefinements[0].Path.Equal(pathdom.NewPlaceholder(0).Field("refined")) ||
		!product.Equal(reg, got.PathRefinements[0].Value, value0) {
		t.Fatalf("PathRefinements = %#v, want $0.refined", got.PathRefinements)
	}
	assertPathStaticMember(t, reg, got.PathStaticMembers, pathdom.NewPlaceholder(0).Field("member"), product.Top())
	assertPathStaticMember(t, reg, got.PathStaticMembers, pathdom.NewPlaceholder(0).Field("frozenMember"), staticFrozenValue)

	assertDynamicAdmission(t, got.DynamicIndexFacts, "dyn-admitted", pathdom.NewPlaceholder(0).Field("items").IndexStr("admitted"), dynamicindex.AdmissionAdmitted)
	assertDynamicAdmission(t, got.DynamicIndexFacts, "dyn-rejected", pathdom.NewPlaceholder(0).Field("items").IndexStr("rejected"), dynamicindex.AdmissionRejected)
	assertDynamicAdmission(t, got.DynamicIndexFacts, "dyn-unknown", pathdom.NewPlaceholder(1).Field("items"), dynamicindex.AdmissionUnknown)

	assertBranchProof(t, got.BranchProofs, pathevidence.BranchProofPathPresence, pathdom.NewPlaceholder(0).Field("ready"), pathdom.Path{}, presence.Present())
	assertBranchProof(t, got.BranchProofs, pathevidence.BranchProofPathEqual, pathdom.NewPlaceholder(0).Field("left"), pathdom.NewPlaceholder(1).Field("right"), presence.Bottom())
	assertBranchProof(t, got.BranchProofs, pathevidence.BranchProofPathNotEqual, pathdom.NewPlaceholder(0).Field("a"), pathdom.NewPlaceholder(1).Field("b"), presence.Bottom())

	selectFact := assertChannelSelect(t, got.ChannelSelects, "select-kind", channelselectfact.FactSelect, pathdom.NewPlaceholder(0).Field("selectResult"), pathdom.Path{})
	if !selectFact.HasDefault {
		t.Fatalf("select-kind HasDefault = false, want true")
	}
	assertChannelSelect(t, got.ChannelSelects, "receive-kind", channelselectfact.FactReceive, pathdom.NewPlaceholder(0).Field("receiveResult"), pathdom.NewPlaceholder(1).Field("receiveCase"))
	assertChannelSelect(t, got.ChannelSelects, "case-kind", channelselectfact.FactCase, pathdom.Path{}, pathdom.NewPlaceholder(1).Field("casePath"))

	assertEffectDelta(t, got.EffectDeltas, "effect-mutation", pathdom.NewPlaceholder(0).Field("mutation"), effectdelta.Mutation, effectdelta.ChangeChanged)
	assertPathInvalidation(t, got.PathInvalidations, pathdom.NewPlaceholder(1).Field("invalidated"))
	assertEffectDelta(t, got.EffectDeltas, "effect-escape", pathdom.NewPlaceholder(0).Field("escape"), effectdelta.Escape, effectdelta.ChangeNone)
	assertEffectDelta(t, got.EffectDeltas, "effect-call", pathdom.NewPlaceholder(1).Field("call"), effectdelta.Call, effectdelta.ChangeUnknown)
	assertEscapeEvent(t, got.EscapeEvents, pathdom.NewPlaceholder(0).Field("sent"), callboundary.EscapeEventSend, true)
	assertFrozenTable(t, got.FrozenTables, pathdom.NewPlaceholder(1).Field("pathFrozen"))
	assertFrozenTable(t, got.FrozenTables, pathdom.NewPlaceholder(0))
	assertFrozenTable(t, got.FrozenTables, pathdom.NewPlaceholder(0).Field("frozenMember"))
	assertFrozenTable(t, got.FrozenTables, pathdom.NewPlaceholder(0).Field("heapChild"))
	assertNoFrozenTable(t, got.FrozenTables, pathdom.NewPlaceholder(0).Field("self"))
}

func TestFromResultDropsNonParameterNormalReturnFactPaths(t *testing.T) {
	reg := standard.Registry()
	param := symbol.ID(911)
	value := presentProduct(reg)
	validParamKey := normalReturnFactProjectTestKey(param, ".kept")
	invalidKeys := []pathdom.PathKey{
		normalReturnFactProjectTestKey(symbol.ID(912), ".local"),
		pathdom.PathKey("sym911.versionless"),
		pathdom.PathKey("s911.stable"),
		pathdom.PathKey("ret[0].value"),
		pathdom.PathKey("global.value"),
		pathdom.PathKey("$1.outOfRange"),
		pathdom.PathKey(".unresolved"),
	}

	exit := state.State{}.
		WritePathKey(reg, validParamKey, value)
	for _, pathKey := range invalidKeys {
		exit = exit.WritePathKey(reg, pathKey, value)
	}
	exit = exit.
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathEqual,
			Path:  validParamKey,
			Other: invalidKeys[0],
		}).
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathNotEqual,
			Path:  invalidKeys[1],
			Other: validParamKey,
		}).
		AddChannelSelectFact(channelselectfact.Fact{
			Select: "invalid-result",
			Kind:   channelselectfact.FactReceive,
			Result: invalidKeys[1],
			Index:  0,
		})

	got := FromResult(normalReturnFactProjectTestResult(reg, exit, param)).NormalReturnFacts

	if len(got.PathRefinements) != 1 {
		t.Fatalf("PathRefinements = %#v, want only valid parameter paths", got.PathRefinements)
	}
	if findPathRefinement(got.PathRefinements, pathdom.NewPlaceholder(0).Field("kept")) == nil {
		t.Fatalf("PathRefinements = %#v, want parameter resolver key rebased", got.PathRefinements)
	}
	if len(got.BranchProofs) != 0 {
		t.Fatalf("BranchProofs = %#v, want proofs with non-parameter path or other path dropped", got.BranchProofs)
	}
	if len(got.ChannelSelects) != 0 {
		t.Fatalf("ChannelSelects = %#v, want fact with non-parameter result path dropped", got.ChannelSelects)
	}
}

func TestFromResultSkipsTopSnapshotsAndTopNormalReturnFacts(t *testing.T) {
	reg := standard.Registry()
	param := symbol.ID(921)
	topFacts := FromResult(
		normalReturnFactProjectTestResult(reg, state.Domain(reg).Top(), param),
	).NormalReturnFacts
	if !normalReturnFactsEmpty(topFacts) {
		t.Fatalf("NormalReturnFacts from top state = %#v, want empty", topFacts)
	}

	paramKey := normalReturnFactProjectTestKey(param, ".value")
	topDynamic := dynamicindex.Top()
	topEffect := effectdelta.Value{
		Before: product.Top(),
		After:  product.Top(),
		Change: effectdelta.ChangeUnknown,
	}
	exit := state.State{}.
		WritePathKey(reg, paramKey, product.Top()).
		WritePathStaticMember(normalReturnFactProjectTestKey(param, ".member"), product.Bottom(reg)).
		WriteDynamicIndexFact(reg, dynamicindex.Key{
			Table: normalReturnFactProjectTestKey(param, ".table"),
			Site:  "dynamic-top",
		}, topDynamic).
		AddBranchProof(pathevidence.BranchProof{
			Kind:     pathevidence.BranchProofPathPresence,
			Path:     normalReturnFactProjectTestKey(param, ".proof"),
			Presence: presence.Top(),
		}).
		AddChannelSelectFact(channelselectfact.Fact{
			Select: "invalid-kind",
			Kind:   0,
			Result: normalReturnFactProjectTestKey(param, ".result"),
			Index:  0,
		}).
		WriteEffectDelta(effectdelta.Key{
			Target: normalReturnFactProjectTestKey(param, ".effect"),
			Site:   "effect-top",
			Kind:   effectdelta.Mutation,
		}, topEffect)

	got := FromResult(normalReturnFactProjectTestResult(reg, exit, param)).NormalReturnFacts
	if !normalReturnFactsEmpty(got) {
		t.Fatalf("NormalReturnFacts = %#v, want top/no-op facts skipped", got)
	}
}

func TestFromResultProjectsHeapTableObjectsFromExitSnapshots(t *testing.T) {
	reg := standard.Registry()
	param := symbol.ID(931)
	tableID := identity.ID{Kind: "table", Site: "summary-project", Index: 1}
	memberKey := pathdom.PathKey(".child")
	value := presentProduct(reg)
	exit := state.State{}.WriteHeapTableObject(reg, tableID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:          value,
		StaticMembers: map[pathdom.PathKey]product.Value{memberKey: value},
	}))

	got := FromResult(normalReturnFactProjectTestResult(reg, exit, param)).HeapTableObjects
	object, ok := got[tableID]
	if !ok {
		t.Fatalf("HeapTableObjects = %#v, want projected object for %v", got, tableID)
	}
	if !product.Equal(reg, object.Root(), value) {
		t.Fatalf("projected heap object root = %#v, want %#v", object.Root(), value)
	}
	if member, ok := object.StaticMember(memberKey); !ok || !product.Equal(reg, member, value) {
		t.Fatalf("projected heap object member = %#v/%v, want %#v", member, ok, value)
	}
}

func TestFromResultProjectsFrozenRootAndNestedChildPaths(t *testing.T) {
	reg := standard.Registry()
	param := symbol.ID(932)
	rootID := identity.ID{Kind: "table", Site: "summary-freeze", Index: 1}
	childID := identity.ID{Kind: "table", Site: "summary-freeze", Index: 2}
	rootValue := product.Set(reg, presentProduct(reg), identity.Key, identity.Singleton(rootID))
	childValue := product.Set(reg, presentProduct(reg), identity.Key, identity.Singleton(childID))
	exit := state.State{}.
		WriteValue(reg, key.SymbolValue(param), rootValue).
		WriteHeapTableObject(reg, rootID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:          rootValue,
			StaticMembers: map[pathdom.PathKey]product.Value{pathdom.PathKey(".child"): childValue},
		})).
		WriteHeapTableObject(reg, childID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: childValue,
		})).
		FreezeTable(rootID).
		FreezeTable(childID)

	got := FromResult(normalReturnFactProjectTestResult(reg, exit, param)).NormalReturnFacts

	assertFrozenTable(t, got.FrozenTables, pathdom.NewPlaceholder(0))
	assertFrozenTable(t, got.FrozenTables, pathdom.NewPlaceholder(0).Field("child"))
}

func TestFromResultProjectsExitStoreRelations(t *testing.T) {
	reg := standard.Registry()
	param0 := symbol.ID(941)
	param1 := symbol.ID(942)

	source := pathdom.Path{Symbol: param0, Version: 1}.Field("stored")
	into := pathdom.Path{Symbol: param1, Version: 1}.Field("container")
	stub := normalReturnFactProjectResultStub{
		reg:   reg,
		graph: cfg.New(),
		exit: state.State{}.AddStoreRelation(state.StoreRelation{
			Source: source.Key(),
			Into:   into.Key(),
		}),
		slots: []key.Value{key.SymbolValue(param0), key.SymbolValue(param1)},
	}

	got := FromResult(stub).NormalReturnFacts
	if len(got.StoreRelations) != 1 ||
		!got.StoreRelations[0].Source.Equal(pathdom.NewPlaceholder(0).Field("stored")) ||
		!got.StoreRelations[0].Into.Equal(pathdom.NewPlaceholder(1).Field("container")) {
		t.Fatalf("StoreRelations = %#v, want exit store relation projected to placeholder paths", got.StoreRelations)
	}
}

func TestFromResultDoesNotProjectBranchLocalStoreRelations(t *testing.T) {
	reg := standard.Registry()
	stateDomain := state.Domain(reg)
	param0 := symbol.ID(943)
	param1 := symbol.ID(944)
	source := pathdom.Path{Symbol: param0, Version: 1}.Field("stored")
	into := pathdom.Path{Symbol: param1, Version: 1}.Field("container")
	leftBranch := state.State{}.AddStoreRelation(state.StoreRelation{
		Source: source.Key(),
		Into:   into.Key(),
	})
	exit := stateDomain.Join(leftBranch, state.State{})

	got := FromResult(normalReturnFactProjectResultStub{
		reg:   reg,
		graph: cfg.New(),
		exit:  exit,
		slots: []key.Value{key.SymbolValue(param0), key.SymbolValue(param1)},
	}).NormalReturnFacts
	if len(got.StoreRelations) != 0 {
		t.Fatalf("StoreRelations = %#v, want branch-local relation removed by exit join", got.StoreRelations)
	}
}

type normalReturnFactProjectResultStub struct {
	reg   *axis.Registry
	graph cfg.Graph
	exit  state.State
	slots []key.Value
}

func normalReturnFactProjectTestResult(
	reg *axis.Registry,
	exit state.State,
	params ...symbol.ID,
) normalReturnFactProjectResultStub {
	slots := make([]key.Value, len(params))
	for i, param := range params {
		slots[i] = key.SymbolValue(param)
	}
	return normalReturnFactProjectResultStub{
		reg:   reg,
		graph: cfg.New(),
		exit:  exit,
		slots: slots,
	}
}

func (r normalReturnFactProjectResultStub) Registry() *axis.Registry { return r.reg }

func (r normalReturnFactProjectResultStub) Graph() cfg.Graph { return r.graph }

func (r normalReturnFactProjectResultStub) ExitState() (state.State, bool) { return r.exit, true }

func (r normalReturnFactProjectResultStub) ReturnPoints() []cfg.Point { return nil }

func (r normalReturnFactProjectResultStub) ParameterValueSlots() []key.Value {
	if len(r.slots) == 0 {
		return nil
	}
	out := make([]key.Value, len(r.slots))
	copy(out, r.slots)
	return out
}

func normalReturnFactProjectTestKey(sym symbol.ID, suffix string) pathdom.PathKey {
	return pathdom.PathKey(pathaddr.VersionedRootString(sym, 1) + suffix)
}

func assertDynamicAdmission(
	t *testing.T,
	facts []callboundary.DynamicIndexFact,
	site string,
	table pathdom.Path,
	admission dynamicindex.Admission,
) {
	t.Helper()
	fact := findDynamicIndexFact(facts, site)
	if fact == nil || !fact.Table.Equal(table) || fact.Value.Admission != admission {
		t.Fatalf("dynamic index %q = %#v, want table %s admission %d", site, fact, table, admission)
	}
}

func assertBranchProof(
	t *testing.T,
	proofs []callboundary.BranchProof,
	kind pathevidence.BranchProofKind,
	path pathdom.Path,
	other pathdom.Path,
	wantPresence presence.Value,
) {
	t.Helper()
	for _, proof := range proofs {
		if proof.Kind != kind || !proof.Path.Equal(path) {
			continue
		}
		switch kind {
		case pathevidence.BranchProofPathPresence:
			if presence.Equal(proof.Presence, wantPresence) {
				return
			}
		case pathevidence.BranchProofPathEqual, pathevidence.BranchProofPathNotEqual:
			if proof.Other.Equal(other) {
				return
			}
		}
	}
	t.Fatalf("BranchProofs = %#v, want kind %d path %s other %s", proofs, kind, path, other)
}

func assertChannelSelect(
	t *testing.T,
	facts []callboundary.ChannelSelectFact,
	selectID string,
	kind channelselectfact.Kind,
	result pathdom.Path,
	casePath pathdom.Path,
) callboundary.ChannelSelectFact {
	t.Helper()
	for _, fact := range facts {
		if string(fact.Select) == selectID &&
			fact.Kind == kind &&
			fact.Result.Equal(result) &&
			fact.Case.Equal(casePath) {
			return fact
		}
	}
	t.Fatalf("ChannelSelects = %#v, want %q kind %d", facts, selectID, kind)
	return callboundary.ChannelSelectFact{}
}

func assertEffectDelta(
	t *testing.T,
	deltas []callboundary.EffectDelta,
	site string,
	target pathdom.Path,
	kind effectdelta.Kind,
	change effectdelta.Change,
) {
	t.Helper()
	delta := findEffectDelta(deltas, site)
	if delta == nil || !delta.Target.Equal(target) || delta.Kind != kind || delta.Value.Change != change {
		t.Fatalf("effect delta %q = %#v, want target %s kind %d change %d", site, delta, target, kind, change)
	}
}

func assertEscapeEvent(
	t *testing.T,
	events []callboundary.EscapeEventFact,
	target pathdom.Path,
	kind callboundary.EscapeEventKind,
	recursive bool,
) {
	t.Helper()
	for _, event := range events {
		if event.Target.Equal(target) && event.Kind == kind && event.Recursive == recursive {
			return
		}
	}
	t.Fatalf("escape events = %#v, want target %s kind %d recursive=%v", events, target, kind, recursive)
}

func assertFrozenTable(t *testing.T, facts []callboundary.FrozenTableFact, target pathdom.Path) {
	t.Helper()
	for _, fact := range facts {
		if fact.Target.Equal(target) {
			return
		}
	}
	t.Fatalf("FrozenTables = %#v, want target %s", facts, target)
}

func assertNoFrozenTable(t *testing.T, facts []callboundary.FrozenTableFact, target pathdom.Path) {
	t.Helper()
	for _, fact := range facts {
		if fact.Target.Equal(target) {
			t.Fatalf("FrozenTables = %#v, did not want target %s", facts, target)
		}
	}
}

func assertPathStaticMember(t *testing.T, reg *axis.Registry, facts []callboundary.PathStaticMemberFact, target pathdom.Path, want product.Value) {
	t.Helper()
	for _, fact := range facts {
		if fact.Path.Equal(target) && product.Equal(reg, fact.Value, want) {
			return
		}
	}
	t.Fatalf("PathStaticMembers = %#v, want %s = %#v", facts, target, want)
}

func assertPathInvalidation(t *testing.T, facts []callboundary.PathInvalidationFact, target pathdom.Path) {
	t.Helper()
	for _, fact := range facts {
		if fact.Path.Equal(target) {
			return
		}
	}
	t.Fatalf("PathInvalidations = %#v, want %s", facts, target)
}

func normalReturnFactsEmpty(facts callboundary.NormalReturnFacts) bool {
	return len(facts.PathRefinements) == 0 &&
		len(facts.PathStaticMembers) == 0 &&
		len(facts.PathInvalidations) == 0 &&
		len(facts.DynamicIndexFacts) == 0 &&
		len(facts.BranchProofs) == 0 &&
		len(facts.ChannelSelects) == 0 &&
		len(facts.FrozenTables) == 0 &&
		len(facts.EffectDeltas) == 0 &&
		len(facts.EscapeEvents) == 0 &&
		len(facts.StoreRelations) == 0
}

func findPathRefinement(facts []callboundary.PathValueFact, path pathdom.Path) *callboundary.PathValueFact {
	for i := range facts {
		if facts[i].Path.Equal(path) {
			return &facts[i]
		}
	}
	return nil
}

func findDynamicIndexFact(facts []callboundary.DynamicIndexFact, site string) *callboundary.DynamicIndexFact {
	for i := range facts {
		if string(facts[i].Site) == site {
			return &facts[i]
		}
	}
	return nil
}

func findEffectDelta(deltas []callboundary.EffectDelta, site string) *callboundary.EffectDelta {
	for i := range deltas {
		if string(deltas[i].Site) == site {
			return &deltas[i]
		}
	}
	return nil
}

func presentProduct(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Present())
}

func absentProduct(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Absent())
}
