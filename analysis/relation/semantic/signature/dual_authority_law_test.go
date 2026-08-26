package signature_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

func TestInputAuthorityIsAClosedHomogeneousOrJoinedSum(t *testing.T) {
	value := newFixture(t)
	carrierDelivery, ok := signature.NewCompleteSpanDelivery(value.denominator.Key())
	if !ok {
		t.Fatal("carrier delivery")
	}

	// Existing literals retain only the compact homogeneous member.  There is
	// no implicit cross-relation fallback hidden behind a missing source field.
	homogeneous := signature.Input{
		Relation: value.relation, Column: value.left, Type: value.leftType,
		Presence: signature.RequirePresent, Delivery: carrierDelivery, Denominator: value.denominator,
	}
	if !homogeneous.Available() || homogeneous.AuthorityKind() != signature.HomogeneousAuthority || !homogeneous.IsHomogeneous() || homogeneous.IsJoined() || homogeneous.SourceAuthority.Declared() {
		t.Fatalf("homogeneous authority=%#v", homogeneous)
	}
	if source, sourceOK := homogeneous.SourceDenominator(); !sourceOK || source != value.denominator || homogeneous.CarrierDenominator() != value.denominator {
		t.Fatalf("homogeneous source/carrier=%#v/%t/%#v", source, sourceOK, homogeneous.CarrierDenominator())
	}

	sourceRelation, sourceColumn, sourceDenominator := joinedSource(t, value, "source")
	joined, joinedOK := signature.NewJoinedInput(sourceRelation, sourceColumn, value.leftType, signature.RequirePresent, carrierDelivery, sourceDenominator, value.denominator)
	if !joinedOK || !joined.Available() || joined.AuthorityKind() != signature.JoinedAuthority || !joined.IsJoined() || joined.IsHomogeneous() || !joined.SourceAuthority.Declared() {
		t.Fatalf("joined authority=%#v/%t", joined, joinedOK)
	}
	if source, sourceOK := joined.SourceDenominator(); !sourceOK || source != sourceDenominator || joined.CarrierDenominator() != value.denominator {
		t.Fatalf("joined source/carrier=%#v/%t/%#v", source, sourceOK, joined.CarrierDenominator())
	}
}

func TestInputAuthorityRefusesAmbiguousOrWrongSourceDeclarations(t *testing.T) {
	value := newFixture(t)
	carrierDelivery, ok := signature.NewCompleteSpanDelivery(value.denominator.Key())
	if !ok {
		t.Fatal("carrier delivery")
	}
	sourceRelation, sourceColumn, sourceDenominator := joinedSource(t, value, "source-refusal")

	// A cross-relation literal with no declared source is not a historical
	// compatibility path; its absent tag means homogeneous and therefore
	// refuses against a distinct carrier relation.
	absentSource := signature.Input{
		Relation: sourceRelation, Column: sourceColumn, Type: value.leftType,
		Presence: signature.RequirePresent, Delivery: carrierDelivery, Denominator: value.denominator,
	}
	if absentSource.Available() || absentSource.AuthorityKind() != signature.HomogeneousAuthority {
		t.Fatalf("cross-relation absent source accepted: %#v", absentSource)
	}

	// A present source on the carrier relation would encode the same authority
	// twice.  The sum deliberately has no redundant third state.
	if _, sameRelationOK := signature.NewJoinedInput(value.relation, value.left, value.leftType, signature.RequirePresent, carrierDelivery, value.denominator, value.denominator); sameRelationOK {
		t.Fatal("same-relation joined authority accepted")
	}

	// The source denominator owns source Relation.  A distinct foreign source
	// denominator cannot be paired with a source column merely because it has
	// a compatible delivery carrier.
	foreignRelation, foreignColumn, foreignDenominator := joinedSource(t, value, "foreign-refusal")
	if foreignRelation == sourceRelation || foreignColumn == sourceColumn || foreignDenominator == sourceDenominator {
		t.Fatal("fixture did not issue a distinct foreign source")
	}
	if _, wrongSourceOK := signature.NewJoinedInput(sourceRelation, sourceColumn, value.leftType, signature.RequirePresent, carrierDelivery, foreignDenominator, value.denominator); wrongSourceOK {
		t.Fatal("wrong source denominator accepted")
	}
}

func joinedSource(t *testing.T, value fixture, label string) (model.RelationID, model.ColumnID, model.DenominatorRef) {
	t.Helper()
	relation, ok := model.IssueRelationID(value.owner, content(t, "relation/joined-"+label))
	if !ok {
		t.Fatal("joined relation")
	}
	column, ok := model.IssueColumnID(relation, content(t, "column/joined-"+label))
	if !ok {
		t.Fatal("joined column")
	}
	key, ok := model.IssueKeyID(relation, content(t, "key/joined-"+label))
	if !ok {
		t.Fatal("joined key")
	}
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("joined denominator")
	}
	return relation, column, denominator
}
