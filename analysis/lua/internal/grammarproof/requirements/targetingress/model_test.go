package targetingress

import "testing"

func TestTargetIngressModelRowsRetainExactRelationFormAndParentVector(t *testing.T) {
	evidence, err := Current()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range evidence.Rows {
		if !row.Relation.Available() || !row.Owner.Available() || row.Form == 0 {
			t.Fatalf("incomplete target ingress row %#v", row)
		}
		for _, parent := range row.Ingress {
			if !parent.Available() {
				t.Fatalf("target ingress row has unavailable parent %#v", row)
			}
		}
	}
}
