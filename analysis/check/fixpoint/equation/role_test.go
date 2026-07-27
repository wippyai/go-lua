package equation_test

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

func TestOperandRoleOwnsIndexedConstruction(t *testing.T) {
	role := equation.IndexedRole(equation.RoleFamilyArgument, 7)
	if role.String() != "argument-00000007" {
		t.Fatalf("indexed role = %q", role)
	}
	if index, ok := role.FixedIndex(equation.RoleFamilyArgument, 8); !ok || index != 7 {
		t.Fatalf("indexed role did not round trip: index=%d ok=%v", index, ok)
	}
	if _, ok := role.Index(equation.RoleFamilyArgumentDisplay); ok {
		t.Fatal("argument role entered the presentation-only family")
	}

	field, ok := reflect.TypeOf(equation.Operand{}).FieldByName("Role")
	if !ok || field.Type != reflect.TypeOf(equation.OperandRole("")) {
		t.Fatalf("Operand.Role type = %v, want equation.OperandRole", field.Type)
	}
}

func TestDisplayRolesCannotBecomeSemanticResults(t *testing.T) {
	for _, role := range []equation.OperandRole{
		equation.RoleResultDisplay,
		equation.RoleResultArity,
		equation.RoleResultSpread,
		equation.IndexedRole(equation.RoleFamilyReturnDisplay, 0),
	} {
		if _, ok := role.SemanticResult(); ok {
			t.Fatalf("presentation/control role %q became a semantic result", role)
		}
	}
	slot := equation.IndexedRole(equation.RoleFamilyResult, 3)
	semantic, ok := slot.SemanticResult()
	if !ok {
		t.Fatalf("result slot %q was not semantic", slot)
	}
	if index, indexed := semantic.Index(); !indexed || index != 3 {
		t.Fatalf("semantic result index = %d, %v", index, indexed)
	}
}
