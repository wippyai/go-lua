package arrangement_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/schema/semantic/output"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

type contributionArrangementFixture struct {
	certificate  certificate.Certificate
	relation     model.RelationID
	column       model.ColumnID
	key          model.KeyID
	scope        model.ScopeID
	expression   model.ExpressionID
	dependency   model.DependencyID
	contribution output.ContributionSpec
}

func newContributionArrangementFixture(t *testing.T, includeContribution bool) contributionArrangementFixture {
	t.Helper()
	owner, ok := model.IssueOwnerID(contributionArrangementToken("owner"))
	if !ok {
		t.Fatal("owner")
	}
	schemaID, ok := model.IssueSchemaID(owner, contributionArrangementToken("schema"))
	if !ok {
		t.Fatal("schema")
	}
	relation, ok := model.IssueRelationID(owner, contributionArrangementToken("relation"))
	if !ok {
		t.Fatal("relation")
	}
	column, ok := model.IssueColumnID(relation, contributionArrangementToken("column"))
	if !ok {
		t.Fatal("column")
	}
	key, ok := model.IssueKeyID(relation, contributionArrangementToken("key"))
	if !ok {
		t.Fatal("key")
	}
	typeID, ok := model.IssueTypeID(owner, contributionArrangementToken("type"))
	if !ok {
		t.Fatal("type")
	}
	scope, ok := model.IssueScopeID(owner, contributionArrangementToken("scope"))
	if !ok {
		t.Fatal("scope")
	}
	expression, ok := model.IssueExpressionID(owner, contributionArrangementToken("expression"))
	if !ok {
		t.Fatal("expression")
	}
	dependency, ok := model.IssueDependencyID(owner, contributionArrangementToken("dependency"))
	if !ok {
		t.Fatal("dependency")
	}
	operation, ok := model.IssueOperationID(owner, contributionArrangementToken("operation"))
	if !ok {
		t.Fatal("operation")
	}
	relationRef, ok := plan.NewRelationRef(relation)
	if !ok {
		t.Fatal("relation ref")
	}
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	outcomes, ok := outcome.NewSet(outcome.Produced, outcome.NoSelection, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	semantic, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: operation, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schemaID},
		Outputs: []signature.Output{{
			Relation: relation, Column: column, Type: typeID,
			Presence: signature.ProducePresent, Denominator: denominator,
		}},
		Cardinality: cardinality,
		Outcomes:    outcomes,
	})
	if !ok {
		t.Fatal("signature")
	}
	capability, ok := model.NewAscendingCapability(typeID)
	if !ok {
		t.Fatal("capability")
	}
	contribution, ok := output.Seal(output.Spec{
		Signature: semantic,
		Port:      output.OutputPort{Operation: semantic.Identity(), Column: column},
		ValueType: typeID, Algebra: capability, Reducer: output.Contributions,
	})
	if !ok {
		t.Fatal("contribution")
	}
	builder := plan.NewBuilder(schemaID)
	if !builder.AddRelation(model.DefineRelationSchema(relation, []model.ColumnID{column}, []model.KeyID{key}, scope)) ||
		!builder.AddColumn(model.DefineColumnSchema(column, typeID)) ||
		!builder.AddKey(model.DefineKeySchema(key, []model.ColumnID{column})) ||
		!builder.AddScope(model.DefineScopeSchema(scope, nil, region.True())) ||
		!builder.AddExpression(plan.DefineExpressionRef(expression, algebra.NewInput(relation))) ||
		!builder.AddDependency(plan.DefineDependency(dependency, expression, []plan.RelationRef{relationRef}, nil, "input")) ||
		!builder.AddSCC(plan.DefineSCC([]plan.DependencyRef{plan.DefineDependencyRef(dependency)}, nil, plan.DefineRecurrence(plan.Acyclic, nil))) ||
		!builder.AddSignature(semantic) ||
		!builder.AddTypeCapability(capability) {
		t.Fatal("declaration")
	}
	if includeContribution && !builder.AddContribution(contribution) {
		t.Fatal("contribution declaration")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("schema")
	}
	checked, refusal := certificate.Check(schema)
	if refusal != nil {
		t.Fatalf("certificate: %v", refusal)
	}
	return contributionArrangementFixture{
		certificate: checked, relation: relation, column: column, key: key,
		scope: scope, expression: expression, dependency: dependency,
		contribution: contribution,
	}
}

func contributionArrangementToken(label string) identity.ContentID {
	value, ok := identity.DeriveContentID("analysis/relation/mount/arrangement/contribution-law/v1", []byte(label))
	if !ok {
		panic("token")
	}
	return value
}

func (value contributionArrangementFixture) derive(t *testing.T) arrangement.Plan {
	t.Helper()
	store, ok := identity.IssueStore()
	if !ok {
		t.Fatal("store")
	}
	fence, ok := address.NewFence(value.certificate.SchemaID(), value.certificate.Digest(), store, identity.MountID{0: 23}, identity.Generation(1))
	if !ok {
		t.Fatal("fence")
	}
	addresses := &addressInventory{
		fence:        fence,
		relations:    map[model.RelationID]uint64{value.relation: 1},
		columns:      map[model.ColumnID]uint64{value.column: 2},
		keys:         map[model.KeyID]uint64{value.key: 3},
		scopes:       map[model.ScopeID]uint64{value.scope: 4},
		expressions:  map[model.ExpressionID]uint64{value.expression: 5},
		dependencies: map[model.DependencyID]uint64{value.dependency: 6},
	}
	book, ok := address.Bind(value.certificate, addresses)
	if !ok {
		t.Fatal("address bind")
	}
	derived, ok := arrangement.Derive(value.certificate, book, &arrangementInventory{fence: fence, slot: 11}, expand.EmptyCatalog(), []binding.PartitionDirectory{})
	if !ok || !derived.Available() {
		t.Fatal("arrangement derive")
	}
	return derived
}

func TestMountedPlanRetainsExactContributionAndDigest(t *testing.T) {
	without := newContributionArrangementFixture(t, false)
	with := newContributionArrangementFixture(t, true)
	withoutPlan := without.derive(t)
	withPlan := with.derive(t)
	if withoutPlan.LogicalDigest() == withPlan.LogicalDigest() || withoutPlan.Digest() == withPlan.Digest() {
		t.Fatal("mounted contribution declaration did not change plan digest")
	}
	resolved, ok := withPlan.Contribution(with.contribution.Port())
	if !ok || !resolved.Equal(with.contribution) {
		t.Fatal("mounted plan exact contribution lookup failed")
	}
	values := withPlan.Contributions()
	if len(values) != 1 || !values[0].Equal(with.contribution) {
		t.Fatalf("mounted contribution vector = %+v", values)
	}
	values[0] = output.ContributionSpec{}
	if got := withPlan.Contributions(); len(got) != 1 || !got[0].Equal(with.contribution) {
		t.Fatal("mounted contribution accessor exposed mutable state")
	}
	foreign, ok := model.IssueColumnID(with.relation, contributionArrangementToken("foreign-column"))
	if !ok {
		t.Fatal("foreign column")
	}
	if _, ok := withPlan.Contribution(output.OutputPort{Operation: with.contribution.Port().Operation, Column: foreign}); ok {
		t.Fatal("mounted plan resolved an undeclared output port")
	}
}

func TestContributionCellDescriptorRequiresExactOperationFenceAndColumn(t *testing.T) {
	fixture := newContributionArrangementFixture(t, true)
	mounted := fixture.derive(t)
	runtime, ok := binding.NewFence(mounted.Fence().SchemaID(), mounted.Fence().MountID(), mounted.Fence().Generation())
	if !ok {
		t.Fatal("runtime fence")
	}
	issuer, ok := binding.NewIssuer(runtime)
	if !ok {
		t.Fatal("issuer")
	}
	row, ok := model.IssueRowID(fixture.relation, contributionArrangementToken("row"))
	if !ok {
		t.Fatal("row")
	}
	membership, ok := binding.NewMembershipView(fixture.relation, []model.RowID{row})
	if !ok {
		t.Fatal("membership")
	}
	denominator, ok := model.NewDenominatorRef(fixture.relation, fixture.key)
	if !ok {
		t.Fatal("denominator")
	}
	witness, ok := issuer.IssueDenominator(denominator, membership, contributionArrangementToken("evidence"))
	if !ok {
		t.Fatal("witness")
	}
	scope, ok := issuer.IssueScope(contributionArrangementToken("scope"))
	if !ok {
		t.Fatal("scope")
	}
	cell, ok := issuer.IssueCell(witness, scope, fixture.column, row)
	if !ok {
		t.Fatal("cell")
	}
	descriptor, ok := mounted.ContributionCell(fixture.contribution.Port().Operation, cell)
	if !ok || !descriptor.Available() || !descriptor.ValidFor(mounted.Fence()) || descriptor.Spec().Digest() != fixture.contribution.Digest() || descriptor.Operation() != fixture.contribution.Port().Operation || descriptor.Column() != cell.Column() {
		t.Fatal("exact mounted output-cell descriptor was not redeemed")
	}

	foreignOperationID, ok := model.IssueOperationID(fixture.contribution.Port().Operation.Operation.Owner(), contributionArrangementToken("foreign-operation"))
	if !ok {
		t.Fatal("foreign operation")
	}
	foreignOperation := signature.Identity{Operation: foreignOperationID, Version: 1}
	if _, ok := mounted.ContributionCell(foreignOperation, cell); ok {
		t.Fatal("foreign operation classified through exact cell descriptor")
	}
	versionSibling := fixture.contribution.Port().Operation
	versionSibling.Version++
	if _, ok := mounted.ContributionCell(versionSibling, cell); ok {
		t.Fatal("sibling operation version classified through exact cell descriptor")
	}
	foreignColumn, ok := model.IssueColumnID(fixture.relation, contributionArrangementToken("foreign-column"))
	if !ok {
		t.Fatal("foreign column")
	}
	foreignCell, ok := issuer.IssueCell(witness, scope, foreignColumn, row)
	if !ok {
		t.Fatal("foreign cell")
	}
	if _, ok := mounted.ContributionCell(fixture.contribution.Port().Operation, foreignCell); ok {
		t.Fatal("unregistered column classified through exact cell descriptor")
	}

	foreignRuntime, ok := binding.NewFence(mounted.Fence().SchemaID(), identity.MountID{99}, mounted.Fence().Generation())
	if !ok {
		t.Fatal("foreign runtime fence")
	}
	foreignIssuer, ok := binding.NewIssuer(foreignRuntime)
	if !ok {
		t.Fatal("foreign issuer")
	}
	foreignScope, ok := foreignIssuer.IssueScope(contributionArrangementToken("foreign-scope"))
	if !ok {
		t.Fatal("foreign scope")
	}
	foreignWitness, ok := foreignIssuer.IssueDenominator(denominator, membership, contributionArrangementToken("foreign-evidence"))
	if !ok {
		t.Fatal("foreign witness")
	}
	foreignFenceCell, ok := foreignIssuer.IssueCell(foreignWitness, foreignScope, fixture.column, row)
	if !ok {
		t.Fatal("foreign fence cell")
	}
	if _, ok := mounted.ContributionCell(fixture.contribution.Port().Operation, foreignFenceCell); ok {
		t.Fatal("foreign mounted fence classified through exact cell descriptor")
	}
}
