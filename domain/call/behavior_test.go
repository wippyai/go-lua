package call

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

func TestTargetBehaviorProjection(t *testing.T) {
	anyType, anyOK := schematype.NewPrimitive(schematype.PrimitiveAny)
	if !anyOK {
		t.Fatal("any type declaration")
	}
	relation := schema.NewEntryID(schema.SurfaceKindStructure, "semantic/runtime-kind/result")
	contract, err := target.Seal(&target.Spec{
		Semantics: domaincontract.NewSemantics(),
		Operations: []vocabulary.OperationSpec{
			{
				Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"behavior-op"}}},
				Input:    vocabulary.ValuesSpec{Fixed: []schematype.Type{anyType}, Tail: vocabulary.ValuesClosed},
				Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{anyType}, Tail: vocabulary.ValuesClosed}}},
				Behavior: &vocabulary.OperationBehaviorSpec{
					Results:    []vocabulary.OperationResultSpec{{Outcome: 0, Result: 0, Source: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0}, Relation: relation}},
					Predicates: []vocabulary.OperationPredicateSpec{{Outcome: 0, Result: 0, Subject: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0}, Relation: relation}},
				},
				Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed},
			},
			{
				Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"plain-op"}}},
				Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
				Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
				Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
			},
		},
	})
	if err != nil || contract == nil {
		t.Fatalf("seal behavior contract: %v", err)
	}
	program, lowerErr := lower.Lower(lower.Source{Name: "behavior_projection.lua", Text: []byte("return 1")})
	if lowerErr != nil || program == nil {
		t.Fatalf("lower behavior module: %v", lowerErr)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "behavior-module", Program: program}}})
	if err != nil || linked == nil {
		t.Fatalf("seal behavior link: %v", err)
	}
	owner := linked.OwnerCapability()
	operation, operationOK := contract.Operations.OperationAt(0)
	if !operationOK {
		t.Fatal("operation handle")
	}
	plainOperation, plainOperationOK := contract.Operations.OperationAt(1)
	if !plainOperationOK {
		t.Fatal("plain operation handle")
	}
	left := behaviorTestAlgebra(contract, owner, operation, plainOperation)
	foreign := behaviorTestAlgebra(contract, owner, operation, plainOperation)
	known, knownOK := left.targetForSelector(1)
	plain, plainOK := left.targetForSelector(2)
	foreignTarget, foreignTargetOK := foreign.targetForSelector(1)
	if !knownOK || !plainOK || !foreignTargetOK {
		t.Fatal("test target rows")
	}
	if !left.OwnsTarget(known) || left.OwnsTarget(foreignTarget) {
		t.Fatal("Target owner fence was not preserved")
	}
	if known.BehaviorResultCount() != 1 || known.BehaviorPredicateCount() != 1 {
		t.Fatalf("behavior rows = %d/%d, want 1/1", known.BehaviorResultCount(), known.BehaviorPredicateCount())
	}
	if plain.BehaviorResultCount() != 0 || plain.BehaviorPredicateCount() != 0 {
		t.Fatal("operation without a behavior descriptor was admitted")
	}
	if foreignTarget.BehaviorResultCount() != 1 || foreignTarget.BehaviorPredicateCount() != 1 {
		t.Fatal("foreign target did not retain its own owner-fenced projection")
	}
	if (Target{}).BehaviorResultCount() != 0 || (Target{}).BehaviorPredicateCount() != 0 {
		t.Fatal("absent target projected behavior")
	}
	outcome, result, source, gotRelation, rowOK := known.BehaviorResultAt(0)
	if !rowOK || outcome != 0 || result != 0 || source.Kind != vocabulary.InputSourceValueFormal || source.Ordinal != 0 || gotRelation != relation {
		t.Fatalf("result row = %d/%d/%#v/%v/%v", outcome, result, source, gotRelation, rowOK)
	}
	predicateOutcome, predicateResult, subject, predicateRelation, predicateOK := known.BehaviorPredicateAt(0)
	if !predicateOK || predicateOutcome != 0 || predicateResult != 0 || subject.Kind != vocabulary.InputSourceValueFormal || subject.Ordinal != 0 || predicateRelation != relation {
		t.Fatalf("predicate row = %d/%d/%#v/%v/%v", predicateOutcome, predicateResult, subject, predicateRelation, predicateOK)
	}
	if _, _, _, _, ok := known.BehaviorResultAt(-1); ok {
		t.Fatal("negative result index accepted")
	}
	if _, _, _, _, ok := known.BehaviorPredicateAt(1); ok {
		t.Fatal("predicate index beyond descriptor accepted")
	}
}

func behaviorTestAlgebra(contract *target.Contract, owner link.OwnerCapability, operation, plain vocabulary.Operation) *Algebra {
	firstKey := targetKey{kind: targetSeed, seedID: identity.ContentID{1}}
	secondKey := targetKey{kind: targetSeed, seedID: identity.ContentID{2}}
	return &Algebra{
		contract: contract, linkOwner: owner, content: algebraContentID(owner),
		targets: []targetRow{
			{key: firstKey, seedOperation: operation, seedFormalID: identity.ContentID{3}, seedKind: 1},
			{key: secondKey, seedOperation: plain, seedFormalID: identity.ContentID{4}, seedKind: 1},
		},
		targetIndex: map[targetKey]selector{firstKey: 1, secondKey: 2},
	}
}
