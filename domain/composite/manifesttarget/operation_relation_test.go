package manifesttarget_test

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/domain/composite/manifesttarget"
	"github.com/wippyai/go-lua/manifest"
	"github.com/wippyai/go-lua/stdlib"
)

func TestStandardManifestPreservesGsubTableSubedgeRelation(t *testing.T) {
	catalogue, err := manifest.Seal(stdlib.Providers()...)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := manifesttarget.SealCatalogue(catalogue)
	if err != nil {
		t.Fatal(err)
	}
	op, ok := contract.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingModule, Owner: []string{"string"}, Member: []string{"gsub"}})
	if !ok {
		t.Fatal("string.gsub missing")
	}
	operand, selector, access, resultOutcome, result, ok := contract.Operations.OperationSubedgeRelation(op)
	if !ok || operand != 2 || selector != 1 || resultOutcome != 0 || result != 0 {
		t.Fatalf("gsub table relation = %d/%d/%d/%d/%d/%v", operand, selector, access, resultOutcome, result, ok)
	}
	if family, ok := contract.Operations.SubedgeFamily(access); !ok || family != vocabulary.SubedgeFamilyIndexGet {
		t.Fatal("gsub table relation lost its IndexGet subedge")
	}
	segment, index, source, input, ok := contract.Operations.SubedgeArgumentOriginAt(access, 0)
	if !ok || segment != vocabulary.ArgumentFixed || index != 0 || source != vocabulary.ArgumentSourceInput || input != (vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 2}) {
		t.Fatal("gsub replacement-table operand changed")
	}
	segment, index, source, input, ok = contract.Operations.SubedgeArgumentOriginAt(access, 1)
	if !ok || segment != vocabulary.ArgumentFixed || index != 1 || source != vocabulary.ArgumentSourceRule || input != (vocabulary.InputSource{}) {
		t.Fatal("gsub dynamic capture-or-match selector became a static input")
	}
	for _, kind := range []flowkind.OutcomeKind{flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel} {
		if _, ok := contract.Operations.OperationSubedgeRelationOutcome(op, kind); !ok {
			t.Fatalf("gsub relation lost %v outcome correlation", kind)
		}
	}
	if contract.Operations.OperationSubedgeRelationEffectAliasCount(op) != 1 {
		t.Fatal("gsub relation lost its effect alias")
	}
	if effect, ok := contract.Operations.OperationSubedgeRelationEffectAliasAt(op, 0); !ok || effect != 0 {
		t.Fatalf("gsub relation effect alias = %d/%v", effect, ok)
	}
}
