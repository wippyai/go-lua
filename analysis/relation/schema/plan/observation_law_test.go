package plan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

func TestObservationDescriptorIsSealedIntoExecutionSchemaDigest(t *testing.T) {
	owner, ok := model.IssueOwnerID(identity.ContentID{0x91})
	if !ok {
		t.Fatal("owner")
	}
	schemaID, ok := model.IssueSchemaID(owner, identity.ContentID{0x92})
	if !ok {
		t.Fatal("schema")
	}
	relation, ok := model.IssueRelationID(owner, identity.ContentID{0x93})
	if !ok {
		t.Fatal("relation")
	}
	column, ok := model.IssueColumnID(relation, identity.ContentID{0x94})
	if !ok {
		t.Fatal("column")
	}
	typeID, ok := model.IssueTypeID(owner, identity.ContentID{0x95})
	if !ok {
		t.Fatal("type")
	}
	key, ok := model.IssueKeyID(relation, identity.ContentID{0x96})
	if !ok {
		t.Fatal("key")
	}
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	scopeID, ok := model.IssueScopeID(owner, identity.ContentID{0x97})
	if !ok {
		t.Fatal("scope")
	}
	operationID, ok := model.IssueOperationID(owner, identity.ContentID{0x98})
	if !ok {
		t.Fatal("operation")
	}
	dependencyID, ok := model.IssueDependencyID(owner, identity.ContentID{0x99})
	if !ok {
		t.Fatal("dependency")
	}
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("delivery")
	}
	outcomes, ok := outcome.NewSet(outcome.Produced, outcome.NoSelection, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	operation, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: operationID, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schemaID},
		Inputs: []signature.Input{{
			Relation: relation, Column: column, Type: typeID,
			Presence: signature.RequirePresent, Delivery: delivery, Denominator: denominator,
		}},
		Outputs:     []signature.Output{{Relation: relation, Column: column, Type: typeID, Presence: signature.ProducePresent, Denominator: denominator}},
		Cardinality: mustCardinality(t), Outcomes: outcomes,
	})
	if !ok {
		t.Fatal("operation seal")
	}
	scope := model.DefineScopeSchema(scopeID, []model.ColumnID{column}, region.True())
	relationSchema := model.DefineRelationSchema(relation, []model.ColumnID{column}, []model.KeyID{key}, scopeID)
	columnSchema := model.DefineColumnSchema(column, typeID)
	keySchema := model.DefineKeySchema(key, []model.ColumnID{column})
	observation := algebra.NewObservationContract(
		dependencyID, operation.Identity(), algebra.NewObservationSource(0, 0, 0), denominator,
		algebra.NewObservationOutput(column, typeID, denominator, mustCardinality(t)),
	)
	build := func(include bool) ExecutionSchema {
		builder := NewBuilder(schemaID)
		if !builder.AddRelation(relationSchema) || !builder.AddColumn(columnSchema) || !builder.AddKey(keySchema) || !builder.AddScope(scope) || !builder.AddSignature(operation) {
			t.Fatal("declaration")
		}
		if include && !builder.AddObservation(observation) {
			t.Fatal("observation declaration")
		}
		value, buildOK := builder.Build()
		if !buildOK {
			t.Fatal("schema build")
		}
		return value
	}
	without := build(false)
	with := build(true)
	if !without.Available() || !with.Available() || len(with.Observations()) != 1 {
		t.Fatal("observation schema unavailable")
	}
	if without.Digest() == with.Digest() {
		t.Fatal("observation declaration did not affect schema digest")
	}
	if with.Observations()[0].Digest() != observation.Digest() {
		t.Fatal("schema changed observation identity")
	}
}

func mustCardinality(t *testing.T) model.Cardinality {
	t.Helper()
	value, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	return value
}
