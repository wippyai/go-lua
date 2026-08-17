package artifact

import "testing"

func TestBoundaryCompilerSortsRowsByStableIdentity(t *testing.T) {
	rows := []BoundaryRow{{id: valuesLawID(3), owner: valuesLawID(4), kind: BoundaryReturn}, {id: valuesLawID(1), owner: valuesLawID(2), kind: BoundaryCapture}}
	radixBoundaryRows(rows)
	if rows[0].id != valuesLawID(1) || rows[1].id != valuesLawID(3) {
		t.Fatalf("boundary order = %v, %v", rows[0].id, rows[1].id)
	}
}
