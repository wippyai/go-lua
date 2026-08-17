package target

import "testing"

func TestSubedgeRowsPreserveCanonicalRoleAndFamily(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{protectedSubedgeOperation("row-edge", false, false, false)}})
	op, _ := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"row-edge"}})
	edge, ok := contract.SubedgeAt(op, 0)
	if !ok {
		t.Fatal("subedge row missing")
	}
	if role, ok := contract.SubedgeRole(edge); !ok || role == 0 {
		t.Fatalf("SubedgeRole = %d/%v", role, ok)
	}
	if family, ok := contract.SubedgeFamily(edge); !ok || family != SubedgeFamilyCall {
		t.Fatalf("SubedgeFamily = %d/%v", family, ok)
	}
}
