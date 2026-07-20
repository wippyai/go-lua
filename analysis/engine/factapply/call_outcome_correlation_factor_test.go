package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestCallOutcomeCorrelationFactorPublishesEveryCorrelationThroughOneCanonicalLane(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	keys := keyspace.New()
	result0 := mustCallOutcomeCorrelationKey(t, keys, pathdom.NewPath(symbol.ID(401), "result0"))
	result1 := mustCallOutcomeCorrelationKey(t, keys, pathdom.NewPath(symbol.ID(402), "result1"))
	argument := mustCallOutcomeCorrelationKey(t, keys, pathdom.NewPath(symbol.ID(403), "argument"))
	placeholder := pathdom.Path{Root: "$0"}
	bindings := []CallOutcomeCorrelationBinding{
		{Shape: callpayload.ReturnConditionPathShape(0, true, placeholder), Trigger: result0, Target: argument},
		{Shape: callpayload.ReturnConditionSlotShape(0, false, 1), Trigger: result0, Target: result1},
		{Shape: callpayload.ReturnPresenceShape(0, presence.Present(), 1, presence.Absent()), Trigger: result0, Target: result1},
	}
	program, err := PrepareCallOutcomeCorrelationFactorProgram(domain, keys, bindings)
	if err != nil {
		t.Fatal(err)
	}
	if len(program.CoordinateSlots()) != len(bindings) {
		t.Fatalf("correlation slots = %d, want %d", len(program.CoordinateSlots()), len(bindings))
	}
	factor, err := domain.LaneTop(program.Lane())
	if err != nil {
		t.Fatal(err)
	}
	pathType, slotType := typ.LiteralString("path-refined"), typ.LiteralString("slot-refined")
	pathValue := typevalue.WithWitness(reg, typevalue.FromType(reg, pathType), pathType)
	slotValue := typevalue.WithWitness(reg, typevalue.FromType(reg, slotType), slotType)
	factor, err = program.Apply(factor, callpayload.CallOutcome{
		ReturnConditionRefinements: []callpayload.CallReturnConditionRefinement{{
			ReturnIndex: 0, ReturnValue: true, Target: placeholder, Value: pathValue,
		}},
		ReturnConditionSlots: []callpayload.CallReturnConditionSlotRefinement{{
			ReturnIndex: 0, ReturnValue: false, TargetIndex: 1, Value: slotValue,
		}},
		ReturnPresenceRelations: []callpayload.CallReturnPresenceRelation{{
			TriggerIndex: 0, TriggerPresence: presence.Present(), TargetIndex: 1, TargetPresence: presence.Absent(),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := domain.ComposeSparse([]state.LaneFactor{factor})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []pathevidence.PathPresenceImplication{
		pathevidence.NewPathTruthinessValueRefinementImplication(result0, true, argument, pathValue),
		pathevidence.NewPathTruthinessValueRefinementImplication(result0, false, result1, slotValue),
		pathevidence.NewPathPresenceImplication(result0, presence.Present(), result1, presence.Absent()),
	} {
		if !got.HasPathPresenceImplication(want) {
			t.Fatalf("canonical correlation lane omitted %#v", want)
		}
	}
}

func TestCallOutcomeCorrelationFactorRejectsUndeclaredDynamicShape(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	keys := keyspace.New()
	trigger := mustCallOutcomeCorrelationKey(t, keys, pathdom.NewPath(symbol.ID(411), "trigger"))
	target := mustCallOutcomeCorrelationKey(t, keys, pathdom.NewPath(symbol.ID(412), "target"))
	program, err := PrepareCallOutcomeCorrelationFactorProgram(domain, keys, []CallOutcomeCorrelationBinding{{
		Shape: callpayload.ReturnConditionSlotShape(0, true, 1), Trigger: trigger, Target: target,
	}})
	if err != nil {
		t.Fatal(err)
	}
	factor, err := domain.LaneTop(program.Lane())
	if err != nil {
		t.Fatal(err)
	}
	valueType := typ.LiteralString("undeclared")
	_, err = program.Apply(factor, callpayload.CallOutcome{
		ReturnConditionSlots: []callpayload.CallReturnConditionSlotRefinement{{
			ReturnIndex: 0, ReturnValue: false, TargetIndex: 1,
			Value: typevalue.WithWitness(reg, typevalue.FromType(reg, valueType), valueType),
		}},
	})
	if err == nil {
		t.Fatal("dynamic correlation outside the sealed site shape was accepted")
	}
}

func TestCallOutcomeCorrelationBoundaryBindsPointOwnedResultsWithoutReturnPathSyntax(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	resolver := visibility.NewResolver(visibility.NewTable(nil))
	authority := NewPathSemanticAuthority(resolver, nil, nil)
	keys := resolver.KeySpace()
	point := cfg.Point(17)
	trigger := mustCallOutcomeCorrelationKey(t, keys, pathdom.Path{Root: "private-call-result-zero"})
	target := mustCallOutcomeCorrelationKey(t, keys, pathdom.Path{Root: "private-call-result-one"})
	shape := callpayload.ReturnPresenceShape(0, presence.Present(), 1, presence.Absent())
	program, err := PrepareCallOutcomeCorrelationFactorProgramAtBoundary(
		authority,
		domain,
		point,
		callboundary.NewPathBindings(nil, nil),
		state.BoundaryRoots{
			{Slot: statekey.CallResult(uint32(point), 0), Path: trigger},
			{Slot: statekey.CallResult(uint32(point), 1), Path: target},
		},
		[]callpayload.CallOutcomeCorrelationShape{shape},
	)
	if err != nil {
		t.Fatal(err)
	}
	factor, err := domain.LaneTop(program.Lane())
	if err != nil {
		t.Fatal(err)
	}
	factor, err = program.Apply(factor, callpayload.CallOutcome{ReturnPresenceRelations: []callpayload.CallReturnPresenceRelation{{
		TriggerIndex: 0, TriggerPresence: presence.Present(), TargetIndex: 1, TargetPresence: presence.Absent(),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := domain.ComposeSparse([]state.LaneFactor{factor})
	if err != nil {
		t.Fatal(err)
	}
	want := pathevidence.NewPathPresenceImplication(trigger, presence.Present(), target, presence.Absent())
	if !got.HasPathPresenceImplication(want) {
		t.Fatalf("point-owned call-result relation omitted %#v", want)
	}
}

func mustCallOutcomeCorrelationKey(t *testing.T, keys *keyspace.KeySpace, path pathdom.Path) keyspace.Key {
	t.Helper()
	key := keys.FromPath(path)
	if key == (keyspace.Key{}) {
		t.Fatalf("could not intern path %v", path)
	}
	return key
}
