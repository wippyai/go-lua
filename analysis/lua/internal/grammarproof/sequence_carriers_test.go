package grammarproof

import "testing"

func TestGeneratedSequenceCarrierContractHasUniqueResolvedCoordinates(t *testing.T) {
	rows, err := SequenceCarriers(moduleRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("sequence-carrier denominator is empty")
	}
	seen := make(map[SequenceCarrier]bool, len(rows))
	for _, row := range rows {
		if row.Tag == "" || seen[row] {
			t.Fatalf("sequence carrier is not a unique resolved coordinate: %#v", row)
		}
		seen[row] = true
	}
}
