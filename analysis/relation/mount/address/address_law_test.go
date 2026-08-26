package address_test

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
)

type fixture struct {
	certificate certificate.Certificate
	owner       model.OwnerID
	relationA   model.RelationID
	relationB   model.RelationID
	columnA     model.ColumnID
	columnB     model.ColumnID
	keyA        model.KeyID
	keyB        model.KeyID
	scopeA      model.ScopeID
	scopeB      model.ScopeID
	expressionA model.ExpressionID
	expressionB model.ExpressionID
	dependencyA model.DependencyID
	dependencyB model.DependencyID
}

type inventory struct {
	fence        address.Fence
	relations    map[model.RelationID]uint64
	columns      map[model.ColumnID]uint64
	keys         map[model.KeyID]uint64
	scopes       map[model.ScopeID]uint64
	expressions  map[model.ExpressionID]uint64
	dependencies map[model.DependencyID]uint64
	calls        map[string]int
}

func (value *inventory) Fence() address.Fence { return value.fence }
func (value *inventory) ResolveRelation(id model.RelationID) (uint64, bool) {
	value.calls["relation"]++
	slot, ok := value.relations[id]
	return slot, ok
}
func (value *inventory) ResolveColumn(id model.ColumnID) (uint64, bool) {
	value.calls["column"]++
	slot, ok := value.columns[id]
	return slot, ok
}
func (value *inventory) ResolveKey(id model.KeyID) (uint64, bool) {
	value.calls["key"]++
	slot, ok := value.keys[id]
	return slot, ok
}
func (value *inventory) ResolveScope(id model.ScopeID) (uint64, bool) {
	value.calls["scope"]++
	slot, ok := value.scopes[id]
	return slot, ok
}
func (value *inventory) ResolveExpression(id model.ExpressionID) (uint64, bool) {
	value.calls["expression"]++
	slot, ok := value.expressions[id]
	return slot, ok
}
func (value *inventory) ResolveDependency(id model.DependencyID) (uint64, bool) {
	value.calls["dependency"]++
	slot, ok := value.dependencies[id]
	return slot, ok
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	owner := issueOwner(t, "owner")
	schemaID := issueSchema(t, owner, "schema")
	relationA := issueRelation(t, owner, "relation-a")
	relationB := issueRelation(t, owner, "relation-b")
	columnA := issueColumn(t, relationA, "column-a")
	columnB := issueColumn(t, relationB, "column-b")
	typeA := issueType(t, owner, "type-a")
	typeB := issueType(t, owner, "type-b")
	keyA := issueKey(t, relationA, "key-a")
	keyB := issueKey(t, relationB, "key-b")
	scopeA := issueScope(t, owner, "scope-a")
	scopeB := issueScope(t, owner, "scope-b")
	expressionA := issueExpression(t, owner, "expression-a")
	expressionB := issueExpression(t, owner, "expression-b")
	dependencyA := issueDependency(t, owner, "dependency-a")
	dependencyB := issueDependency(t, owner, "dependency-b")

	refA, _ := plan.NewRelationRef(relationA)
	refB, _ := plan.NewRelationRef(relationB)
	exprA := plan.DefineExpressionRef(expressionA, algebra.NewInput(relationA))
	exprB := plan.DefineExpressionRef(expressionB, algebra.NewInput(relationB))
	depA := plan.DefineDependency(dependencyA, expressionA, []plan.RelationRef{refA}, nil, "a")
	depB := plan.DefineDependency(dependencyB, expressionB, []plan.RelationRef{refB}, nil, "b")
	dependencyRefA := plan.DefineDependencyRef(dependencyA)
	dependencyRefB := plan.DefineDependencyRef(dependencyB)
	sccA := plan.DefineSCC([]plan.DependencyRef{dependencyRefA}, nil, plan.DefineRecurrence(plan.Acyclic, nil))
	sccB := plan.DefineSCC([]plan.DependencyRef{dependencyRefB}, nil, plan.DefineRecurrence(plan.Acyclic, nil))
	builder := plan.NewBuilder(schemaID)
	for _, value := range []model.RelationSchema{
		model.DefineRelationSchema(relationA, []model.ColumnID{columnA}, []model.KeyID{keyA}, scopeA),
		model.DefineRelationSchema(relationB, []model.ColumnID{columnB}, []model.KeyID{keyB}, scopeB),
	} {
		if !builder.AddRelation(value) {
			t.Fatal("add relation")
		}
	}
	for _, value := range []model.ColumnSchema{model.DefineColumnSchema(columnA, typeA), model.DefineColumnSchema(columnB, typeB)} {
		if !builder.AddColumn(value) {
			t.Fatal("add column")
		}
	}
	for _, value := range []model.KeySchema{model.DefineKeySchema(keyA, []model.ColumnID{columnA}), model.DefineKeySchema(keyB, []model.ColumnID{columnB})} {
		if !builder.AddKey(value) {
			t.Fatal("add key")
		}
	}
	for _, value := range []model.ScopeSchema{model.DefineScopeSchema(scopeA, nil, region.True()), model.DefineScopeSchema(scopeB, nil, region.True())} {
		if !builder.AddScope(value) {
			t.Fatal("add scope")
		}
	}
	for _, value := range []plan.ExpressionRef{exprA, exprB} {
		if !builder.AddExpression(value) {
			t.Fatal("add expression")
		}
	}
	for _, value := range []plan.Dependency{depA, depB} {
		if !builder.AddDependency(value) {
			t.Fatal("add dependency")
		}
	}
	for _, value := range []plan.SCC{sccA, sccB} {
		if !builder.AddSCC(value) {
			t.Fatal("add SCC")
		}
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("build schema")
	}
	certificateValue, refusal := certificate.Check(schema)
	if refusal != nil {
		t.Fatalf("certificate refused: %v", refusal)
	}
	return fixture{certificate: certificateValue, owner: owner, relationA: relationA, relationB: relationB, columnA: columnA, columnB: columnB, keyA: keyA, keyB: keyB, scopeA: scopeA, scopeB: scopeB, expressionA: expressionA, expressionB: expressionB, dependencyA: dependencyA, dependencyB: dependencyB}
}

func (value fixture) inventory(t *testing.T, reverse bool) *inventory {
	t.Helper()
	store, ok := identity.IssueStore()
	if !ok {
		t.Fatal("issue store")
	}
	mount := identity.MountID{0: 7}
	generation := identity.Generation(1)
	fence, ok := address.NewFence(value.certificate.SchemaID(), value.certificate.Digest(), store, mount, generation)
	if !ok {
		t.Fatal("new fence")
	}
	inv := &inventory{
		fence:        fence,
		relations:    map[model.RelationID]uint64{value.relationA: 10, value.relationB: 11},
		columns:      map[model.ColumnID]uint64{value.columnA: 20, value.columnB: 21},
		keys:         map[model.KeyID]uint64{value.keyA: 30, value.keyB: 31},
		scopes:       map[model.ScopeID]uint64{value.scopeA: 40, value.scopeB: 41},
		expressions:  map[model.ExpressionID]uint64{value.expressionA: 50, value.expressionB: 51},
		dependencies: map[model.DependencyID]uint64{value.dependencyA: 60, value.dependencyB: 61},
		calls:        make(map[string]int),
	}
	if reverse {
		inv.relations[value.relationA], inv.relations[value.relationB] = 11, 10
		inv.columns[value.columnA], inv.columns[value.columnB] = 21, 20
		inv.keys[value.keyA], inv.keys[value.keyB] = 31, 30
		inv.scopes[value.scopeA], inv.scopes[value.scopeB] = 41, 40
		inv.expressions[value.expressionA], inv.expressions[value.expressionB] = 51, 50
		inv.dependencies[value.dependencyA], inv.dependencies[value.dependencyB] = 61, 60
	}
	return inv
}

func TestBindRoundTripAllCertifiedIdentityClasses(t *testing.T) {
	value := newFixture(t)
	inv := value.inventory(t, false)
	book, ok := address.Bind(value.certificate, inv)
	if !ok || !book.Available() {
		t.Fatal("valid inventory refused")
	}
	if got, ok := book.Relation(value.relationA); !ok || got.ID() != value.relationA || !got.ValidFor(book.Fence()) {
		t.Fatal("relation did not round-trip")
	}
	if got, ok := book.Column(value.columnA); !ok || got.ID() != value.columnA || !got.ValidFor(book.Fence()) {
		t.Fatal("column did not round-trip")
	}
	if got, ok := book.Key(value.keyA); !ok || got.ID() != value.keyA || !got.ValidFor(book.Fence()) {
		t.Fatal("key did not round-trip")
	}
	if got, ok := book.Scope(value.scopeA); !ok || got.ID() != value.scopeA || !got.ValidFor(book.Fence()) {
		t.Fatal("scope did not round-trip")
	}
	if got, ok := book.Expression(value.expressionA); !ok || got.ID() != value.expressionA || !got.ValidFor(book.Fence()) {
		t.Fatal("expression did not round-trip")
	}
	if got, ok := book.Dependency(value.dependencyA); !ok || got.ID() != value.dependencyA || !got.ValidFor(book.Fence()) {
		t.Fatal("dependency did not round-trip")
	}
	for _, name := range []string{"relation", "column", "key", "scope", "expression", "dependency"} {
		if inv.calls[name] != 2 {
			t.Fatalf("%s resolver called %d times, want exactly once per certified ID", name, inv.calls[name])
		}
	}
}

func TestBindRefusesMissingZeroForeignAndStaleFences(t *testing.T) {
	value := newFixture(t)
	base := value.inventory(t, false)
	zero := *base
	zero.fence = address.Fence{}
	if book, ok := address.Bind(value.certificate, &zero); ok || book.Available() {
		t.Fatal("zero fence accepted")
	}
	foreign := *base
	foreignFence, ok := address.NewFence(value.certificate.SchemaID(), identity.ContentID{0: 9}, base.fence.StoreID(), base.fence.MountID(), base.fence.Generation())
	if ok {
		foreign.fence = foreignFence
	} else {
		t.Fatal("foreign fence fixture unavailable")
	}
	if book, ok := address.Bind(value.certificate, &foreign); ok || book.Available() {
		t.Fatal("foreign certificate fence accepted")
	}
	missing := value.inventory(t, false)
	delete(missing.relations, value.relationA)
	if book, ok := address.Bind(value.certificate, missing); ok || book.Available() {
		t.Fatal("missing relation accepted")
	}
}

func TestBindRefusesDuplicateSlotsWithinEachNamespace(t *testing.T) {
	value := newFixture(t)
	for name := range map[string]struct{}{"relations": {}, "columns": {}, "keys": {}, "scopes": {}, "expressions": {}, "dependencies": {}} {
		inv := value.inventory(t, false)
		switch name {
		case "relations":
			inv.relations[value.relationB] = inv.relations[value.relationA]
		case "columns":
			inv.columns[value.columnB] = inv.columns[value.columnA]
		case "keys":
			inv.keys[value.keyB] = inv.keys[value.keyA]
		case "scopes":
			inv.scopes[value.scopeB] = inv.scopes[value.scopeA]
		case "expressions":
			inv.expressions[value.expressionB] = inv.expressions[value.expressionA]
		case "dependencies":
			inv.dependencies[value.dependencyB] = inv.dependencies[value.dependencyA]
		}
		if book, ok := address.Bind(value.certificate, inv); ok || book.Available() {
			t.Fatalf("duplicate %s slot accepted", name)
		}
	}
}

func TestBindRefusesZeroSlots(t *testing.T) {
	value := newFixture(t)
	for name := range map[string]struct{}{"relations": {}, "columns": {}, "keys": {}, "scopes": {}, "expressions": {}, "dependencies": {}} {
		inv := value.inventory(t, false)
		switch name {
		case "relations":
			inv.relations[value.relationA] = 0
		case "columns":
			inv.columns[value.columnA] = 0
		case "keys":
			inv.keys[value.keyA] = 0
		case "scopes":
			inv.scopes[value.scopeA] = 0
		case "expressions":
			inv.expressions[value.expressionA] = 0
		case "dependencies":
			inv.dependencies[value.dependencyA] = 0
		}
		if book, ok := address.Bind(value.certificate, inv); ok || book.Available() {
			t.Fatalf("zero %s slot accepted", name)
		}
	}
}

func TestBindIsDeterministicAndPhysicalReorderChangesOnlyBookDigest(t *testing.T) {
	value := newFixture(t)
	certificateDigest := value.certificate.Digest()
	firstInventory := value.inventory(t, false)
	first, ok := address.Bind(value.certificate, firstInventory)
	if !ok {
		t.Fatal("first bind refused")
	}
	replayInventory := value.inventory(t, false)
	replayInventory.fence = firstInventory.fence
	replay, ok := address.Bind(value.certificate, replayInventory)
	if !ok || first.Digest() != replay.Digest() {
		t.Fatal("equivalent binds were not deterministic")
	}
	reorderedInventory := value.inventory(t, true)
	reorderedInventory.fence = firstInventory.fence
	reordered, ok := address.Bind(value.certificate, reorderedInventory)
	if !ok || first.Digest() == reordered.Digest() {
		t.Fatal("physical reorder did not change book digest")
	}
	if value.certificate.Digest() != certificateDigest {
		t.Fatal("certificate identity changed")
	}
	if first.RelationIDs()[0] == (model.RelationID{}) || len(first.RelationIDs()) != 2 {
		t.Fatal("relation enumeration unavailable")
	}
	ids := first.RelationIDs()
	ids[0] = model.RelationID{}
	if first.RelationIDs()[0] == (model.RelationID{}) {
		t.Fatal("relation enumeration was not defensive")
	}
}

func TestAddressLogicalIdentityDoesNotDependOnLocalSlot(t *testing.T) {
	value := newFixture(t)
	first, ok := address.Bind(value.certificate, value.inventory(t, false))
	if !ok {
		t.Fatal("first bind refused")
	}
	secondInventory := value.inventory(t, true)
	second, ok := address.Bind(value.certificate, secondInventory)
	if !ok {
		t.Fatal("second bind refused")
	}
	left, ok := first.Relation(value.relationA)
	if !ok {
		t.Fatal("missing first address")
	}
	right, ok := second.Relation(value.relationA)
	if !ok {
		t.Fatal("missing second address")
	}
	if left.ID() != right.ID() || left.ID() != value.relationA {
		t.Fatal("local physical coordinate became logical identity")
	}
	other, ok := first.Relation(value.relationB)
	if !ok {
		t.Fatal("missing second relation address")
	}
	if left == other {
		t.Fatal("distinct typed addresses collapsed despite distinct logical IDs/slots")
	}
	staleFence, ok := address.NewFence(value.certificate.SchemaID(), value.certificate.Digest(), first.Fence().StoreID(), first.Fence().MountID(), first.Fence().Generation().Next())
	if !ok {
		t.Fatal("new stale fence")
	}
	if left.ValidFor(staleFence) {
		t.Fatal("stale address accepted")
	}
	foreignFence, ok := address.NewFence(value.certificate.SchemaID(), value.certificate.Digest(), first.Fence().StoreID(), identity.MountID{0: 99}, first.Fence().Generation())
	if !ok {
		t.Fatal("new foreign fence")
	}
	if left.ValidFor(foreignFence) {
		t.Fatal("foreign address accepted")
	}
}

func TestAddressKeepsItsCoordinatePrivateAndComparable(t *testing.T) {
	typeOfAddress := reflect.TypeOf(address.Address[model.RelationID]{})
	for index := 0; index < typeOfAddress.NumField(); index++ {
		if typeOfAddress.Field(index).PkgPath == "" {
			t.Fatalf("address field %q is exported", typeOfAddress.Field(index).Name)
		}
	}
	if _, ok := typeOfAddress.MethodByName("Slot"); ok {
		t.Fatal("address exposes a physical slot accessor")
	}
	var addresses map[address.Address[model.RelationID]]struct{}
	addresses = make(map[address.Address[model.RelationID]]struct{})
	_ = addresses
}

func TestBindSnapshotsInventoryBeforeItCanMutate(t *testing.T) {
	value := newFixture(t)
	inv := value.inventory(t, false)
	book, ok := address.Bind(value.certificate, inv)
	if !ok {
		t.Fatal("bind refused")
	}
	beforeDigest := book.Digest()
	before, ok := book.Relation(value.relationA)
	if !ok {
		t.Fatal("missing bound relation")
	}
	inv.relations[value.relationA] = 999
	inv.fence = address.Fence{}
	after, ok := book.Relation(value.relationA)
	if !ok || after.ID() != before.ID() || !after.ValidFor(book.Fence()) || book.Digest() != beforeDigest {
		t.Fatal("inventory mutation changed the immutable book")
	}
}

func issueOwner(t *testing.T, label string) model.OwnerID {
	id, ok := model.IssueOwnerID(token(t, label))
	if !ok {
		t.Fatal("issue owner")
	}
	return id
}
func issueSchema(t *testing.T, owner model.OwnerID, label string) model.SchemaID {
	id, ok := model.IssueSchemaID(owner, token(t, label))
	if !ok {
		t.Fatal("issue schema")
	}
	return id
}
func issueRelation(t *testing.T, owner model.OwnerID, label string) model.RelationID {
	id, ok := model.IssueRelationID(owner, token(t, label))
	if !ok {
		t.Fatal("issue relation")
	}
	return id
}
func issueColumn(t *testing.T, relation model.RelationID, label string) model.ColumnID {
	id, ok := model.IssueColumnID(relation, token(t, label))
	if !ok {
		t.Fatal("issue column")
	}
	return id
}
func issueType(t *testing.T, owner model.OwnerID, label string) model.TypeID {
	id, ok := model.IssueTypeID(owner, token(t, label))
	if !ok {
		t.Fatal("issue type")
	}
	return id
}
func issueKey(t *testing.T, relation model.RelationID, label string) model.KeyID {
	id, ok := model.IssueKeyID(relation, token(t, label))
	if !ok {
		t.Fatal("issue key")
	}
	return id
}
func issueScope(t *testing.T, owner model.OwnerID, label string) model.ScopeID {
	id, ok := model.IssueScopeID(owner, token(t, label))
	if !ok {
		t.Fatal("issue scope")
	}
	return id
}
func issueExpression(t *testing.T, owner model.OwnerID, label string) model.ExpressionID {
	id, ok := model.IssueExpressionID(owner, token(t, label))
	if !ok {
		t.Fatal("issue expression")
	}
	return id
}
func issueDependency(t *testing.T, owner model.OwnerID, label string) model.DependencyID {
	id, ok := model.IssueDependencyID(owner, token(t, label))
	if !ok {
		t.Fatal("issue dependency")
	}
	return id
}
func token(t *testing.T, label string) identity.ContentID {
	id, ok := identity.DeriveContentID("analysis/relation/mount/address/test/v1", []byte(label))
	if !ok {
		t.Fatal("derive token")
	}
	return id
}
