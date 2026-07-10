package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
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

func TestLazyCallResultReaderDefersInitializationUntilRead(t *testing.T) {
	reader := &lazyCallResultReader{
		ctx: transfer.NodeContext{Point: cfg.Point(10)},
	}

	read := reader.ReadLazy()
	if reader.initialized {
		t.Fatal("ReadLazy initialized call-result materialization before a read")
	}

	_ = read(cfg.Point(11))
	if !reader.initialized {
		t.Fatal("lazy read did not initialize call-result materialization when read")
	}
}

func TestSamePointReadUsesCurrentStateWithoutInitializingLazyMaterialization(t *testing.T) {
	point := cfg.Point(10)
	reader := &lazyCallResultReader{
		ctx: transfer.NodeContext{Point: point},
	}

	read := readWithCurrentPointState(point, reader.ReadLazy(), state.State{})
	_ = read(point)
	if reader.initialized {
		t.Fatal("same-point read initialized materialization instead of using current state")
	}

	_ = read(point + 1)
	if !reader.initialized {
		t.Fatal("non-current read did not initialize call-result materialization")
	}
}

func TestApplyCallProducerReturnSlotsClearsAndWritesInOnePhase(t *testing.T) {
	reg := standard.Registry()
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

	got := applyCallProducerReturnSlots(
		transfer.NodeContext{Registry: reg},
		site,
		in,
		callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: written}}},
		true,
	)

	assertValue(t, reg, got, key.ReturnSlot(0), written)
	assertValue(t, reg, got, key.ReturnSlot(1), product.Bottom(reg))
	assertValue(t, reg, got, key.ReturnSlot(2), preserve)
}

func TestApplyCallMaterializationConstrainsFixedResultWithoutProducer(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(10)
	written := typevalue.FromType(reg, typ.Number)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{}),
		},
		CallResultValues: map[cfg.Point]factflow.CallResultValueSet{
			point: factflow.NewCallResultValueSet(factflow.NewCallResultValue(0, written)),
		},
	})

	got := materializeCallOutcome(
		transfer.NodeContext{Registry: reg, Point: point},
		facts,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		func(cfg.Point) state.State { return state.State{} },
		state.State{},
		state.State{},
	)

	assertValue(t, reg, got, key.ReturnSlot(0), written)
}

func TestApplyCallMaterializationFixedResultOverridesBottomOutcome(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(10)
	target := symbol.ID(77)
	written := typevalue.FromType(reg, typ.String)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			point: factflow.NewCallSite(factflow.CallSiteConfig{
				ResultTargets: []factflow.CallResultTarget{
					factflow.NewCallResultTarget(factflow.CallResultTargetLocalAssignment, 0, 0, target, path.NewPath(target, "value")),
				},
			}),
		},
		CallResultValues: map[cfg.Point]factflow.CallResultValueSet{
			point: factflow.NewCallResultValueSet(factflow.NewCallResultValue(0, written)),
		},
	})

	got := materializeCallOutcome(
		transfer.NodeContext{Registry: reg, Point: point},
		facts,
		nil,
		func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
			return callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: product.Bottom(reg)}}}
		},
		nil,
		nil,
		nil,
		nil,
		func(cfg.Point) state.State { return state.State{} },
		state.State{},
		state.State{},
	)

	assertValue(t, reg, got, key.ReturnSlot(0), written)
}

func TestConstrainReturnSlotDoesNotWeakenProvenSlotWithExplicitAny(t *testing.T) {
	reg := standard.Registry()
	optionalString := typeexpr.Optional(typ.String)
	proven := typevalue.FromType(reg, optionalString)
	untrusted := typevalue.WithWitness(reg,
		product.Set(reg, proven, evidence.Key, evidence.ExplicitTop()),
		optionalString,
	)
	edit := state.State{}.
		WriteReturnSlot(reg, 0, proven).
		EditValues(reg)

	constrainReturnSlotEdit(
		transfer.NodeContext{Registry: reg},
		&edit,
		factflow.NewCallResultValue(0, untrusted),
	)
	got := edit.Done()

	assertValue(t, reg, got, key.ReturnSlot(0), proven)
}

func TestConstrainReturnSlotDoesNotWeakenProvenSlotWithUnknown(t *testing.T) {
	reg := standard.Registry()
	proven := typevalue.FromType(reg, typ.String)
	unknown := typevalue.FromType(reg, typ.Unknown)
	edit := state.State{}.
		WriteReturnSlot(reg, 0, proven).
		EditValues(reg)

	constrainReturnSlotEdit(
		transfer.NodeContext{Registry: reg},
		&edit,
		factflow.NewCallResultValue(0, unknown),
	)
	got := edit.Done()

	assertValue(t, reg, got, key.ReturnSlot(0), proven)
}

func TestConstrainReturnSlotDoesNotWeakenProvenRecordSlotWithUnknown(t *testing.T) {
	reg := standard.Registry()
	record := typetable.NewRecord().
		Field("id", typ.String).
		Field("count", typ.Number).
		Build()
	proven := typevalue.FromType(reg, record)
	unknown := typevalue.FromType(reg, typ.Unknown)
	edit := state.State{}.
		WriteReturnSlot(reg, 0, proven).
		EditValues(reg)

	constrainReturnSlotEdit(
		transfer.NodeContext{Registry: reg},
		&edit,
		factflow.NewCallResultValue(0, unknown),
	)
	got := edit.Done()

	assertValue(t, reg, got, key.ReturnSlot(0), proven)
}

func TestConstrainReturnSlotUsesFixedTypedFactOverUnknownOutcome(t *testing.T) {
	reg := standard.Registry()
	record := typetable.NewRecord().
		Field("code", typ.String).
		Field("message", typ.String).
		Build()
	unknown := typevalue.FromType(reg, typ.Unknown)
	fixed := typevalue.FromType(reg, typeexpr.Optional(record))
	edit := state.State{}.
		WriteReturnSlot(reg, 0, unknown).
		EditValues(reg)

	constrainReturnSlotEdit(
		transfer.NodeContext{Registry: reg},
		&edit,
		factflow.NewCallResultValue(0, fixed),
	)
	got := edit.Done()

	assertValue(t, reg, got, key.ReturnSlot(0), fixed)
}
