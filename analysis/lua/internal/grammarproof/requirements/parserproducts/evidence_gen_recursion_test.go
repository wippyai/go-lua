package parserproducts

import "testing"

func TestGeneratedParserProductRecursionObligationsNameBothStages(t *testing.T) {
	rows := generatedRecursion()
	if len(rows) == 0 || len(rows) != len(Generated.Recursion) {
		t.Fatalf("generated recursion = %d/%d", len(rows), len(Generated.Recursion))
	}
	seenStages := make(map[[2]uint8]bool)
	for _, row := range rows {
		if row.Nonterminal == "" || row.Family == 0 || row.Stage == 0 {
			t.Fatalf("incomplete generated recursion %#v", row)
		}
		seenStages[[2]uint8{uint8(row.Family), uint8(row.Stage)}] = true
	}
	if len(seenStages) != len(rows) {
		t.Fatal("generated recursion obligations duplicate family/stage coordinates")
	}
}
