package factapply

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestApplyCallProducerResultsClearsAndWritesInOnePhase(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(17)
	result := symbol.ID(22)
	stale0 := presentValue(reg)
	stale1 := absentValue(reg)
	preserve := product.Top()
	written := nilSourceValue(reg)
	in := state.State{}.
		WriteReturnSlot(reg, 0, stale0).
		WriteReturnSlot(reg, 1, stale1).
		WriteReturnSlot(reg, 2, preserve)
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, result, path.NewPath(result, "value")),
			factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 1, 1, result, path.NewPath(result, "err")),
		},
	}).View()

	got := mustApplyExternalCallFactorPrefix(t, reg, point, site, in,
		callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: written}}})

	assertValue(t, reg, got, key.CallResult(uint32(point), 0), written)
	assertValue(t, reg, got, key.CallResult(uint32(point), 1), product.Bottom(reg))
	assertValue(t, reg, got, key.ReturnSlot(2), preserve)
}

func TestAdjacentCallProducersKeepPointAndSlotIdentity(t *testing.T) {
	reg := standard.Registry()
	firstPoint, secondPoint := cfg.Point(41), cfg.Point(42)
	firstHead := typevalue.LiteralString(reg, "first-head")
	firstTail := typevalue.LiteralString(reg, "first-tail")
	secondHead := typevalue.LiteralString(reg, "second-head")
	secondTail := typevalue.LiteralString(reg, "second-tail")
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetExpression, 0, 0, 0, path.Path{}),
			factflow.NewCallResultTarget(factflow.CallResultTargetExpression, 1, 1, 0, path.Path{}),
		},
	}).View()
	out := mustApplyExternalCallFactorPrefix(t, reg, firstPoint, site, state.Reachable(state.State{}),
		callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: firstHead}, {Index: 1, Value: firstTail}}})
	out = mustApplyExternalCallFactorPrefix(t, reg, secondPoint, site, out,
		callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: secondHead}, {Index: 1, Value: secondTail}}})
	for _, expected := range []struct {
		point cfg.Point
		slot  uint32
		value product.Value
	}{
		{firstPoint, 0, firstHead}, {firstPoint, 1, firstTail},
		{secondPoint, 0, secondHead}, {secondPoint, 1, secondTail},
	} {
		assertValue(t, reg, out, key.CallResult(uint32(expected.point), expected.slot), expected.value)
	}
	if got := out.ReadValue(reg, key.ReturnSlot(0)); !product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatal("call producer wrote the function return tuple before N5")
	}
}

func mustApplyExternalCallFactorPrefix(
	t *testing.T,
	reg *axis.Registry,
	point cfg.Point,
	site factflow.CallSiteView,
	input state.State,
	outcome callpayload.CallOutcome,
) state.State {
	t.Helper()
	got, _, err := applyConcreteExternalCallFactorPrefix(
		context.Background(), nil, state.RegisteredProductDomain(reg), point, site, input, outcome,
	)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestConstrainCallResultDoesNotWeakenProvenCellWithExplicitAny(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(19)
	optionalString := typeexpr.Optional(typ.String)
	proven := typevalue.FromType(reg, optionalString)
	untrusted := typevalue.WithWitness(reg,
		product.Set(reg, proven, evidence.Key, evidence.ExplicitTop()),
		optionalString,
	)
	got := applyCallResultValueForTest(t, reg, point, proven, untrusted)

	assertValue(t, reg, got, key.CallResult(uint32(point), 0), proven)
}

func TestConstrainCallResultDoesNotWeakenProvenCellWithUnknown(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(23)
	proven := typevalue.FromType(reg, typ.String)
	unknown := typevalue.FromType(reg, typ.Unknown)
	got := applyCallResultValueForTest(t, reg, point, proven, unknown)

	assertValue(t, reg, got, key.CallResult(uint32(point), 0), proven)
}

func TestConstrainCallResultDoesNotWeakenProvenRecordWithUnknown(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(29)
	record := typetable.NewRecord().
		Field("id", typ.String).
		Field("count", typ.Number).
		Build()
	proven := typevalue.FromType(reg, record)
	unknown := typevalue.FromType(reg, typ.Unknown)
	got := applyCallResultValueForTest(t, reg, point, proven, unknown)

	assertValue(t, reg, got, key.CallResult(uint32(point), 0), proven)
}

func TestConstrainCallResultUsesFixedTypedFactOverUnknownOutcome(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(31)
	record := typetable.NewRecord().
		Field("code", typ.String).
		Field("message", typ.String).
		Build()
	unknown := typevalue.FromType(reg, typ.Unknown)
	fixed := typevalue.FromType(reg, typeexpr.Optional(record))
	got := applyCallResultValueForTest(t, reg, point, unknown, fixed)

	assertValue(t, reg, got, key.CallResult(uint32(point), 0), fixed)
}

func applyCallResultValueForTest(t *testing.T, reg *axis.Registry, point cfg.Point, current, fixed product.Value) state.State {
	t.Helper()
	transaction := PlanCallResultTransaction(factflow.NewFacts(factflow.FactsInput{
		CallResultValues: map[cfg.Point]factflow.CallResultValueSet{
			point: factflow.NewCallResultValueSet(factflow.NewCallResultValue(0, fixed)),
		},
	}), point)
	input := state.State{}.WriteValue(reg, key.CallResult(uint32(point), 0), current)
	result := ApplyConcreteCallResultTransaction(ConcreteCallResultRequest{
		Context:     transfer.NodeContext{Context: context.Background(), Registry: reg, Point: point},
		Transaction: transaction, Phase: ConcreteCallResultPhaseMaterialize, Output: input,
	})
	if result.Canceled {
		t.Fatal("canonical call-result materialization was canceled")
	}
	return result.Output
}
