package target_test

import (
	"testing"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/analysis/program/target/profile"
)

func TestGsubTableReplacementClosedDenominatorAndAliases(t *testing.T) {
	contract, err := profile.Contract()
	if err != nil {
		t.Fatal(err)
	}
	op, ok := contract.Lookup(target.BindingSpec{Namespace: target.BindingModule, Owner: []string{"string"}, Member: []string{"gsub"}})
	if !ok {
		t.Fatal("string.gsub missing")
	}
	replacement, key, access, resultOutcome, result, ok := contract.GsubTableReplacement(op)
	if !ok || replacement != 2 || key != target.GsubTableKeyFirstCaptureOrWholeMatch || result != 0 {
		t.Fatalf("gsub table branch = %d/%d/%d/%d/%d/%v", replacement, key, access, resultOutcome, result, ok)
	}
	if family, ok := contract.SubedgeFamily(access); !ok || family != target.SubedgeFamilyIndexGet {
		t.Fatal("gsub table branch lost exact IndexGet route")
	}
	segment, index, source, input, ok := contract.ArgumentOriginAt(access, 0)
	if !ok || segment != target.ArgumentFixed || index != 0 || source != target.ArgumentSourceInput || input != (target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 2}) {
		t.Fatal("gsub replacement table input changed")
	}
	segment, index, source, input, ok = contract.ArgumentOriginAt(access, 1)
	if !ok || segment != target.ArgumentFixed || index != 1 || source != target.ArgumentSourceRule || input != (target.InputSource{}) {
		t.Fatal("gsub first-capture-or-whole-match key became a static or callback input")
	}
	for _, kind := range []flowkind.OutcomeKind{flowkind.OutcomeNormal, flowkind.OutcomeReturn} {
		if got, ok := contract.GsubTableReplacementOutcome(op, kind); !ok || got != resultOutcome {
			t.Fatalf("gsub %v result alias = %d/%v, want %d", kind, got, ok, resultOutcome)
		}
	}
	for _, kind := range []flowkind.OutcomeKind{flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel} {
		if _, ok := contract.GsubTableReplacementOutcome(op, kind); !ok {
			t.Fatalf("gsub %v route alias absent", kind)
		}
	}
	if contract.GsubTableReplacementEffectAliasCount(op) != 1 {
		t.Fatal("gsub table branch lacks its exact effect alias")
	}
	if effect, ok := contract.GsubTableReplacementEffectAliasAt(op, 0); !ok || effect != 0 {
		t.Fatalf("gsub table effect alias = %d/%v", effect, ok)
	}
	other, found := contract.OperationAt(0)
	if !found || other == op {
		t.Fatal("non-gsub denominator unavailable")
	}
	if _, _, _, _, _, ok := contract.GsubTableReplacement(other); ok {
		t.Fatal("non-gsub operation acquired table replacement branch")
	}
}

func TestGsubTableReplacementRejectsShortcutAndMalformedBranches(t *testing.T) {
	mutate := func(change func(*target.OperationSpec)) error {
		spec := profile.Spec()
		for index := range spec.Operations {
			if len(spec.Operations[index].Bindings) != 0 && spec.Operations[index].Bindings[0].Namespace == target.BindingModule && len(spec.Operations[index].Bindings[0].Owner) == 1 && spec.Operations[index].Bindings[0].Owner[0] == "string" && len(spec.Operations[index].Bindings[0].Member) == 1 && spec.Operations[index].Bindings[0].Member[0] == "gsub" {
				change(&spec.Operations[index])
				break
			}
		}
		_, err := target.Seal(&spec)
		return err
	}
	if err := mutate(func(op *target.OperationSpec) { op.GsubTableReplacement.Replacement = 1 }); err == nil {
		t.Fatal("callback-like replacement formal accepted")
	}
	if err := mutate(func(op *target.OperationSpec) { op.GsubTableReplacement.Access = 1 }); err == nil {
		t.Fatal("function callback access accepted as table IndexGet")
	}
	if err := mutate(func(op *target.OperationSpec) { op.GsubTableReplacement.EffectAliases = nil }); err == nil {
		t.Fatal("gsub table branch without effect alias accepted")
	}
	if err := mutate(func(op *target.OperationSpec) { op.GsubTableReplacement.Result = 1 }); err == nil {
		t.Fatal("uncorrelated gsub result accepted")
	}
}

func TestGsubTableReplacementPermutationPreservesContent(t *testing.T) {
	left := profile.Spec()
	right := profile.Spec()
	for index := range right.Operations {
		op := &right.Operations[index]
		if len(op.Bindings) == 0 || op.Bindings[0].Namespace != target.BindingModule || len(op.Bindings[0].Owner) != 1 || op.Bindings[0].Owner[0] != "string" || len(op.Bindings[0].Member) != 1 || op.Bindings[0].Member[0] != "gsub" {
			continue
		}
		op.Subedges[0], op.Subedges[1] = op.Subedges[1], op.Subedges[0]
		op.GsubTableReplacement.Access = 1
		break
	}
	first, err := target.Seal(&left)
	if err != nil {
		t.Fatal(err)
	}
	second, err := target.Seal(&right)
	if err != nil {
		t.Fatal(err)
	}
	if first.ContentID() != second.ContentID() {
		t.Fatal("subedge authoring permutation changed gsub table branch identity")
	}
}
