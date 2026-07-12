package body

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestCallOutcomeContextPathValueProjectsResolvableReceiverMember(t *testing.T) {
	reg := standard.Registry()
	types := typevalue.NewCache()
	referencesType := typetable.NewMap(typ.String, typ.String)
	receiverType := typetable.NewRecord().Field("references", referencesType).Build()
	receiver := types.FromTypeWithWitness(reg, receiverType)
	want, ok := types.RuntimeIndex(reg, receiver, typevalue.LiteralString(reg, "references"))
	if !ok {
		t.Fatal("test setup receiver.references projection failed")
	}
	resolver := visibility.NewResolver(nil)
	static := &Static{
		registry: reg, typeValues: types, facts: factflow.NewFacts(factflow.FactsInput{}), visibility: resolver,
	}
	ctx := static.callOutcomeContext(types)
	if ctx.PathValue == nil {
		t.Fatal("CallOutcomeContext PathValue callback missing")
	}
	sym := symbol.ID(9)
	in := state.State{}.WriteValue(reg, key.SymbolValue(sym), receiver)
	got, ok := ctx.PathValue(transfer.NodeContext{Registry: reg, Point: cfg.Point(1)}, pathdom.NewPath(sym, "graph").Field("references"), in)
	if !ok || !product.Equal(reg, got, want) {
		t.Fatalf("graph.references = %#v/%v, want %#v", got, ok, want)
	}
}
