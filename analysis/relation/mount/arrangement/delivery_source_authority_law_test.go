package arrangement

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

func TestDeliveryRequirementUsesDeclaredSourceAuthority(t *testing.T) {
	owner := deliverySourceOwner(t)
	carrierRelation, carrierKey, carrierDenominator := deliverySourceDenominator(t, owner, "carrier")
	sourceRelation, sourceKey, sourceDenominator := deliverySourceDenominator(t, owner, "source")
	// Source denominators must remain on the same source relation; reissue the
	// alternate key there to distinguish key authority without changing the
	// delivered column's owner.
	alternateSourceKey, ok := model.IssueKeyID(sourceRelation, deliverySourceToken(t, "key/source-alternate"))
	if !ok {
		t.Fatal("alternate source key")
	}
	alternateSourceDenominator, ok := model.NewDenominatorRef(sourceRelation, alternateSourceKey)
	if !ok {
		t.Fatal("alternate source denominator")
	}
	if carrierRelation == sourceRelation || carrierKey == sourceKey || sourceDenominator == alternateSourceDenominator {
		t.Fatal("fixture did not produce distinct authorities")
	}
	sourceColumn, ok := model.IssueColumnID(sourceRelation, deliverySourceToken(t, "column/source"))
	if !ok {
		t.Fatal("source column")
	}
	typeID, ok := model.IssueTypeID(owner, deliverySourceToken(t, "type"))
	if !ok {
		t.Fatal("type")
	}
	delivery, ok := signature.NewCompleteSpanDelivery(carrierKey)
	if !ok {
		t.Fatal("delivery")
	}
	input, ok := signature.NewJoinedInput(sourceRelation, sourceColumn, typeID, signature.RequirePresent, delivery, sourceDenominator, carrierDenominator)
	if !ok {
		t.Fatal("joined input")
	}
	operationID, ok := model.IssueOperationID(owner, deliverySourceToken(t, "operation"))
	if !ok {
		t.Fatal("operation")
	}
	requirement, ok := newDeliveryRequirement(signature.Identity{Operation: operationID, Version: 1}, 0, input)
	if !ok {
		t.Fatal("requirement")
	}
	access, ok := requirement.Access()
	if !ok {
		t.Fatal("access")
	}
	want, ok := newAccess(sourceRelation, sourceKey, []model.ColumnID{sourceColumn})
	if !ok || !access.Equal(want) {
		t.Fatalf("delivery access = %#v, want source authority %#v", access, want)
	}

	otherInput, ok := signature.NewJoinedInput(sourceRelation, sourceColumn, typeID, signature.RequirePresent, delivery, alternateSourceDenominator, carrierDenominator)
	if !ok {
		t.Fatal("other joined input")
	}
	other, ok := newDeliveryRequirement(requirement.Operation(), 0, otherInput)
	if !ok {
		t.Fatal("other requirement")
	}
	if requirement.equal(other) || bytes.Equal(deliveryRequirementDigest(requirement), deliveryRequirementDigest(other)) {
		t.Fatal("joined source authority was erased from delivery identity")
	}
	if !deliveryRequirementLess(requirement, other) && !deliveryRequirementLess(other, requirement) {
		t.Fatal("joined source authority was erased from canonical order")
	}
}

func deliverySourceOwner(t *testing.T) model.OwnerID {
	t.Helper()
	owner, ok := model.IssueOwnerID(deliverySourceToken(t, "owner"))
	if !ok {
		t.Fatal("owner")
	}
	return owner
}

func deliverySourceDenominator(t *testing.T, owner model.OwnerID, label string) (model.RelationID, model.KeyID, model.DenominatorRef) {
	t.Helper()
	relation, ok := model.IssueRelationID(owner, deliverySourceToken(t, "relation/"+label))
	if !ok {
		t.Fatal("relation")
	}
	key, ok := model.IssueKeyID(relation, deliverySourceToken(t, "key/"+label))
	if !ok {
		t.Fatal("key")
	}
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	return relation, key, denominator
}

func deliverySourceToken(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("analysis/relation/mount/arrangement/delivery-source-law/v1", []byte(label))
	if !ok {
		t.Fatal("token")
	}
	return value
}
