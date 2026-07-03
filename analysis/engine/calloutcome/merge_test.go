package calloutcome

import (
	"reflect"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
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
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestSupplementalFactLanesMatchCallOutcomeFieldRoles(t *testing.T) {
	payloadRoles := make(map[string]callpayload.CallOutcomeFieldRole)
	for _, role := range callpayload.CallOutcomeFieldRoles() {
		if role.FieldName == "" {
			t.Fatal("call payload role with empty field name")
		}
		if _, ok := payloadRoles[role.FieldName]; ok {
			t.Fatalf("call payload role %s registered more than once", role.FieldName)
		}
		payloadRoles[role.FieldName] = role
	}

	supplementalRoles := make(map[string]supplementalFactLane)
	for _, lane := range supplementalFactLanes {
		if lane.role.FieldName == "" {
			t.Fatal("supplemental fact lane with empty field name")
		}
		if lane.merge == nil {
			t.Fatalf("supplemental fact lane %s has nil merge function", lane.role.FieldName)
		}
		if _, ok := supplementalRoles[lane.role.FieldName]; ok {
			t.Fatalf("supplemental fact lane %s registered more than once", lane.role.FieldName)
		}
		role, ok := payloadRoles[lane.role.FieldName]
		if !ok {
			t.Fatalf("supplemental fact lane %s has no call payload role", lane.role.FieldName)
		}
		if lane.role != role {
			t.Fatalf("supplemental fact lane %s role = %#v, payload role = %#v", lane.role.FieldName, lane.role, role)
		}
		supplementalRoles[lane.role.FieldName] = lane
	}

	for _, role := range payloadRoles {
		switch role.FieldName {
		case "Results", "PostReturnAuthority":
			if _, ok := supplementalRoles[role.FieldName]; ok {
				t.Fatalf("%s is handled outside supplemental fact lanes", role.FieldName)
			}
			continue
		}
		if _, ok := supplementalRoles[role.FieldName]; !ok {
			t.Fatalf("call payload role %s has no supplemental fact lane", role.FieldName)
		}
	}
}

func TestBuildSupplementalFactLanesRejectsMissingHandler(t *testing.T) {
	handlers := supplementalFactLaneTestHandlers()
	delete(handlers, "NormalReturnFacts")
	requirePanic(t, func() {
		_ = buildSupplementalFactLanes(supplementalFactLaneHandlers(handlers))
	})
}

func TestBuildSupplementalFactLanesRejectsOrphanHandler(t *testing.T) {
	handlers := supplementalFactLaneTestHandlers()
	handlers["NotAField"] = supplementalFactLaneHandler{
		fieldName: "NotAField",
		merge:     func(*axis.Registry, *callpayload.CallOutcome, callpayload.CallOutcome) {},
	}
	requirePanic(t, func() {
		_ = buildSupplementalFactLanes(supplementalFactLaneHandlers(handlers))
	})
}

func TestBuildSupplementalFactLanesRejectsInvalidHandler(t *testing.T) {
	handlers := supplementalFactLaneTestHandlers()
	handlers["NormalReturnFacts"] = supplementalFactLaneHandler{fieldName: "NormalReturnFacts"}
	requirePanic(t, func() {
		_ = buildSupplementalFactLanes(supplementalFactLaneHandlers(handlers))
	})
}

func TestCallOutcomeFieldRolesCoverStructFields(t *testing.T) {
	fields := make(map[string]struct{})
	typ := reflect.TypeOf(callpayload.CallOutcome{})
	for i := 0; i < typ.NumField(); i++ {
		fields[typ.Field(i).Name] = struct{}{}
	}
	for _, role := range callpayload.CallOutcomeFieldRoles() {
		if _, ok := fields[role.FieldName]; !ok {
			t.Fatalf("call payload role references missing field %s", role.FieldName)
		}
		delete(fields, role.FieldName)
	}
	for field := range fields {
		t.Fatalf("CallOutcome.%s has no exported field role", field)
	}
}

func supplementalFactLaneTestHandlers() map[string]supplementalFactLaneHandler {
	out := make(map[string]supplementalFactLaneHandler)
	for _, lane := range supplementalFactLanes {
		out[lane.role.FieldName] = supplementalFactLaneHandler{
			fieldName:          lane.role.FieldName,
			merge:              lane.merge,
			mergeAuthoritative: lane.mergeAuthoritative,
		}
	}
	return out
}

func supplementalFactLaneHandlers(in map[string]supplementalFactLaneHandler) []supplementalFactLaneHandler {
	out := make([]supplementalFactLaneHandler, 0, len(in))
	for _, handler := range in {
		out = append(out, handler)
	}
	return out
}

func requirePanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}

func TestWithSupplementalKeepsPrimarySlotsFillsMissingSlotsAndMergesSideFactsWithoutAuthority(t *testing.T) {
	reg := standard.Registry()
	primaryValue := product.Absent(reg)
	supplementalValue := product.Top()
	primary := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
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
		}
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
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
		}
	}

	got := WithSupplemental(primary, supplemental)(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)

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

func TestComposeSupplementalCompactsNilProvidersAndPreservesMergeOrder(t *testing.T) {
	reg := standard.Registry()
	firstValue := product.Absent(reg)
	secondValue := product.Top()
	var calls []string
	first := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		calls = append(calls, "first")
		return callpayload.CallOutcome{
			PostReturnAuthority: true,
			Results:             []callpayload.CallResult{{Index: 0, Value: firstValue}},
			ParamObligations: []callpayload.CallParamObligation{
				{ParamIndex: 0, Value: firstValue},
			},
		}
	}
	second := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		calls = append(calls, "second")
		return callpayload.CallOutcome{
			Results: []callpayload.CallResult{{Index: 1, Value: secondValue}},
			ParamObligations: []callpayload.CallParamObligation{
				{ParamIndex: 1, Value: secondValue},
			},
			ParamConditions: []callpayload.CallParamCondition{
				{ParamIndex: 1, Value: true},
			},
		}
	}

	provider := ComposeSupplemental(nil, first, nil, second)
	if provider == nil {
		t.Fatal("ComposeSupplemental returned nil for non-nil providers")
	}
	got := provider(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)

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

func TestWithSupplementalAuthorityBlocksSupplementalPostReturnFacts(t *testing.T) {
	reg := standard.Registry()
	primaryValue := product.Absent(reg)
	supplementalValue := product.Top()
	primary := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
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
		}
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
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
		}
	}

	got := WithSupplemental(primary, supplemental)(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)

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

func TestWithSupplementalAuthorityAllowsSpecificRefinementOfExistingWeakResultSlot(t *testing.T) {
	reg := standard.Registry()
	primaryValue := product.Set(reg, product.Top(), evidence.Key, evidence.GradualTop())
	supplementalValue := typeValue(reg, typ.Number)
	primary := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{
			PostReturnAuthority: true,
			Results:             []callpayload.CallResult{{Index: 0, Value: primaryValue}},
			ReturnConditionRefinements: []callpayload.CallReturnConditionRefinement{
				{ReturnIndex: 0, ReturnValue: true, Target: pathdom.NewPlaceholder(0), Value: supplementalValue},
			},
		}
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{
			Results: []callpayload.CallResult{
				{Index: 0, Value: supplementalValue},
				{Index: 1, Value: supplementalValue},
			},
			ReturnPresenceRelations: []callpayload.CallReturnPresenceRelation{
				{TriggerIndex: 0, TriggerPresence: presence.Present(), TargetIndex: 1, TargetPresence: presence.Present()},
			},
		}
	}

	got := WithSupplemental(primary, supplemental)(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)

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

func TestWithSupplementalAuthorityAllowsNarrowerExistingResultSlotRefinement(t *testing.T) {
	reg := standard.Registry()
	primaryValue := product.WithPresence(reg, typeValue(reg, typ.Number), presence.Maybe())
	supplementalValue := typeValue(reg, typ.Number)
	primary := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{
			PostReturnAuthority: true,
			Results:             []callpayload.CallResult{{Index: 0, Value: primaryValue}},
			ReturnConditionRefinements: []callpayload.CallReturnConditionRefinement{
				{ReturnIndex: 0, ReturnValue: true, Target: pathdom.NewPlaceholder(0), Value: supplementalValue},
			},
		}
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{
			Results: []callpayload.CallResult{{Index: 0, Value: supplementalValue}},
		}
	}

	got := WithSupplemental(primary, supplemental)(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)

	gotType, ok := typevalue.TypeOf(reg, got.Results[0].Value)
	if len(got.Results) != 1 || !ok || !typ.TypeEquals(gotType, typ.Number) {
		t.Fatalf("result slot = %#v type %v/%v, want exact number", got.Results, gotType, ok)
	}
}

func TestWithSupplementalPreservesPrimaryAuthorityWhenSupplementalIsWeak(t *testing.T) {
	reg := standard.Registry()
	primaryValue := typeValue(reg, typ.String)
	supplementalValue := product.Top()
	primary := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{
			PostReturnAuthority: true,
			Results:             []callpayload.CallResult{{Index: 0, Value: primaryValue}},
			ParamObligations: []callpayload.CallParamObligation{
				{ParamIndex: 0, Value: primaryValue},
			},
		}
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
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
		}
	}

	got := WithSupplemental(primary, supplemental)(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)

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

func TestWithSupplementalPreservesPrimaryNonTypeEvidence(t *testing.T) {
	reg := standard.Registry()
	primaryValue := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	supplementalValue := typeValue(reg, typ.String)
	primary := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: primaryValue}}}
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: supplementalValue}}}
	}

	got := WithSupplemental(primary, supplemental)(
		transfer.NodeContext{Registry: reg},
		factflow.NewCallSite(factflow.CallSiteConfig{}).View(),
		state.State{},
		nil,
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

func TestWithSupplementalMergesHeapTableObjectsWithoutAuthority(t *testing.T) {
	reg := standard.Registry()
	tableID := identity.ID{Kind: "table", Site: "compose", Index: 1}
	primary := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{
			HeapTableObjects: map[identity.ID]heapidentity.TableObject{
				tableID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Absent(reg)}),
			},
		}
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{
			HeapTableObjects: map[identity.ID]heapidentity.TableObject{
				tableID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Top()}),
			},
		}
	}

	got := WithSupplemental(primary, supplemental)(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)
	object, ok := got.HeapTableObjects[tableID]
	if !ok {
		t.Fatalf("HeapTableObjects = %#v, want %v", got.HeapTableObjects, tableID)
	}
	if !product.Equal(reg, object.Root(), product.Top()) {
		t.Fatalf("merged heap object root = %#v, want top", object.Root())
	}
}

func TestWithSupplementalMergesPlacementFactsWithoutAuthority(t *testing.T) {
	tableID := identity.ID{Kind: "table", Site: "compose-placement", Index: 1}
	otherID := identity.ID{Kind: "table", Site: "compose-placement", Index: 2}
	primary := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{
			Placements: map[identity.ID]placement.Value{
				tableID: placement.Stack,
			},
		}
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{
			Placements: map[identity.ID]placement.Value{
				tableID: placement.SharedHeap,
				otherID: placement.OwnedHeap,
			},
		}
	}

	got := WithSupplemental(primary, supplemental)(transfer.NodeContext{}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)
	if got.Placements[tableID] != placement.SharedHeap {
		t.Fatalf("placement[%v] = %s, want shared-heap", tableID, got.Placements[tableID])
	}
	if got.Placements[otherID] != placement.OwnedHeap {
		t.Fatalf("placement[%v] = %s, want owned-heap", otherID, got.Placements[otherID])
	}
}

func TestWithSupplementalMergesParamLengthFloorsWithoutAuthority(t *testing.T) {
	primary := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{
			ParamLengthFloors: []callpayload.CallParamLengthFloor{
				{Path: pathdom.NewPlaceholder(0), Floor: 2},
			},
		}
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{
			ParamLengthFloors: []callpayload.CallParamLengthFloor{
				{Path: pathdom.NewPlaceholder(1), Floor: 3},
			},
		}
	}

	got := WithSupplemental(primary, supplemental)(transfer.NodeContext{}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)
	if len(got.ParamLengthFloors) != 2 {
		t.Fatalf("ParamLengthFloors = %#v, want primary and supplemental floors", got.ParamLengthFloors)
	}
	if !got.ParamLengthFloors[0].Path.Equal(pathdom.NewPlaceholder(0)) || got.ParamLengthFloors[0].Floor != 2 ||
		!got.ParamLengthFloors[1].Path.Equal(pathdom.NewPlaceholder(1)) || got.ParamLengthFloors[1].Floor != 3 {
		t.Fatalf("ParamLengthFloors = %#v, want ordered primary then supplemental floors", got.ParamLengthFloors)
	}
}

func TestWithSupplementalAuthorityBlocksSupplementalHeapTableObjects(t *testing.T) {
	reg := standard.Registry()
	primaryID := identity.ID{Kind: "table", Site: "compose", Index: 2}
	supplementalID := identity.ID{Kind: "table", Site: "compose", Index: 3}
	primary := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{
			PostReturnAuthority: true,
			HeapTableObjects: map[identity.ID]heapidentity.TableObject{
				primaryID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Absent(reg)}),
			},
		}
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{
			HeapTableObjects: map[identity.ID]heapidentity.TableObject{
				supplementalID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: product.Top()}),
			},
		}
	}

	got := WithSupplemental(primary, supplemental)(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)
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

func TestWithSupplementalAuthorityKeepsHeapFactsForAuthoritativeResultIdentity(t *testing.T) {
	reg := standard.Registry()
	resultID := identity.ID{Kind: "table", Site: "compose-result", Index: 1}
	resultValue := product.Set(reg, product.Top(), identity.Key, identity.Singleton(resultID))
	memberKey := keyspace.New().FromPath(pathdom.NewPath(symbol.ID(1), "root").Field("run"))
	primary := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{
			PostReturnAuthority: true,
			Results:             []callpayload.CallResult{{Index: 0, Value: resultValue}},
		}
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{
			HeapTableObjects: map[identity.ID]heapidentity.TableObject{
				resultID: heapidentity.NewTableObject(heapidentity.TableObjectConfig{
					Root: resultValue,
					StaticMembers: map[keyspace.Key]product.Value{
						memberKey: typevalue.FromType(reg, typ.String),
					},
				}),
			},
		}
	}

	got := WithSupplemental(primary, supplemental)(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)
	object, ok := got.HeapTableObjects[resultID]
	if !ok {
		t.Fatalf("HeapTableObjects = %#v, want supplemental facts for authoritative result identity", got.HeapTableObjects)
	}
	if value, ok := object.StaticMember(memberKey); !ok {
		t.Fatalf("static member = %v/%v, want propagated member for authoritative result object", value, ok)
	}
}

func TestWithSupplementalAuthorityBlocksSupplementalPlacementFacts(t *testing.T) {
	primaryID := identity.ID{Kind: "table", Site: "compose-placement", Index: 4}
	supplementalID := identity.ID{Kind: "table", Site: "compose-placement", Index: 5}
	primary := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{
			PostReturnAuthority: true,
			Placements: map[identity.ID]placement.Value{
				primaryID: placement.Stack,
			},
		}
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{
			Placements: map[identity.ID]placement.Value{
				primaryID:      placement.SharedHeap,
				supplementalID: placement.OwnedHeap,
			},
		}
	}

	got := WithSupplemental(primary, supplemental)(transfer.NodeContext{}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)
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

func TestWithSupplementalAuthorityBlocksSupplementalParamLengthFloors(t *testing.T) {
	primary := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{
			PostReturnAuthority: true,
			ParamLengthFloors: []callpayload.CallParamLengthFloor{
				{Path: pathdom.NewPlaceholder(0), Floor: 2},
			},
		}
	}
	supplemental := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{
			ParamLengthFloors: []callpayload.CallParamLengthFloor{
				{Path: pathdom.NewPlaceholder(1), Floor: 3},
			},
		}
	}

	got := WithSupplemental(primary, supplemental)(transfer.NodeContext{}, factflow.NewCallSite(factflow.CallSiteConfig{}).View(), state.State{}, nil)
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
