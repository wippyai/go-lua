package target

import "testing"

func TestContinuationQueriesReturnCanonicalSuspensionCoordinates(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []OperationSpec{deltaSuspension(ReentryMany)}})
	op, ok := contract.Lookup(BindingSpec{Namespace: BindingBuiltin, Member: []string{"suspend"}})
	if !ok || contract.SuspensionCount(op) != 1 {
		t.Fatalf("suspension lookup = %d/%v", contract.SuspensionCount(op), ok)
	}
	yield, reentry, source, multiplicity, ok := contract.SuspensionAt(op, 0)
	if !ok || yield != 1 || reentry != 0 || source != ReentryByCall || multiplicity != ReentryMany {
		t.Fatalf("SuspensionAt = %d/%d/%d/%d/%v", yield, reentry, source, multiplicity, ok)
	}
	if _, _, _, _, ok := contract.SuspensionAt(op, 1); ok {
		t.Fatal("out-of-range suspension query resolved")
	}
}
