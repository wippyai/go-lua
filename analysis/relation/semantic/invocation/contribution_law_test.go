package invocation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/semantic/output"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

type contributionLawFixture struct {
	owner       model.OwnerID
	foreign     binding.Fence
	fence       binding.Fence
	issuer      binding.Issuer
	address     InvocationAddress
	port        output.OutputPort
	spec        output.ContributionSpec
	destination binding.CellToken
	lineage     model.LineageRef
	typeID      model.TypeID
}

func contributionLawContent(value byte) identity.ContentID { return identity.ContentID{value} }

func newContributionLawFixture(t *testing.T) contributionLawFixture {
	t.Helper()
	owner, ok := model.IssueOwnerID(contributionLawContent(1))
	if !ok {
		t.Fatal("owner")
	}
	schema, ok := model.IssueSchemaID(owner, contributionLawContent(2))
	if !ok {
		t.Fatal("schema")
	}
	fence, ok := binding.NewFence(schema, identity.MountID{3}, identity.Generation(1))
	if !ok {
		t.Fatal("fence")
	}
	foreign, ok := binding.NewFence(schema, identity.MountID{4}, identity.Generation(1))
	if !ok {
		t.Fatal("foreign fence")
	}
	relation, ok := model.IssueRelationID(owner, contributionLawContent(5))
	if !ok {
		t.Fatal("relation")
	}
	column, ok := model.IssueColumnID(relation, contributionLawContent(6))
	if !ok {
		t.Fatal("column")
	}
	key, ok := model.IssueKeyID(relation, contributionLawContent(7))
	if !ok {
		t.Fatal("key")
	}
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	typeID, ok := model.IssueTypeID(owner, contributionLawContent(8))
	if !ok {
		t.Fatal("type")
	}
	capability, ok := model.NewAscendingCapability(typeID)
	if !ok {
		t.Fatal("capability")
	}
	operation, ok := model.IssueOperationID(owner, contributionLawContent(9))
	if !ok {
		t.Fatal("operation")
	}
	port := output.OutputPort{Operation: signature.Identity{Operation: operation, Version: 1}, Column: column}
	outcomes, ok := outcome.NewSet(outcome.Produced, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	sealedSignature, ok := signature.Seal(signature.Spec{
		Identity:    signature.Identity{Operation: operation, Version: 1},
		Fence:       signature.Fence{Owner: owner, Schema: schema},
		Outputs:     []signature.Output{{Relation: relation, Column: column, Type: typeID, Presence: signature.ProducePresent, Denominator: denominator}},
		Cardinality: mustCardinality(t, model.ExactlyOne),
		Outcomes:    outcomes,
	})
	if !ok {
		t.Fatal("signature")
	}
	spec, ok := output.Seal(output.Spec{Signature: sealedSignature, Port: port, ValueType: typeID, Algebra: capability, Reducer: output.Contributions})
	if !ok {
		t.Fatal("contribution spec")
	}
	membership, ok := binding.NewMembershipView(relation, []model.RowID{mustRow(t, relation, 10)})
	if !ok {
		t.Fatal("membership")
	}
	issuer, ok := binding.NewIssuer(fence)
	if !ok {
		t.Fatal("issuer")
	}
	witness, ok := issuer.IssueDenominator(denominator, membership, contributionLawContent(10))
	if !ok {
		t.Fatal("witness")
	}
	scope, ok := issuer.IssueScope(contributionLawContent(11))
	if !ok {
		t.Fatal("scope")
	}
	row, _ := membership.At(0)
	destination, ok := issuer.IssueCell(witness, scope, column, row)
	if !ok {
		t.Fatal("destination")
	}
	tuple, ok := NewTupleSources([]model.RowID{row})
	if !ok {
		t.Fatal("tuple")
	}
	vector, ok := NewSourceVector([]TupleSources{tuple})
	if !ok {
		t.Fatal("vector")
	}
	address, ok := New(scope, []SourceVector{vector})
	if !ok {
		t.Fatal("address")
	}
	lineage, ok := model.IssueLineageRef(owner, contributionLawContent(12))
	if !ok {
		t.Fatal("lineage")
	}
	return contributionLawFixture{owner: owner, foreign: foreign, fence: fence, issuer: issuer, address: address, port: port, spec: spec, destination: destination, lineage: lineage, typeID: typeID}
}

func mustCardinality(t *testing.T, kind model.CardinalityKind) model.Cardinality {
	t.Helper()
	value, ok := model.NewCardinality(kind, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	return value
}

func mustRow(t *testing.T, relation model.RelationID, content byte) model.RowID {
	t.Helper()
	value, ok := model.IssueRowID(relation, contributionLawContent(content))
	if !ok {
		t.Fatal("row")
	}
	return value
}

func (fixture contributionLawFixture) side(t *testing.T, value byte) binding.ContributionSide {
	t.Helper()
	payload, ok := fixture.issuer.IssueValue(fixture.typeID, contributionLawContent(value))
	if !ok {
		t.Fatal("value")
	}
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		t.Fatal("presence")
	}
	result, ok := binding.NewContributionSide(payload, presence, fixture.lineage)
	if !ok {
		t.Fatal("side")
	}
	return result
}

func TestContributionTransportCarriesAfterWithoutInventingBeforeAbsence(t *testing.T) {
	fixture := newContributionLawFixture(t)
	after := fixture.side(t, 20)
	transition, ok := NewContributionTransition(fixture.spec, fixture.address, fixture.destination, fixture.fence, binding.NoContributionSide(), after)
	if !ok || !transition.Available() || transition.Replacement() {
		t.Fatal("after transition refused or misclassified")
	}
	if _, present := transition.Before(); present {
		t.Fatal("after-only transition invented a Before side")
	}
	got, present := transition.After()
	if !present || !got.Same(after) || transition.Port() != fixture.port || !transition.Destination().Same(fixture.destination) {
		t.Fatal("after payload was not retained exactly")
	}
}

func TestContributionTransportCarriesBeforeAsExactRemoval(t *testing.T) {
	fixture := newContributionLawFixture(t)
	before := fixture.side(t, 21)
	transition, ok := NewContributionTransition(fixture.spec, fixture.address, fixture.destination, fixture.fence, before, binding.NoContributionSide())
	if !ok || !transition.Available() || transition.Replacement() {
		t.Fatal("before transition refused or misclassified")
	}
	got, present := transition.Before()
	if !present || !got.Same(before) {
		t.Fatal("before payload was not retained exactly")
	}
	if _, present := transition.After(); present {
		t.Fatal("before-only transition invented an After side")
	}
}

func TestContributionTransportCarriesReplacementAtomically(t *testing.T) {
	fixture := newContributionLawFixture(t)
	before, after := fixture.side(t, 22), fixture.side(t, 23)
	transition, ok := NewContributionTransition(fixture.spec, fixture.address, fixture.destination, fixture.fence, before, after)
	if !ok || !transition.Available() || !transition.Replacement() {
		t.Fatal("replacement transition refused or lost a side")
	}
	gotBefore, beforeOK := transition.Before()
	gotAfter, afterOK := transition.After()
	if !beforeOK || !afterOK || !gotBefore.Same(before) || !gotAfter.Same(after) {
		t.Fatal("replacement sides were not exact")
	}
}

func TestContributionTransportRefusesMalformedAndForeignPayloads(t *testing.T) {
	fixture := newContributionLawFixture(t)
	if _, ok := NewContributionTransition(fixture.spec, fixture.address, fixture.destination, fixture.fence, binding.NoContributionSide(), binding.NoContributionSide()); ok {
		t.Fatal("transition with no supplied side accepted")
	}
	foreignIssuer, ok := binding.NewIssuer(fixture.foreign)
	if !ok {
		t.Fatal("foreign issuer")
	}
	foreignValue, ok := foreignIssuer.IssueValue(fixture.typeID, contributionLawContent(24))
	if !ok {
		t.Fatal("foreign value")
	}
	presence, _ := model.NewPresence(model.Present)
	foreignSide, ok := binding.NewContributionSide(foreignValue, presence, fixture.lineage)
	if !ok {
		t.Fatal("foreign side")
	}
	if _, ok := NewContributionTransition(fixture.spec, fixture.address, fixture.destination, fixture.fence, binding.NoContributionSide(), foreignSide); ok {
		t.Fatal("foreign fenced payload accepted")
	}
	foreignRelation, ok := model.IssueRelationID(fixture.owner, contributionLawContent(27))
	if !ok {
		t.Fatal("foreign relation")
	}
	foreignColumn, ok := model.IssueColumnID(foreignRelation, contributionLawContent(28))
	if !ok {
		t.Fatal("foreign column")
	}
	foreignKey, ok := model.IssueKeyID(foreignRelation, contributionLawContent(29))
	if !ok {
		t.Fatal("foreign key")
	}
	foreignDenominator, ok := model.NewDenominatorRef(foreignRelation, foreignKey)
	if !ok {
		t.Fatal("foreign denominator")
	}
	foreignRow := mustRow(t, foreignRelation, 30)
	foreignMembership, ok := binding.NewMembershipView(foreignRelation, []model.RowID{foreignRow})
	if !ok {
		t.Fatal("foreign membership")
	}
	foreignWitness, ok := fixture.issuer.IssueDenominator(foreignDenominator, foreignMembership, contributionLawContent(31))
	if !ok {
		t.Fatal("foreign witness")
	}
	foreignScope, ok := fixture.issuer.IssueScope(contributionLawContent(32))
	if !ok {
		t.Fatal("foreign scope")
	}
	foreignDestination, ok := fixture.issuer.IssueCell(foreignWitness, foreignScope, foreignColumn, foreignRow)
	if !ok {
		t.Fatal("foreign destination")
	}
	if _, ok := NewContributionTransition(fixture.spec, fixture.address, foreignDestination, fixture.fence, binding.NoContributionSide(), fixture.side(t, 29)); ok {
		t.Fatal("foreign destination relation accepted")
	}
}
