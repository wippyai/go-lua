package artifact

import "testing"

func TestStaticNodeRowsRejectInvalidKind(t *testing.T) {
	row := StaticTypeNodeRow{id: valuesLawID(1), owner: valuesLawID(2), kind: StaticNodeInvalid}
	if row.Available() {
		t.Fatal("invalid static node kind was admitted")
	}
}
