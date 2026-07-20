package factapply

import (
	"context"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
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
	materialized := ApplyConcreteChannelSelectTransaction(ConcreteChannelSelectRequest{
		Context: transfer.NodeContext{Registry: reg, Point: point}, TypeValues: typevalue.NewCache(),
		Transaction: transaction, Output: state.Reachable(state.State{}),
	})
	if got := materialized.Output.ReadValue(reg, key.CallResult(uint32(point), 0)); product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatal("channel-select transaction did not write its point-owned scalar result")
	}
	if got := materialized.Output.ReadValue(reg, key.ReturnSlot(0)); !product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatal("channel-select transaction wrote the function return tuple before N5")
	}
}

func TestChannelSelectTransactionUsesConcreteAndStablePathAuthority(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(37)
	resultID, caseID := symbol.ID(411), symbol.ID(412)
	result := pathdom.NewPath(resultID, "select").Field("result")
	selectedCase := pathdom.NewPath(caseID, "select").Field("case")
	payload := typevalue.LiteralString(reg, "payload")
	event := factflow.NewChannelSelect(factflow.ChannelSelectConfig{
		SelectID: factflow.ChannelSelectID("select-37"), Kind: factflow.ChannelSelectReceive,
		ResultPath: result, HasResultPath: true, CasePath: selectedCase, HasCasePath: true,
		PayloadValue: payload, HasPayloadValue: true, Index: 4,
	})
	facts := factflow.NewFacts(factflow.FactsInput{ChannelSelects: map[cfg.Point]factflow.ChannelSelectSet{
		point: factflow.NewChannelSelectSet(event),
	}})
	transaction := PlanChannelSelectTransaction(facts, point)
	builder := visibility.NewBuilder()
	builder.Define(point, resultID, "select")
	builder.Define(point, caseID, "select")
	resolver := visibility.NewResolver(builder.Build())
	resultKey, resultOK := visibility.AddressAt(resolver, point, result).VisibleStateKey()
	caseKey, caseOK := visibility.AddressAt(resolver, point, selectedCase).VisibleStateKey()
	if !resultOK || !caseOK {
		t.Fatal("test paths did not resolve to stable State keys")
	}
	want := channelselectfact.Fact{
		Select: channelselectfact.ID("select-37"), Kind: channelselectfact.FactReceive,
		Result: resultKey, Case: caseKey, Payload: payload, HasPayload: true, Index: 4,
	}

	concrete := ApplyConcreteChannelSelectTransaction(ConcreteChannelSelectRequest{
		Context: transfer.NodeContext{Registry: reg, Point: point}, Resolver: resolver,
		Transaction: transaction, Output: state.State{},
	})
	if concrete.Canceled || !concrete.Output.HasChannelSelectFact(want) {
		t.Fatal("concrete transaction did not publish the exact N3 channel-select fact")
	}

	authority := NewPathSemanticAuthority(resolver, nil, nil)
	throughAuthority, err := authority.ApplyChannelSelect(context.Background(), reg, transaction, state.State{})
	if err != nil {
		t.Fatal(err)
	}
	if !throughAuthority.HasChannelSelectFact(want) {
		t.Fatal("stable path authority did not execute the shared channel-select transaction")
	}

	canceledContext, session := cancellation.Attach(context.Background())
	session.Token().Cancel(context.Canceled)
	transactionInput := state.Reachable(state.State{}).WriteValue(reg, key.SymbolValue(resultID), payload)
	rolledBack, err := authority.ApplyChannelSelect(canceledContext, reg, transaction, transactionInput)
	if err == nil {
		t.Fatal("pre-canceled N3 authority did not report cancellation")
	}
	if !state.Domain(reg).Equal(rolledBack, transactionInput) {
		t.Fatal("canceled N3 authority did not preserve its evolving transaction input")
	}
}
