package witness_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
)

type recurrenceFixture struct {
	owner       model.OwnerID
	schema      model.SchemaID
	relationA   model.RelationID
	relationB   model.RelationID
	columnA     model.ColumnID
	columnB     model.ColumnID
	keyA        model.KeyID
	keyB        model.KeyID
	scopeA      model.ScopeID
	scopeB      model.ScopeID
	typeID      model.TypeID
	dependencyA model.DependencyID
	dependencyB model.DependencyID
	expressionA model.ExpressionID
	expressionB model.ExpressionID
}

type recurrenceInventory struct {
	fence        address.Fence
	relations    map[model.RelationID]uint64
	columns      map[model.ColumnID]uint64
	keys         map[model.KeyID]uint64
	scopes       map[model.ScopeID]uint64
	expressions  map[model.ExpressionID]uint64
	dependencies map[model.DependencyID]uint64
	regions      map[model.ScopeID]witness.Region
	accesses     []arrangement.Access
}

func (inventory *recurrenceInventory) Fence() address.Fence { return inventory.fence }
func (inventory *recurrenceInventory) ResolveRelation(id model.RelationID) (uint64, bool) {
	value, ok := inventory.relations[id]
	return value, ok
}
func (inventory *recurrenceInventory) ResolveColumn(id model.ColumnID) (uint64, bool) {
	value, ok := inventory.columns[id]
	return value, ok
}
func (inventory *recurrenceInventory) ResolveKey(id model.KeyID) (uint64, bool) {
	value, ok := inventory.keys[id]
	return value, ok
}
func (inventory *recurrenceInventory) ResolveScope(id model.ScopeID) (uint64, bool) {
	value, ok := inventory.scopes[id]
	return value, ok
}
func (inventory *recurrenceInventory) ResolveExpression(id model.ExpressionID) (uint64, bool) {
	value, ok := inventory.expressions[id]
	return value, ok
}
func (inventory *recurrenceInventory) ResolveDependency(id model.DependencyID) (uint64, bool) {
	value, ok := inventory.dependencies[id]
	return value, ok
}
func (inventory *recurrenceInventory) Resolve(access arrangement.Access) (arrangement.Handle, bool) {
	for index, candidate := range inventory.accesses {
		if candidate.Equal(access) {
			return arrangement.NewHandle(inventory.fence, uint64(index+1))
		}
	}
	inventory.accesses = append(inventory.accesses, access)
	return arrangement.NewHandle(inventory.fence, uint64(len(inventory.accesses)))
}
func (inventory *recurrenceInventory) ScopeRegion(id model.ScopeID) (witness.Region, bool) {
	value, ok := inventory.regions[id]
	return value, ok
}
func (inventory *recurrenceInventory) ResolveDenominator(model.DenominatorRef) (witness.DenominatorEvidence, bool) {
	return witness.DenominatorEvidence{}, false
}

func newRecurrenceFixture(t *testing.T) recurrenceFixture {
	t.Helper()
	owner := issueOwner(t, "recurrence-owner")
	value := recurrenceFixture{
		owner:       owner,
		schema:      issueSchema(t, owner, "recurrence-schema"),
		relationA:   issueRelation(t, owner, "recurrence-relation-a"),
		relationB:   issueRelation(t, owner, "recurrence-relation-b"),
		dependencyA: issueDependency(t, owner, "recurrence-dependency-a"),
		dependencyB: issueDependency(t, owner, "recurrence-dependency-b"),
		expressionA: issueExpression(t, owner, "recurrence-expression-a"),
		expressionB: issueExpression(t, owner, "recurrence-expression-b"),
	}
	value.columnA = issueColumn(t, value.relationA, "recurrence-column-a")
	value.columnB = issueColumn(t, value.relationB, "recurrence-column-b")
	value.keyA = issueKey(t, value.relationA, "recurrence-key-a")
	value.keyB = issueKey(t, value.relationB, "recurrence-key-b")
	value.scopeA = issueScope(t, owner, "recurrence-scope-a")
	value.scopeB = issueScope(t, owner, "recurrence-scope-b")
	value.typeID = issueType(t, owner, "recurrence-type")
	return value
}

func issueDependency(t *testing.T, owner model.OwnerID, label string) model.DependencyID {
	t.Helper()
	value, ok := model.IssueDependencyID(owner, content(t, label))
	if !ok {
		t.Fatal("issue dependency")
	}
	return value
}

func issueExpression(t *testing.T, owner model.OwnerID, label string) model.ExpressionID {
	t.Helper()
	value, ok := model.IssueExpressionID(owner, content(t, label))
	if !ok {
		t.Fatal("issue expression")
	}
	return value
}

func recurrenceSchema(t *testing.T, value recurrenceFixture, kind plan.RecurrenceKind) plan.ExecutionSchema {
	t.Helper()
	refA, ok := plan.NewRelationRef(value.relationA)
	if !ok {
		t.Fatal("relation ref A")
	}
	refB, ok := plan.NewRelationRef(value.relationB)
	if !ok {
		t.Fatal("relation ref B")
	}
	dependencyRefA := plan.DefineDependencyRef(value.dependencyA)
	dependencyRefB := plan.DefineDependencyRef(value.dependencyB)
	expressionA := plan.DefineExpressionRef(value.expressionA, algebra.NewProject(
		algebra.NewInput(value.relationB),
		algebra.NewProjectContract(value.relationA, []algebra.ColumnMapping{algebra.NewColumnMapping(value.columnB, value.columnA)}, value.keyA),
	))
	expressionB := plan.DefineExpressionRef(value.expressionB, algebra.NewProject(
		algebra.NewInput(value.relationA),
		algebra.NewProjectContract(value.relationB, []algebra.ColumnMapping{algebra.NewColumnMapping(value.columnA, value.columnB)}, value.keyB),
	))
	dependencyA := plan.DefineDependency(value.dependencyA, value.expressionA, []plan.RelationRef{refB}, []plan.RelationRef{refA}, "recurrence-a")
	dependencyB := plan.DefineDependency(value.dependencyB, value.expressionB, []plan.RelationRef{refA}, []plan.RelationRef{refB}, "recurrence-b")
	edgeAB := plan.DefineDependencyEdge(dependencyRefA, dependencyRefB)
	edgeBA := plan.DefineDependencyEdge(dependencyRefB, dependencyRefA)
	headA := plan.DefineWideningHead(dependencyRefA, refA)
	headB := plan.DefineWideningHead(dependencyRefB, refB)
	component := plan.DefineSCC([]plan.DependencyRef{dependencyRefA, dependencyRefB}, []plan.DependencyEdge{edgeAB, edgeBA}, plan.DefineRecurrence(kind, []plan.WideningHead{headA, headB}))
	builder := plan.NewBuilder(value.schema)
	if !builder.AddRelation(model.DefineRelationSchema(value.relationA, []model.ColumnID{value.columnA}, []model.KeyID{value.keyA}, value.scopeA)) ||
		!builder.AddRelation(model.DefineRelationSchema(value.relationB, []model.ColumnID{value.columnB}, []model.KeyID{value.keyB}, value.scopeB)) ||
		!builder.AddColumn(model.DefineColumnSchema(value.columnA, value.typeID)) ||
		!builder.AddColumn(model.DefineColumnSchema(value.columnB, value.typeID)) ||
		!builder.AddKey(model.DefineKeySchema(value.keyA, []model.ColumnID{value.columnA})) ||
		!builder.AddKey(model.DefineKeySchema(value.keyB, []model.ColumnID{value.columnB})) ||
		!builder.AddScope(model.DefineScopeSchema(value.scopeA, nil)) ||
		!builder.AddScope(model.DefineScopeSchema(value.scopeB, nil)) ||
		!builder.AddExpression(expressionA) || !builder.AddExpression(expressionB) ||
		!builder.AddDependency(dependencyA) || !builder.AddDependency(dependencyB) || !builder.AddSCC(component) {
		t.Fatal("recurrence declarations")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("recurrence schema build")
	}
	return schema
}

func recurrenceMount(t *testing.T, value recurrenceFixture, cert certificate.Certificate) witness.Mounted {
	t.Helper()
	store, ok := identity.IssueStore()
	if !ok {
		t.Fatal("recurrence store")
	}
	fence, ok := address.NewFence(value.schema, cert.Digest(), store, identity.MountID{0x53}, identity.Generation(1))
	if !ok {
		t.Fatal("recurrence fence")
	}
	inventory := &recurrenceInventory{
		fence:        fence,
		relations:    map[model.RelationID]uint64{value.relationA: 1, value.relationB: 2},
		columns:      map[model.ColumnID]uint64{value.columnA: 1, value.columnB: 2},
		keys:         map[model.KeyID]uint64{value.keyA: 1, value.keyB: 2},
		scopes:       map[model.ScopeID]uint64{value.scopeA: 1, value.scopeB: 2},
		expressions:  map[model.ExpressionID]uint64{value.expressionA: 1, value.expressionB: 2},
		dependencies: map[model.DependencyID]uint64{value.dependencyA: 1, value.dependencyB: 2},
		regions:      map[model.ScopeID]witness.Region{value.scopeA: finite("recurrence-region-a"), value.scopeB: finite("recurrence-region-b")},
	}
	mounted, ok := witness.Specialize(cert, inventory, nil, algebraRegistry{algebra: testAlgebra{typeID: value.typeID}})
	if !ok || !mounted.Available() {
		t.Fatal("positive recurrence mount refused")
	}
	return mounted
}

func TestMountedAdmitsExactPositiveRecurrenceHeads(t *testing.T) {
	value := newRecurrenceFixture(t)
	schema := recurrenceSchema(t, value, plan.Positive)
	cert, refusal := certificate.Check(schema)
	if refusal != nil || !cert.Available() {
		t.Fatalf("positive recurrence certificate refused: %v", refusal)
	}
	mounted := recurrenceMount(t, value, cert)
	headA, ok := mounted.Widening(value.dependencyA, value.relationA)
	if !ok || !headA.Available() || headA.Dependency() != value.dependencyA || headA.Relation() != value.relationA || !headA.Evidence().Available() {
		t.Fatal("exact widening head A was not admitted")
	}
	if headB, ok := mounted.Widening(value.dependencyB, value.relationB); !ok || !headB.Available() || !headB.Evidence().Available() {
		t.Fatal("exact widening head B was not admitted")
	}
	if _, ok := mounted.Widening(value.dependencyA, value.relationB); ok {
		t.Fatal("wrong relation widening pair accepted")
	}
	if _, ok := mounted.Widening(value.dependencyB, value.relationA); ok {
		t.Fatal("wrong dependency/relation widening pair accepted")
	}
}

func TestMountedCatalogueAndWideningPermitsAreCanonicalAndDefensive(t *testing.T) {
	value := newRecurrenceFixture(t)
	cert, refusal := certificate.Check(recurrenceSchema(t, value, plan.Positive))
	if refusal != nil || !cert.Available() {
		t.Fatalf("positive recurrence certificate refused: %v", refusal)
	}
	mounted := recurrenceMount(t, value, cert)

	columns := mounted.ColumnIDs()
	if len(columns) != 2 || !columns[0].Available() || !columns[1].Available() || columns[0] == columns[1] {
		t.Fatalf("mounted column catalogue = %#v", columns)
	}
	bookColumns := mounted.Book().ColumnIDs()
	if len(bookColumns) != len(columns) || bookColumns[0] != columns[0] || bookColumns[1] != columns[1] {
		t.Fatal("mounted column catalogue diverged from the canonical Book order")
	}
	digest := mounted.Digest()
	repeatedColumns := mounted.ColumnIDs()
	if len(repeatedColumns) != len(columns) || repeatedColumns[0] != columns[0] || repeatedColumns[1] != columns[1] {
		t.Fatal("mounted column catalogue was not deterministic")
	}
	columns[0] = model.ColumnID{}
	untouchedColumns := mounted.ColumnIDs()
	if len(untouchedColumns) != 2 || !untouchedColumns[0].Available() || mounted.Digest() != digest {
		t.Fatal("column catalogue exposed mutable storage")
	}

	permits := mounted.WideningPermits()
	if len(permits) != 2 {
		t.Fatalf("widening permit count = %d", len(permits))
	}
	repeatedPermits := mounted.WideningPermits()
	for index, permit := range permits {
		if !permit.Available() || permit.Dependency() != repeatedPermits[index].Dependency() || permit.Relation() != repeatedPermits[index].Relation() || permit.Evidence() != repeatedPermits[index].Evidence() {
			t.Fatal("widening permit projection was not deterministic")
		}
	}
	permits[0] = witness.WideningPermit{}
	if recovered, ok := mounted.Widening(value.dependencyA, value.relationA); !ok || !recovered.Available() {
		t.Fatal("widening permit projection exposed mutable storage")
	}
}

func TestMountedRejectsAcyclicRecurrencePolicyWithHeads(t *testing.T) {
	value := newRecurrenceFixture(t)
	schema := recurrenceSchema(t, value, plan.Acyclic)
	cert, refusal := certificate.Check(schema)
	if refusal == nil || cert.Available() {
		t.Fatal("acyclic recurrence carrying widening heads was accepted")
	}
}
