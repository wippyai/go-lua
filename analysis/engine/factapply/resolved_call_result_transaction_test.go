package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestResolvedCallResultTransactionIsExactN0Handoff(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(3)
	want := typevalue.String(reg)
	transaction, ok := NewResolvedCallResultTransaction(reg, point, 0, want)
	if !ok || !transaction.Valid(reg) || transaction.Point() != point || transaction.Len() != 1 ||
		!transaction.HasMaterializeSteps() || transaction.HasPostconditionSteps() || transaction.HasPublicationSteps() {
		t.Fatalf("resolved transaction = %#v/%v", transaction, ok)
	}
	slot := key.CallResult(uint32(point), 0)
	input := state.Reachable(state.State{}).WriteValue(reg, slot, product.Bottom(reg))
	result := ApplyConcreteCallResultTransaction(ConcreteCallResultRequest{
		Context: transfer.NodeContext{Registry: reg, Point: point}, Transaction: transaction,
		Phase: ConcreteCallResultPhaseMaterialize, Output: input,
	})
	if result.Canceled || !product.Equal(reg, result.Output.ReadValue(reg, slot), want) {
		t.Fatal("resolved N0 transaction did not materialize its exact result")
	}
	if invalid, accepted := NewResolvedCallResultTransaction(reg, point, -1, want); accepted || invalid.Len() != 0 {
		t.Fatal("negative result index produced transaction authority")
	}
}
