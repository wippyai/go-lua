package projectsummary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestProjectSummaryReachabilityTreatsEntryAsReachablePoint(t *testing.T) {
	graph := cfg.New()
	body := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), body, false)
	graph.AddEdge(body, graph.Exit(), false)

	reach := cfg.NewReachability(graph)
	if !reach.CanReach(graph.Entry(), graph.Exit()) {
		t.Fatalf("entry should reach exit through project-summary reachability cache")
	}
	if !reach.CanReach(graph.Entry(), graph.Entry()) {
		t.Fatalf("entry should reach itself through project-summary reachability cache")
	}
}

func TestFromResultProjectsNormalReturnFactsFromExitSnapshots(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
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
	numFloorRootKey := pathdom.NewPath(param0, "param0").Key()
	numFloorMemberKey := normalReturnFactProjectTestKey(param1, ".index")
	relIKey := normalReturnFactProjectTestKey(param0, ".i")
	relJKey := normalReturnFactProjectTestKey(param1, ".j")
	relArrayKey := pathdom.NewPath(param0, "param0").Key()
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
	heapChildKey, ok := ks.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "heapChild"}})
	if !ok {
		t.Fatal("heapChild suffix key failed")
	}
	selfKey, ok := ks.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "self"}})
	if !ok {
		t.Fatal("self suffix key failed")
	}

	exit := state.State{}.
		WriteValue(reg, key.SymbolValue(param0), frozenValue).
		WritePathKey(reg, ks, refineKey, value0).
		WritePathStaticMember(ks, staticKey, product.Top()).
		WritePathStaticMember(ks, staticFrozenKey, staticFrozenValue).
		WriteHeapTableObject(reg, frozenID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: frozenValue,
			StaticMembers: map[keyspace.Key]product.Value{
				heapChildKey: heapFrozenValue,
				selfKey:      frozenValue,
			},
		})).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: mustStateKey(t, ks, dynAdmittedKey), Site: "dyn-admitted"}, dynamicindex.Fact{
			KeyPresence: presence.Present(),
			KeyValue:    value0,
			Value:       value1,
			Admission:   dynamicindex.AdmissionAdmitted,
		}).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: mustStateKey(t, ks, dynRejectedKey), Site: "dyn-rejected"}, dynamicindex.Fact{
			KeyPresence: presence.Absent(),
			KeyValue:    value1,
			Value:       value0,
			Admission:   dynamicindex.AdmissionRejected,
		}).
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: mustStateKey(t, ks, dynUnknownKey), Site: "dyn-unknown"}, dynamicindex.Fact{
			KeyPresence: presence.Maybe(),
			KeyValue:    value0,
			Value:       value1,
			Admission:   dynamicindex.AdmissionUnknown,
		}).
		AddBranchProof(pathevidence.BranchProof{
			Kind:     pathevidence.BranchProofPathPresence,
			Path:     mustStateKey(t, ks, branchPresenceKey),
			Presence: presence.Present(),
		}).
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathEqual,
			Path:  mustStateKey(t, ks, branchEqualLeftKey),
			Other: mustStateKey(t, ks, branchEqualRightKey),
		}).
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathNotEqual,
			Path:  mustStateKey(t, ks, branchNotEqualLeftKey),
			Other: mustStateKey(t, ks, branchNotEqualRightKey),
		}).
		WriteNumFloor(ks, testStateKey(t, numFloorRootKey), 1).
		WriteNumFloor(ks, testStateKey(t, numFloorMemberKey), 2).
		WriteScaledConstraint(
			1, state.RelValueOperand(testStateKey(t, relIKey)),
			1, state.RelValueOperand(testStateKey(t, relJKey)),
			state.RelLengthOperand(testStateKey(t, relArrayKey)),
			0,
		).
		AddChannelSelectFact(channelselectfact.Fact{
			Select:     "select-kind",
			Kind:       channelselectfact.FactSelect,
			Result:     testStateKey(t, selectResultKey),
			Index:      0,
			HasDefault: true,
		}).
		AddChannelSelectFact(channelselectfact.Fact{
			Select: "receive-kind",
			Kind:   channelselectfact.FactReceive,
			Result: testStateKey(t, receiveResultKey),
			Case:   testStateKey(t, receiveCaseKey),
			Index:  1,
		}).
		AddChannelSelectFact(channelselectfact.Fact{
			Select: "case-kind",
			Kind:   channelselectfact.FactCase,
			Case:   testStateKey(t, casePathKey),
			Index:  2,
		}).
		WriteEffectDelta(effectdelta.Key{
			Target: mustStateKey(t, ks, mutationKey),
			Site:   "effect-mutation",
			Kind:   effectdelta.Mutation,
		}, effectdelta.Value{
			Before: value0,
			After:  value1,
			Change: effectdelta.ChangeChanged,
		}).
		WriteEffectDelta(effectdelta.Key{
			Target: mustStateKey(t, ks, invalidationKey),
			Site:   callboundary.PathInvalidationEffectSite(),
			Kind:   effectdelta.Mutation,
		}, effectdelta.Top()).
		WriteEffectDelta(effectdelta.Key{
			Target: mustStateKey(t, ks, escapeKey),
			Site:   "effect-escape",
			Kind:   effectdelta.Escape,
		}, effectdelta.Value{
			Before: value0,
			After:  value0,
			Change: effectdelta.ChangeNone,
		}).
		AddEscapeEvent(state.EscapeEvent{
			Target:    testStateKey(t, sendEventKey),
			Kind:      callboundary.EscapeEventSend,
			Recursive: true,
		}).
		WriteEffectDelta(effectdelta.Key{
			Target: mustStateKey(t, ks, freezeEventKey),
			Site:   callboundary.FrozenTableEffectSite(),
			Kind:   effectdelta.Freeze,
		}, effectdelta.Top()).
		WriteEffectDelta(effectdelta.Key{
			Target: mustStateKey(t, ks, callKey),
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

	got := FromResult(normalReturnFactProjectTestResult(reg, ks, exit, param0, param1)).NormalReturnFacts

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
	assertNumFloor(t, got.NumFloors, pathdom.NewPlaceholder(0), 1)
	assertNumFloor(t, got.NumFloors, pathdom.NewPlaceholder(1).Field("index"), 2)
	assertRelConstraint(t, got.RelConstraints, pathdom.NewPlaceholder(0).Field("i"), pathdom.NewPlaceholder(1).Field("j"), pathdom.NewPlaceholder(0), true, 0)

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
	ks := keyspace.New()
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
		WritePathKey(reg, ks, validParamKey, value)
	for _, pathKey := range invalidKeys {
		exit = exit.WritePathKey(reg, ks, pathKey, value)
		if stateKey, ok := pathaddr.StateKeyFromPathKey(pathKey); ok {
			exit = exit.WriteNumFloor(ks, stateKey, 1)
		}
	}
	exit = exit.WriteNumFloor(ks, testStateKey(t, pathdom.NewPath(param, "param").Key()), 1)
	exit = exit.
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathEqual,
			Path:  mustStateKey(t, ks, validParamKey),
			Other: mustStateKey(t, ks, invalidKeys[0]),
		}).
		AddBranchProof(pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathNotEqual,
			Path:  mustStateKey(t, ks, invalidKeys[1]),
			Other: mustStateKey(t, ks, validParamKey),
		}).
		AddChannelSelectFact(channelselectfact.Fact{
			Select: "invalid-result",
			Kind:   channelselectfact.FactReceive,
			Result: testStateKey(t, invalidKeys[1]),
			Index:  0,
		})

	got := FromResult(normalReturnFactProjectTestResult(reg, ks, exit, param)).NormalReturnFacts

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
	if len(got.NumFloors) != 1 || !got.NumFloors[0].Path.Equal(pathdom.NewPlaceholder(0)) || got.NumFloors[0].Floor != 1 {
		t.Fatalf("NumFloors = %#v, want only valid root parameter floor", got.NumFloors)
	}
}

func TestFromResultDoesNotProjectExitNumFloorForReassignedParameter(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	param := symbol.ID(913)
	paramKey := pathdom.NewPath(param, "param").Key()
	exit := state.State{}.WriteNumFloor(ks, testStateKey(t, paramKey), 2)

	got := FromResult(normalReturnFactProjectResultStub{
		reg:        reg,
		graph:      cfg.New(),
		exit:       exit,
		slots:      []key.Value{key.SymbolValue(param)},
		keys:       ks,
		reassigned: map[key.Value]struct{}{key.SymbolValue(param): {}},
	}).NormalReturnFacts

	if len(got.NumFloors) != 0 {
		t.Fatalf("NumFloors = %#v, want no caller floor evidence for reassigned callee parameter", got.NumFloors)
	}
}

func TestProjectNormalReturnParamConditionsUseBranchPathEvidence(t *testing.T) {
	reg := standard.Registry()
	param := symbol.ID(924)
	graph, branch, ret, _ := normalReturnBranchGraph(t, true)
	paramPath := pathdom.NewPath(param, "")
	stub := normalReturnFactProjectResultStub{
		reg:   reg,
		graph: graph,
		slots: []key.Value{key.SymbolValue(param)},
		statesAt: map[cfg.Point]state.State{
			ret:          presentState(reg, param),
			graph.Exit(): presentState(reg, param),
		},
		branchEvidence: map[cfg.Point][]factflow.BranchPathEvidence{
			branch: {factflow.NewBranchPathTruthyEvidenceWithOppositeOnEdge(paramPath, true)},
		},
	}

	got := projectNormalReturnParamConditions(reg, stub)
	if len(got) != 1 || got[0] != summary.ParamConditionTruthy {
		t.Fatalf("projectNormalReturnParamConditions = %#v, want parameter truthy from branch evidence", got)
	}
}

func TestProjectNormalReturnParamConditionsRequireDirectFalsyProof(t *testing.T) {
	reg := standard.Registry()
	param := symbol.ID(925)
	graph, branch, ret, _ := normalReturnBranchGraph(t, false)
	paramPath := pathdom.NewPath(param, "")
	stub := normalReturnFactProjectResultStub{
		reg:   reg,
		graph: graph,
		slots: []key.Value{key.SymbolValue(param)},
		statesAt: map[cfg.Point]state.State{
			ret:          presentState(reg, param),
			graph.Exit(): presentState(reg, param),
		},
		branchEvidence: map[cfg.Point][]factflow.BranchPathEvidence{
			branch: {factflow.NewBranchPathTruthyEvidenceOnEdge(paramPath, true)},
		},
	}

	if got := projectNormalReturnParamConditions(reg, stub); len(got) != 0 {
		t.Fatalf("projectNormalReturnParamConditions = %#v, want no falsy condition from implied truthy evidence", got)
	}
}

func TestProjectNormalReturnParamEqualitiesUseBranchPathRelations(t *testing.T) {
	reg := standard.Registry()
	left := symbol.ID(926)
	right := symbol.ID(927)
	graph, branch, ret, _ := normalReturnBranchGraph(t, true)
	stub := normalReturnFactProjectResultStub{
		reg:   reg,
		graph: graph,
		slots: []key.Value{key.SymbolValue(left), key.SymbolValue(right)},
		statesAt: map[cfg.Point]state.State{
			ret:          presentState(reg, left),
			graph.Exit(): presentState(reg, left),
		},
		branchRelations: map[cfg.Point][]factflow.BranchPathRelation{
			branch: {
				factflow.NewBranchPathEquality(
					pathdom.NewPath(left, ""),
					pathdom.NewPath(right, ""),
					true,
					false,
				),
			},
		},
	}

	got := projectNormalReturnParamEqualities(reg, stub)
	if len(got) != 1 || got[0].Left != 0 || got[0].Right != 1 {
		t.Fatalf("projectNormalReturnParamEqualities = %#v, want parameter equality from branch relation", got)
	}
}

func TestFromResultProjectsReturnParamLiteralCasesFromBranchFacts(t *testing.T) {
	reg := standard.Registry()
	status := symbol.ID(928)
	graph, branch, ret, _ := normalReturnBranchGraph(t, true)
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("NewValueSourceShape returned false")
	}
	returnSource, ok := factflow.NewExpressionValueSource(factflow.ExprRef(928), 0, factflow.NoValueSourceIndex, 0, shape)
	if !ok {
		t.Fatal("NewExpressionValueSource(return) returned false")
	}
	when := typevalue.NewCache().FromTypeWithWitness(reg, typ.LiteralString("ready"))
	value := typevalue.NewCache().FromTypeWithWitness(reg, typ.String)
	stub := normalReturnFactProjectResultStub{
		reg:          reg,
		graph:        graph,
		slots:        []key.Value{key.SymbolValue(status)},
		returnPoints: []cfg.Point{ret},
		returnSources: map[cfg.Point][]factflow.ValueSource{
			ret: {returnSource},
		},
		returnSourceValues: map[cfg.Point]map[factflow.ValueSource]product.Value{
			ret: {returnSource: value},
		},
		statesAt: map[cfg.Point]state.State{
			ret:          presentState(reg, status),
			graph.Exit(): presentState(reg, status),
		},
		branchSufficientLiteralCases: map[cfg.Point][]factflow.BranchSufficientLiteralCase{
			branch: {
				factflow.NewBranchSufficientLiteralCase(pathdom.NewPath(status, "").Field("kind"), when, true),
			},
		},
	}

	got := FromResult(stub).ReturnParamLiteralCases
	if len(got) != 1 {
		t.Fatalf("ReturnParamLiteralCases = %#v, want one case", got)
	}
	if got[0].ParamIndex != 0 || len(got[0].ParamSuffix) != 1 || got[0].ParamSuffix[0].Name != "kind" {
		t.Fatalf("ReturnParamLiteralCases[0] parameter = %#v, want $0.kind", got[0])
	}
	if !typ.TypeEquals(got[0].When, typ.LiteralString("ready")) {
		t.Fatalf("ReturnParamLiteralCases[0].When = %s, want \"ready\"", got[0].When)
	}
	if got[0].ReturnIndex != 0 || !product.Equal(reg, got[0].Value, value) {
		t.Fatalf("ReturnParamLiteralCases[0] return = %#v, want return[0] value", got[0])
	}
}

func TestFromResultProjectsComplementReturnParamLiteralCasesFromFiniteDomain(t *testing.T) {
	reg := standard.Registry()
	status := symbol.ID(929)
	graph, branch, ret, _ := normalReturnBranchGraph(t, false)
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("NewValueSourceShape returned false")
	}
	returnSource, ok := factflow.NewExpressionValueSource(factflow.ExprRef(929), 0, factflow.NoValueSourceIndex, 0, shape)
	if !ok {
		t.Fatal("NewExpressionValueSource(return) returned false")
	}
	cache := typevalue.NewCache()
	paramPath := pathdom.NewPath(status, "")
	when := cache.FromTypeWithWitness(reg, typ.LiteralString("num"))
	domain := cache.FromTypeWithWitness(reg, typeexpr.Union(typ.LiteralString("num"), typ.LiteralString("str")))
	value := cache.FromTypeWithWitness(reg, typ.String)
	stub := normalReturnFactProjectResultStub{
		reg:          reg,
		graph:        graph,
		slots:        []key.Value{key.SymbolValue(status)},
		returnPoints: []cfg.Point{ret},
		returnSources: map[cfg.Point][]factflow.ValueSource{
			ret: {returnSource},
		},
		returnSourceValues: map[cfg.Point]map[factflow.ValueSource]product.Value{
			ret: {returnSource: value},
		},
		pathBoundaryValues: map[cfg.Point]map[pathdom.PathKey]product.Value{
			branch: {paramPath.Key(): domain},
		},
		statesAt: map[cfg.Point]state.State{
			ret:          presentState(reg, status),
			graph.Exit(): presentState(reg, status),
		},
		branchSufficientLiteralCases: map[cfg.Point][]factflow.BranchSufficientLiteralCase{
			branch: {
				factflow.NewBranchSufficientLiteralCase(paramPath, when, true),
			},
		},
	}

	got := FromResult(stub).ReturnParamLiteralCases
	if len(got) != 1 {
		t.Fatalf("ReturnParamLiteralCases = %#v, want one complement case", got)
	}
	if got[0].ParamIndex != 0 || len(got[0].ParamSuffix) != 0 {
		t.Fatalf("ReturnParamLiteralCases[0] parameter = %#v, want $0", got[0])
	}
	if !typ.TypeEquals(got[0].When, typ.LiteralString("str")) {
		t.Fatalf("ReturnParamLiteralCases[0].When = %s, want \"str\"", got[0].When)
	}
	if got[0].ReturnIndex != 0 || !product.Equal(reg, got[0].Value, value) {
		t.Fatalf("ReturnParamLiteralCases[0] return = %#v, want return[0] value", got[0])
	}
}

func TestFromResultProjectsReturnedDynamicKeyMembershipFacts(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	table := symbol.ID(914)
	keys := symbol.ID(915)
	returnPoint := cfg.Point(914)
	returnExpr := factflow.ExprRef(914)
	keysPath := pathdom.NewPath(keys, "keys")
	tablePath := pathdom.NewPath(table, "source")
	keysKey := ks.FromPath(keysPath)
	tableKey := testStateKey(t, tablePath.Key())
	site := dynamicindex.Site("project.returned.keys")
	value := presentProduct(reg)

	exit := state.State{}.
		WriteDynamicIndexFact(reg, dynamicindex.Key{Table: keysKey, Site: site}, dynamicindex.Fact{
			KeyPresence: presence.Present(),
			KeyValue:    value,
			Value:       value,
			Admission:   dynamicindex.AdmissionAdmitted,
		}).
		AddDynamicIndexValueKeyMembership(keysKey, site, tableKey)

	got := FromResult(normalReturnFactProjectResultStub{
		reg:          reg,
		graph:        cfg.New(),
		exit:         exit,
		slots:        []key.Value{key.SymbolValue(table)},
		keys:         ks,
		returnPoints: []cfg.Point{returnPoint},
		returnSources: map[cfg.Point][]factflow.ValueSource{
			returnPoint: {{Kind: factflow.ValueSourceExpression, ExprRef: returnExpr, HasExpr: true}},
		},
		exprPaths: map[factflow.ExprRef]pathdom.Path{
			returnExpr: keysPath,
		},
	}).NormalReturnFacts

	assertDynamicAdmission(t, got.DynamicIndexFacts, string(site), pathdom.Path{Root: "ret[0]"}, dynamicindex.AdmissionAdmitted)
	if len(got.DynamicValueKeys) != 1 ||
		!got.DynamicValueKeys[0].Container.Equal(pathdom.Path{Root: "ret[0]"}) ||
		got.DynamicValueKeys[0].Site != site ||
		!got.DynamicValueKeys[0].Table.Equal(pathdom.NewPlaceholder(0)) {
		t.Fatalf("DynamicValueKeys = %#v, want ret[0] values proven as keys of $0", got.DynamicValueKeys)
	}
}

func TestFromResultProjectsReturnedPathStaticMemberFacts(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	obj := symbol.ID(916)
	returnPoint := cfg.Point(916)
	returnExpr := factflow.ExprRef(916)
	objStatePath := pathdom.Path{Root: "obj", Symbol: obj, Version: 1}
	objReturnPath := pathdom.NewPath(obj, "obj")
	memberValue := presentProduct(reg)

	exit := state.State{}.
		WritePathStaticMember(ks, objStatePath.Field("get_x").Key(), memberValue)
	if _, ok := exit.ReadPathStaticMember(ks, objStatePath.Field("get_x").Key()); !ok {
		t.Fatalf("test setup did not write static member for %s", objStatePath.Field("get_x").Key())
	}
	stub := normalReturnFactProjectResultStub{
		reg:          reg,
		graph:        cfg.New(),
		exit:         exit,
		keys:         ks,
		returnPoints: []cfg.Point{returnPoint},
		returnSources: map[cfg.Point][]factflow.ValueSource{
			returnPoint: {{Kind: factflow.ValueSourceExpression, ExprRef: returnExpr, HasExpr: true}},
		},
		exprPaths: map[factflow.ExprRef]pathdom.Path{
			returnExpr: objReturnPath,
		},
	}

	got := FromResult(stub).NormalReturnFacts

	assertPathStaticMember(t, reg, got.PathStaticMembers, pathdom.Path{Root: "ret[0]"}.Field("get_x"), memberValue)
}

func TestFromResultSkipsTopSnapshotsAndTopNormalReturnFacts(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	param := symbol.ID(921)
	topFacts := FromResult(
		normalReturnFactProjectTestResult(reg, ks, state.Domain(reg).Top(), param),
	).NormalReturnFacts
	if !topFacts.Empty() {
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
		WritePathKey(reg, ks, paramKey, product.Top()).
		WritePathStaticMember(ks, normalReturnFactProjectTestKey(param, ".member"), product.Bottom(reg)).
		WriteDynamicIndexFact(reg, dynamicindex.Key{
			Table: mustStateKey(t, ks, normalReturnFactProjectTestKey(param, ".table")),
			Site:  "dynamic-top",
		}, topDynamic).
		AddBranchProof(pathevidence.BranchProof{
			Kind:     pathevidence.BranchProofPathPresence,
			Path:     mustStateKey(t, ks, normalReturnFactProjectTestKey(param, ".proof")),
			Presence: presence.Top(),
		}).
		AddChannelSelectFact(channelselectfact.Fact{
			Select: "invalid-kind",
			Kind:   0,
			Result: testStateKey(t, normalReturnFactProjectTestKey(param, ".result")),
			Index:  0,
		}).
		WriteEffectDelta(effectdelta.Key{
			Target: mustStateKey(t, ks, normalReturnFactProjectTestKey(param, ".effect")),
			Site:   "effect-top",
			Kind:   effectdelta.Mutation,
		}, topEffect)

	got := FromResult(normalReturnFactProjectTestResult(reg, ks, exit, param)).NormalReturnFacts
	assertPathInvalidation(t, got.PathInvalidations, pathdom.NewPlaceholder(0).Field("table"))
	got.PathInvalidations = nil
	if !got.Empty() {
		t.Fatalf("NormalReturnFacts = %#v, want only dynamic top path invalidation and other top/no-op facts skipped", got)
	}
}

func TestFromResultProjectsAssignmentBasedParameterInvalidationWithoutExitState(t *testing.T) {
	reg := standard.Registry()
	param := symbol.ID(925)
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)
	stub := normalReturnFactProjectAssignmentStub{
		normalReturnFactProjectResultStub: normalReturnFactProjectResultStub{
			reg:   reg,
			graph: graph,
			exit:  state.State{},
			slots: []key.Value{key.SymbolValue(param)},
		},
		pathInvalidations: map[cfg.Point]factflow.PathDescendantInvalidation{
			assign: factflow.NewPathDescendantInvalidation(pathdom.NewPath(param, "")),
		},
	}

	got := FromResult(stub).NormalReturnFacts
	assertPathInvalidation(t, got.PathInvalidations, pathdom.NewPlaceholder(0))
	assertStructuralPreservingPathInvalidation(t, got.PathInvalidations, pathdom.NewPlaceholder(0))
	assertNonClearingPathInvalidation(t, got.PathInvalidations, pathdom.NewPlaceholder(0))
}

func TestFromResultProjectsPathFactParameterInvalidationWithoutSemanticAssignment(t *testing.T) {
	reg := standard.Registry()
	param := symbol.ID(92503)
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)
	stub := normalReturnFactProjectAssignmentStub{
		normalReturnFactProjectResultStub: normalReturnFactProjectResultStub{
			reg:   reg,
			graph: graph,
			exit:  state.State{},
			slots: []key.Value{key.SymbolValue(param)},
		},
		pathAssignments: map[cfg.Point]factflow.PathAssignment{
			assign: factflow.NewPathAssignment(pathdom.NewPath(param, "").Field("metadata"), factflow.NewUnknownValueSource(factflow.NoValueSourceIndex)),
		},
	}

	got := FromResult(stub).NormalReturnFacts
	want := pathdom.NewPlaceholder(0).Field("metadata")
	assertPathInvalidation(t, got.PathInvalidations, want)
	assertStructuralPreservingPathInvalidation(t, got.PathInvalidations, want)
	assertClearingPathInvalidation(t, got.PathInvalidations, want)
}

func TestFromResultProjectsDescendantInvalidationWithoutSemanticAssignment(t *testing.T) {
	reg := standard.Registry()
	param := symbol.ID(92504)
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)
	stub := normalReturnFactProjectAssignmentStub{
		normalReturnFactProjectResultStub: normalReturnFactProjectResultStub{
			reg:   reg,
			graph: graph,
			exit:  state.State{},
			slots: []key.Value{key.SymbolValue(param)},
		},
		pathInvalidations: map[cfg.Point]factflow.PathDescendantInvalidation{
			assign: factflow.NewPathDescendantInvalidation(pathdom.NewPath(param, "").Field("metadata")),
		},
	}

	got := FromResult(stub).NormalReturnFacts
	want := pathdom.NewPlaceholder(0).Field("metadata")
	assertPathInvalidation(t, got.PathInvalidations, want)
	assertStructuralPreservingPathInvalidation(t, got.PathInvalidations, want)
	assertNonClearingPathInvalidation(t, got.PathInvalidations, want)
}

func TestFromResultProjectsReturnArithmeticObligationFromValueSource(t *testing.T) {
	reg := standard.Registry()
	param := symbol.ID(92501)
	graph := cfg.New()
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), ret, false)
	graph.AddEdge(ret, graph.Exit(), false)
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("NewValueSourceShape returned false")
	}
	returnSource, ok := factflow.NewExpressionValueSource(factflow.ExprRef(1), 0, factflow.NoValueSourceIndex, 0, shape)
	if !ok {
		t.Fatal("NewExpressionValueSource(return) returned false")
	}
	leftSource, ok := factflow.NewExpressionValueSource(factflow.ExprRef(2), 0, factflow.NoValueSourceIndex, 0, shape)
	if !ok {
		t.Fatal("NewExpressionValueSource(left) returned false")
	}
	rightSource, ok := factflow.NewIntegerLiteralValueSource(2, 1, factflow.NoValueSourceIndex, 0, shape)
	if !ok {
		t.Fatal("NewIntegerLiteralValueSource returned false")
	}
	op, ok := factflow.NewBinaryExpressionOperation("*", leftSource, rightSource)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}
	paramValue := typevalue.NewCache().FromTypeWithWitness(reg, typ.String)
	paramState := state.Domain(reg).Bottom().WriteValue(reg, key.SymbolValue(param), paramValue)
	stub := normalReturnFactProjectResultStub{
		reg:           reg,
		graph:         graph,
		exit:          paramState,
		slots:         []key.Value{key.SymbolValue(param)},
		returnPoints:  []cfg.Point{ret},
		entryState:    paramState,
		hasEntryState: true,
		statesAt: map[cfg.Point]state.State{
			ret: paramState,
		},
		returnSources: map[cfg.Point][]factflow.ValueSource{
			ret: {returnSource},
		},
		exprPaths: map[factflow.ExprRef]pathdom.Path{
			factflow.ExprRef(2): {Symbol: param},
		},
		exprOps: map[factflow.ExprRef]factflow.ExpressionOperation{
			factflow.ExprRef(1): op,
		},
	}

	got := FromResult(stub)

	if len(got.ParamObligations) != 1 {
		t.Fatalf("param obligations = %#v, want one number obligation", got.ParamObligations)
	}
	if kind := product.Get(reg, got.ParamObligations[0], runtimekind.Key); !runtimekind.Equal(kind, runtimekind.Singleton(runtimekind.Number)) {
		t.Fatalf("param obligation runtime kind = %s, want number", kind)
	}
}

func TestFromResultReturnArithmeticSourceCycleTerminates(t *testing.T) {
	reg := standard.Registry()
	param := symbol.ID(92502)
	graph := cfg.New()
	ret := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), ret, false)
	graph.AddEdge(ret, graph.Exit(), false)
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("NewValueSourceShape returned false")
	}
	selfSource, ok := factflow.NewExpressionValueSource(factflow.ExprRef(1), 0, factflow.NoValueSourceIndex, 0, shape)
	if !ok {
		t.Fatal("NewExpressionValueSource(self) returned false")
	}
	literalSource, ok := factflow.NewIntegerLiteralValueSource(1, 1, factflow.NoValueSourceIndex, 0, shape)
	if !ok {
		t.Fatal("NewIntegerLiteralValueSource returned false")
	}
	op, ok := factflow.NewBinaryExpressionOperation("+", selfSource, literalSource)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}
	paramValue := typevalue.NewCache().FromTypeWithWitness(reg, typ.String)
	paramState := state.Domain(reg).Bottom().WriteValue(reg, key.SymbolValue(param), paramValue)
	stub := normalReturnFactProjectResultStub{
		reg:           reg,
		graph:         graph,
		exit:          paramState,
		slots:         []key.Value{key.SymbolValue(param)},
		returnPoints:  []cfg.Point{ret},
		entryState:    paramState,
		hasEntryState: true,
		statesAt: map[cfg.Point]state.State{
			ret: paramState,
		},
		returnSources: map[cfg.Point][]factflow.ValueSource{
			ret: {selfSource},
		},
		exprOps: map[factflow.ExprRef]factflow.ExpressionOperation{
			factflow.ExprRef(1): op,
		},
	}

	got := FromResult(stub)

	if len(got.ParamObligations) != 0 {
		t.Fatalf("param obligations = %#v, want none for cyclic source without parameter evidence", got.ParamObligations)
	}
}

func TestFromResultProjectsDynamicContainerAssignmentAsStructuralPreservingInvalidation(t *testing.T) {
	reg := standard.Registry()
	param := symbol.ID(9251)
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)
	stub := normalReturnFactProjectAssignmentStub{
		normalReturnFactProjectResultStub: normalReturnFactProjectResultStub{
			reg:   reg,
			graph: graph,
			exit:  state.State{},
			slots: []key.Value{key.SymbolValue(param)},
		},
		pathInvalidations: map[cfg.Point]factflow.PathDescendantInvalidation{
			assign: factflow.NewPathDescendantInvalidation(pathdom.NewPath(param, "").Field("metadata")),
		},
	}

	got := FromResult(stub).NormalReturnFacts
	want := pathdom.NewPlaceholder(0).Field("metadata")
	assertPathInvalidation(t, got.PathInvalidations, want)
	assertStructuralPreservingPathInvalidation(t, got.PathInvalidations, want)
	assertNonClearingPathInvalidation(t, got.PathInvalidations, want)
}

func TestFromResultProjectsDirectFieldAssignmentAsStructuralPreservingTargetClearInvalidation(t *testing.T) {
	reg := standard.Registry()
	param := symbol.ID(9252)
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)
	stub := normalReturnFactProjectAssignmentStub{
		normalReturnFactProjectResultStub: normalReturnFactProjectResultStub{
			reg:   reg,
			graph: graph,
			exit:  state.State{},
			slots: []key.Value{key.SymbolValue(param)},
		},
		pathAssignments: map[cfg.Point]factflow.PathAssignment{
			assign: factflow.NewPathAssignment(pathdom.NewPath(param, "").Field("metadata"), factflow.NewUnknownValueSource(factflow.NoValueSourceIndex)),
		},
	}

	got := FromResult(stub).NormalReturnFacts
	want := pathdom.NewPlaceholder(0).Field("metadata")
	assertPathInvalidation(t, got.PathInvalidations, want)
	assertStructuralPreservingPathInvalidation(t, got.PathInvalidations, want)
	assertClearingPathInvalidation(t, got.PathInvalidations, want)
}

func TestFromResultProjectsCapturedRootReassignmentInvalidation(t *testing.T) {
	reg := standard.Registry()
	captured := symbol.ID(926)
	fn := &ast.FunctionExpr{}
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)
	stub := normalReturnFactProjectAssignmentStub{
		normalReturnFactProjectResultStub: normalReturnFactProjectResultStub{
			reg:   reg,
			graph: graph,
			exit:  state.State{},
		},
		fn: fn,
		captures: []bind.Capture{{
			Captured:     captured,
			CapturedName: "value",
		}},
		rootAssignments: map[cfg.Point]factflow.RootAssignment{
			assign: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, captured, pathdom.NewPath(captured, "value"), factflow.NewUnknownValueSource(factflow.NoValueSourceIndex)),
		},
	}

	got := FromResult(stub).NormalReturnFacts
	assertPathInvalidation(t, got.PathInvalidations, pathdom.NewPath(captured, ""))
}

func TestFromResultProjectsCapturedRootReassignmentPersistentWrite(t *testing.T) {
	reg := standard.Registry()
	captured := symbol.ID(927)
	fn := &ast.FunctionExpr{}
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)
	value := presentProduct(reg)
	stub := normalReturnFactProjectAssignmentStub{
		normalReturnFactProjectResultStub: normalReturnFactProjectResultStub{
			reg:   reg,
			graph: graph,
			exit:  state.State{}.WriteValue(reg, key.SymbolValue(captured), value),
		},
		fn: fn,
		captures: []bind.Capture{{
			Captured:     captured,
			CapturedName: "captured",
		}},
		rootAssignments: map[cfg.Point]factflow.RootAssignment{
			assign: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, captured, pathdom.NewPath(captured, "captured"), factflow.NewUnknownValueSource(factflow.NoValueSourceIndex)),
		},
	}

	got := FromResult(stub).NormalReturnFacts
	assertPathValueFact(t, reg, got.PersistentPathWrites, pathdom.NewPath(captured, ""), portableBoundaryValue(reg, value))
}

func TestFromResultProjectsLoweredCapturedRootReassignmentPersistentWrite(t *testing.T) {
	reg := standard.Registry()
	captured := symbol.ID(9270)
	fn := &ast.FunctionExpr{}
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)
	value := presentProduct(reg)
	source := factflow.ValueSource{Kind: factflow.ValueSourceUnknown}
	stub := normalReturnFactProjectAssignmentStub{
		normalReturnFactProjectResultStub: normalReturnFactProjectResultStub{
			reg:   reg,
			graph: graph,
			exit:  state.State{},
		},
		fn: fn,
		captures: []bind.Capture{{
			Captured:     captured,
			CapturedName: "captured",
		}},
		rootAssignments: map[cfg.Point]factflow.RootAssignment{
			assign: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, captured, pathdom.NewPath(captured, "captured"), source),
		},
		sourceValues: map[cfg.Point]product.Value{
			assign: value,
		},
	}

	got := FromResult(stub).NormalReturnFacts
	assertPathValueFact(t, reg, got.PersistentPathWrites, pathdom.NewPath(captured, ""), portableBoundaryValue(reg, value))
}

func TestFromResultProjectsCapturedRootReassignmentPersistentWriteFromPresentSource(t *testing.T) {
	reg := standard.Registry()
	captured := symbol.ID(9271)
	fn := &ast.FunctionExpr{}
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	record := typetable.NewRecord().
		Field("apply", typ.Func().Returns(typ.String).Build()).
		Build()
	optional := typeexpr.Optional(record)
	optionalValue := typevalue.WithWitness(reg, typevalue.FromType(reg, optional), optional)
	sourceValue := typevalue.WithWitness(reg, typevalue.FromType(reg, record), record)
	source := factflow.NewUnknownValueSource(factflow.NoValueSourceIndex)
	stub := normalReturnFactProjectAssignmentStub{
		normalReturnFactProjectResultStub: normalReturnFactProjectResultStub{
			reg:   reg,
			graph: graph,
			exit:  state.State{}.WriteValue(reg, key.SymbolValue(captured), optionalValue),
		},
		fn: fn,
		captures: []bind.Capture{{
			Captured:     captured,
			CapturedName: "captured",
		}},
		rootAssignments: map[cfg.Point]factflow.RootAssignment{
			assign: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, captured, pathdom.NewPath(captured, "captured"), source),
		},
		sourceValues: map[cfg.Point]product.Value{
			assign: sourceValue,
		},
	}

	got := FromResult(stub).NormalReturnFacts
	fact := findPathRefinement(got.PersistentPathWrites, pathdom.NewPath(captured, ""))
	if fact == nil {
		t.Fatalf("PersistentPathWrites = %#v, want captured write", got.PersistentPathWrites)
	}
	gotType, ok := typevalue.TypeOf(reg, fact.Value)
	if !ok || !typ.TypeEquals(gotType, record) {
		t.Fatalf("persistent write type = %v/%v, want present %v", gotType, ok, record)
	}
}

func TestFromResultDoesNotProjectParameterRootReassignmentInvalidation(t *testing.T) {
	reg := standard.Registry()
	param := symbol.ID(928)
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)
	stub := normalReturnFactProjectAssignmentStub{
		normalReturnFactProjectResultStub: normalReturnFactProjectResultStub{
			reg:   reg,
			graph: graph,
			exit:  state.State{},
			slots: []key.Value{key.SymbolValue(param)},
		},
		rootAssignments: map[cfg.Point]factflow.RootAssignment{
			assign: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, param, pathdom.NewPath(param, "param"), factflow.NewUnknownValueSource(factflow.NoValueSourceIndex)),
		},
	}

	got := FromResult(stub).NormalReturnFacts
	if len(got.PathInvalidations) != 0 {
		t.Fatalf("PathInvalidations = %#v, want none for callee-local parameter rebinding", got.PathInvalidations)
	}
}

func TestFromResultProjectsHeapTableObjectsFromExitSnapshots(t *testing.T) {
	reg := standard.Registry()
	param := symbol.ID(931)
	tableID := identity.ID{Kind: "table", Site: "summary-project", Index: 1}
	ks := keyspace.New()
	memberKey, ok := ks.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "child"}})
	if !ok {
		t.Fatal("child suffix key failed")
	}
	value := presentProduct(reg)
	exit := state.State{}.WriteHeapTableObject(reg, tableID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:          value,
		StaticMembers: map[keyspace.Key]product.Value{memberKey: value},
	}))

	got := FromResult(normalReturnFactProjectTestResult(reg, ks, exit, param)).HeapTableObjects
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
	ks := keyspace.New()
	childKey, ok := ks.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "child"}})
	if !ok {
		t.Fatal("child suffix key failed")
	}
	exit := state.State{}.
		WriteValue(reg, key.SymbolValue(param), rootValue).
		WriteHeapTableObject(reg, rootID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root:          rootValue,
			StaticMembers: map[keyspace.Key]product.Value{childKey: childValue},
		})).
		WriteHeapTableObject(reg, childID, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
			Root: childValue,
		})).
		FreezeTable(rootID).
		FreezeTable(childID)

	got := FromResult(normalReturnFactProjectTestResult(reg, ks, exit, param)).NormalReturnFacts

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
			Source: testStateKey(t, source.Key()),
			Into:   testStateKey(t, into.Key()),
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

func TestFromResultProjectsAssignmentStoreRelationFromPathFactWithoutSemanticAssignment(t *testing.T) {
	reg := standard.Registry()
	param0 := symbol.ID(9411)
	param1 := symbol.ID(9412)
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)
	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("NewValueSourceShape returned false")
	}
	source, ok := factflow.NewExpressionValueSource(factflow.ExprRef(1), 0, factflow.NoValueSourceIndex, 0, shape)
	if !ok {
		t.Fatal("NewExpressionValueSource returned false")
	}
	stub := normalReturnFactProjectAssignmentStub{
		normalReturnFactProjectResultStub: normalReturnFactProjectResultStub{
			reg:   reg,
			graph: graph,
			exit:  state.State{},
			slots: []key.Value{key.SymbolValue(param0), key.SymbolValue(param1)},
			exprPaths: map[factflow.ExprRef]pathdom.Path{
				factflow.ExprRef(1): pathdom.NewPath(param0, ""),
			},
		},
		pathAssignments: map[cfg.Point]factflow.PathAssignment{
			assign: factflow.NewPathAssignment(pathdom.NewPath(param1, "").Field("container"), source),
		},
	}

	got := FromResult(stub).NormalReturnFacts
	if len(got.StoreRelations) != 1 ||
		!got.StoreRelations[0].Source.Equal(pathdom.NewPlaceholder(0)) ||
		!got.StoreRelations[0].Into.Equal(pathdom.NewPlaceholder(1).Field("container")) {
		t.Fatalf("StoreRelations = %#v, want assignment store relation projected from path fact", got.StoreRelations)
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
		Source: testStateKey(t, source.Key()),
		Into:   testStateKey(t, into.Key()),
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

func TestFromResultProjectsMandatoryCallOutcomeLifecycleFacts(t *testing.T) {
	reg := standard.Registry()
	param := symbol.ID(945)
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)

	stub := normalReturnFactProjectCallStub{
		normalReturnFactProjectResultStub: normalReturnFactProjectResultStub{
			reg:   reg,
			graph: graph,
			exit:  state.State{},
			slots: []key.Value{key.SymbolValue(param)},
		},
		calls: map[cfg.Point]factflow.CallSite{
			call: lifecycleCallSite(1),
		},
		paths: map[factflow.ExprRef]pathdom.Path{
			1: {Symbol: param, Version: 1},
		},
		outcomes: map[cfg.Point]callpayload.CallOutcome{
			call: {
				NormalReturnFacts: callboundary.NormalReturnFacts{
					LifecycleFacts: []callboundary.LifecycleFact{
						{
							Target:   pathdom.NewPlaceholder(0),
							Kind:     callboundary.LifecycleTransition,
							Protocol: typestate.Protocol("transaction"),
							From:     typestate.State("active"),
							To:       typestate.State("finished"),
						},
					},
				},
			},
		},
	}

	got := FromResult(stub).NormalReturnFacts
	assertLifecycleFact(t, got.LifecycleFacts, pathdom.NewPlaceholder(0), callboundary.LifecycleTransition)
}

func TestFromResultDoesNotProjectBranchLocalCallOutcomeLifecycleFacts(t *testing.T) {
	reg := standard.Registry()
	param := symbol.ID(946)
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, call, true)
	graph.AddEdge(branch, graph.Exit(), false)
	graph.AddEdge(call, graph.Exit(), false)

	stub := normalReturnFactProjectCallStub{
		normalReturnFactProjectResultStub: normalReturnFactProjectResultStub{
			reg:   reg,
			graph: graph,
			exit:  state.State{},
			slots: []key.Value{key.SymbolValue(param)},
		},
		calls: map[cfg.Point]factflow.CallSite{
			call: lifecycleCallSite(1),
		},
		paths: map[factflow.ExprRef]pathdom.Path{
			1: {Symbol: param, Version: 1},
		},
		outcomes: map[cfg.Point]callpayload.CallOutcome{
			call: {
				NormalReturnFacts: callboundary.NormalReturnFacts{
					LifecycleFacts: []callboundary.LifecycleFact{
						{
							Target:   pathdom.NewPlaceholder(0),
							Kind:     callboundary.LifecycleTransition,
							Protocol: typestate.Protocol("transaction"),
							From:     typestate.State("active"),
							To:       typestate.State("finished"),
						},
					},
				},
			},
		},
	}

	got := FromResult(stub).NormalReturnFacts
	if len(got.LifecycleFacts) != 0 {
		t.Fatalf("LifecycleFacts = %#v, want none for branch-local lifecycle call", got.LifecycleFacts)
	}
}

func TestFromResultProjectsMandatoryCallOutcomeLifecycleFactsForCapturedPath(t *testing.T) {
	reg := standard.Registry()
	captured := symbol.ID(947)
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)
	capturedPath := pathdom.Path{Symbol: captured, Version: 1}

	stub := normalReturnFactProjectCallStub{
		normalReturnFactProjectResultStub: normalReturnFactProjectResultStub{
			reg:   reg,
			graph: graph,
			exit:  state.State{},
		},
		calls: map[cfg.Point]factflow.CallSite{
			call: lifecycleCallSite(1),
		},
		paths: map[factflow.ExprRef]pathdom.Path{
			1: capturedPath,
		},
		outcomes: map[cfg.Point]callpayload.CallOutcome{
			call: {
				NormalReturnFacts: callboundary.NormalReturnFacts{
					LifecycleFacts: []callboundary.LifecycleFact{
						{
							Target:   pathdom.NewPlaceholder(0),
							Kind:     callboundary.LifecycleTransition,
							Protocol: typestate.Protocol("transaction"),
							From:     typestate.State("active"),
							To:       typestate.State("finished"),
						},
					},
				},
			},
		},
	}

	got := FromResult(stub).NormalReturnFacts
	assertLifecycleFact(t, got.LifecycleFacts, capturedPath, callboundary.LifecycleTransition)
}

func TestFromResultProjectsMandatoryCallOutcomePersistentPathWrites(t *testing.T) {
	reg := standard.Registry()
	captured := symbol.ID(948)
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)
	capturedPath := pathdom.NewPath(captured, "captured")
	value := presentProduct(reg)

	stub := normalReturnFactProjectCallStub{
		normalReturnFactProjectResultStub: normalReturnFactProjectResultStub{
			reg:   reg,
			graph: graph,
			exit:  state.State{},
		},
		calls: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{Context: factflow.CallSiteContextStatement}),
		},
		outcomes: map[cfg.Point]callpayload.CallOutcome{
			call: {
				NormalReturnFacts: callboundary.NormalReturnFacts{
					PersistentPathWrites: []callboundary.PathValueFact{{Path: capturedPath, Value: value}},
				},
			},
		},
	}

	got := FromResult(stub).NormalReturnFacts
	assertPathValueFact(t, reg, got.PersistentPathWrites, capturedPath, value)
}

func TestFromResultDoesNotProjectBranchLocalCallOutcomePersistentPathWrites(t *testing.T) {
	reg := standard.Registry()
	captured := symbol.ID(949)
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, call, true)
	graph.AddEdge(branch, graph.Exit(), false)
	graph.AddEdge(call, graph.Exit(), false)
	capturedPath := pathdom.NewPath(captured, "captured")

	stub := normalReturnFactProjectCallStub{
		normalReturnFactProjectResultStub: normalReturnFactProjectResultStub{
			reg:   reg,
			graph: graph,
			exit:  state.State{},
		},
		calls: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{Context: factflow.CallSiteContextStatement}),
		},
		outcomes: map[cfg.Point]callpayload.CallOutcome{
			call: {
				NormalReturnFacts: callboundary.NormalReturnFacts{
					PersistentPathWrites: []callboundary.PathValueFact{{Path: capturedPath, Value: presentProduct(reg)}},
				},
			},
		},
	}

	got := FromResult(stub).NormalReturnFacts
	if len(got.PersistentPathWrites) != 0 {
		t.Fatalf("PersistentPathWrites = %#v, want none for branch-local call", got.PersistentPathWrites)
	}
}

func TestFromResultProjectsGuardedCallOutcomePersistentPathWritesWhenBypassEdgeUnreachable(t *testing.T) {
	reg := standard.Registry()
	captured := symbol.ID(950)
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, call, true)
	graph.AddEdge(branch, graph.Exit(), false)
	graph.AddEdge(call, graph.Exit(), false)
	capturedPath := pathdom.NewPath(captured, "captured")
	value := presentProduct(reg)

	stub := normalReturnFactProjectCallStub{
		normalReturnFactProjectResultStub: normalReturnFactProjectResultStub{
			reg:   reg,
			graph: graph,
			exit:  state.State{},
			edgeReach: map[[2]cfg.Point]bool{
				{branch, graph.Exit()}: false,
			},
		},
		calls: map[cfg.Point]factflow.CallSite{
			call: factflow.NewCallSite(factflow.CallSiteConfig{Context: factflow.CallSiteContextStatement}),
		},
		outcomes: map[cfg.Point]callpayload.CallOutcome{
			call: {
				NormalReturnFacts: callboundary.NormalReturnFacts{
					PersistentPathWrites: []callboundary.PathValueFact{{Path: capturedPath, Value: value}},
				},
			},
		},
	}

	got := FromResult(stub).NormalReturnFacts
	assertPathValueFact(t, reg, got.PersistentPathWrites, capturedPath, value)
}

type normalReturnFactProjectResultStub struct {
	reg                          *axis.Registry
	graph                        cfg.Graph
	exit                         state.State
	slots                        []key.Value
	keys                         *keyspace.KeySpace
	reassigned                   map[key.Value]struct{}
	returnPoints                 []cfg.Point
	returnSources                map[cfg.Point][]factflow.ValueSource
	exprPaths                    map[factflow.ExprRef]pathdom.Path
	exprOps                      map[factflow.ExprRef]factflow.ExpressionOperation
	entryState                   state.State
	hasEntryState                bool
	statesAt                     map[cfg.Point]state.State
	edgeReach                    map[[2]cfg.Point]bool
	branchEvidence               map[cfg.Point][]factflow.BranchPathEvidence
	branchRelations              map[cfg.Point][]factflow.BranchPathRelation
	branchSufficientLiteralCases map[cfg.Point][]factflow.BranchSufficientLiteralCase
	returnSourceValues           map[cfg.Point]map[factflow.ValueSource]product.Value
	pathBoundaryValues           map[cfg.Point]map[pathdom.PathKey]product.Value
}

type normalReturnFactProjectAssignmentStub struct {
	normalReturnFactProjectResultStub
	rootAssignments   map[cfg.Point]factflow.RootAssignment
	pathAssignments   map[cfg.Point]factflow.PathAssignment
	pathInvalidations map[cfg.Point]factflow.PathDescendantInvalidation
	sourceValues      map[cfg.Point]product.Value
	fn                *ast.FunctionExpr
	captures          []bind.Capture
	kinds             map[symbol.ID]symbol.Kind
}

type normalReturnFactProjectCallStub struct {
	normalReturnFactProjectResultStub
	calls    map[cfg.Point]factflow.CallSite
	paths    map[factflow.ExprRef]pathdom.Path
	outcomes map[cfg.Point]callpayload.CallOutcome
}

func (r normalReturnFactProjectCallStub) CallSiteView(point cfg.Point) (factflow.CallSiteView, bool) {
	site, ok := r.calls[point]
	return site.View(), ok
}

func (r normalReturnFactProjectCallStub) ExpressionPathRef(ref factflow.ExprRef) (pathdom.Path, bool) {
	p, ok := r.paths[ref]
	return p, ok
}

func (r normalReturnFactProjectCallStub) CallOutcomeAt(point cfg.Point) (callpayload.CallOutcome, bool) {
	outcome, ok := r.outcomes[point]
	return outcome, ok
}

func lifecycleCallSite(ref factflow.ExprRef) factflow.CallSite {
	shape, _ := factflow.NewValueSourceShape(true, false, false, false)
	source, _ := factflow.NewExpressionValueSource(ref, 0, factflow.NoValueSourceIndex, 0, shape)
	return factflow.NewCallSite(factflow.CallSiteConfig{
		ArgumentSources: []factflow.ValueSource{source},
	})
}

func (r normalReturnFactProjectAssignmentStub) RootAssignment(point cfg.Point) (factflow.RootAssignment, bool) {
	fact, ok := r.rootAssignments[point]
	return fact, ok
}

func (r normalReturnFactProjectAssignmentStub) PathAssignment(point cfg.Point) (factflow.PathAssignment, bool) {
	fact, ok := r.pathAssignments[point]
	return fact, ok
}

func (r normalReturnFactProjectAssignmentStub) PathDescendantInvalidation(point cfg.Point) (factflow.PathDescendantInvalidation, bool) {
	fact, ok := r.pathInvalidations[point]
	return fact, ok
}

func (r normalReturnFactProjectAssignmentStub) SourceValueAtBoundary(point cfg.Point, _ factflow.ValueSource) (product.Value, bool) {
	value, ok := r.sourceValues[point]
	return value, ok
}

func (r normalReturnFactProjectAssignmentStub) Function() *ast.FunctionExpr {
	return r.fn
}

func (r normalReturnFactProjectAssignmentStub) DirectCaptures(fn *ast.FunctionExpr) []bind.Capture {
	if fn == nil || fn != r.fn || len(r.captures) == 0 {
		return nil
	}
	out := make([]bind.Capture, len(r.captures))
	copy(out, r.captures)
	return out
}

func (r normalReturnFactProjectAssignmentStub) SymbolKind(id symbol.ID) (symbol.Kind, bool) {
	if len(r.kinds) == 0 {
		return 0, false
	}
	kind, ok := r.kinds[id]
	return kind, ok
}

func normalReturnFactProjectTestResult(
	reg *axis.Registry,
	ks *keyspace.KeySpace,
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
		keys:  ks,
	}
}

func (r normalReturnFactProjectResultStub) Registry() *axis.Registry { return r.reg }

func (r normalReturnFactProjectResultStub) KeySpace() *keyspace.KeySpace {
	if r.keys == nil {
		return keyspace.New()
	}
	return r.keys
}

func (r normalReturnFactProjectResultStub) Graph() cfg.Graph { return r.graph }

func (r normalReturnFactProjectResultStub) ExitState() (state.State, bool) { return r.exit, true }

func (r normalReturnFactProjectResultStub) EntryState() (state.State, bool) {
	if !r.hasEntryState {
		return state.State{}, false
	}
	return r.entryState, true
}

func (r normalReturnFactProjectResultStub) StateAt(point cfg.Point) (state.State, bool) {
	if len(r.statesAt) == 0 {
		return state.State{}, false
	}
	stateAt, ok := r.statesAt[point]
	return stateAt, ok
}

func (r normalReturnFactProjectResultStub) BranchPathEvidence(point cfg.Point) []factflow.BranchPathEvidence {
	if len(r.branchEvidence) == 0 {
		return nil
	}
	out := r.branchEvidence[point]
	if len(out) == 0 {
		return nil
	}
	copyOut := make([]factflow.BranchPathEvidence, len(out))
	copy(copyOut, out)
	return copyOut
}

func (r normalReturnFactProjectResultStub) BranchPathRelations(point cfg.Point) []factflow.BranchPathRelation {
	if len(r.branchRelations) == 0 {
		return nil
	}
	out := r.branchRelations[point]
	if len(out) == 0 {
		return nil
	}
	copyOut := make([]factflow.BranchPathRelation, len(out))
	copy(copyOut, out)
	return copyOut
}

func (r normalReturnFactProjectResultStub) BranchSufficientLiteralCases(point cfg.Point) []factflow.BranchSufficientLiteralCase {
	if len(r.branchSufficientLiteralCases) == 0 {
		return nil
	}
	out := r.branchSufficientLiteralCases[point]
	if len(out) == 0 {
		return nil
	}
	copyOut := make([]factflow.BranchSufficientLiteralCase, len(out))
	copy(copyOut, out)
	return copyOut
}

func (r normalReturnFactProjectResultStub) SourceValueAtBoundary(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	values, ok := r.returnSourceValues[point]
	if !ok {
		return product.Value{}, false
	}
	value, ok := values[source]
	return value, ok
}

func (r normalReturnFactProjectResultStub) PathValueAtBoundary(point cfg.Point, p pathdom.Path) (product.Value, bool) {
	values, ok := r.pathBoundaryValues[point]
	if !ok {
		return product.Value{}, false
	}
	value, ok := values[p.Key()]
	return value, ok
}

func (r normalReturnFactProjectResultStub) PointDominates(dominator, point cfg.Point) bool {
	if r.graph == nil {
		return false
	}
	return dominance.ComputeImmediateDominatorInfo(r.graph).Dominates(dominator, point)
}

func (r normalReturnFactProjectResultStub) EdgeCanCompleteNormally(from, to cfg.Point) bool {
	if r.edgeReach == nil {
		return true
	}
	reachable, ok := r.edgeReach[[2]cfg.Point{from, to}]
	if !ok {
		return true
	}
	return reachable
}

func (r normalReturnFactProjectResultStub) ReturnPoints() []cfg.Point {
	if len(r.returnPoints) == 0 {
		return nil
	}
	out := make([]cfg.Point, len(r.returnPoints))
	copy(out, r.returnPoints)
	return out
}

func (r normalReturnFactProjectResultStub) ReturnValueSources(point cfg.Point) ([]factflow.ValueSource, bool) {
	sources, ok := r.returnSources[point]
	if !ok {
		return nil, false
	}
	out := make([]factflow.ValueSource, len(sources))
	copy(out, sources)
	return out, true
}

func (r normalReturnFactProjectResultStub) ExpressionPathRef(ref factflow.ExprRef) (pathdom.Path, bool) {
	p, ok := r.exprPaths[ref]
	return p, ok
}

func (r normalReturnFactProjectResultStub) ExpressionOperationRef(ref factflow.ExprRef) (factflow.ExpressionOperation, bool) {
	op, ok := r.exprOps[ref]
	return op, ok
}

func (r normalReturnFactProjectResultStub) ParameterValueSlots() []key.Value {
	if len(r.slots) == 0 {
		return nil
	}
	out := make([]key.Value, len(r.slots))
	copy(out, r.slots)
	return out
}

func (r normalReturnFactProjectResultStub) ReassignedParameterValueSlots() map[key.Value]struct{} {
	return r.reassigned
}

func normalReturnFactProjectTestKey(sym symbol.ID, suffix string) pathdom.PathKey {
	return pathdom.PathKey(pathaddr.VersionedRootString(sym, 1) + suffix)
}

func mustStateKey(t *testing.T, ks *keyspace.KeySpace, key pathdom.PathKey) keyspace.Key {
	t.Helper()
	k, ok := ks.FromStateKey(key)
	if !ok {
		t.Fatalf("FromStateKey(%q) failed", key)
	}
	return k
}

func testStateKey(t *testing.T, key pathdom.PathKey) pathaddr.StateKey {
	t.Helper()
	got, ok := pathaddr.StateKeyFromPathKey(key)
	if !ok {
		t.Fatalf("StateKeyFromPathKey(%q) failed", key)
	}
	return got
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

func assertLifecycleFact(t *testing.T, facts []callboundary.LifecycleFact, target pathdom.Path, kind callboundary.LifecycleKind) {
	t.Helper()
	for _, fact := range facts {
		if fact.Target.Equal(target) && fact.Kind == kind {
			return
		}
	}
	t.Fatalf("LifecycleFacts = %#v, want target %s kind %d", facts, target, kind)
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

func assertPathValueFact(t *testing.T, reg *axis.Registry, facts []callboundary.PathValueFact, target pathdom.Path, want product.Value) {
	t.Helper()
	for _, fact := range facts {
		if fact.Path.Equal(target) && product.Equal(reg, fact.Value, want) {
			return
		}
	}
	t.Fatalf("PathValueFacts = %#v, want %s = %#v", facts, target, want)
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

func assertStructuralPreservingPathInvalidation(t *testing.T, facts []callboundary.PathInvalidationFact, target pathdom.Path) {
	t.Helper()
	for _, fact := range facts {
		if fact.Path.Equal(target) && fact.PreserveStructuralWitness {
			return
		}
	}
	t.Fatalf("PathInvalidations = %#v, want structural-preserving %s", facts, target)
}

func assertClearingPathInvalidation(t *testing.T, facts []callboundary.PathInvalidationFact, target pathdom.Path) {
	t.Helper()
	for _, fact := range facts {
		if fact.Path.Equal(target) && fact.ClearTarget {
			return
		}
	}
	t.Fatalf("PathInvalidations = %#v, want target-clearing %s", facts, target)
}

func assertNonClearingPathInvalidation(t *testing.T, facts []callboundary.PathInvalidationFact, target pathdom.Path) {
	t.Helper()
	for _, fact := range facts {
		if fact.Path.Equal(target) && !fact.ClearTarget {
			return
		}
	}
	t.Fatalf("PathInvalidations = %#v, want non-target-clearing %s", facts, target)
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

func assertNumFloor(t *testing.T, facts []callboundary.NumFloorFact, target pathdom.Path, floor int64) {
	t.Helper()
	for _, fact := range facts {
		if fact.Path.Equal(target) && fact.Floor == floor {
			return
		}
	}
	t.Fatalf("NumFloors = %#v, want %s >= %d", facts, target, floor)
}

func assertRelConstraint(
	t *testing.T,
	facts []callboundary.RelConstraintFact,
	a pathdom.Path,
	b pathdom.Path,
	c pathdom.Path,
	cIsLength bool,
	k int64,
) {
	t.Helper()
	for _, fact := range facts {
		if fact.CoA == 1 && fact.CoB == 1 && fact.K == k &&
			fact.C.Path.Equal(c) && fact.C.IsLength == cIsLength &&
			((fact.A.Path.Equal(a) && fact.B.Path.Equal(b)) || (fact.A.Path.Equal(b) && fact.B.Path.Equal(a))) {
			return
		}
	}
	t.Fatalf("RelConstraints = %#v, want %s + %s - %s <= %d", facts, a, b, c, k)
}

func presentProduct(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Present())
}

func presentState(reg *axis.Registry, sym symbol.ID) state.State {
	return state.State{}.WriteValue(reg, key.SymbolValue(sym), presentProduct(reg))
}

func normalReturnBranchGraph(t *testing.T, normalTrue bool) (cfg.Graph, cfg.Point, cfg.Point, cfg.Point) {
	t.Helper()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	trueReturn := graph.AddNode(cfg.NodeReturn)
	falseReturn := graph.AddNode(cfg.NodeReturn)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, trueReturn, true)
	graph.AddEdge(branch, falseReturn, false)
	graph.AddEdge(trueReturn, graph.Exit(), false)
	graph.AddEdge(falseReturn, graph.Exit(), false)
	if normalTrue {
		return graph, branch, trueReturn, falseReturn
	}
	return graph, branch, falseReturn, trueReturn
}

func absentProduct(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Absent())
}

// TestProjectBranchProofKindPreservesAllKinds pins that every in-state branch
// proof kind survives projection into the call-boundary summary. IndexInRange was
// previously dropped by the default arm, silently losing the interproc
// index-in-range relation; all four relational kinds must round-trip.
func TestProjectBranchProofKindPreservesAllKinds(t *testing.T) {
	for _, kind := range []pathevidence.BranchProofKind{
		pathevidence.BranchProofPathPresence,
		pathevidence.BranchProofPathEqual,
		pathevidence.BranchProofPathNotEqual,
		pathevidence.BranchProofIndexInRange,
	} {
		got, ok := projectBranchProofKind(kind)
		if !ok || got != kind {
			t.Fatalf("projectBranchProofKind(%v) = (%v, %v), want (%v, true)", kind, got, ok, kind)
		}
	}
	if _, ok := projectBranchProofKind(pathevidence.BranchProofKind(0)); ok {
		t.Fatalf("the zero (unset) branch proof kind must not project")
	}
}
