package parsersource

import "testing"

func TestASTConstantDiscoveryResolvesZeroAndNonZeroDiscriminants(t *testing.T) {
	root := moduleRoot(t)
	rows, err := DiscoverConstants(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("AST constant discovery returned no decidable constants")
	}
	var zero, nonZero bool
	for _, row := range rows {
		if row.Name == "AttrKeyUnknown" && row.Zero {
			zero = true
		}
		if row.Name == "AttrKeyDot" && !row.Zero {
			nonZero = true
		}
	}
	if !zero || !nonZero {
		t.Fatalf("constant rows omit expected zero/non-zero lexer discriminants: %#v", rows)
	}
}
