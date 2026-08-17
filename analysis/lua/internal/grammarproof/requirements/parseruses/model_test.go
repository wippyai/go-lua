package parseruses

import "testing"

func TestParserUsesModelCarriesRoleAxesAndExactParentCoordinates(t *testing.T) {
	evidence, err := Build(testProducts())
	if err != nil {
		t.Fatal(err)
	}
	for _, slot := range evidence.UseSlots {
		if slot.ParentForm == "" || slot.ParentField == "" || slot.Role == UseRoleInvalid || slot.Target == ProgramUseInvalid {
			t.Fatalf("incomplete parser-use slot %#v", slot)
		}
	}
	for _, path := range evidence.UsePaths {
		if path.ParentProduction == "" || path.ParentForm == "" || path.ParentField == "" || path.Term == 0 {
			t.Fatalf("incomplete parser-use path %#v", path)
		}
	}
}
