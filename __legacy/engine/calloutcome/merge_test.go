package calloutcome

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	callpayload "github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/symbol"
	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
)

func testPrepareCallOutcome(t *testing.T, program callpayload.CallOutcomeProgram, ctx transfer.NodeContext, site factflow.CallSiteView) callpayload.CallOutcomeSiteProgram {
	t.Helper()
	prepared, err := program.PrepareSite(ctx, site)
	if err != nil {
		t.Fatal(err)
	}
	return prepared
}

func testEvaluateCallOutcome(t *testing.T, program callpayload.CallOutcomeProgram, ctx transfer.NodeContext, site factflow.CallSiteView, input callpayload.CallOutcomeInput) callpayload.CallOutcome {
	t.Helper()
	if site.ResultTargetCount() == 0 {
		arguments := make([]factflow.ValueSource, 16)
		results := make([]factflow.CallResultTarget, 16)
		for index := range results {
			results[index] = factflow.NewCallResultTarget(factflow.CallResultTargetExpression, index, index, 0, pathdom.Path{})
		}
		site = factflow.NewCallSite(factflow.CallSiteConfig{ArgumentSources: arguments, ResultTargets: results}).View()
	}
	prepared := testPrepareCallOutcome(t, program, ctx, site)
	outcome, err := prepared.Evaluate(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	return outcome
}

func TestComposeSupplementalRetainsOrderedLeafProofSeeds(t *testing.T) {
	first := callpayload.NormalReturnPathPresenceProofSeed(pathdom.NewPlaceholder(0), presence.Absent())
	second := callpayload.NormalReturnPathPresenceProofSeed(pathdom.NewPlaceholder(1), presence.Present())
	program := ComposeSupplemental(
		ComposeSupplemental(supplementalProofSeedProgram(first), supplementalProofSeedProgram(second)),
		supplementalProofSeedProgram(first),
	)
	site := factflow.NewCallSite(factflow.CallSiteConfig{ResultTargets: []factflow.CallResultTarget{
		factflow.NewCallResultTarget(factflow.CallResultTargetExpression, 0, 0, 0, pathdom.Path{}),
		factflow.NewCallResultTarget(factflow.CallResultTargetExpression, 1, 1, 0, pathdom.Path{}),
	}}).View()
	prepared := testPrepareCallOutcome(t, program, transfer.NodeContext{}, site)
	if prepared.ProofSeedCount() != 0 {
		t.Fatal("MergeSupplemental exposed a merged proof footprint")
	}
	left, exact := prepared.Component(0)
	if !exact || left.ProofSeedCount() != 0 {
		t.Fatal("nested supplemental merge did not retain its provider tree")
	}
	leftFirst, exact := left.Component(0)
	if !exact {
		t.Fatal("first nested supplemental leaf is absent")
	}
	got, exact := leftFirst.ProofSeed(0)
	if !exact || !got.Path.Equal(first.Path) || !presence.Equal(got.Presence, first.Presence) {
		t.Fatalf("first leaf proof seed = %#v exact:%v, want %#v", got, exact, first)
	}
	right, exact := prepared.Component(1)
	if !exact {
		t.Fatal("right supplemental leaf is absent")
	}
	got, exact = right.ProofSeed(0)
	if !exact || !got.Path.Equal(first.Path) || !presence.Equal(got.Presence, first.Presence) {
		t.Fatalf("right leaf proof seed = %#v exact:%v, want %#v", got, exact, first)
	}
}

func supplementalProofSeedProgram(seed callpayload.CallOutcomeProofSeed) callpayload.CallOutcomeProgram {
	return callpayload.SealCallOutcomeProgram("supplemental proof seed", []string{"NormalReturnFacts"}, state.LaneSet{}, state.LaneSet{},
		func(transfer.NodeContext, factflow.CallSiteView) (callpayload.CallOutcomeSiteShape, error) {
			return callpayload.CallOutcomeSiteShape{FieldNames: []string{"NormalReturnFacts"}, ProofSeeds: []callpayload.CallOutcomeProofSeed{seed}}, nil
		}, nil,
		func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
			return callpayload.CallOutcome{}, nil
		},
	)
}

func testOutcomeProgram(
	evaluate func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error),
	correlations ...callpayload.CallOutcomeCorrelationShape,
) callpayload.CallOutcomeProgram {
	if evaluate == nil {
		return callpayload.CallOutcomeProgram{}
	}
	roles := callpayload.CallOutcomeFieldRoles()
	fields := make([]string, len(roles))
	for index, role := range roles {
		fields[index] = role.FieldName
	}
	shape := func(transfer.NodeContext, factflow.CallSiteView) (callpayload.CallOutcomeSiteShape, error) {
		return callpayload.CallOutcomeSiteShape{FieldNames: fields, Correlations: correlations}, nil
	}
	return callpayload.SealCallOutcomeProgram("calloutcome test", fields, state.LaneSet{}, state.LaneSet{}, shape, nil, evaluate)
}

func fieldSuffix(name string) pathdom.Path {
	return pathdom.Path{Segments: []segment.Segment{{Kind: segment.SegmentField, Name: name}}}
}

func TestComposeSupplementalKeepsPrimarySlotsFillsMissingSlotsAndMergesSideFactsWithoutAuthority(t *testing.T) {
	reg := standard.Registry()
	primaryValue := product.Absent(reg)
	supplementalValue := product.Top()
	primary := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			Results: []callpayload.CallResult{{Index: 0, Value: primaryValue}},
			NormalReturnFacts: callboundary.NormalReturnFacts{
				PathRefinements: []callboundary.PathValueFact{
					{Path: pathdom.NewPlaceholder(0), Value: primaryValue},
				},
			},
			ParamConditions: []callpayload.CallParamCondition{
				{ParamIndex: 0, Value: true},
			},
		}, nil
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			Results: []callpayload.CallResult{{Index: 0, Value: product.Top()}, {Index: 1, Value: supplementalValue}},
			NormalReturnFacts: callboundary.NormalReturnFacts{
				PathRefinements: []callboundary.PathValueFact{
					{Path: pathdom.NewPlaceholder(1), Value: supplementalValue},
				},
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
			ParamConditions: []callpayload.CallParamCondition{
				{ParamIndex: 1, Value: false},
			},
			ReturnPresenceRelations: []callpayload.CallReturnPresenceRelation{
				{TriggerIndex: 1, TriggerPresence: presence.Present(), TargetIndex: 0, TargetPresence: presence.Absent()},
			},
			ReturnConditionSlots: []callpayload.CallReturnConditionSlotRefinement{
				{ReturnIndex: 0, ReturnValue: false, TargetIndex: 1, Value: primaryValue},
			},
		}, nil
	}

	got := testEvaluateCallOutcome(t, ComposeSupplemental(
		testOutcomeProgram(primary),
		testOutcomeProgram(supplemental,
			callpayload.ReturnPresenceShape(1, presence.Present(), 0, presence.Absent()),
			callpayload.ReturnConditionSlotShape(0, false, 1)),
	), transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), callpayload.CallOutcomeInput{})

	if len(got.Results) != 2 {
		t.Fatalf("got %d results, want 2: %#v", len(got.Results), got.Results)
	}
	if got.Results[0].Index != 0 || !product.Equal(reg, got.Results[0].Value, primaryValue) {
		t.Fatalf("primary slot = %#v, want index 0 primary value", got.Results[0])
	}
	if got.Results[1].Index != 1 || !product.Equal(reg, got.Results[1].Value, supplementalValue) {
		t.Fatalf("supplemental slot = %#v, want index 1 supplemental value", got.Results[1])
	}
	if len(got.ParamConditions) != 2 ||
		got.ParamConditions[0].ParamIndex != 0 || !got.ParamConditions[0].Value ||
		got.ParamConditions[1].ParamIndex != 1 || got.ParamConditions[1].Value {
		t.Fatalf("param conditions = %#v, want primary and supplemental facts", got.ParamConditions)
	}
	if len(got.NormalReturnFacts.PathRefinements) != 2 ||
		!got.NormalReturnFacts.PathRefinements[0].Path.Equal(pathdom.NewPlaceholder(0)) ||
		!got.NormalReturnFacts.PathRefinements[1].Path.Equal(pathdom.NewPlaceholder(1)) {
		t.Fatalf("normal return facts = %#v, want primary and supplemental path refinements", got.NormalReturnFacts)
	}
	if len(got.NormalReturnFacts.LifecycleFacts) != 1 ||
		!got.NormalReturnFacts.LifecycleFacts[0].Target.Equal(pathdom.NewPlaceholder(0)) ||
		got.NormalReturnFacts.LifecycleFacts[0].Kind != callboundary.LifecycleTransition {
		t.Fatalf("lifecycle facts = %#v, want supplemental lifecycle fact", got.NormalReturnFacts.LifecycleFacts)
	}
	if len(got.ReturnPresenceRelations) != 1 ||
		got.ReturnPresenceRelations[0].TriggerIndex != 1 ||
		!presence.Equal(got.ReturnPresenceRelations[0].TriggerPresence, presence.Present()) ||
		got.ReturnPresenceRelations[0].TargetIndex != 0 ||
		!presence.Equal(got.ReturnPresenceRelations[0].TargetPresence, presence.Absent()) {
		t.Fatalf("return presence relations = %#v, want supplemental relation", got.ReturnPresenceRelations)
	}
	if len(got.ReturnConditionSlots) != 1 ||
		got.ReturnConditionSlots[0].ReturnIndex != 0 ||
		got.ReturnConditionSlots[0].ReturnValue ||
		got.ReturnConditionSlots[0].TargetIndex != 1 {
		t.Fatalf("return condition slots = %#v, want supplemental relation", got.ReturnConditionSlots)
	}
}

func TestComposeSupplementalHeapObjectsPreservesOneSidedStaticMembers(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	id := identity.ID{Kind: "lua.table", Site: "supplemental", Index: 1}
	key, ok := heapidentity.StaticMemberSuffixKey(ks, fieldSuffix("field").Segments)
	if !ok {
		t.Fatal("failed to build static member key")
	}
	root := product.Top()
	member := product.Set(reg, product.Top(), evidence.Key, evidence.ExplicitTop())
	primary := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			HeapTableObjects: map[identity.ID]heapidentity.TableObject{
				id: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
					Root:          root,
					StaticMembers: map[keyspace.Key]product.Value{key: member},
				}),
			},
		}, nil
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			HeapTableObjects: map[identity.ID]heapidentity.TableObject{
				id: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: root}),
			},
		}, nil
	}

	got := testEvaluateCallOutcome(t, ComposeSupplemental(testOutcomeProgram(primary), testOutcomeProgram(supplemental)), transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), callpayload.CallOutcomeInput{})

	object, ok := got.HeapTableObjects[id]
	if !ok {
		t.Fatalf("heap objects = %#v, want %v", got.HeapTableObjects, id)
	}
	gotMember, ok := object.StaticMember(key)
	if !ok || !product.Equal(reg, gotMember, member) {
		t.Fatalf("static member = %#v/%v, want %#v", gotMember, ok, member)
	}
}

func TestComposeSupplementalCompactsNilProvidersAndPreservesMergeOrder(t *testing.T) {
	reg := standard.Registry()
	firstValue := product.Absent(reg)
	secondValue := product.Top()
	var calls []string
	first := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		calls = append(calls, "first")
		return callpayload.CallOutcome{
			PostReturnAuthority: true,
			Results:             []callpayload.CallResult{{Index: 0, Value: firstValue}},
			ParamObligations: []callpayload.CallParamObligation{
				{ParamIndex: 0, Value: firstValue},
			},
		}, nil
	}
	second := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		calls = append(calls, "second")
		return callpayload.CallOutcome{
			Results: []callpayload.CallResult{{Index: 1, Value: secondValue}},
			ParamObligations: []callpayload.CallParamObligation{
				{ParamIndex: 1, Value: secondValue},
			},
			ParamConditions: []callpayload.CallParamCondition{
				{ParamIndex: 1, Value: true},
			},
		}, nil
	}

	provider := ComposeSupplemental(testOutcomeProgram(nil), testOutcomeProgram(first), testOutcomeProgram(nil), testOutcomeProgram(second))
	if provider.Empty() {
		t.Fatal("ComposeSupplemental returned nil for non-nil providers")
	}
	got := testEvaluateCallOutcome(t, provider, transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), callpayload.CallOutcomeInput{})

	if len(calls) != 2 || calls[0] != "first" || calls[1] != "second" {
		t.Fatalf("provider calls = %v, want first then second", calls)
	}
	if len(got.Results) != 1 || got.Results[0].Index != 0 {
		t.Fatalf("results = %#v, want authoritative first result only", got.Results)
	}
	if len(got.ParamObligations) != 2 {
		t.Fatalf("param obligations = %#v, want diagnostics from both providers", got.ParamObligations)
	}
	if len(got.ParamConditions) != 0 {
		t.Fatalf("param conditions = %#v, want post-return facts blocked by authority", got.ParamConditions)
	}
}

func TestMergeSupplementalRefinesFreeTypeParamResultWithConcreteCallableReturn(t *testing.T) {
	reg := standard.Registry()
	param := typ.NewTypeParam("T", nil)
	genericResult := typetable.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", param).
		Build()
	concreteResult := typetable.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", typ.String).
		Build()
	genericValue := typevalue.FromType(reg, genericResult)
	concreteValue := typevalue.FromType(reg, concreteResult)

	got := MergeSupplemental(reg,
		callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: genericValue}}},
		callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: concreteValue}}},
	)

	if len(got.Results) != 1 {
		t.Fatalf("results = %#v, want one refined slot", got.Results)
	}
	if !product.Equal(reg, got.Results[0].Value, concreteValue) {
		t.Fatalf("result slot = %#v, want concrete callable return", got.Results[0].Value)
	}
}

func TestMergeSupplementalDoesNotWeakenProvenResultWithExplicitAny(t *testing.T) {
	reg := standard.Registry()
	optionalString := typeexpr.Optional(typ.String)
	proven := typevalue.FromType(reg, optionalString)
	untrusted := typevalue.WithWitness(reg,
		product.Set(reg, proven, evidence.Key, evidence.ExplicitTop()),
		optionalString,
	)

	got := MergeSupplemental(reg,
		callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: proven}}},
		callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: untrusted}}},
	)

	if len(got.Results) != 1 {
		t.Fatalf("results = %#v, want one result", got.Results)
	}
	if !product.Equal(reg, got.Results[0].Value, proven) {
		t.Fatalf("result value was weakened by explicit-any supplemental: got %v want %v", got.Results[0].Value, proven)
	}
}

func TestMergeSupplementalTrustedReturnReplacesUntrustedTopWithTypeWitness(t *testing.T) {
	reg := standard.Registry()
	optionalString := typeexpr.Optional(typ.String)
	proven := typevalue.FromType(reg, optionalString)
	untrusted := typevalue.WithWitness(reg,
		product.Set(reg, proven, evidence.Key, evidence.ExplicitTop()),
		optionalString,
	)

	got := MergeSupplemental(reg,
		callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: untrusted}}},
		callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: proven}}},
	)

	if len(got.Results) != 1 {
		t.Fatalf("results = %#v, want one result", got.Results)
	}
	if !product.Equal(reg, got.Results[0].Value, proven) {
		t.Fatalf("result value kept explicit-any evidence: got %v want %v", got.Results[0].Value, proven)
	}
}

func TestComposeSupplementalAuthorityBlocksSupplementalPostReturnFacts(t *testing.T) {
	reg := standard.Registry()
	primaryValue := product.Absent(reg)
	supplementalValue := product.Top()
	primary := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			PostReturnAuthority: true,
			Results:             []callpayload.CallResult{{Index: 0, Value: primaryValue}},
			NormalReturnFacts: callboundary.NormalReturnFacts{
				PathRefinements: []callboundary.PathValueFact{
					{Path: pathdom.NewPlaceholder(0), Value: primaryValue},
				},
			},
			ParamObligations: []callpayload.CallParamObligation{
				{ParamIndex: 0, Value: primaryValue},
			},
			ParamConditions: []callpayload.CallParamCondition{
				{ParamIndex: 0, Value: true},
			},
		}, nil
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			PostReturnAuthority: true,
			Results:             []callpayload.CallResult{{Index: 1, Value: supplementalValue}},
			NormalReturnFacts: callboundary.NormalReturnFacts{
				PathRefinements: []callboundary.PathValueFact{
					{Path: pathdom.NewPlaceholder(1), Value: supplementalValue},
				},
			},
			ParamObligations: []callpayload.CallParamObligation{
				{ParamIndex: 1, Value: supplementalValue},
			},
			ParamConditions: []callpayload.CallParamCondition{
				{ParamIndex: 1, Value: false},
			},
			ReturnPresenceRelations: []callpayload.CallReturnPresenceRelation{
				{TriggerIndex: 1, TriggerPresence: presence.Present(), TargetIndex: 0, TargetPresence: presence.Absent()},
			},
			ReturnConditionSlots: []callpayload.CallReturnConditionSlotRefinement{
				{ReturnIndex: 0, ReturnValue: false, TargetIndex: 1, Value: primaryValue},
			},
		}, nil
	}

	got := testEvaluateCallOutcome(t, ComposeSupplemental(
		testOutcomeProgram(primary),
		testOutcomeProgram(supplemental,
			callpayload.ReturnPresenceShape(1, presence.Present(), 0, presence.Absent()),
			callpayload.ReturnConditionSlotShape(0, false, 1)),
	), transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), callpayload.CallOutcomeInput{})

	if !got.PostReturnAuthority {
		t.Fatalf("PostReturnAuthority = false, want true")
	}
	if len(got.Results) != 1 {
		t.Fatalf("got %d results, want only authoritative primary slot: %#v", len(got.Results), got.Results)
	}
	if got.Results[0].Index != 0 || !product.Equal(reg, got.Results[0].Value, primaryValue) {
		t.Fatalf("primary slot = %#v, want index 0 primary value", got.Results[0])
	}
	if len(got.ParamObligations) != 2 ||
		got.ParamObligations[0].ParamIndex != 0 ||
		got.ParamObligations[1].ParamIndex != 1 {
		t.Fatalf("param obligations = %#v, want primary plus supplemental diagnostics", got.ParamObligations)
	}
	if len(got.ParamConditions) != 1 ||
		got.ParamConditions[0].ParamIndex != 0 ||
		!got.ParamConditions[0].Value {
		t.Fatalf("param conditions = %#v, want only primary post-return condition", got.ParamConditions)
	}
	if len(got.NormalReturnFacts.PathRefinements) != 1 ||
		!got.NormalReturnFacts.PathRefinements[0].Path.Equal(pathdom.NewPlaceholder(0)) {
		t.Fatalf("normal return facts = %#v, want only primary path refinement", got.NormalReturnFacts)
	}
	if len(got.ReturnPresenceRelations) != 0 {
		t.Fatalf("return presence relations = %#v, want supplemental post-return relation blocked", got.ReturnPresenceRelations)
	}
	if len(got.ReturnConditionSlots) != 0 {
		t.Fatalf("return condition slots = %#v, want supplemental post-return relation blocked", got.ReturnConditionSlots)
	}
}

func TestComposeSupplementalAuthorityAllowsSpecificRefinementOfExistingWeakResultSlot(t *testing.T) {
	reg := standard.Registry()
	primaryValue := product.Set(reg, product.Top(), evidence.Key, evidence.GradualTop())
	supplementalValue := typeValue(reg, typ.Number)
	primary := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			PostReturnAuthority: true,
			Results:             []callpayload.CallResult{{Index: 0, Value: primaryValue}},
			ReturnConditionRefinements: []callpayload.CallReturnConditionRefinement{
				{ReturnIndex: 0, ReturnValue: true, Target: pathdom.NewPlaceholder(0), Value: supplementalValue},
			},
		}, nil
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			Results: []callpayload.CallResult{
				{Index: 0, Value: supplementalValue},
				{Index: 1, Value: supplementalValue},
			},
			ReturnPresenceRelations: []callpayload.CallReturnPresenceRelation{
				{TriggerIndex: 0, TriggerPresence: presence.Present(), TargetIndex: 1, TargetPresence: presence.Present()},
			},
		}, nil
	}

	got := testEvaluateCallOutcome(t, ComposeSupplemental(
		testOutcomeProgram(primary, callpayload.ReturnConditionPathShape(0, true, pathdom.NewPlaceholder(0))),
		testOutcomeProgram(supplemental, callpayload.ReturnPresenceShape(0, presence.Present(), 1, presence.Present())),
	), transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), callpayload.CallOutcomeInput{})

	if !got.PostReturnAuthority {
		t.Fatal("PostReturnAuthority = false, want authoritative primary preserved")
	}
	if len(got.Results) != 1 {
		t.Fatalf("results = %#v, want only existing authoritative slot refined", got.Results)
	}
	gotType, ok := typevalue.TypeOf(reg, got.Results[0].Value)
	if got.Results[0].Index != 0 || !ok || !typ.TypeEquals(gotType, typ.Number) {
		t.Fatalf("result slot = %#v type %v/%v, want exact number refinement of slot 0", got.Results[0], gotType, ok)
	}
	if gotEvidence := product.Get(reg, got.Results[0].Value, evidence.Key); gotEvidence.IsGradualTop() || gotEvidence.IsExplicitTop() {
		t.Fatalf("result slot kept weak fallback evidence %s, want supplemental proof evidence", gotEvidence)
	}
	if len(got.ReturnPresenceRelations) != 0 {
		t.Fatalf("return presence relations = %#v, want supplemental post-return facts blocked", got.ReturnPresenceRelations)
	}
}

func TestComposeSupplementalAuthorityAllowsNarrowerExistingResultSlotRefinement(t *testing.T) {
	reg := standard.Registry()
	primaryValue := product.WithPresence(reg, typeValue(reg, typ.Number), presence.Maybe())
	supplementalValue := typeValue(reg, typ.Number)
	primary := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			PostReturnAuthority: true,
			Results:             []callpayload.CallResult{{Index: 0, Value: primaryValue}},
			ReturnConditionRefinements: []callpayload.CallReturnConditionRefinement{
				{ReturnIndex: 0, ReturnValue: true, Target: pathdom.NewPlaceholder(0), Value: supplementalValue},
			},
		}, nil
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			Results: []callpayload.CallResult{{Index: 0, Value: supplementalValue}},
		}, nil
	}

	got := testEvaluateCallOutcome(t, ComposeSupplemental(
		testOutcomeProgram(primary, callpayload.ReturnConditionPathShape(0, true, pathdom.NewPlaceholder(0))),
		testOutcomeProgram(supplemental),
	), transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), callpayload.CallOutcomeInput{})

	gotType, ok := typevalue.TypeOf(reg, got.Results[0].Value)
	if len(got.Results) != 1 || !ok || !typ.TypeEquals(gotType, typ.Number) {
		t.Fatalf("result slot = %#v type %v/%v, want exact number", got.Results, gotType, ok)
	}
}

func TestComposeSupplementalPreservesPrimaryAuthorityWhenSupplementalIsWeak(t *testing.T) {
	reg := standard.Registry()
	primaryValue := typeValue(reg, typ.String)
	supplementalValue := product.Top()
	primary := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			PostReturnAuthority: true,
			Results:             []callpayload.CallResult{{Index: 0, Value: primaryValue}},
			ParamObligations: []callpayload.CallParamObligation{
				{ParamIndex: 0, Value: primaryValue},
			},
		}, nil
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			Results: []callpayload.CallResult{{Index: 0, Value: supplementalValue}},
			NormalReturnFacts: callboundary.NormalReturnFacts{
				PathRefinements: []callboundary.PathValueFact{
					{Path: pathdom.NewPlaceholder(0), Value: supplementalValue},
				},
			},
			ParamObligations: []callpayload.CallParamObligation{
				{ParamIndex: 1, Value: supplementalValue},
			},
		}, nil
	}

	got := testEvaluateCallOutcome(t, ComposeSupplemental(testOutcomeProgram(primary), testOutcomeProgram(supplemental)), transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), callpayload.CallOutcomeInput{})

	if !got.PostReturnAuthority {
		t.Fatal("PostReturnAuthority = false, want authoritative primary preserved")
	}
	if len(got.Results) != 1 || got.Results[0].Index != 0 || !product.Equal(reg, got.Results[0].Value, primaryValue) {
		t.Fatalf("results = %#v, want primary result only", got.Results)
	}
	if len(got.NormalReturnFacts.PathRefinements) != 0 {
		t.Fatalf("normal return facts = %#v, want weak supplemental post-return facts blocked", got.NormalReturnFacts)
	}
	if len(got.ParamObligations) != 2 {
		t.Fatalf("param obligations = %#v, want diagnostic obligations from both providers", got.ParamObligations)
	}
}

func TestComposeSupplementalPreservesPrimaryNonTypeEvidence(t *testing.T) {
	reg := standard.Registry()
	primaryValue := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	supplementalValue := typeValue(reg, typ.String)
	primary := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: primaryValue}}}, nil
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: supplementalValue}}}, nil
	}

	got := testEvaluateCallOutcome(t, ComposeSupplemental(testOutcomeProgram(primary), testOutcomeProgram(supplemental)),
		transfer.NodeContext{Registry: reg},
		factflow.NewCallSite(factflow.CallSiteConfig{}).View(), callpayload.CallOutcomeInput{},
	)

	if len(got.Results) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got.Results), got.Results)
	}
	if gotPresence := product.PresenceOf(got.Results[0].Value); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("presence = %s, want present", gotPresence)
	}
	gotType, ok := typevalue.TypeOf(reg, got.Results[0].Value)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("type = %v/%v, want string", gotType, ok)
	}
}

func TestComposeSupplementalPromotesGradualResultEvidenceToExplicit(t *testing.T) {
	reg := standard.Registry()
	gradual := product.Set(reg, typevalue.FromType(reg, typ.Any), evidence.Key, evidence.GradualTop())
	explicit := product.Set(reg, typevalue.FromType(reg, typ.Any), evidence.Key, evidence.ExplicitTop())
	primary := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: gradual}}}, nil
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: explicit}}}, nil
	}

	got := testEvaluateCallOutcome(t, ComposeSupplemental(testOutcomeProgram(primary), testOutcomeProgram(supplemental)),
		transfer.NodeContext{Registry: reg},
		factflow.NewCallSite(factflow.CallSiteConfig{}).View(), callpayload.CallOutcomeInput{},
	)

	if len(got.Results) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got.Results), got.Results)
	}
	if ev := product.Get(reg, got.Results[0].Value, evidence.Key); !ev.IsExplicitTop() {
		t.Fatalf("evidence = %s, want explicit top", ev)
	}
}

func TestComposeSupplementalDoesNotDemoteExplicitResultEvidenceToGradual(t *testing.T) {
	reg := standard.Registry()
	explicit := product.Set(reg, typevalue.FromType(reg, typ.Any), evidence.Key, evidence.ExplicitTop())
	gradual := product.Set(reg, typevalue.FromType(reg, typ.Any), evidence.Key, evidence.GradualTop())
	primary := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: explicit}}}, nil
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: gradual}}}, nil
	}

	got := testEvaluateCallOutcome(t, ComposeSupplemental(testOutcomeProgram(primary), testOutcomeProgram(supplemental)),
		transfer.NodeContext{Registry: reg},
		factflow.NewCallSite(factflow.CallSiteConfig{}).View(), callpayload.CallOutcomeInput{},
	)

	if len(got.Results) != 1 {
		t.Fatalf("got %d results, want 1: %#v", len(got.Results), got.Results)
	}
	if ev := product.Get(reg, got.Results[0].Value, evidence.Key); !ev.IsExplicitTop() {
		t.Fatalf("evidence = %s, want explicit top", ev)
	}
}

func TestComposeSupplementalMergesHeapTableObjectsWithoutAuthority(t *testing.T) {
	reg := standard.Registry()
	tableID := identity.ID{Kind: "table", Site: "compose", Index: 1}
	primary := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			HeapTableObjects: map[identity.ID]heapidentity.TableObject{
				tableID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Absent(reg)}),
			},
		}, nil
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			HeapTableObjects: map[identity.ID]heapidentity.TableObject{
				tableID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Top()}),
			},
		}, nil
	}

	got := testEvaluateCallOutcome(t, ComposeSupplemental(testOutcomeProgram(primary), testOutcomeProgram(supplemental)), transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), callpayload.CallOutcomeInput{})
	object, ok := got.HeapTableObjects[tableID]
	if !ok {
		t.Fatalf("HeapTableObjects = %#v, want %v", got.HeapTableObjects, tableID)
	}
	if !product.Equal(reg, object.Root(), product.Top()) {
		t.Fatalf("merged heap object root = %#v, want top", object.Root())
	}
}

func TestComposeSupplementalMergesPlacementFactsWithoutAuthority(t *testing.T) {
	tableID := identity.ID{Kind: "table", Site: "compose-placement", Index: 1}
	otherID := identity.ID{Kind: "table", Site: "compose-placement", Index: 2}
	primary := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			Placements: map[identity.ID]placement.Value{
				tableID: placement.Stack,
			},
		}, nil
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			Placements: map[identity.ID]placement.Value{
				tableID: placement.SharedHeap,
				otherID: placement.OwnedHeap,
			},
		}, nil
	}

	got := testEvaluateCallOutcome(t, ComposeSupplemental(testOutcomeProgram(primary), testOutcomeProgram(supplemental)), transfer.NodeContext{}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), callpayload.CallOutcomeInput{})
	if got.Placements[tableID] != placement.SharedHeap {
		t.Fatalf("placement[%v] = %s, want shared-heap", tableID, got.Placements[tableID])
	}
	if got.Placements[otherID] != placement.OwnedHeap {
		t.Fatalf("placement[%v] = %s, want owned-heap", otherID, got.Placements[otherID])
	}
}

func TestComposeSupplementalMergesParamLengthFloorsWithoutAuthority(t *testing.T) {
	primary := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			ParamLengthFloors: []callpayload.CallParamLengthFloor{
				{Path: pathdom.NewPlaceholder(0), Floor: 2},
			},
		}, nil
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			ParamLengthFloors: []callpayload.CallParamLengthFloor{
				{Path: pathdom.NewPlaceholder(1), Floor: 3},
			},
		}, nil
	}

	got := testEvaluateCallOutcome(t, ComposeSupplemental(testOutcomeProgram(primary), testOutcomeProgram(supplemental)), transfer.NodeContext{}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), callpayload.CallOutcomeInput{})
	if len(got.ParamLengthFloors) != 2 {
		t.Fatalf("ParamLengthFloors = %#v, want primary and supplemental floors", got.ParamLengthFloors)
	}
	if !got.ParamLengthFloors[0].Path.Equal(pathdom.NewPlaceholder(0)) || got.ParamLengthFloors[0].Floor != 2 ||
		!got.ParamLengthFloors[1].Path.Equal(pathdom.NewPlaceholder(1)) || got.ParamLengthFloors[1].Floor != 3 {
		t.Fatalf("ParamLengthFloors = %#v, want ordered primary then supplemental floors", got.ParamLengthFloors)
	}
}

func TestComposeSupplementalAuthorityBlocksSupplementalHeapTableObjects(t *testing.T) {
	reg := standard.Registry()
	primaryID := identity.ID{Kind: "table", Site: "compose", Index: 2}
	supplementalID := identity.ID{Kind: "table", Site: "compose", Index: 3}
	primary := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			PostReturnAuthority: true,
			HeapTableObjects: map[identity.ID]heapidentity.TableObject{
				primaryID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Absent(reg)}),
			},
		}, nil
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			HeapTableObjects: map[identity.ID]heapidentity.TableObject{
				supplementalID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Top()}),
			},
		}, nil
	}

	got := testEvaluateCallOutcome(t, ComposeSupplemental(testOutcomeProgram(primary), testOutcomeProgram(supplemental)), transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), callpayload.CallOutcomeInput{})
	if len(got.HeapTableObjects) != 1 {
		t.Fatalf("HeapTableObjects = %#v, want only authoritative primary object", got.HeapTableObjects)
	}
	if _, ok := got.HeapTableObjects[primaryID]; !ok {
		t.Fatalf("HeapTableObjects = %#v, want primary identity", got.HeapTableObjects)
	}
	if _, ok := got.HeapTableObjects[supplementalID]; ok {
		t.Fatalf("HeapTableObjects = %#v, want supplemental identity blocked", got.HeapTableObjects)
	}
}

func TestComposeSupplementalAuthorityKeepsHeapFactsForAuthoritativeResultIdentity(t *testing.T) {
	reg := standard.Registry()
	resultID := identity.ID{Kind: "table", Site: "compose-result", Index: 1}
	resultValue := product.Set(reg, product.Top(), identity.Key, identity.Singleton(resultID))
	memberKey := keyspace.New().FromPath(pathdom.NewPath(symbol.ID(1), "root").Field("run"))
	primary := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			PostReturnAuthority: true,
			Results:             []callpayload.CallResult{{Index: 0, Value: resultValue}},
		}, nil
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			HeapTableObjects: map[identity.ID]heapidentity.TableObject{
				resultID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
					Root: resultValue,
					StaticMembers: map[keyspace.Key]product.Value{
						memberKey: typevalue.FromType(reg, typ.String),
					},
				}),
			},
		}, nil
	}

	got := testEvaluateCallOutcome(t, ComposeSupplemental(testOutcomeProgram(primary), testOutcomeProgram(supplemental)), transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), callpayload.CallOutcomeInput{})
	object, ok := got.HeapTableObjects[resultID]
	if !ok {
		t.Fatalf("HeapTableObjects = %#v, want supplemental facts for authoritative result identity", got.HeapTableObjects)
	}
	if value, ok := object.StaticMember(memberKey); !ok {
		t.Fatalf("static member = %v/%v, want propagated member for authoritative result object", value, ok)
	}
}

func TestComposeSupplementalAuthorityDropsEmptyHeapFactForAuthoritativeResultIdentity(t *testing.T) {
	reg := standard.Registry()
	resultID := identity.ID{Kind: "table", Site: "compose-result-empty", Index: 1}
	resultValue := product.Set(reg, typevalue.FromType(reg, typetable.NewRecord().Build()), identity.Key, identity.Singleton(resultID))
	primary := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			PostReturnAuthority: true,
			Results:             []callpayload.CallResult{{Index: 0, Value: resultValue}},
		}, nil
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			HeapTableObjects: map[identity.ID]heapidentity.TableObject{
				resultID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: resultValue}),
			},
		}, nil
	}

	got := testEvaluateCallOutcome(t, ComposeSupplemental(testOutcomeProgram(primary), testOutcomeProgram(supplemental)), transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), callpayload.CallOutcomeInput{})

	if len(got.HeapTableObjects) != 0 {
		t.Fatalf("HeapTableObjects = %#v, want empty no-op heap fact filtered", got.HeapTableObjects)
	}
}

func TestComposeSupplementalAuthorityBlocksSupplementalPlacementFacts(t *testing.T) {
	primaryID := identity.ID{Kind: "table", Site: "compose-placement", Index: 4}
	supplementalID := identity.ID{Kind: "table", Site: "compose-placement", Index: 5}
	primary := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			PostReturnAuthority: true,
			Placements: map[identity.ID]placement.Value{
				primaryID: placement.Stack,
			},
		}, nil
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			Placements: map[identity.ID]placement.Value{
				primaryID:      placement.SharedHeap,
				supplementalID: placement.OwnedHeap,
			},
		}, nil
	}

	got := testEvaluateCallOutcome(t, ComposeSupplemental(testOutcomeProgram(primary), testOutcomeProgram(supplemental)), transfer.NodeContext{}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), callpayload.CallOutcomeInput{})
	if len(got.Placements) != 1 {
		t.Fatalf("Placements = %#v, want only authoritative primary placement", got.Placements)
	}
	if got.Placements[primaryID] != placement.Stack {
		t.Fatalf("placement[%v] = %s, want primary stack placement", primaryID, got.Placements[primaryID])
	}
	if _, ok := got.Placements[supplementalID]; ok {
		t.Fatalf("Placements = %#v, want supplemental placement blocked", got.Placements)
	}
}

func TestComposeSupplementalAuthorityBlocksSupplementalParamLengthFloors(t *testing.T) {
	primary := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			PostReturnAuthority: true,
			ParamLengthFloors: []callpayload.CallParamLengthFloor{
				{Path: pathdom.NewPlaceholder(0), Floor: 2},
			},
		}, nil
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		return callpayload.CallOutcome{
			ParamLengthFloors: []callpayload.CallParamLengthFloor{
				{Path: pathdom.NewPlaceholder(1), Floor: 3},
			},
		}, nil
	}

	got := testEvaluateCallOutcome(t, ComposeSupplemental(testOutcomeProgram(primary), testOutcomeProgram(supplemental)), transfer.NodeContext{}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), callpayload.CallOutcomeInput{})
	if len(got.ParamLengthFloors) != 1 {
		t.Fatalf("ParamLengthFloors = %#v, want only authoritative primary floor", got.ParamLengthFloors)
	}
	if !got.ParamLengthFloors[0].Path.Equal(pathdom.NewPlaceholder(0)) || got.ParamLengthFloors[0].Floor != 2 {
		t.Fatalf("ParamLengthFloors = %#v, want primary floor", got.ParamLengthFloors)
	}
}

func typeValue(reg *axis.Registry, t typ.Type) product.Value {
	return typevalue.WithWitness(reg, typevalue.FromType(reg, t), t)
}
