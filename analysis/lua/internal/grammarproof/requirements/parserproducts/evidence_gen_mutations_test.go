package parserproducts

import "testing"

func TestGeneratedParserProductMutationsRetainTypedEditCoordinates(t *testing.T) {
	rows := generatedMutations()
	if len(rows) == 0 || len(rows) != len(Generated.Mutations) {
		t.Fatalf("generated mutations = %d/%d", len(rows), len(Generated.Mutations))
	}
	for _, row := range rows {
		if row.Production == "" || row.Edit.Kind == EditInvalid || row.Edit.Value == 0 || row.Edit.Place.Root == PlaceRootInvalid {
			t.Fatalf("incomplete generated mutation %#v", row)
		}
	}
}
