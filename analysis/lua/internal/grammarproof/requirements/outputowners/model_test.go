package outputowners

import "testing"

func TestOutputOwnerModelRowsRetainDeclaredRelationOwner(t *testing.T) {
	evidence, err := Current()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range evidence.Rows {
		if !row.Owner.Available() || !row.Relation.Available() {
			t.Fatalf("invalid generated owner row %#v", row)
		}
	}
}
