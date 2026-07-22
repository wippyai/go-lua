package factapply

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

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
	slot := key.CallResult(uint32(point), 0)
	program, err := PrepareCallResultMaterializeFactorProgram(reg, transaction, func(point, result uint32) (key.Value, bool) {
		return key.CallResult(point, result), true
	})
	if err != nil {
		t.Fatal(err)
	}
	values, err := program.Apply(context.Background(), nil, state.ValueFactor[key.Value]{Values: map[key.Value]product.Value{slot: current}})
	if err != nil {
		t.Fatal(err)
	}
	return state.State{}.WriteValue(reg, slot, values.Values[slot])
}
