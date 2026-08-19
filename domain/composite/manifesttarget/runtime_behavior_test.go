package manifesttarget_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/domain/composite/manifesttarget"
	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/manifest"
	"github.com/wippyai/go-lua/stdlib"
)

func TestStandardProviderProjectsRuntimeKindResultBehavior(t *testing.T) {
	catalogue, err := manifest.Seal(stdlib.Providers()...)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := manifesttarget.SealCatalogue(catalogue)
	if err != nil {
		t.Fatal(err)
	}
	op, ok := contract.Lookup(vocabulary.BindingSpec{
		Namespace: vocabulary.BindingBuiltin,
		Member:    []string{"type"},
	})
	if !ok {
		t.Fatal("base.type operation missing")
	}
	if contract.BehaviorResultCount(op) != 1 {
		t.Fatalf("base.type behavior result count = %d, want 1", contract.BehaviorResultCount(op))
	}
	if contract.BehaviorPredicateCount(op) != 1 {
		t.Fatalf("base.type behavior predicate count = %d, want 1", contract.BehaviorPredicateCount(op))
	}
	outcome, result, source, relation, ok := contract.BehaviorResultAt(op, 0)
	wantRelation := schema.NewEntryID(schema.SurfaceKindStructure, runtimekind.RuntimeKindResultRelationKey)
	if !ok || outcome != 0 || result != 0 || source != (vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0}) || relation != wantRelation {
		t.Fatalf("base.type behavior = outcome:%d result:%d source:%#v relation:%v ok:%v; want normal result 0 over input 0 and relation %v", outcome, result, source, relation, ok, wantRelation)
	}
	predicateOutcome, predicateResult, predicateSubject, predicateRelation, predicateOK := contract.BehaviorPredicateAt(op, 0)
	wantPredicateRelation := schema.NewEntryID(schema.SurfaceKindStructure, runtimekind.RuntimeKindPredicateRelationKey)
	if !predicateOK || predicateOutcome != 0 || predicateResult != 0 || predicateSubject != (vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0}) || predicateRelation != wantPredicateRelation {
		t.Fatalf("base.type predicate = outcome:%d result:%d subject:%#v relation:%v ok:%v; want normal result 0 over input 0 and relation %v", predicateOutcome, predicateResult, predicateSubject, predicateRelation, predicateOK, wantPredicateRelation)
	}
}
