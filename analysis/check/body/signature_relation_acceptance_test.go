package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

type acceptanceSignatureLookup struct{ sig signature.Function }

func (l acceptanceSignatureLookup) Lookup(string) (signature.Function, bool) {
	return l.sig.Clone(), true
}

func TestValidateGraphPureSignatureRelationsMatchCanonicalOutcomes(t *testing.T) {
	prepared, _ := validateGraphSemanticProgramFixture(t)
	matched := 0
	for rawPoint := 0; rawPoint < prepared.operationPlan.PointCount(); rawPoint++ {
		point := cfg.Point(rawPoint)
		op, ok := prepared.operationPlan.SignatureCallOperation(point)
		if !ok {
			continue
		}
		returns, exact := effectlowering.StaticScalarSignatureReturns(prepared.registry, prepared.typeValues, op.Signature())
		if !exact {
			continue
		}
		site, ok := prepared.facts.CallSiteView(point)
		if !ok {
			t.Fatalf("signature descriptor at point %d lost call-site evidence", point)
		}
		oracle := effectlowering.SignatureOutcomeProvider(effectlowering.SignatureOutcomeProviderConfig{
			Signatures:  acceptanceSignatureLookup{sig: op.Signature()},
			NameForSite: func(transfer.NodeContext, factflow.CallSiteView) (string, bool) { return "resolved", true },
			Facts:       prepared.facts, TypeValues: prepared.typeValues,
		})
		outcome := oracle(transfer.NodeContext{Point: point, Registry: prepared.registry}, site, state.State{}, func(cfg.Point) state.State { return state.State{} })
		if !outcome.PostReturnAuthority || len(outcome.Results) != len(returns) {
			t.Fatalf("point %d canonical authority/results = %v/%d, want true/%d", point, outcome.PostReturnAuthority, len(outcome.Results), len(returns))
		}
		for i, result := range outcome.Results {
			if result.Index != i || !product.Equal(prepared.registry, result.Value, returns[i]) {
				t.Fatalf("point %d result %d differs from canonical signature outcome", point, i)
			}
		}
		matched++
	}
	if matched != 6 {
		t.Fatalf("pure static signature relations = %d, want 6 string.format calls", matched)
	}
}
