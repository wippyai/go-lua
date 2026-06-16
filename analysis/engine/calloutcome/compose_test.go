package calloutcome

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestWithSupplementalKeepsPrimarySlotsFillsMissingSlotsAndMergesSideFactsWithoutAuthority(t *testing.T) {
	reg := standard.Registry()
	primaryValue := product.Absent(reg)
	supplementalValue := product.Top()
	primary := func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) factapply.CallOutcome {
		return factapply.CallOutcome{
			Results: []factapply.CallResult{{Index: 0, Value: primaryValue}},
			NormalReturnFacts: callboundary.NormalReturnFacts{
				PathRefinements: []callboundary.PathValueFact{
					{Path: pathdom.NewPlaceholder(0), Value: primaryValue},
				},
			},
			ParamConditions: []factapply.CallParamCondition{
				{ParamIndex: 0, Value: true},
			},
		}
	}
	supplemental := func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) factapply.CallOutcome {
		return factapply.CallOutcome{
			Results: []factapply.CallResult{{Index: 0, Value: product.Top()}, {Index: 1, Value: supplementalValue}},
			NormalReturnFacts: callboundary.NormalReturnFacts{
				PathRefinements: []callboundary.PathValueFact{
					{Path: pathdom.NewPlaceholder(1), Value: supplementalValue},
				},
			},
			ParamConditions: []factapply.CallParamCondition{
				{ParamIndex: 1, Value: false},
			},
			ReturnPresenceRelations: []factapply.CallReturnPresenceRelation{
				{TriggerIndex: 1, TriggerPresence: presence.Present(), TargetIndex: 0, TargetPresence: presence.Absent()},
			},
		}
	}

	got := WithSupplemental(primary, supplemental)(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}), state.State{}, nil)

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
	if len(got.ReturnPresenceRelations) != 1 ||
		got.ReturnPresenceRelations[0].TriggerIndex != 1 ||
		!presence.Equal(got.ReturnPresenceRelations[0].TriggerPresence, presence.Present()) ||
		got.ReturnPresenceRelations[0].TargetIndex != 0 ||
		!presence.Equal(got.ReturnPresenceRelations[0].TargetPresence, presence.Absent()) {
		t.Fatalf("return presence relations = %#v, want supplemental relation", got.ReturnPresenceRelations)
	}
}

func TestWithSupplementalAuthorityBlocksSupplementalPostReturnFacts(t *testing.T) {
	reg := standard.Registry()
	primaryValue := product.Absent(reg)
	supplementalValue := product.Top()
	primary := func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) factapply.CallOutcome {
		return factapply.CallOutcome{
			PostReturnAuthority: true,
			Results:             []factapply.CallResult{{Index: 0, Value: primaryValue}},
			NormalReturnFacts: callboundary.NormalReturnFacts{
				PathRefinements: []callboundary.PathValueFact{
					{Path: pathdom.NewPlaceholder(0), Value: primaryValue},
				},
			},
			ParamObligations: []factapply.CallParamObligation{
				{ParamIndex: 0, Value: primaryValue},
			},
			ParamConditions: []factapply.CallParamCondition{
				{ParamIndex: 0, Value: true},
			},
		}
	}
	supplemental := func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) factapply.CallOutcome {
		return factapply.CallOutcome{
			PostReturnAuthority: true,
			Results:             []factapply.CallResult{{Index: 1, Value: supplementalValue}},
			NormalReturnFacts: callboundary.NormalReturnFacts{
				PathRefinements: []callboundary.PathValueFact{
					{Path: pathdom.NewPlaceholder(1), Value: supplementalValue},
				},
			},
			ParamObligations: []factapply.CallParamObligation{
				{ParamIndex: 1, Value: supplementalValue},
			},
			ParamConditions: []factapply.CallParamCondition{
				{ParamIndex: 1, Value: false},
			},
			ReturnPresenceRelations: []factapply.CallReturnPresenceRelation{
				{TriggerIndex: 1, TriggerPresence: presence.Present(), TargetIndex: 0, TargetPresence: presence.Absent()},
			},
		}
	}

	got := WithSupplemental(primary, supplemental)(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}), state.State{}, nil)

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
}

func TestWithSupplementalPreservesPrimaryAuthorityWhenSupplementalIsWeak(t *testing.T) {
	reg := standard.Registry()
	primaryValue := typeValue(reg, typ.String)
	supplementalValue := product.Top()
	primary := func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) factapply.CallOutcome {
		return factapply.CallOutcome{
			PostReturnAuthority: true,
			Results:             []factapply.CallResult{{Index: 0, Value: primaryValue}},
			ParamObligations: []factapply.CallParamObligation{
				{ParamIndex: 0, Value: primaryValue},
			},
		}
	}
	supplemental := func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) factapply.CallOutcome {
		return factapply.CallOutcome{
			Results: []factapply.CallResult{{Index: 0, Value: supplementalValue}},
			NormalReturnFacts: callboundary.NormalReturnFacts{
				PathRefinements: []callboundary.PathValueFact{
					{Path: pathdom.NewPlaceholder(0), Value: supplementalValue},
				},
			},
			ParamObligations: []factapply.CallParamObligation{
				{ParamIndex: 1, Value: supplementalValue},
			},
		}
	}

	got := WithSupplemental(primary, supplemental)(transfer.NodeContext{Registry: reg}, factflow.NewCallSite(factflow.CallSiteConfig{}), state.State{}, nil)

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
	primary := func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) factapply.CallOutcome {
		return factapply.CallOutcome{Results: []factapply.CallResult{{Index: 0, Value: primaryValue}}}
	}
	supplemental := func(transfer.NodeContext, factflow.CallSite, state.State, func(cfg.Point) state.State) factapply.CallOutcome {
		return factapply.CallOutcome{Results: []factapply.CallResult{{Index: 0, Value: supplementalValue}}}
	}

	got := WithSupplemental(primary, supplemental)(
		transfer.NodeContext{Registry: reg},
		factflow.NewCallSite(factflow.CallSiteConfig{}),
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

func typeValue(reg *axis.Registry, t typ.Type) product.Value {
	return typevalue.WithWitness(reg, typevalue.FromType(reg, t), t)
}
