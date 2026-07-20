package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
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
	prepared := validateGraphPreparedFixture(t)
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
		inputProgram, err := effectlowering.SealSignatureOutcomeOperands(state.RegisteredProductDomain(prepared.registry), prepared.visibility.KeySpace())
		if err != nil {
			t.Fatal(err)
		}
		oracle := effectlowering.SignatureOutcomeProvider(effectlowering.SignatureOutcomeProviderConfig{
			Signatures:  acceptanceSignatureLookup{sig: op.Signature()},
			NameForSite: func(transfer.NodeContext, factflow.CallSiteView) (string, bool) { return "resolved", true },
			Facts:       prepared.facts, TypeValues: prepared.typeValues, KeySpace: prepared.visibility.KeySpace(), InputProgram: inputProgram,
		})
		ctx := transfer.NodeContext{Point: point, Registry: prepared.registry}
		outcome := testEvaluateCallOutcome(t, oracle, ctx, site, sealedBodyCallInput(t, oracle, ctx, site, state.State{}, callpayload.CallOutcomeValueOperands{}))
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

func TestValidateGraphOwnsEightDurableAllocationSites(t *testing.T) {
	prepared := validateGraphPreparedFixture(t)
	count := 0
	for rawPoint := 0; rawPoint < prepared.operationPlan.PointCount(); rawPoint++ {
		op, ok := prepared.operationPlan.SignatureAllocationOperation(cfg.Point(rawPoint))
		if !ok {
			continue
		}
		site := op.Site()
		if site.Template != "stdlib.table.create:return:0" || site.Ordinal != uint32(rawPoint) || site.Owner == 0 {
			t.Fatalf("allocation site at point %d = %#v, want owner + lexical CFG ordinal", rawPoint, site)
		}
		count++
	}
	if count != 8 {
		t.Fatalf("durable allocation sites = %d, want 8 table.create calls", count)
	}
}

func TestValidateGraphAllocationRelationsMatchCanonicalOutcomesWithoutSolves(t *testing.T) {
	prepared := validateGraphPreparedFixture(t)
	shape := transformer.Shape{
		Params:   uint32(len(prepared.operationPlan.BoundaryParams())),
		Captures: uint32(len(prepared.operationPlan.BoundaryCaptures())),
		Globals:  uint32(len(prepared.operationPlan.BoundaryGlobals())),
	}
	entries := transformer.NewPlanCompiler().EligibilityCensus(prepared.registry, prepared.cfg.Graph, prepared.operationPlan, shape)
	exactCalls := make(map[cfg.Point]bool)
	for _, entry := range entries {
		if entry.Family == "CallSites" && entry.Exact {
			exactCalls[entry.Point] = true
		}
	}
	matched := 0
	for rawPoint := 0; rawPoint < prepared.operationPlan.PointCount(); rawPoint++ {
		point := cfg.Point(rawPoint)
		allocation, ok := prepared.operationPlan.SignatureAllocationOperation(point)
		if !ok {
			continue
		}
		if !exactCalls[point] {
			t.Fatalf("allocation call point %d is not compiler-exact", point)
		}
		call, _ := prepared.operationPlan.SignatureCallOperation(point)
		site, _ := prepared.facts.CallSiteView(point)
		ks := keyspace.New()
		inputProgram, err := effectlowering.SealSignatureOutcomeOperands(state.RegisteredProductDomain(prepared.registry), ks)
		if err != nil {
			t.Fatal(err)
		}
		materialized, exact := effectlowering.MaterializeStaticAllocation(prepared.registry, prepared.typeValues, ks, point, allocation.Template(), nil)
		if !exact {
			t.Fatalf("allocation point %d failed canonical materialization", point)
		}
		oracleProvider := effectlowering.SignatureOutcomeProvider(effectlowering.SignatureOutcomeProviderConfig{
			Signatures:  acceptanceSignatureLookup{sig: call.Signature()},
			NameForSite: func(transfer.NodeContext, factflow.CallSiteView) (string, bool) { return "resolved", true },
			Facts:       prepared.facts, TypeValues: prepared.typeValues, KeySpace: ks, InputProgram: inputProgram,
		})
		ctx := transfer.NodeContext{Registry: prepared.registry, Point: point}
		oracle := testEvaluateCallOutcome(t, oracleProvider, ctx, site, sealedBodyCallInput(t, oracleProvider, ctx, site, state.State{}, callpayload.CallOutcomeValueOperands{}))
		if len(oracle.Results) != 1 || !product.Equal(prepared.registry, oracle.Results[0].Value, materialized.Result) ||
			len(oracle.HeapTableObjects) != len(materialized.Objects) || len(oracle.Placements) != len(materialized.Placements) {
			t.Fatalf("allocation point %d differs from canonical provider", point)
		}
		matched++
	}
	if matched != 8 {
		t.Fatalf("exact allocation relations = %d, want 8 table.create calls", matched)
	}
}
