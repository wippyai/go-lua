package programlaw

import "testing"

func TestProgramLawModelKeepsOperationAndBoundaryRowsTyped(t *testing.T) {
	operations := OperationRequirements()
	boundaries := BoundaryRequirements()
	if len(operations) != 25 || len(boundaries) != 8 {
		t.Fatalf("law families = operations %d boundaries %d, want 25/8", len(operations), len(boundaries))
	}
	for _, row := range Requirements() {
		if row.Site == SiteInvalid {
			t.Fatalf("invalid law row %#v", row)
		}
	}
	for _, row := range operations {
		if row.Site != SiteUnary && row.Site != SiteBinary && row.Site != SiteSelect {
			t.Fatalf("operation row crossed family boundary %#v", row)
		}
	}
}
