package witness_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

type mountInventory struct {
	fence    address.Fence
	relation model.RelationID
	column   model.ColumnID
	key      model.KeyID
	scope    model.ScopeID
	scope2   model.ScopeID
	typeID   model.TypeID
	region   witness.Region
	region2  witness.Region
}

func (inventory *mountInventory) Fence() address.Fence { return inventory.fence }
func (inventory *mountInventory) ResolveRelation(id model.RelationID) (uint64, bool) {
	return 1, id == inventory.relation
}
func (inventory *mountInventory) ResolveColumn(id model.ColumnID) (uint64, bool) {
	return 2, id == inventory.column
}
func (inventory *mountInventory) ResolveKey(id model.KeyID) (uint64, bool) {
	return 3, id == inventory.key
}
func (inventory *mountInventory) ResolveScope(id model.ScopeID) (uint64, bool) {
	if id == inventory.scope {
		return 4, true
	}
	if inventory.scope2.Available() && id == inventory.scope2 {
		return 5, true
	}
	return 0, false
}
func (inventory *mountInventory) ResolveExpression(model.ExpressionID) (uint64, bool) {
	return 0, false
}
func (inventory *mountInventory) ResolveDependency(model.DependencyID) (uint64, bool) {
	return 0, false
}
func (inventory *mountInventory) Resolve(access arrangement.Access) (arrangement.Handle, bool) {
	// Keep the fake physical coordinates distinct for the three certificate
	// access shapes used by this law: authority key, output vector, and input
	// semantic access. Arrangement itself owns the real coordinate semantics.
	slot := uint64(2) // output vector
	if access.Key().Available() {
		slot = 1 // authority key
	}
	if access.Key().Available() && len(access.Columns()) != 0 {
		slot = 3 // input semantic access
	}
	return arrangement.NewHandle(inventory.fence, slot)
}
func (inventory *mountInventory) ScopeRegion(id model.ScopeID) (witness.Region, bool) {
	if id == inventory.scope {
		return inventory.region, true
	}
	if inventory.scope2.Available() && id == inventory.scope2 {
		return inventory.region2, inventory.region2 != nil
	}
	return nil, false
}
func (inventory *mountInventory) ResolveDenominator(model.DenominatorRef) (witness.DenominatorEvidence, bool) {
	return witness.DenominatorEvidence{}, false
}

type algebraRegistry struct{ algebra testAlgebra }

func (registry algebraRegistry) Resolve(typeID model.TypeID) (binding.ValueAlgebra, bool) {
	return registry.algebra, registry.algebra.Type() == typeID
}

type testAlgebra struct{ typeID model.TypeID }

func (algebra testAlgebra) Type() model.TypeID { return algebra.typeID }
func (algebra testAlgebra) Join(left, right binding.ValueToken) (binding.ValueToken, bool) {
	if left.Type() != algebra.typeID || right.Type() != algebra.typeID {
		return binding.ValueToken{}, false
	}
	return right, true
}
func (algebra testAlgebra) Widen(left, right binding.ValueToken) (binding.ValueToken, bool) {
	return algebra.Join(left, right)
}
func (algebra testAlgebra) LessOrEqual(left, right binding.ValueToken) bool {
	return left.Type() == algebra.typeID && right.Type() == algebra.typeID
}

type operationFactory struct{ value signature.Signature }

func (factory operationFactory) Bind(value signature.Signature) (binding.Binding, bool) {
	if value.Digest() != factory.value.Digest() {
		return nil, false
	}
	return operationBinding{value: value}, true
}

type operationBinding struct{ value signature.Signature }

func (bindingValue operationBinding) Signature() signature.Signature { return bindingValue.value }
func (bindingValue operationBinding) NewWorker(binding.Fence) (binding.Worker, bool) {
	return nil, false
}

func TestSpecializeAdmitsAddressArrangementRegionAndAlgebra(t *testing.T) {
	owner := issueOwner(t, "owner")
	schemaID := issueSchema(t, owner, "schema")
	relation := issueRelation(t, owner, "relation")
	column := issueColumn(t, relation, "column")
	key := issueKey(t, relation, "key")
	scope := issueScope(t, owner, "scope")
	scope2 := issueScope(t, owner, "scope2")
	typeID := issueType(t, owner, "type")
	builder := plan.NewBuilder(schemaID)
	if !builder.AddRelation(model.DefineRelationSchema(relation, []model.ColumnID{column}, []model.KeyID{key}, scope)) ||
		!builder.AddColumn(model.DefineColumnSchema(column, typeID)) ||
		!builder.AddKey(model.DefineKeySchema(key, []model.ColumnID{column})) ||
		!builder.AddScope(model.DefineScopeSchema(scope, nil)) ||
		!builder.AddScope(model.DefineScopeSchema(scope2, nil)) {
		t.Fatal("add declaration")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("build schema")
	}
	cert, refusal := certificate.Check(schema)
	if refusal != nil || !cert.Available() {
		t.Fatalf("check schema: %v", refusal)
	}
	store, ok := identity.IssueStore()
	if !ok {
		t.Fatal("issue store")
	}
	fence, ok := address.NewFence(schemaID, cert.Digest(), store, identity.MountID{1}, identity.Generation(1))
	if !ok {
		t.Fatal("new fence")
	}
	inventory := &mountInventory{fence: fence, relation: relation, column: column, key: key, scope: scope, scope2: scope2, typeID: typeID, region: finite("scope"), region2: finite("scope2")}
	mounted, ok := witness.Specialize(cert, inventory, nil, algebraRegistry{algebra: testAlgebra{typeID: typeID}})
	if !ok || !mounted.Available() {
		t.Fatal("valid mount refused")
	}
	if len(mounted.AlgebraRequirements()) != 1 || mounted.AlgebraRequirements()[0] != typeID {
		t.Fatal("algebra requirement was not admitted")
	}
	algebraValue, ok := mounted.Algebra(typeID)
	if !ok || algebraValue.Type() != typeID {
		t.Fatal("algebra lookup failed")
	}
	scopeValue, ok := mounted.Scope(scope)
	if !ok || !scopeValue.Available() {
		t.Fatal("scope lookup failed")
	}
	region, ok := mounted.RegionForScope(scopeValue)
	if !ok {
		t.Fatal("scope region lookup failed")
	}
	identityValue, ok := region.Identity()
	if !ok || identityValue != finite("scope").id {
		t.Fatal("scope region identity changed")
	}
	left, leftOK := mounted.Scope(scope)
	right, rightOK := mounted.Scope(scope2)
	if !leftOK || !rightOK {
		t.Fatal("second scope lookup failed")
	}
	joined, joinedOK := mounted.ConjoinScopes(left, right)
	if !joinedOK || !joined.Available() || !mounted.EntailsScopes(joined, left) || !mounted.EntailsScopes(joined, right) {
		t.Fatal("scope conjunction did not retain both formulas")
	}
	joinedRegion, joinedRegionOK := mounted.RegionForScope(joined)
	if !joinedRegionOK || !joinedRegion.Entails(region) {
		t.Fatal("dynamic scope formula was not recoverable")
	}
	reversed, reversedOK := mounted.ConjoinScopes(right, left)
	if !reversedOK || !joined.Same(reversed) {
		t.Fatal("scope conjunction was not canonical/commutative")
	}
	idempotent, idempotentOK := mounted.ConjoinScopes(left, left)
	if !idempotentOK || !idempotent.Same(left) || !mounted.EntailsScopes(left, left) {
		t.Fatal("scope conjunction was not idempotent/reflexive")
	}
	staleInventory := *inventory
	staleInventory.fence, _ = address.NewFence(schemaID, cert.Digest(), store, identity.MountID{1}, identity.Generation(2))
	staleMounted, staleOK := witness.Specialize(cert, &staleInventory, nil, algebraRegistry{algebra: testAlgebra{typeID: typeID}})
	if !staleOK {
		t.Fatal("stale fixture mount refused")
	}
	staleScope, staleScopeOK := staleMounted.Scope(scope)
	if !staleScopeOK {
		t.Fatal("stale scope lookup failed")
	}
	if _, ok := mounted.ConjoinScopes(left, staleScope); ok || mounted.EntailsScopes(left, staleScope) {
		t.Fatal("stale scope crossed the exact runtime fence")
	}
	if len(mounted.WideningHeads()) != 0 || len(mounted.Denominators()) != 0 {
		t.Fatal("empty certificate projections were not preserved")
	}
}

func TestSpecializeRefusesForeignFenceAndMissingRegion(t *testing.T) {
	owner := issueOwner(t, "hostile-owner")
	schemaID := issueSchema(t, owner, "hostile-schema")
	relation := issueRelation(t, owner, "hostile-relation")
	column := issueColumn(t, relation, "hostile-column")
	key := issueKey(t, relation, "hostile-key")
	scope := issueScope(t, owner, "hostile-scope")
	typeID := issueType(t, owner, "hostile-type")
	builder := plan.NewBuilder(schemaID)
	if !builder.AddRelation(model.DefineRelationSchema(relation, []model.ColumnID{column}, []model.KeyID{key}, scope)) || !builder.AddColumn(model.DefineColumnSchema(column, typeID)) || !builder.AddKey(model.DefineKeySchema(key, []model.ColumnID{column})) || !builder.AddScope(model.DefineScopeSchema(scope, nil)) {
		t.Fatal("add declaration")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("build schema")
	}
	cert, refusal := certificate.Check(schema)
	if refusal != nil {
		t.Fatalf("check schema: %v", refusal)
	}
	store, _ := identity.IssueStore()
	fence, _ := address.NewFence(schemaID, cert.Digest(), store, identity.MountID{1}, identity.Generation(1))
	base := &mountInventory{fence: fence, relation: relation, column: column, key: key, scope: scope, typeID: typeID, region: finite("hostile-scope")}
	if _, ok := witness.Specialize(cert, base, nil, algebraRegistry{algebra: testAlgebra{typeID: typeID}}); !ok {
		t.Fatal("baseline hostile fixture refused")
	}
	foreign := *base
	foreign.fence, _ = address.NewFence(schemaID, identity.ContentID{0: 9}, store, identity.MountID{2}, identity.Generation(1))
	if mounted, ok := witness.Specialize(cert, &foreign, nil, algebraRegistry{algebra: testAlgebra{typeID: typeID}}); ok || mounted.Available() {
		t.Fatal("foreign arrangement/address fence accepted")
	}
	missingRegion := *base
	missingRegion.region = nil
	if mounted, ok := witness.Specialize(cert, &missingRegion, nil, algebraRegistry{algebra: testAlgebra{typeID: typeID}}); ok || mounted.Available() {
		t.Fatal("missing scope formula accepted")
	}
}

func TestSpecializeAdmitsExactOperationAndRejectsWrongDenominatorRows(t *testing.T) {
	value := newSemanticAdmissionFixture(t)
	mounted, ok := witness.Specialize(value.cert, &value.evidence, operationFactory{value: value.operation}, algebraRegistry{algebra: testAlgebra{typeID: value.typeID}})
	if !ok || !mounted.Available() {
		t.Fatal("semantic mount refused")
	}
	if _, ok := mounted.Binding(value.operation.Identity()); !ok {
		t.Fatal("exact operation binding missing")
	}
	scopeValue, ok := mounted.Scope(value.scope)
	if !ok || !scopeValue.Available() {
		t.Fatal("admitted scope missing")
	}
	scopeToken, ok := mounted.ScopeToken(scopeValue)
	if !ok || !scopeToken.ValidFor(mounted.RuntimeFence()) {
		t.Fatal("narrow scope token projection refused")
	}
	if witnessValue, ok := mounted.Denominator(value.denominator); !ok || !witnessValue.Contains(value.row) {
		t.Fatal("denominator witness missing")
	}
	cell, ok := mounted.IssueCell(value.denominator, scopeValue, value.column, value.row)
	if !ok || !cell.ValidFor(mounted.RuntimeFence()) {
		t.Fatal("mounted cell issuance refused")
	}
	valueToken, ok := mounted.IssueValue(value.typeID, content(t, "semantic-value"))
	if !ok || !valueToken.ValidFor(mounted.RuntimeFence()) {
		t.Fatal("mounted value issuance refused")
	}
	foreignType := issueType(t, value.owner, "semantic-foreign-type")
	if _, ok := mounted.IssueValue(foreignType, content(t, "foreign-value")); ok {
		t.Fatal("value for unadmitted type accepted")
	}
	if index, ok := mounted.RowIndex(value.denominator, value.row); !ok || index != 0 {
		t.Fatalf("row index = %d/%v", index, ok)
	}
	if _, ok := mounted.RowIndex(value.denominator, model.RowID{}); ok {
		t.Fatal("zero row accepted")
	}
	duplicateEvidence := value.evidence
	duplicateEvidence.rows = []model.RowID{value.row, value.row}
	if duplicateMounted, duplicateOK := witness.Specialize(value.cert, &duplicateEvidence, operationFactory{value: value.operation}, algebraRegistry{algebra: testAlgebra{typeID: value.typeID}}); duplicateOK || duplicateMounted.Available() {
		t.Fatal("duplicate denominator row accepted")
	}
	foreignRelation := issueRelation(t, value.owner, "semantic-foreign-relation")
	foreignRow, foreignRowOK := model.IssueRowID(foreignRelation, content(t, "semantic-foreign-row"))
	if !foreignRowOK {
		t.Fatal("issue foreign row")
	}
	foreignEvidence := value.evidence
	foreignEvidence.rows = []model.RowID{foreignRow}
	if foreignMounted, foreignOK := witness.Specialize(value.cert, &foreignEvidence, operationFactory{value: value.operation}, algebraRegistry{algebra: testAlgebra{typeID: value.typeID}}); foreignOK || foreignMounted.Available() {
		t.Fatal("wrong-relation denominator row accepted")
	}
}

type semanticAdmissionFixture struct {
	owner       model.OwnerID
	schema      model.SchemaID
	relation    model.RelationID
	column      model.ColumnID
	key         model.KeyID
	scope       model.ScopeID
	typeID      model.TypeID
	cert        certificate.Certificate
	operation   signature.Signature
	denominator model.DenominatorRef
	row         model.RowID
	evidence    evidenceInventory
}

func newSemanticAdmissionFixture(t *testing.T) semanticAdmissionFixture {
	t.Helper()
	owner := issueOwner(t, "semantic-owner")
	schemaID := issueSchema(t, owner, "semantic-schema")
	relation := issueRelation(t, owner, "semantic-relation")
	column := issueColumn(t, relation, "semantic-column")
	key := issueKey(t, relation, "semantic-key")
	scope := issueScope(t, owner, "semantic-scope")
	typeID := issueType(t, owner, "semantic-type")
	operation, ok := model.IssueOperationID(owner, content(t, "semantic-operation"))
	if !ok {
		t.Fatal("issue operation")
	}
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("issue denominator")
	}
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	outcomes, ok := outcome.NewSet(outcome.Produced, outcome.NoSelection, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	operationValue, ok := signature.Seal(signature.Spec{
		Identity:  signature.Identity{Operation: operation, Version: 1},
		Fence:     signature.Fence{Owner: owner, Schema: schemaID},
		Inputs:    []signature.Input{{Relation: relation, Column: column, Type: typeID, Presence: signature.RequirePresent, Delivery: delivery, Denominator: denominator}},
		Outputs:   []signature.Output{{Relation: relation, Column: column, Type: typeID, Presence: signature.ProducePresent}},
		Authority: signature.OutputAuthority{Denominator: denominator}, Cardinality: cardinality, Outcomes: outcomes,
	})
	if !ok {
		t.Fatal("seal operation")
	}
	builder := plan.NewBuilder(schemaID)
	if !builder.AddRelation(model.DefineRelationSchema(relation, []model.ColumnID{column}, []model.KeyID{key}, scope)) || !builder.AddColumn(model.DefineColumnSchema(column, typeID)) || !builder.AddKey(model.DefineKeySchema(key, []model.ColumnID{column})) || !builder.AddScope(model.DefineScopeSchema(scope, nil)) || !builder.AddSignature(operationValue) {
		t.Fatal("add semantic declaration")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("build semantic schema")
	}
	cert, refusal := certificate.Check(schema)
	if refusal != nil || !cert.Available() {
		t.Fatalf("check semantic schema: %v", refusal)
	}
	store, ok := identity.IssueStore()
	if !ok {
		t.Fatal("issue store")
	}
	fence, ok := address.NewFence(schemaID, cert.Digest(), store, identity.MountID{3}, identity.Generation(1))
	if !ok {
		t.Fatal("issue fence")
	}
	row, ok := model.IssueRowID(relation, content(t, "semantic-row"))
	if !ok {
		t.Fatal("issue row")
	}
	semanticInventory := &mountInventory{fence: fence, relation: relation, column: column, key: key, scope: scope, typeID: typeID, region: finite("semantic-scope")}
	evidence := evidenceInventory{mountInventory: semanticInventory, denominator: denominator, rows: []model.RowID{row}, evidence: content(t, "semantic-evidence")}
	return semanticAdmissionFixture{owner: owner, schema: schemaID, relation: relation, column: column, key: key, scope: scope, typeID: typeID, cert: cert, operation: operationValue, denominator: denominator, row: row, evidence: evidence}
}

type evidenceInventory struct {
	*mountInventory
	denominator model.DenominatorRef
	rows        []model.RowID
	evidence    identity.ContentID
}

func (inventory *evidenceInventory) ResolveDenominator(ref model.DenominatorRef) (witness.DenominatorEvidence, bool) {
	if ref != inventory.denominator {
		return witness.DenominatorEvidence{}, false
	}
	return witness.NewDenominatorEvidence(inventory.rows, inventory.evidence)
}

func content(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("witness/specialize/test/v1", []byte(label))
	if !ok {
		t.Fatal("derive content")
	}
	return value
}

func issueOwner(t *testing.T, label string) model.OwnerID {
	value, ok := model.IssueOwnerID(content(t, "owner/"+label))
	if !ok {
		t.Fatal("issue owner")
	}
	return value
}
func issueSchema(t *testing.T, owner model.OwnerID, label string) model.SchemaID {
	value, ok := model.IssueSchemaID(owner, content(t, "schema/"+label))
	if !ok {
		t.Fatal("issue schema")
	}
	return value
}
func issueRelation(t *testing.T, owner model.OwnerID, label string) model.RelationID {
	value, ok := model.IssueRelationID(owner, content(t, "relation/"+label))
	if !ok {
		t.Fatal("issue relation")
	}
	return value
}
func issueColumn(t *testing.T, relation model.RelationID, label string) model.ColumnID {
	value, ok := model.IssueColumnID(relation, content(t, "column/"+label))
	if !ok {
		t.Fatal("issue column")
	}
	return value
}
func issueKey(t *testing.T, relation model.RelationID, label string) model.KeyID {
	value, ok := model.IssueKeyID(relation, content(t, "key/"+label))
	if !ok {
		t.Fatal("issue key")
	}
	return value
}
func issueScope(t *testing.T, owner model.OwnerID, label string) model.ScopeID {
	value, ok := model.IssueScopeID(owner, content(t, "scope/"+label))
	if !ok {
		t.Fatal("issue scope")
	}
	return value
}
func issueType(t *testing.T, owner model.OwnerID, label string) model.TypeID {
	value, ok := model.IssueTypeID(owner, content(t, "type/"+label))
	if !ok {
		t.Fatal("issue type")
	}
	return value
}
