package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestOperatorsModelRowsRetainTypedOperatorShape(t *testing.T) {
	input := operatorFixture()
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	input.Operators.TypeOf[0].Scope = 0
	input.Operators.KeyOf[0].Inner = 0
	input.Operators.IndexAccess[0].Object = 0
	input.Operators.Conditional[0].Else = 0
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	view := component.View().Operators()
	if scope, operand, ok := view.TypeOfs().Get(keyspace.MakeTerm(keyspace.FamilyTypeOf, 1)); !ok ||
		scope != keyspace.MakeTerm(keyspace.FamilyCell, 1) || operand != keyspace.MakeTerm(keyspace.FamilyRead, 1) {
		t.Fatalf("TypeOf model row = %v/%v/%v", scope, operand, ok)
	}
	if inner, ok := view.KeyOfs().Get(keyspace.MakeTerm(keyspace.FamilyTypeKeyOf, 1)); !ok ||
		inner != keyspace.MakeTerm(keyspace.FamilyTypeOf, 1) {
		t.Fatalf("KeyOf model row = %v/%v", inner, ok)
	}
	if object, index, ok := view.IndexAccesses().Get(keyspace.MakeTerm(keyspace.FamilyTypeIndexAccess, 1)); !ok ||
		object != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) || index != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2) {
		t.Fatalf("IndexAccess model row = %v/%v/%v", object, index, ok)
	}
	if check, extends, thenTerm, elseTerm, ok := view.Conditionals().Get(keyspace.MakeTerm(keyspace.FamilyTypeConditional, 1)); !ok ||
		check != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3) || extends != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 4) ||
		thenTerm != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 5) || elseTerm != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 6) {
		t.Fatalf("Conditional model row = %v/%v/%v/%v/%v", check, extends, thenTerm, elseTerm, ok)
	}
}
