package index

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
)

type canonicalLawInventory struct {
	fence       address.Fence
	relation    model.RelationID
	column      model.ColumnID
	key         model.KeyID
	scope       model.ScopeID
	expression  model.ExpressionID
	denominator model.DenominatorRef
	row         model.RowID
	accesses    []arrangement.Access
}

func (inventory *canonicalLawInventory) Fence() address.Fence { return inventory.fence }
func (inventory *canonicalLawInventory) ResolveRelation(id model.RelationID) (uint64, bool) {
	return 1, id == inventory.relation
}
func (inventory *canonicalLawInventory) ResolveColumn(id model.ColumnID) (uint64, bool) {
	return 2, id == inventory.column
}
func (inventory *canonicalLawInventory) ResolveKey(id model.KeyID) (uint64, bool) {
	return 3, id == inventory.key
}
func (inventory *canonicalLawInventory) ResolveScope(id model.ScopeID) (uint64, bool) {
	return 4, id == inventory.scope
}
func (inventory *canonicalLawInventory) ResolveExpression(id model.ExpressionID) (uint64, bool) {
	return 5, id == inventory.expression
}
func (inventory *canonicalLawInventory) ResolveDependency(model.DependencyID) (uint64, bool) {
	return 0, false
}
func (inventory *canonicalLawInventory) Resolve(access arrangement.Access) (arrangement.Handle, bool) {
	for index, prior := range inventory.accesses {
		if prior.Equal(access) {
			return arrangement.NewHandle(inventory.fence, uint64(index+1))
		}
	}
	inventory.accesses = append(inventory.accesses, access)
	return arrangement.NewHandle(inventory.fence, uint64(len(inventory.accesses)))
}
func (inventory *canonicalLawInventory) ResolveDenominator(ref model.DenominatorRef) (witness.DenominatorEvidence, bool) {
	if ref != inventory.denominator {
		return witness.DenominatorEvidence{}, false
	}
	return witness.NewDenominatorEvidence([]model.RowID{inventory.row}, inventory.row.Content())
}
func (inventory *canonicalLawInventory) ResolveExpand(model.ExpandContract) ([]expand.Vector, bool) {
	return nil, false
}

type canonicalLawEquality struct{ typeID model.TypeID }

func (equality canonicalLawEquality) Type() model.TypeID { return equality.typeID }
func (equality canonicalLawEquality) Equal(left, right binding.ValueToken) bool {
	return left.Available() && right.Available() && left.Type() == equality.typeID && right.Type() == equality.typeID
}

type canonicalLawRegistry struct{ typeID model.TypeID }

func (registry canonicalLawRegistry) Resolve(typeID model.TypeID) (binding.ValueAlgebra, bool) {
	return nil, false
}
func (registry canonicalLawRegistry) ResolveEquality(typeID model.TypeID) (binding.ValueEquality, bool) {
	return canonicalLawEquality{typeID: registry.typeID}, registry.typeID == typeID
}

type canonicalLawFixture struct {
	relation model.RelationID
	first    model.RowID
	second   model.RowID
	region   support.Mask
	values   []binding.ValueToken
	mounted  witness.Mounted
}

func newCanonicalLawFixture(t *testing.T) canonicalLawFixture {
	t.Helper()
	content := func(label string) identity.ContentID {
		value, ok := identity.DeriveContentID("relation/state/index/canonical-law", []byte(label))
		if !ok {
			t.Fatalf("derive %s", label)
		}
		return value
	}
	owner, ok := model.IssueOwnerID(content("owner"))
	if !ok {
		t.Fatal("owner")
	}
	relation, ok := model.IssueRelationID(owner, content("relation"))
	if !ok {
		t.Fatal("relation")
	}
	first, ok := model.IssueRowID(relation, content("row-first"))
	if !ok {
		t.Fatal("first row")
	}
	second, ok := model.IssueRowID(relation, content("row-second"))
	if !ok {
		t.Fatal("second row")
	}
	typeID, ok := model.IssueTypeID(owner, content("type"))
	if !ok {
		t.Fatal("type")
	}
	schemaID, ok := model.IssueSchemaID(owner, content("schema"))
	if !ok {
		t.Fatal("schema")
	}
	column, ok := model.IssueColumnID(relation, content("column"))
	if !ok {
		t.Fatal("column")
	}
	key, ok := model.IssueKeyID(relation, content("key"))
	if !ok {
		t.Fatal("key")
	}
	scope, ok := model.IssueScopeID(owner, content("scope"))
	if !ok {
		t.Fatal("scope")
	}
	expression, ok := model.IssueExpressionID(owner, content("expression"))
	if !ok {
		t.Fatal("expression")
	}
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	builder := plan.NewBuilder(schemaID)
	capability, ok := model.NewEquatableCapability(typeID)
	if !ok || !builder.AddTypeCapability(capability) ||
		!builder.AddRelation(model.DefineRelationSchema(relation, []model.ColumnID{column}, []model.KeyID{key}, scope)) ||
		!builder.AddColumn(model.DefineColumnSchema(column, typeID)) ||
		!builder.AddKey(model.DefineKeySchema(key, []model.ColumnID{column})) ||
		!builder.AddScope(model.DefineScopeSchema(scope, nil, region.True())) ||
		!builder.AddExpression(plan.DefineExpressionRef(expression, algebra.NewGroup(algebra.NewInput(relation), algebra.NewGroupContract(key, cardinality)))) {
		t.Fatal("schema declarations")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("schema build")
	}
	cert, refusal := certificate.Check(schema)
	if refusal != nil || !cert.Available() {
		t.Fatalf("certificate: %v", refusal)
	}
	storeID, ok := identity.IssueStore()
	if !ok {
		t.Fatal("store")
	}
	fence, ok := address.NewFence(schemaID, cert.Digest(), storeID, identity.MountID{0xA1}, identity.Generation(1))
	if !ok {
		t.Fatal("fence")
	}
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	row := first
	inventory := &canonicalLawInventory{
		fence: fence, relation: relation, column: column, key: key, scope: scope,
		expression: expression, denominator: denominator, row: row,
	}
	lineageOwner, ok := model.IssueOwnerID(content("lineage-owner"))
	if !ok {
		t.Fatal("lineage owner")
	}
	lineageFactory, ok := lineage.NewFactory(lineageOwner)
	if !ok {
		t.Fatal("lineage factory")
	}
	mounted, ok := witness.Specialize(cert, inventory, nil, canonicalLawRegistry{typeID: typeID}, lineageFactory)
	if !ok || !mounted.Available() {
		t.Fatal("mounted witness")
	}
	value, ok := mounted.IssueValue(typeID, content("value"))
	if !ok {
		t.Fatal("value")
	}
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatalf("manager: %v", err)
	}
	region, ok := support.True(manager)
	if !ok {
		t.Fatal("region")
	}
	return canonicalLawFixture{relation: relation, first: first, second: second, region: region, values: []binding.ValueToken{value}, mounted: mounted}
}

func TestCanonicalRowsRejectEqualKeyWithDifferentOwnerRow(t *testing.T) {
	fixture := newCanonicalLawFixture(t)
	rows, ok := canonicalRowsWithEquality(fixture.mounted, []row{
		{key: geometry.Key(0), relation: fixture.relation, logical: fixture.first, region: fixture.region, values: fixture.values},
		{key: geometry.Key(1), relation: fixture.relation, logical: fixture.second, region: fixture.region, values: fixture.values},
	})
	if ok || rows != nil {
		t.Fatalf("equal keyed tuple with distinct owner rows was admitted: ok=%v rows=%v", ok, rows)
	}
}

func TestCanonicalRowsAllowsEqualKeyAcrossDisjointFibersWithOneOwnerRow(t *testing.T) {
	fixture := newCanonicalLawFixture(t)
	manager := fixture.region.Manager()
	work := support.New(manager)
	if work == nil {
		t.Fatal("support work")
	}
	left, ok := work.Literal(1, true)
	if !ok {
		t.Fatal("left fiber")
	}
	right, ok := work.Literal(1, false)
	if !ok || !work.Seal() {
		t.Fatal("right fiber")
	}
	rows, ok := canonicalRowsWithEquality(fixture.mounted, []row{
		{key: geometry.Key(0), relation: fixture.relation, logical: fixture.first, region: left, values: fixture.values},
		{key: geometry.Key(1), relation: fixture.relation, logical: fixture.first, region: right, values: fixture.values},
	})
	if !ok || len(rows) != 2 {
		t.Fatalf("same keyed tuple with one owner row was refused: ok=%v rows=%d", ok, len(rows))
	}
}

func TestCanonicalTrieRedeemsEquivalentOpaqueHandle(t *testing.T) {
	fixture := newCanonicalLawFixture(t)
	opaque, ok := identity.DeriveContentID("relation/state/index/canonical-law", []byte("value-second"))
	if !ok {
		t.Fatal("second opaque value")
	}
	second, ok := fixture.mounted.IssueValue(fixture.values[0].Type(), opaque)
	if !ok {
		t.Fatal("second value")
	}
	left := row{key: geometry.Key(0), relation: fixture.relation, logical: fixture.first, region: fixture.region, values: fixture.values}
	right := row{key: geometry.Key(0), relation: fixture.relation, logical: fixture.first, region: fixture.region, values: []binding.ValueToken{second}}
	if !semanticRowEqual(fixture.mounted, left, right) {
		t.Fatal("owner equality was replaced by opaque handle equality")
	}
	canonical, ok := canonicalRowsWithEquality(fixture.mounted, []row{left, right})
	if !ok || len(canonical) != 1 {
		t.Fatalf("equivalent handles were not one canonical row: ok=%v rows=%d", ok, len(canonical))
	}
	root := buildTrieWithMounted(canonical, 1, 0, fixture.mounted)
	if root == nil || len(root.children) != 1 {
		t.Fatalf("equivalent handles did not share one trie edge: root=%v children=%d", root != nil, len(root.children))
	}
	if got := findEdge(root.children, second, fixture.mounted); got != 0 {
		t.Fatalf("equivalent opaque query missed representative edge: position=%d", got)
	}
}
