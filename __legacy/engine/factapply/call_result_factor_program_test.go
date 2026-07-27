package factapply

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestCallResultMaterializeFactorProgramUsesCarrierNeutralRoots(t *testing.T) {
	reg := concreteRootTransactionRegistry(t, "test.call-result-factor")
	point := cfg.Point(901)
	ready := typevalue.LiteralString(reg, "ready")
	transaction := PlanCallResultTransaction(factflow.NewFacts(factflow.FactsInput{
		CallResultValues: map[cfg.Point]factflow.CallResultValueSet{
			point: factflow.NewCallResultValueSet(factflow.NewCallResultValue(3, ready)),
		},
	}), point)
	type formalRoot uint32
	program, err := PrepareCallResultMaterializeFactorProgram(reg, transaction, func(_, result uint32) (formalRoot, bool) {
		return formalRoot(result + 100), true
	})
	if err != nil || program.Len() != 1 {
		t.Fatalf("prepare factor-native N0 = %d, %v", program.Len(), err)
	}
	input := state.ValueFactor[formalRoot]{Values: map[formalRoot]product.Value{999: product.Top()}}
	got, err := program.Apply(context.Background(), nil, input)
	if err != nil || !product.Equal(reg, got.Values[103], ready) || !product.Equal(reg, got.Values[999], product.Top()) {
		t.Fatalf("formal-root N0 = %#v, %v", got, err)
	}
	top := state.ValueFactor[formalRoot]{Top: true}
	gotTop, err := program.Apply(context.Background(), nil, top)
	if err != nil || !gotTop.Top || len(gotTop.Values) != 0 {
		t.Fatalf("Values Top is not an N0 fixed point: %#v, %v", gotTop, err)
	}
}

func TestCallResultMaterializeFactorProgramUsesRegisteredReducedProduct(t *testing.T) {
	reg := axis.NewRegistry()
	reg = reg.Freeze()
	point := cfg.Point(904)
	fixed := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	transaction := PlanCallResultTransaction(factflow.NewFacts(factflow.FactsInput{
		CallResultValues: map[cfg.Point]factflow.CallResultValueSet{
			point: factflow.NewCallResultValueSet(factflow.NewCallResultValue(0, fixed)),
		},
	}), point)
	program, err := PrepareCallResultMaterializeFactorProgram(reg, transaction, func(_, result uint32) (uint32, bool) {
		return result + 1, true
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := program.Apply(context.Background(), nil, state.ValueFactor[uint32]{})
	if err != nil || !product.Equal(reg, got.Values[1], fixed) {
		t.Fatalf("reduced-product N0 = %#v, %v", got, err)
	}
}

func TestCallResultMaterializeFactorCancellationIsAtomic(t *testing.T) {
	reg := concreteRootTransactionRegistry(t, "test.call-result-factor-cancel")
	point := cfg.Point(902)
	transaction := PlanCallResultTransaction(factflow.NewFacts(factflow.FactsInput{
		CallResultValues: map[cfg.Point]factflow.CallResultValueSet{
			point: factflow.NewCallResultValueSet(factflow.NewCallResultValue(0, typevalue.LiteralString(reg, "ready"))),
		},
	}), point)
	program, err := PrepareCallResultMaterializeFactorProgram(reg, transaction, func(point, result uint32) (key.Value, bool) {
		return key.CallResult(point, result), true
	})
	if err != nil {
		t.Fatal(err)
	}
	input := state.ValueLaneFactor{Values: map[key.Value]product.Value{key.SymbolValue(91): product.Top()}}
	_, session := cancellation.Attach(context.Background())
	session.Token().Cancel(context.Canceled)
	got, err := program.Apply(context.Background(), session.Token(), input)
	if err == nil || !state.ValueFactorLattice[key.Value](reg).Equal(got, input) {
		t.Fatalf("canceled N0 published a prefix: %#v, %v", got, err)
	}
}
