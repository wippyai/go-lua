package artifact

import "testing"

func TestPointAttachmentRowsRequireBothParentIdentities(t *testing.T) {
	row := PointAttachmentRow{site: valuesLawID(1), point: valuesLawID(2)}
	if !row.Available() || row.SiteID() != valuesLawID(1) || row.PointID() != valuesLawID(2) {
		t.Fatal("complete point attachment unavailable")
	}
	row.point = valuesLawID(0)
	if row.Available() {
		t.Fatal("point attachment admitted missing point identity")
	}
}
