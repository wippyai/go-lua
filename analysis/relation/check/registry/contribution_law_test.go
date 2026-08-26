package registry_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/registry"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/schema/semantic/output"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

type contributionRegistryFixture struct {
	owner         model.OwnerID
	schema        model.SchemaID
	operation     model.OperationID
	relation      model.RelationID
	column        model.ColumnID
	otherColumn   model.ColumnID
	key           model.KeyID
	typeID        model.TypeID
	capability    model.TypeCapability
	decodeOnly    model.TypeCapability
	semantic      signature.Signature
	otherSemantic signature.Signature
	contribution  output.ContributionSpec
}

func newContributionRegistryFixture(t *testing.T) contributionRegistryFixture {
	t.Helper()
	owner := issueOwner(t, "contribution-owner")
	schema := issueSchema(t, owner, "contribution-schema")
	operation := issueOperation(t, owner, "contribution-operation")
	relation := issueRelation(t, owner, "contribution-relation")
	column := issueColumn(t, relation, "contribution-column")
	otherColumn := issueColumn(t, relation, "contribution-other-column")
	key := issueKey(t, relation, "contribution-key")
	typeID := issueType(t, owner, "contribution-type")
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	capability, ok := model.NewAscendingCapability(typeID)
	if !ok {
		t.Fatal("capability")
	}
	decodeOnly, ok := model.NewDecodeOnlyCapability(typeID)
	if !ok {
		t.Fatal("decode-only capability")
	}
	identityValue := signature.Identity{Operation: operation, Version: 1}
	semantic, ok := signature.Seal(signature.Spec{
		Identity: identityValue,
		Fence:    signature.Fence{Owner: owner, Schema: schema},
		Outputs: []signature.Output{{
			Relation: relation, Column: column, Type: typeID,
			Presence: signature.ProducePresent, Denominator: denominator,
		}},
	})
	if !ok {
		t.Fatal("signature")
	}
	otherSemantic, ok := signature.Seal(signature.Spec{
		Identity: identityValue,
		Fence:    signature.Fence{Owner: owner, Schema: schema},
		Outputs: []signature.Output{{
			Relation: relation, Column: otherColumn, Type: typeID,
			Presence: signature.ProducePresent, Denominator: denominator,
		}},
	})
	if !ok {
		t.Fatal("other signature")
	}
	contribution, ok := output.Seal(output.Spec{
		Signature: semantic,
		Port:      output.OutputPort{Operation: identityValue, Column: column},
		ValueType: typeID,
		Algebra:   capability,
		Reducer:   output.Contributions,
	})
	if !ok {
		t.Fatal("contribution")
	}
	return contributionRegistryFixture{
		owner: owner, schema: schema, operation: operation, relation: relation,
		column: column, otherColumn: otherColumn, key: key, typeID: typeID,
		capability: capability, decodeOnly: decodeOnly, semantic: semantic,
		otherSemantic: otherSemantic, contribution: contribution,
	}
}

func issueOwner(t *testing.T, label string) model.OwnerID {
	t.Helper()
	value, ok := model.IssueOwnerID(token(label))
	if !ok {
		t.Fatal("owner")
	}
	return value
}

func issueSchema(t *testing.T, owner model.OwnerID, label string) model.SchemaID {
	t.Helper()
	value, ok := model.IssueSchemaID(owner, token(label))
	if !ok {
		t.Fatal("schema")
	}
	return value
}

func issueOperation(t *testing.T, owner model.OwnerID, label string) model.OperationID {
	t.Helper()
	value, ok := model.IssueOperationID(owner, token(label))
	if !ok {
		t.Fatal("operation")
	}
	return value
}

func issueRelation(t *testing.T, owner model.OwnerID, label string) model.RelationID {
	t.Helper()
	value, ok := model.IssueRelationID(owner, token(label))
	if !ok {
		t.Fatal("relation")
	}
	return value
}

func issueColumn(t *testing.T, relation model.RelationID, label string) model.ColumnID {
	t.Helper()
	value, ok := model.IssueColumnID(relation, token(label))
	if !ok {
		t.Fatal("column")
	}
	return value
}

func issueKey(t *testing.T, relation model.RelationID, label string) model.KeyID {
	t.Helper()
	value, ok := model.IssueKeyID(relation, token(label))
	if !ok {
		t.Fatal("key")
	}
	return value
}

func issueType(t *testing.T, owner model.OwnerID, label string) model.TypeID {
	t.Helper()
	value, ok := model.IssueTypeID(owner, token(label))
	if !ok {
		t.Fatal("type")
	}
	return value
}

func token(label string) identity.ContentID {
	value, ok := identity.DeriveContentID("relation/check/registry/contribution-law/v1", []byte(label))
	if !ok {
		panic("token")
	}
	return value
}

func contributionSchema(t *testing.T, value contributionRegistryFixture, semantic signature.Signature, capability model.TypeCapability, duplicate bool) plan.ExecutionSchema {
	return contributionSchemaValue(t, value, semantic, capability, value.contribution, duplicate)
}

func contributionSchemaValue(t *testing.T, value contributionRegistryFixture, semantic signature.Signature, capability model.TypeCapability, contribution output.ContributionSpec, duplicate bool) plan.ExecutionSchema {
	t.Helper()
	builder := plan.NewBuilder(value.schema)
	if !builder.AddSignature(semantic) || !builder.AddTypeCapability(capability) || !builder.AddContribution(contribution) {
		t.Fatal("declaration")
	}
	if duplicate && !builder.AddContribution(contribution) {
		t.Fatal("duplicate declaration")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("schema")
	}
	return schema
}

func hasCode(view *registry.View, code registry.Code) bool {
	for _, issue := range view.Issues() {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func TestContributionRegistryValidatesExactPortAndCapability(t *testing.T) {
	value := newContributionRegistryFixture(t)
	valid := registry.Build(contributionSchema(t, value, value.semantic, value.capability, false))
	if !valid.Valid() {
		t.Fatalf("valid contribution refused: %v", valid.Issues())
	}
	resolved, ok := valid.Contribution(value.contribution.Port())
	if !ok || !resolved.Equal(value.contribution) {
		t.Fatal("exact contribution lookup failed")
	}

	foreignSignature := registry.Build(contributionSchema(t, value, value.otherSemantic, value.capability, false))
	if foreignSignature.Valid() || !hasCode(foreignSignature, registry.CodeContributionPort) {
		t.Fatalf("foreign signature output was admitted: %v", foreignSignature.Issues())
	}
	presenceOutputs := value.semantic.Outputs()
	presenceOutputs[0].Presence = signature.ProduceOpaque
	presenceSemantic, ok := signature.Seal(signature.Spec{
		Identity:    value.semantic.Identity(),
		Fence:       value.semantic.Fence(),
		Outputs:     presenceOutputs,
		Cardinality: value.semantic.Cardinality(),
	})
	if !ok {
		t.Fatal("presence signature")
	}
	presenceMismatch := registry.Build(contributionSchema(t, value, presenceSemantic, value.capability, false))
	if presenceMismatch.Valid() || !hasCode(presenceMismatch, registry.CodeContributionPort) {
		t.Fatalf("signature presence mismatch was admitted: %v", presenceMismatch.Issues())
	}

	foreignCapability := registry.Build(contributionSchema(t, value, value.semantic, value.decodeOnly, false))
	if foreignCapability.Valid() || !hasCode(foreignCapability, registry.CodeContributionCapability) {
		t.Fatalf("foreign capability was admitted: %v", foreignCapability.Issues())
	}

	optionalOutputs := value.semantic.Outputs()
	optionalOutputs[0].Presence = signature.ProduceOptional
	optionalSemantic, ok := signature.Seal(signature.Spec{
		Identity:    value.semantic.Identity(),
		Fence:       value.semantic.Fence(),
		Outputs:     optionalOutputs,
		Cardinality: value.semantic.Cardinality(),
	})
	if !ok {
		t.Fatal("optional signature")
	}
	optional := value.contribution
	// A ContributionSpec retains the exact presence from its admission
	// signature; this declaration is intentionally preserved by output.Seal,
	// then refused here until a closed-world producer denominator exists.
	optional, ok = output.Seal(output.Spec{
		Signature: optionalSemantic,
		Port:      value.contribution.Port(),
		ValueType: value.typeID,
		Algebra:   value.capability,
		Reducer:   output.Contributions,
	})
	if !ok {
		t.Fatal("optional contribution")
	}
	negative := registry.Build(contributionSchemaValue(t, value, optionalSemantic, value.capability, optional, false))
	if negative.Valid() || !hasCode(negative, registry.CodeContributionPresence) {
		t.Fatalf("negative presence contribution was admitted: %v", negative.Issues())
	}
}

func TestContributionRegistryRefusesDuplicateOutputPort(t *testing.T) {
	value := newContributionRegistryFixture(t)
	view := registry.Build(contributionSchema(t, value, value.semantic, value.capability, true))
	if view.Valid() || !hasCode(view, registry.CodeContributionDuplicate) {
		t.Fatalf("duplicate output port was admitted: %v", view.Issues())
	}
}
