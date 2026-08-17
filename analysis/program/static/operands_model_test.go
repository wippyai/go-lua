package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestOperandsModelRowsRetainExactCrossOwnerHandles(t *testing.T) {
	input := operandsFixture(t)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	input.Operands.Claim[0].Claim = 0
	input.Operands.TypeValue[0].Target = 0
	input.Operands.Annotation[0].Scope = 0
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	view := component.View().Operands()
	if claim, ok := view.Claims().At(0); !ok || claim != keyspace.MakeTerm(keyspace.FamilyValueClaim, 1) {
		t.Fatalf("ClaimTarget model row = %v/%v", claim, ok)
	}
	if target, ok := view.TypeValues().Target(keyspace.MakeTerm(keyspace.FamilyTypeValue, 1)); !ok ||
		target != keyspace.MakeTerm(keyspace.FamilyTypeRef, 1) {
		t.Fatalf("TypeValueTarget model row = %v/%v", target, ok)
	}
	if row, ok := view.Annotations().Get(keyspace.MakeTerm(keyspace.FamilyAnnotation, 1)); !ok ||
		row.Scope != keyspace.MakeTerm(keyspace.FamilyValueClaim, 1) {
		t.Fatalf("Annotation model row = %+v/%v", row, ok)
	}
}
