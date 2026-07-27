package factapply

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPreparedChannelSelectRetainsLexicalDuplicatesDefaultAndOneEvaluator(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(71)
	casePath := pathdom.NewPath(symbol.ID(71), "channel")
	resultPath := pathdom.NewPath(symbol.ID(72), "result")
	payload := typevalue.LiteralString(reg, "message")
	facts := factflow.NewFacts(factflow.FactsInput{ChannelSelects: map[cfg.Point]factflow.ChannelSelectSet{
		point: factflow.NewChannelSelectSet(
			factflow.NewChannelSelect(factflow.ChannelSelectConfig{SelectID: "prepared", Kind: factflow.ChannelSelectSelect, ResultPath: resultPath, HasResultPath: true, HasDefault: true}),
			factflow.NewChannelSelect(factflow.ChannelSelectConfig{SelectID: "prepared", Kind: factflow.ChannelSelectReceive, CasePath: casePath, HasCasePath: true, Index: 1}),
			factflow.NewChannelSelect(factflow.ChannelSelectConfig{SelectID: "prepared", Kind: factflow.ChannelSelectReceive, CasePath: casePath, HasCasePath: true, PayloadValue: payload, HasPayloadValue: true, Index: 2}),
		),
	}})
	transaction := PlanChannelSelectTransaction(facts, point)
	prepared, err := PrepareChannelSelectTransaction(reg, transaction,
		func(path pathdom.Path) (pathaddr.StateKey, bool) {
			return pathaddr.StateKeyFromPathKey(path.Key())
		},
		func(_ cfg.Point, index int) (string, bool) { return "result", index >= 0 },
	)
	if err != nil || !prepared.Complete() {
		t.Fatalf("prepare = %#v, %v", prepared, err)
	}
	reads := 0
	evaluated, err := EvaluatePreparedChannelSelect(context.Background(), reg, typevalue.NewCache(), prepared,
		func(path PreparedChannelSelectPath) (value product.Value, ok bool) {
			reads++
			if !path.SourcePath().Equal(casePath) || !path.Bound() {
				t.Fatal("prepared reader lost exact duplicate case path")
			}
			return typevalue.FromType(reg, typ.Instantiate(ambient.ChannelGeneric(), typ.String)), true
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := evaluated.Facts(); len(got) != 3 || got[0].Kind != channelselectfact.FactSelect || got[1].Index != 1 || got[2].Index != 2 {
		t.Fatalf("ordered facts = %#v", got)
	}
	if writes := evaluated.ResultWrites(); len(writes) != 1 || writes[0].Target != "result" {
		t.Fatalf("result writes = %#v", writes)
	}
	if reads != 1 {
		t.Fatalf("path reads = %d, want one non-payload duplicate", reads)
	}
}
