package parserproducts

import "testing"

func TestGeneratedParserProductCarriersStateExactChildCoordinates(t *testing.T) {
	rows := generatedCarriers()
	if len(rows) == 0 || len(rows) != len(Generated.Carriers) {
		t.Fatalf("generated carriers = %d/%d, want checked-in contract", len(rows), len(Generated.Carriers))
	}
	for _, row := range rows {
		if row.Form == "" || row.Field == "" || row.ChildType == "" || row.Cardinality == 0 {
			t.Fatalf("incomplete generated carrier %#v", row)
		}
	}
}
