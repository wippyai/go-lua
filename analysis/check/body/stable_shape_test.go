package body

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

// TestCallPrefixMutationKindConsultsTrackedFieldsLikeStaticMemberWriteSibling
// proves callPrefixMutationKind consults its fields parameter the way its
// sibling static-member-write branch in prefixKillingMutationKind does. A
// call outcome that declares mutation of field x, with a structural-witness
// proof that nothing else changed, must not invalidate a guard/shape fact
// that only tracks field y; the same outcome must still invalidate a
// guard/shape fact that tracks field x.
func TestCallPrefixMutationKindConsultsTrackedFieldsLikeStaticMemberWriteSibling(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(40)
	sym := symbol.ID(4001)
	receiver := pathdom.NewPath(sym, "t")
	id := testTableIdentity(40, 1)

	st := state.State{}.WriteValue(reg, statekey.SymbolValue(sym), identityvalue.Present(reg, id))
	builder := visibility.NewBuilder()
	builder.Define(point, sym, "t")

	shape, ok := factflow.NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("NewValueSourceShape returned false")
	}
	receiverSource, ok := factflow.NewPathValueSource(receiver.Key(), 0, 0, 0, shape)
	if !ok {
		t.Fatal("NewPathValueSource returned false")
	}
	callSite := factflow.NewCallSite(factflow.CallSiteConfig{
		Point:           point,
		HasPoint:        true,
		ArgumentSources: []factflow.ValueSource{receiverSource},
	})
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{point: callSite},
	})

	outcome := callpayload.CallOutcome{
		ParamPathInvalidations: []callpayload.CallParamPathInvalidation{
			{Path: pathdom.NewPlaceholder(0).Field("x"), PreserveStructuralWitness: true},
		},
	}

	result := &Result{
		registry:   reg,
		visibility: visibility.NewResolver(builder.Build()),
		flow:       transfer.Result{point: st},
		facts:      facts,
		published:  PublishedFacts{callOutcomes: map[cfg.Point]callpayload.CallOutcome{point: outcome}},
	}

	fieldsY := map[string]struct{}{"y": {}}
	if kind, ok := result.callPrefixMutationKind(point, id, receiver, fieldsY); !ok || kind != prefixMutationNone {
		t.Fatalf("callPrefixMutationKind(fields={y}) = %v/%v, want prefixMutationNone/true", kind, ok)
	}

	fieldsX := map[string]struct{}{"x": {}}
	if kind, ok := result.callPrefixMutationKind(point, id, receiver, fieldsX); !ok || kind != prefixMutationUnknown {
		t.Fatalf("callPrefixMutationKind(fields={x}) = %v/%v, want prefixMutationUnknown/true", kind, ok)
	}
}
