package programsupply

import "testing"

func TestProgramSupplyModelKeepsTypedLawFamiliesSeparate(t *testing.T) {
	evidence, err := Current()
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.ProgramLaws) == 0 || len(evidence.StaticLaws) == 0 || len(evidence.BinderLaws) == 0 {
		t.Fatalf("current typed law families = %#v, want all populated", evidence)
	}
	for _, row := range evidence.ProgramLaws {
		if len(row.Terminals) == 0 {
			t.Fatalf("Program law row lacks terminal identity %#v", row)
		}
		for _, terminal := range row.Terminals {
			if !terminal.Available() {
				t.Fatalf("Program law row has unavailable terminal %#v", row)
			}
		}
	}
}
