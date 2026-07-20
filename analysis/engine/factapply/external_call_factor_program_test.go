package factapply

import (
	"context"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestExternalCallFactorPrefixMatchesConcreteSeventeenLaneTransaction(t *testing.T) {
	const userAxis userlattice.AxisID = "test.external-call-factor"
	reg := concreteRootTransactionRegistry(t, userAxis)
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(1701)
	ks := keyspace.New()
	input := state.Reachable(state.State{})
	for _, seed := range concreteRootTransactionLaneSeeds(t, reg, ks, userAxis) {
		input = domain.Lattice().Join(input, seed)
	}
	stale := typevalue.LiteralString(reg, "stale")
	input = input.
		WriteValue(reg, statekey.CallResult(uint32(point), 0), stale).
		WriteValue(reg, statekey.CallResult(uint32(point), 1), stale)
	site := externalCallFactorTestSite().View()
	ready := typevalue.LiteralString(reg, "ready")
	outcome := callpayload.CallOutcome{
		Results:          []callpayload.CallResult{{Index: 0, Value: ready}, {Index: 9, Value: product.Top()}},
		SuspensionKnown:  true,
		MaySuspend:       true,
		ParamObligations: []callpayload.CallParamObligation{{ParamIndex: 0, Value: ready}},
		PathObligations:  []callpayload.CallPathObligation{{Path: pathdom.NewPath(symbol.ID(91), "field"), Value: ready}},
	}

	// This is a one-time differential against the pre-factor concrete prefix.
	// Runtime execution below enters only the factor program.
	want, err := domain.ApplyCallBoundary(input)
	if err != nil {
		t.Fatal(err)
	}
	edit := want.EditValues(reg)
	edit.Write(statekey.CallResult(uint32(point), 0), ready)
	edit.Write(statekey.CallResult(uint32(point), 1), product.Bottom(reg))
	want = edit.Done()
	got, diagnostics, err := applyConcreteExternalCallFactorPrefix(
		context.Background(), nil, domain, point, site, input, outcome,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !domain.Lattice().Equal(got, want) {
		t.Fatal("factor-native external-call prefix differs from concrete transaction")
	}
	if !diagnostics.Equal(reg, callpayload.DiagnosticOutputFromCallOutcome(reg, outcome)) {
		t.Fatalf("factor diagnostics = %#v", diagnostics)
	}
	if gotResult := got.ReadValue(reg, statekey.CallResult(uint32(point), 0)); !product.Equal(reg, gotResult, ready) {
		t.Fatal("owned result 0 was not materialized")
	}
	if gotResult := got.ReadValue(reg, statekey.CallResult(uint32(point), 1)); !product.Equal(reg, gotResult, product.Bottom(reg)) {
		t.Fatal("omitted owned result 1 was not cleared")
	}
}

func TestExternalCallFactorPrefixUnresolvedResultsAndCancellationAreExact(t *testing.T) {
	const userAxis userlattice.AxisID = "test.external-call-factor-cancel"
	reg := concreteRootTransactionRegistry(t, userAxis)
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(1702)
	site := externalCallFactorTestSite().View()
	input := state.Reachable(state.State{}).WriteValue(reg, statekey.SymbolValue(77), typevalue.LiteralString(reg, "keep"))

	unresolved, _, err := applyConcreteExternalCallFactorPrefix(
		context.Background(), nil, domain, point, site, input, callpayload.CallOutcome{},
	)
	if err != nil {
		t.Fatal(err)
	}
	for index := uint32(0); index < 2; index++ {
		if got := unresolved.ReadValue(reg, statekey.CallResult(uint32(point), index)); !product.Equal(reg, got, product.Top()) {
			t.Fatalf("unresolved result %d = %#v, want Top", index, got)
		}
	}

	_, session := cancellation.Attach(context.Background())
	session.Token().Cancel(context.Canceled)
	canceled, _, err := applyConcreteExternalCallFactorPrefix(
		context.Background(), session.Token(), domain, point, site, input,
		callpayload.CallOutcome{Results: []callpayload.CallResult{{Index: 0, Value: product.Top()}}},
	)
	if err == nil || !domain.Lattice().Equal(canceled, input) {
		t.Fatal("canceled external-call factor transaction published a prefix")
	}
}

func externalCallFactorTestSite() factflow.CallSite {
	return factflow.NewCallSite(factflow.CallSiteConfig{ResultTargets: []factflow.CallResultTarget{
		factflow.NewCallResultTarget(factflow.CallResultTargetExpression, 0, 0, 0, pathdom.Path{}),
		factflow.NewCallResultTarget(factflow.CallResultTargetExpression, 1, 1, 0, pathdom.Path{}),
	}})
}
