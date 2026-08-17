package parserproducts

import "testing"

func TestGeneratedParserProductFieldsCloseEveryRepresentationState(t *testing.T) {
	rows := generatedFields()
	if len(rows) == 0 || len(rows) != len(Generated.Fields) {
		t.Fatalf("generated fields = %d/%d, want checked-in contract", len(rows), len(Generated.Fields))
	}
	var absent, present bool
	for _, row := range rows {
		if row.Form == "" || row.Field == "" || row.Disposition == DispositionInvalid {
			t.Fatalf("incomplete generated field row %#v", row)
		}
		absent = absent || row.State != 0 && row.Source == ""
		present = present || row.Source != ""
	}
	if !absent || !present {
		t.Fatal("generated fields do not include both unobserved and witnessed states")
	}
}
