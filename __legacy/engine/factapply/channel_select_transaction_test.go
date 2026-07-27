package factapply

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestPlanChannelSelectTransactionOwnsExactN3PublicationOrder(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(31)
	result := pathdom.NewPath(symbol.ID(401), "result").Field("value")
	selectedCase := pathdom.NewPath(symbol.ID(402), "case").Field("channel")
	payload := typevalue.LiteralString(reg, "message")
	facts := factflow.NewFacts(factflow.FactsInput{ChannelSelects: map[cfg.Point]factflow.ChannelSelectSet{
		point: factflow.NewChannelSelectSet(
			factflow.NewChannelSelect(factflow.ChannelSelectConfig{
				SelectID: factflow.ChannelSelectID("select-31"), Kind: factflow.ChannelSelectSelect,
				ResultPath: result, HasResultPath: true, HasDefault: true,
			}),
			factflow.NewChannelSelect(factflow.ChannelSelectConfig{
				SelectID: factflow.ChannelSelectID("select-31"), Kind: factflow.ChannelSelectReceive,
				ResultPath: result, HasResultPath: true, CasePath: selectedCase, HasCasePath: true,
				PayloadValue: payload, HasPayloadValue: true, Index: 2,
			}),
		),
	}})

	transaction := PlanChannelSelectTransaction(facts, point)
	if transaction.Point() != point || transaction.Len() != 2 || !transaction.HasPublicationSteps() || !transaction.Valid(reg) {
		t.Fatalf("transaction point/len/publication/valid = %d/%d/%t/%t", transaction.Point(), transaction.Len(), transaction.HasPublicationSteps(), transaction.Valid(reg))
	}
	first, ok := transaction.Step(0)
	if !ok || first.Event().Kind() != factflow.ChannelSelectSelect {
		t.Fatal("first transaction member is not the select publication")
	}
	second, ok := transaction.Step(1)
	if !ok || second.Event().Kind() != factflow.ChannelSelectReceive || second.Event().Index() != 2 {
		t.Fatal("second transaction member is not the receive publication")
	}
	if _, ok := transaction.Step(2); ok {
		t.Fatal("transaction exposed an out-of-range step")
	}

	mutated, _ := second.Event().CasePath()
	mutated.Segments[0].Name = "mutated"
	again, _ := transaction.Step(1)
	gotPath, _ := again.Event().CasePath()
	if !gotPath.Equal(selectedCase) {
		t.Fatal("channel-select transaction exposed mutable path storage")
	}
}
