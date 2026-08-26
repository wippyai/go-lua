package apply

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

func TestJoinedSpanGroupSharesCarrierNotSourceRelation(t *testing.T) {
	content := func(label string) identity.ContentID {
		value, ok := identity.DeriveContentID("engine/relation/apply/dual-authority-law", []byte(label))
		if !ok {
			t.Fatalf("content %q", label)
		}
		return value
	}
	owner, ok := model.IssueOwnerID(content("owner"))
	if !ok {
		t.Fatal("owner")
	}
	carrierRelation, _ := model.IssueRelationID(owner, content("relation/carrier"))
	sourceRelation, _ := model.IssueRelationID(owner, content("relation/source"))
	otherCarrierRelation, _ := model.IssueRelationID(owner, content("relation/other-carrier"))
	carrierColumn, _ := model.IssueColumnID(carrierRelation, content("column/carrier"))
	sourceColumn, _ := model.IssueColumnID(sourceRelation, content("column/source"))
	typeID, _ := model.IssueTypeID(owner, content("type"))
	carrierKey, _ := model.IssueKeyID(carrierRelation, content("key/carrier"))
	sourceKey, _ := model.IssueKeyID(sourceRelation, content("key/source"))
	otherCarrierKey, _ := model.IssueKeyID(otherCarrierRelation, content("key/other-carrier"))
	carrier, _ := model.NewDenominatorRef(carrierRelation, carrierKey)
	source, _ := model.NewDenominatorRef(sourceRelation, sourceKey)
	otherCarrier, _ := model.NewDenominatorRef(otherCarrierRelation, otherCarrierKey)
	delivery, _ := signature.NewCompleteSpanDelivery(carrierKey)
	otherDelivery, _ := signature.NewCompleteSpanDelivery(otherCarrierKey)

	homogeneous, homogeneousOK := signature.NewHomogeneousInput(carrierRelation, carrierColumn, typeID, signature.RequirePresent, delivery, carrier)
	joined, joinedOK := signature.NewJoinedInput(sourceRelation, sourceColumn, typeID, signature.RequirePresent, delivery, source, carrier)
	foreign, foreignOK := signature.NewJoinedInput(sourceRelation, sourceColumn, typeID, signature.RequirePresent, otherDelivery, source, otherCarrier)
	if !homogeneousOK || !joinedOK || !foreignOK {
		t.Fatal("input contracts")
	}
	if !sameGroupInput(homogeneous, joined) {
		t.Fatal("one Complete carrier could not deliver homogeneous and joined source slots")
	}
	if sameGroupInput(joined, foreign) {
		t.Fatal("distinct carrier range was grouped by source relation")
	}
}
