package model_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
)

func content(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("relation-model-law", []byte(label))
	if !ok {
		t.Fatalf("derive %q", label)
	}
	return value
}

func issueOwner(t *testing.T, label string) model.OwnerID {
	t.Helper()
	value, ok := model.IssueOwnerID(content(t, "owner/"+label))
	if !ok {
		t.Fatalf("issue owner %q", label)
	}
	return value
}

func issueRelation(t *testing.T, owner model.OwnerID, label string) model.RelationID {
	t.Helper()
	value, ok := model.IssueRelationID(owner, content(t, "relation/"+label))
	if !ok {
		t.Fatalf("issue relation %q", label)
	}
	return value
}

func issueColumn(t *testing.T, relation model.RelationID, label string) model.ColumnID {
	t.Helper()
	value, ok := model.IssueColumnID(relation, content(t, "column/"+label))
	if !ok {
		t.Fatalf("issue column %q", label)
	}
	return value
}

func issueKey(t *testing.T, relation model.RelationID, label string) model.KeyID {
	t.Helper()
	value, ok := model.IssueKeyID(relation, content(t, "key/"+label))
	if !ok {
		t.Fatalf("issue key %q", label)
	}
	return value
}

func scopeRegion(t *testing.T, label string) region.Region {
	t.Helper()
	atom, ok := region.NewAtom(content(t, "scope-region/"+label))
	if !ok {
		t.Fatalf("issue scope region atom %q", label)
	}
	value, ok := region.FromAtom(atom)
	if !ok {
		t.Fatalf("construct scope region %q", label)
	}
	return value
}

func TestOwnerIssuedIdentityIsOrderIndependentAndZeroSafe(t *testing.T) {
	authority := issueOwner(t, "stable")
	relation := issueRelation(t, authority, "values")
	column := issueColumn(t, relation, "value")
	key := issueKey(t, relation, "primary")

	relationAgain, ok := model.IssueRelationID(authority, relation.Content())
	if !ok || relationAgain != relation {
		t.Fatalf("relation identity changed with issuance order")
	}
	columnAgain, ok := model.IssueColumnID(relation, column.Content())
	if !ok || columnAgain != column {
		t.Fatalf("column identity changed with issuance order")
	}
	keyAgain, ok := model.IssueKeyID(relation, key.Content())
	if !ok || keyAgain != key {
		t.Fatalf("key identity changed with issuance order")
	}

	foreignAuthority := issueOwner(t, "foreign")
	foreignRelation, ok := model.IssueRelationID(foreignAuthority, relation.Content())
	if !ok || foreignRelation == relation {
		t.Fatalf("owner fence did not distinguish foreign relation")
	}
	if _, ok := model.IssueOwnerID(identity.ContentID{}); ok {
		t.Fatalf("zero owner token accepted")
	}
	if _, ok := model.IssueRelationID(model.OwnerID{}, content(t, "relation/zero-owner")); ok {
		t.Fatalf("zero relation owner accepted")
	}
	if _, ok := model.IssueRelationID(authority, identity.ContentID{}); ok {
		t.Fatalf("zero relation token accepted")
	}
}

func TestAdditionalNominalIdentityKindsRemainFenced(t *testing.T) {
	authority := issueOwner(t, "logical-identities")
	relation := issueRelation(t, authority, "rows")
	schemaToken := content(t, "schema")
	operationToken := content(t, "operation")
	typeToken := content(t, "type")
	rowToken := content(t, "row")

	schema, ok := model.IssueSchemaID(authority, schemaToken)
	if !ok || !schema.Available() || schema.Owner() != authority || schema.Content() != schemaToken {
		t.Fatalf("schema identity was not issued with its owner fence")
	}
	operation, ok := model.IssueOperationID(authority, operationToken)
	if !ok || !operation.Available() || operation.Owner() != authority || operation.Content() != operationToken {
		t.Fatalf("operation identity was not issued with its owner fence")
	}
	typeID, ok := model.IssueTypeID(authority, typeToken)
	if !ok || !typeID.Available() || typeID.Owner() != authority || typeID.Content() != typeToken {
		t.Fatalf("type identity was not issued with its owner fence")
	}
	row, ok := model.IssueRowID(relation, rowToken)
	if !ok || !row.Available() || row.Relation() != relation || row.Owner() != authority || row.Content() != rowToken {
		t.Fatalf("row identity was not issued with its relation fence")
	}

	schemaAgain, ok := model.IssueSchemaID(authority, schemaToken)
	if !ok || schemaAgain != schema {
		t.Fatalf("schema identity changed with issuance order")
	}
	operationAgain, ok := model.IssueOperationID(authority, operationToken)
	if !ok || operationAgain != operation {
		t.Fatalf("operation identity changed with issuance order")
	}
	typeAgain, ok := model.IssueTypeID(authority, typeToken)
	if !ok || typeAgain != typeID {
		t.Fatalf("type identity changed with issuance order")
	}
	rowAgain, ok := model.IssueRowID(relation, rowToken)
	if !ok || rowAgain != row {
		t.Fatalf("row identity changed with issuance order")
	}

	foreignAuthority := issueOwner(t, "logical-identities-foreign")
	foreignSchema, ok := model.IssueSchemaID(foreignAuthority, schemaToken)
	if !ok || foreignSchema == schema {
		t.Fatalf("schema owner fence did not distinguish foreign identity")
	}
	foreignOperation, ok := model.IssueOperationID(foreignAuthority, operationToken)
	if !ok || foreignOperation == operation {
		t.Fatalf("operation owner fence did not distinguish foreign identity")
	}
	foreignType, ok := model.IssueTypeID(foreignAuthority, typeToken)
	if !ok || foreignType == typeID {
		t.Fatalf("type owner fence did not distinguish foreign identity")
	}
	foreignRelation := issueRelation(t, authority, "rows-foreign")
	foreignRow, ok := model.IssueRowID(foreignRelation, rowToken)
	if !ok || foreignRow == row || foreignRow.Relation() != foreignRelation || foreignRow.Owner() != row.Owner() {
		t.Fatalf("row relation fence did not distinguish foreign identity")
	}

	if _, ok := model.IssueSchemaID(model.OwnerID{}, schemaToken); ok {
		t.Fatalf("zero schema owner accepted")
	}
	if _, ok := model.IssueOperationID(authority, identity.ContentID{}); ok {
		t.Fatalf("zero operation token accepted")
	}
	if _, ok := model.IssueTypeID(authority, identity.ContentID{}); ok {
		t.Fatalf("zero type token accepted")
	}
	if _, ok := model.IssueRowID(model.RelationID{}, rowToken); ok {
		t.Fatalf("zero row relation accepted")
	}
	if _, ok := model.IssueRowID(relation, identity.ContentID{}); ok {
		t.Fatalf("zero row token accepted")
	}
}

func TestPresenceIsClosedAndRefusedCarriesAnIdentity(t *testing.T) {
	kinds := []model.PresenceKind{
		model.Present,
		model.ProvenAbsent,
		model.UnprovenMissing,
		model.AuthenticatedOpaque,
	}
	for index, kind := range kinds {
		presence, ok := model.NewPresence(kind)
		if !ok || !presence.Available() || !presence.Is(kind) {
			t.Fatalf("kind %s did not construct", kind)
		}
		if _, ok := presence.Reason(); ok {
			t.Fatalf("non-refusal %s exposed a reason", kind)
		}
		for _, otherKind := range kinds[index+1:] {
			other, valid := model.NewPresence(otherKind)
			if !valid || presence == other {
				t.Fatalf("presence kinds collapsed: %s and %s", kind, otherKind)
			}
		}
	}
	if _, ok := model.NewPresence(model.Refused); ok {
		t.Fatalf("refused status accepted without reason")
	}
	if (model.Presence{}).Available() {
		t.Fatalf("zero presence is available")
	}

	authority := issueOwner(t, "refusal")
	reason, ok := model.IssueRefusalID(authority, content(t, "refused/missing-route"))
	if !ok {
		t.Fatalf("issue refusal reason")
	}
	refused, ok := model.NewRefused(reason)
	if !ok || !refused.Available() || !refused.Is(model.Refused) {
		t.Fatalf("valid refusal did not construct")
	}
	gotReason, ok := refused.Reason()
	if !ok || gotReason != reason {
		t.Fatalf("refusal reason was not preserved")
	}
	if _, ok := model.NewRefused(model.RefusalID{}); ok {
		t.Fatalf("zero refusal reason accepted")
	}
}

func TestDefinitionsAreImmutableCopies(t *testing.T) {
	authority := issueOwner(t, "immutable")
	relation := issueRelation(t, authority, "cells")
	first := issueColumn(t, relation, "first")
	second := issueColumn(t, relation, "second")
	key := issueKey(t, relation, "primary")
	typeID, ok := model.IssueTypeID(authority, content(t, "type/first"))
	if !ok {
		t.Fatalf("issue column type id")
	}
	scopeID, ok := model.IssueScopeID(authority, content(t, "scope/immutable"))
	if !ok {
		t.Fatalf("issue scope id")
	}

	dimensions := []model.ColumnID{first, second}
	columns := []model.ColumnID{first, second}
	keys := []model.KeyID{key}
	scope := model.DefineScopeSchema(scopeID, dimensions, scopeRegion(t, "immutable"))
	relationSchema := model.DefineRelationSchema(relation, columns, keys, scope.ID())
	keySchema := model.DefineKeySchema(key, dimensions)
	columnSchema := model.DefineColumnSchema(first, typeID)

	dimensions[0] = model.ColumnID{}
	columns[0] = model.ColumnID{}
	keys[0] = model.KeyID{}
	if !scope.HasDimension(first) || !relationSchema.HasColumn(first) || !relationSchema.HasKey(key) || relationSchema.Scope() != scope.ID() || len(keySchema.Columns()) != 2 || columnSchema.ID() != first || columnSchema.Type() != typeID {
		t.Fatalf("definition retained caller slice mutation")
	}

	returnedDimensions := scope.Dimensions()
	returnedDimensions[0] = model.ColumnID{}
	returnedColumns := relationSchema.Columns()
	returnedColumns[0] = model.ColumnID{}
	returnedKeys := relationSchema.Keys()
	returnedKeys[0] = model.KeyID{}
	keyColumns := keySchema.Columns()
	keyColumns[0] = model.ColumnID{}
	if !scope.HasDimension(first) || !relationSchema.HasColumn(first) || !relationSchema.HasKey(key) || keySchema.Columns()[0] != first {
		t.Fatalf("definition exposed mutable backing storage")
	}
}

func TestMalformedDefinitionsRemainRepresentableForChecker(t *testing.T) {
	primaryOwner := issueOwner(t, "primary")
	foreignOwner := issueOwner(t, "foreign")
	primaryRelation := issueRelation(t, primaryOwner, "primary")
	foreignRelation := issueRelation(t, foreignOwner, "foreign")
	primaryColumn := issueColumn(t, primaryRelation, "primary")
	foreignColumn := issueColumn(t, foreignRelation, "foreign")
	primaryKey := issueKey(t, primaryRelation, "primary")
	foreignKey := issueKey(t, foreignRelation, "foreign")
	primaryType, ok := model.IssueTypeID(primaryOwner, content(t, "type/primary"))
	if !ok {
		t.Fatalf("issue primary type id")
	}
	foreignType, ok := model.IssueTypeID(foreignOwner, content(t, "type/foreign"))
	if !ok {
		t.Fatalf("issue foreign type id")
	}
	foreignScopeID, ok := model.IssueScopeID(foreignOwner, content(t, "scope/foreign"))
	if !ok {
		t.Fatalf("issue foreign scope id")
	}

	// The model stores malformed references; the checker, not this package,
	// rejects foreign ownership, zero IDs, and duplicate declarations.
	foreignScope := model.DefineScopeSchema(foreignScopeID, []model.ColumnID{primaryColumn}, scopeRegion(t, "foreign"))
	foreignRelationSchema := model.DefineRelationSchema(primaryRelation, []model.ColumnID{foreignColumn, foreignColumn}, []model.KeyID{primaryKey, primaryKey}, foreignScope.ID())
	zeroKeySchema := model.DefineKeySchema(model.KeyID{}, []model.ColumnID{primaryColumn})
	zeroScopeRelation := model.DefineRelationSchema(primaryRelation, nil, nil, model.ScopeID{})
	foreignColumnSchema := model.DefineColumnSchema(primaryColumn, foreignType)
	zeroTypeColumnSchema := model.DefineColumnSchema(primaryColumn, model.TypeID{})
	if !foreignScope.Available() || !foreignRelationSchema.Available() || foreignRelationSchema.Scope() != foreignScope.ID() || zeroKeySchema.Available() || zeroScopeRelation.Available() || zeroScopeRelation.Scope() != (model.ScopeID{}) {
		t.Fatalf("definitions lost structural representation before checking")
	}
	if len(foreignRelationSchema.Columns()) != 2 || len(foreignRelationSchema.Keys()) != 2 || len(foreignScope.Dimensions()) != 1 {
		t.Fatalf("malformed definition was normalized away")
	}
	if !zeroKeySchema.Equal(model.DefineKeySchema(model.KeyID{}, []model.ColumnID{primaryColumn})) {
		t.Fatalf("structural equality rejected an invalid definition")
	}
	if !foreignColumnSchema.Available() || foreignColumnSchema.ID() != primaryColumn || foreignColumnSchema.Type() != foreignType {
		t.Fatalf("foreign column/type references were rejected before checking")
	}
	if zeroTypeColumnSchema.Available() || zeroTypeColumnSchema.Type().Available() {
		t.Fatalf("zero column type was not preserved as an invalid definition")
	}
	if primaryType == foreignType {
		t.Fatalf("test type identities unexpectedly collapsed")
	}
	if _, ok := model.NewDenominatorRef(primaryRelation, foreignKey); ok {
		t.Fatalf("foreign denominator key accepted")
	}
}

func TestCardinalityLineageAndDenominatorRemainIndependent(t *testing.T) {
	authority := issueOwner(t, "independent")
	relation := issueRelation(t, authority, "cells")
	key := issueKey(t, relation, "primary")

	exact, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok || !exact.Available() {
		t.Fatalf("exact cardinality rejected")
	}
	bounded, ok := model.NewCardinality(model.BoundedMany, 3)
	if !ok || !bounded.Available() {
		t.Fatalf("bounded cardinality rejected")
	}
	complete, ok := model.NewCardinality(model.CompleteDenominator, 0)
	if !ok || !complete.Available() || complete.Kind().String() != "CompleteDenominator" {
		t.Fatalf("complete-denominator cardinality rejected")
	}
	if _, ok := complete.Bound(); ok {
		t.Fatalf("complete-denominator cardinality exposed a numeric bound")
	}
	if _, ok := model.NewCardinality(model.CompleteDenominator, 1); ok {
		t.Fatalf("complete-denominator cardinality accepted a numeric bound")
	}
	if _, ok := model.NewCardinality(model.BoundedMany, 0); ok {
		t.Fatalf("zero bounded cardinality accepted")
	}
	if exact == bounded {
		t.Fatalf("cardinality kinds collapsed")
	}
	bound, ok := bounded.Bound()
	if !ok || bound != 3 {
		t.Fatalf("bounded cardinality lost its bound")
	}

	lineage, ok := model.IssueLineageRef(authority, content(t, "lineage"))
	if !ok || !lineage.Available() {
		t.Fatalf("lineage identity rejected")
	}
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok || !denominator.Available() || denominator.Relation() != relation || denominator.Key() != key {
		t.Fatalf("denominator identity rejected")
	}
	if _, ok := model.NewDenominatorRef(relation, model.KeyID{}); ok {
		t.Fatalf("zero denominator key accepted")
	}
}
