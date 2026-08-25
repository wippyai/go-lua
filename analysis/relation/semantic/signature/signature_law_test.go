package signature_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

func content(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("semantic-signature-law", []byte(label))
	if !ok {
		t.Fatalf("derive %q", label)
	}
	return value
}

type fixture struct {
	owner       model.OwnerID
	relation    model.RelationID
	left        model.ColumnID
	right       model.ColumnID
	result      model.ColumnID
	denominator model.DenominatorRef
	fence       signature.Fence
	operation   signature.Identity
	leftType    model.TypeID
	rightType   model.TypeID
	resultType  model.TypeID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	owner, ok := model.IssueOwnerID(content(t, "owner"))
	if !ok {
		t.Fatalf("issue owner")
	}
	relation, ok := model.IssueRelationID(owner, content(t, "relation"))
	if !ok {
		t.Fatalf("issue relation")
	}
	left, ok := model.IssueColumnID(relation, content(t, "column/left"))
	if !ok {
		t.Fatalf("issue left")
	}
	right, ok := model.IssueColumnID(relation, content(t, "column/right"))
	if !ok {
		t.Fatalf("issue right")
	}
	result, ok := model.IssueColumnID(relation, content(t, "column/result"))
	if !ok {
		t.Fatalf("issue result")
	}
	key, ok := model.IssueKeyID(relation, content(t, "key/denominator"))
	if !ok {
		t.Fatalf("issue key")
	}
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatalf("issue denominator")
	}
	schema, ok := model.IssueSchemaID(owner, content(t, "schema"))
	if !ok {
		t.Fatalf("issue schema")
	}
	operationID, ok := model.IssueOperationID(owner, content(t, "operation"))
	if !ok {
		t.Fatalf("issue operation")
	}
	leftType, ok := model.IssueTypeID(owner, content(t, "type/left"))
	if !ok {
		t.Fatalf("issue left type")
	}
	rightType, ok := model.IssueTypeID(owner, content(t, "type/right"))
	if !ok {
		t.Fatalf("issue right type")
	}
	resultType, ok := model.IssueTypeID(owner, content(t, "type/result"))
	if !ok {
		t.Fatalf("issue result type")
	}
	return fixture{
		owner:       owner,
		relation:    relation,
		left:        left,
		right:       right,
		result:      result,
		denominator: denominator,
		fence:       signature.Fence{Owner: owner, Schema: schema},
		operation:   signature.Identity{Operation: operationID, Version: 1},
		leftType:    leftType, rightType: rightType, resultType: resultType,
	}
}

func validSpec(t *testing.T, value fixture, inputs []signature.Input) signature.Spec {
	t.Helper()
	accepted, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Refused)
	if !ok {
		t.Fatalf("accepted outcomes")
	}
	exact, exactOK := model.NewCardinality(model.ExactlyOne, 0)
	if !exactOK {
		t.Fatalf("construct exact cardinality")
	}
	return signature.Spec{
		Identity: value.operation,
		Fence:    value.fence,
		Inputs:   inputs,
		Outputs:  []signature.Output{{Relation: value.relation, Column: value.result, Type: value.resultType, Presence: signature.ProducePresent}},
		Authority: signature.OutputAuthority{
			Denominator: value.denominator,
		},
		Cardinality: exact,
		Outcomes:    accepted,
	}
}

func TestSignatureIdentityRetainsOrderedInputAndOutputContracts(t *testing.T) {
	value := newFixture(t)
	delivery := scalarDelivery(t)
	inputs := []signature.Input{
		{Relation: value.relation, Column: value.left, Type: value.leftType, Presence: signature.RequirePresent, Delivery: delivery, Denominator: value.denominator},
		{Relation: value.relation, Column: value.right, Type: value.rightType, Presence: signature.RequirePresent, Delivery: delivery, Denominator: value.denominator},
	}
	first, ok := signature.Seal(validSpec(t, value, inputs))
	if !ok {
		t.Fatalf("seal first signature")
	}
	reversed := []signature.Input{inputs[1], inputs[0]}
	second, ok := signature.Seal(validSpec(t, value, reversed))
	if !ok {
		t.Fatalf("seal reversed signature")
	}
	if first.Digest() == second.Digest() {
		t.Fatalf("ordered input identity was erased")
	}
	if got, ok := first.InputAt(0); !ok || got.Column != value.left {
		t.Fatalf("first input order changed")
	}
	copyInputs := first.Inputs()
	copyInputs[0] = inputs[1]
	if got, ok := first.InputAt(0); !ok || got.Column != value.left {
		t.Fatalf("signature exposed mutable input storage")
	}
}

func TestSignatureSealFreezesMalformedCrossContractReferences(t *testing.T) {
	value := newFixture(t)
	input := []signature.Input{{Relation: value.relation, Column: value.left, Type: value.leftType, Presence: signature.RequirePresent, Delivery: scalarDelivery(t), Denominator: value.denominator}}
	spec := validSpec(t, value, input)
	spec.Authority.Denominator = model.DenominatorRef{}
	foreign, ok := signature.Seal(spec)
	if !ok || !foreign.Available() {
		t.Fatalf("foreign output authority was not frozen")
	}
	spec = validSpec(t, value, input)
	bounded, ok := model.NewCardinality(model.BoundedMany, 2)
	if !ok {
		t.Fatalf("issue bound")
	}
	spec.Cardinality = bounded
	mismatch, ok := signature.Seal(spec)
	if !ok || !mismatch.Available() {
		t.Fatalf("cardinality mutation was not frozen")
	}
	spec.Inputs = append(spec.Inputs, signature.Input{})
	if malformed, ok := signature.Seal(spec); !ok || !malformed.Available() {
		t.Fatalf("structurally malformed input was not frozen")
	}
}

func TestDeliveryShapeAndLogicalOrderAreSignatureIdentity(t *testing.T) {
	value := newFixture(t)
	scalar := scalarDelivery(t)
	first, ok := signature.Seal(validSpec(t, value, []signature.Input{{Relation: value.relation, Column: value.left, Type: value.leftType, Presence: signature.RequirePresent, Delivery: scalar, Denominator: value.denominator}}))
	if !ok {
		t.Fatalf("seal scalar signature")
	}
	span, ok := signature.NewBoundedSpanDelivery(3, value.denominator.Key())
	if !ok {
		t.Fatalf("construct bounded delivery")
	}
	second, ok := signature.Seal(validSpec(t, value, []signature.Input{{Relation: value.relation, Column: value.left, Type: value.leftType, Presence: signature.RequirePresent, Delivery: span, Denominator: value.denominator}}))
	if !ok {
		t.Fatalf("seal span signature")
	}
	if first.Digest() == second.Digest() {
		t.Fatalf("delivery shape was erased from identity")
	}
	otherRelation, ok := model.IssueRelationID(value.owner, content(t, "relation/order-other"))
	if !ok {
		t.Fatalf("issue order relation")
	}
	otherKey, ok := model.IssueKeyID(otherRelation, content(t, "key/order-other"))
	if !ok {
		t.Fatalf("issue order key")
	}
	otherSpan, ok := signature.NewBoundedSpanDelivery(3, otherKey)
	if !ok {
		t.Fatalf("construct other bounded delivery")
	}
	third, ok := signature.Seal(validSpec(t, value, []signature.Input{{Relation: value.relation, Column: value.left, Type: value.leftType, Presence: signature.RequirePresent, Delivery: otherSpan, Denominator: value.denominator}}))
	if !ok || third.Digest() == second.Digest() {
		t.Fatalf("logical order key was erased from identity")
	}
}

func TestSignaturePolicyAccessorsPreserveSealedSpec(t *testing.T) {
	value := newFixture(t)
	delivery := scalarDelivery(t)
	spec := validSpec(t, value, []signature.Input{{
		Relation: value.relation, Column: value.left, Type: value.leftType,
		Presence: signature.RequirePresent, Delivery: delivery, Denominator: value.denominator,
	}})
	sealed, ok := signature.Seal(spec)
	if !ok {
		t.Fatalf("seal policy signature")
	}
	if !sealed.Outcomes().Equal(spec.Outcomes) {
		t.Fatalf("sealed policy accessor changed the contract")
	}
	copyOf := sealed.Outcomes().Codes()
	copyOf[0] = outcome.Invalid
	if sealed.Outcomes().Contains(outcome.Invalid) {
		t.Fatalf("outcome accessor exposed mutable signature storage")
	}
}

func scalarDelivery(t *testing.T) signature.Delivery {
	t.Helper()
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatalf("construct scalar delivery")
	}
	return delivery
}
