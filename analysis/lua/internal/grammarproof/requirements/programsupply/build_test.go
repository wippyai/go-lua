package programsupply

import "testing"

func TestProgramSupplyBuildSealsTypedDenominatorClosure(t *testing.T) {
	evidence, err := Build()
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.Validate(denominatorEntries()); err != nil {
		t.Fatal(err)
	}
	if len(evidence.ProgramLaws) == 0 || len(evidence.StaticLaws) == 0 || len(evidence.BinderLaws) == 0 {
		t.Fatalf("Program supply rows = program %d static %d binder %d", len(evidence.ProgramLaws), len(evidence.StaticLaws), len(evidence.BinderLaws))
	}
}
