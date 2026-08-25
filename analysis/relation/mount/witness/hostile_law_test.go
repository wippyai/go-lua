package witness_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
)

type basicMountFixture struct {
	mounted   witness.Mounted
	cert      certificate.Certificate
	inventory *mountInventory
	schema    model.SchemaID
	owner     model.OwnerID
	relation  model.RelationID
	column    model.ColumnID
	typeID    model.TypeID
	scope     model.ScopeID
	scope2    model.ScopeID
	store     identity.StoreID
}

func newBasicMountFixture(t *testing.T) basicMountFixture {
	t.Helper()
	owner := issueOwner(t, "hostile/basic-owner")
	schemaID := issueSchema(t, owner, "hostile/basic-schema")
	relation := issueRelation(t, owner, "hostile/basic-relation")
	column := issueColumn(t, relation, "hostile/basic-column")
	key := issueKey(t, relation, "hostile/basic-key")
	scope := issueScope(t, owner, "hostile/basic-scope")
	scope2 := issueScope(t, owner, "hostile/basic-scope2")
	typeID := issueType(t, owner, "hostile/basic-type")
	builder := plan.NewBuilder(schemaID)
	if !builder.AddRelation(model.DefineRelationSchema(relation, []model.ColumnID{column}, []model.KeyID{key}, scope)) ||
		!builder.AddColumn(model.DefineColumnSchema(column, typeID)) ||
		!builder.AddKey(model.DefineKeySchema(key, []model.ColumnID{column})) ||
		!builder.AddScope(model.DefineScopeSchema(scope, nil)) ||
		!builder.AddScope(model.DefineScopeSchema(scope2, nil)) {
		t.Fatal("basic declarations")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("basic schema")
	}
	cert, refusal := certificate.Check(schema)
	if refusal != nil || !cert.Available() {
		t.Fatalf("basic certificate: %v", refusal)
	}
	store, ok := identity.IssueStore()
	if !ok {
		t.Fatal("basic store")
	}
	fence, ok := address.NewFence(schemaID, cert.Digest(), store, identity.MountID{0x41}, identity.Generation(1))
	if !ok {
		t.Fatal("basic fence")
	}
	inventory := &mountInventory{
		fence: fence, relation: relation, column: column, key: key,
		scope: scope, scope2: scope2, typeID: typeID,
		region: finite("hostile/basic-region"), region2: finite("hostile/basic-region2"),
	}
	mounted, ok := witness.Specialize(cert, inventory, nil, algebraRegistry{algebra: testAlgebra{typeID: typeID}})
	if !ok || !mounted.Available() {
		t.Fatal("basic mount")
	}
	return basicMountFixture{mounted: mounted, cert: cert, inventory: inventory, schema: schemaID, owner: owner, relation: relation, column: column, typeID: typeID, scope: scope, scope2: scope2, store: store}
}

func TestMountedScopeConjunctionCanonicalAndFenceLaw(t *testing.T) {
	value := newBasicMountFixture(t)
	left, leftOK := value.mounted.Scope(value.scope)
	right, rightOK := value.mounted.Scope(value.scope2)
	if !leftOK || !rightOK {
		t.Fatal("scope admission")
	}
	joined, joinedOK := value.mounted.ConjoinScopes(left, right)
	if !joinedOK || !joined.Available() || !value.mounted.EntailsScopes(joined, left) || !value.mounted.EntailsScopes(joined, right) {
		t.Fatal("conjunction did not entail operands")
	}
	reversed, reversedOK := value.mounted.ConjoinScopes(right, left)
	if !reversedOK || !joined.Same(reversed) {
		t.Fatal("conjunction was not commutative/canonical")
	}
	idempotent, idempotentOK := value.mounted.ConjoinScopes(left, left)
	if !idempotentOK || !idempotent.Same(left) || !value.mounted.EntailsScopes(left, left) {
		t.Fatal("conjunction was not idempotent or entailment reflexive")
	}
	joinedRegion, regionOK := value.mounted.RegionForScope(joined)
	leftRegion, leftRegionOK := value.mounted.RegionForScope(left)
	if !regionOK || !leftRegionOK || !joinedRegion.Entails(leftRegion) {
		t.Fatal("dynamic conjunction was not recoverable")
	}

	staleInventory := *value.inventory
	staleInventory.fence, _ = address.NewFence(value.schema, value.cert.Digest(), value.store, identity.MountID{0x41}, identity.Generation(2))
	stale, staleOK := witness.Specialize(value.cert, &staleInventory, nil, algebraRegistry{algebra: testAlgebra{typeID: value.typeID}})
	if !staleOK {
		t.Fatal("stale mount fixture")
	}
	staleScope, staleScopeOK := stale.Scope(value.scope)
	if !staleScopeOK || staleScope.ValidFor(value.mounted.RuntimeFence()) {
		t.Fatal("stale scope retained current fence")
	}
	if _, ok := value.mounted.ConjoinScopes(left, staleScope); ok || value.mounted.EntailsScopes(left, staleScope) {
		t.Fatal("stale scope crossed current mount")
	}

	foreignInventory := *value.inventory
	foreignInventory.fence, _ = address.NewFence(value.schema, value.cert.Digest(), value.store, identity.MountID{0x42}, identity.Generation(1))
	foreign, foreignOK := witness.Specialize(value.cert, &foreignInventory, nil, algebraRegistry{algebra: testAlgebra{typeID: value.typeID}})
	if !foreignOK {
		t.Fatal("foreign mount fixture")
	}
	foreignScope, foreignScopeOK := foreign.Scope(value.scope)
	if !foreignScopeOK || foreignScope.ValidFor(value.mounted.RuntimeFence()) {
		t.Fatal("foreign scope retained current fence")
	}
	if _, ok := value.mounted.ConjoinScopes(left, foreignScope); ok {
		t.Fatal("foreign scope crossed current mount")
	}
}

func TestMountedExactWideningLookupRefusesUnadmitted(t *testing.T) {
	value := newBasicMountFixture(t)
	dependencyID, ok := model.IssueDependencyID(value.owner, content(t, "hostile/unadmitted-dependency"))
	if !ok {
		t.Fatal("dependency")
	}
	dependency := plan.DefineDependencyRef(dependencyID)
	relation, ok := plan.NewRelationRef(value.relation)
	if !ok {
		t.Fatal("relation ref")
	}
	if _, ok := value.mounted.Widening(dependency, relation); ok {
		t.Fatal("unadmitted widening head accepted")
	}
	if _, ok := value.mounted.Widening(plan.DependencyRef{}, plan.RelationRef{}); ok {
		t.Fatal("zero widening head accepted")
	}
}

func TestDenominatorEvidenceRejectsNilDuplicateAndPreservesOrder(t *testing.T) {
	owner := issueOwner(t, "hostile/evidence-owner")
	relation := issueRelation(t, owner, "hostile/evidence-relation")
	first, ok := model.IssueRowID(relation, content(t, "hostile/evidence-first"))
	if !ok {
		t.Fatal("first row")
	}
	second, ok := model.IssueRowID(relation, content(t, "hostile/evidence-second"))
	if !ok {
		t.Fatal("second row")
	}
	evidenceID := content(t, "hostile/evidence-proof")
	if _, ok := witness.NewDenominatorEvidence(nil, evidenceID); ok {
		t.Fatal("nil row vector accepted")
	}
	if _, ok := witness.NewDenominatorEvidence([]model.RowID{first, first}, evidenceID); ok {
		t.Fatal("duplicate row vector accepted")
	}
	rows := []model.RowID{first, second}
	evidence, ok := witness.NewDenominatorEvidence(rows, evidenceID)
	if !ok || !evidence.Available() {
		t.Fatal("ordered evidence refused")
	}
	rows[0] = second
	copyOf := evidence.Rows()
	if len(copyOf) != 2 || copyOf[0] != first || copyOf[1] != second {
		t.Fatal("evidence order was not frozen")
	}
}
