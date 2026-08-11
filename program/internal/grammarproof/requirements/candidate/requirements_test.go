package candidate

import "testing"

func TestCandidateSchemaContractIsClosedAndExplicit(t *testing.T) {
	rows := Requirements()
	if len(rows) != 79 || len(Discharged()) != 79 || MissingCount() != 0 {
		t.Fatalf("candidate schema = required:%d discharged:%d missing:%d, want 79/79/0", len(rows), len(Discharged()), MissingCount())
	}
	seen := make(map[Requirement]bool, len(rows))
	for _, row := range rows {
		if row.Subject.Family == FamilyInvalid || row.Branch == BranchInvalid {
			t.Fatalf("invalid candidate row: %#v", row)
		}
		if seen[row] {
			t.Fatalf("duplicate candidate row: %#v", row)
		}
		seen[row] = true
	}
}
