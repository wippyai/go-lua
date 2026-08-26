package witness_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
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
	rowA        model.RowID
	rowB        model.RowID
	scopeA      model.ScopeID
	scopeB      model.ScopeID
	typeID      model.TypeID
	dependencyA model.DependencyID
	dependencyB model.DependencyID
	expressionA model.ExpressionID
	expressionB model.ExpressionID
	operationA  model.OperationID
	operationB  model.OperationID
}

type recurrenceInventory struct {
	fence        address.Fence
	relations    map[model.RelationID]uint64
	columns      map[model.ColumnID]uint64
	keys         map[model.KeyID]uint64
	scopes       map[model.ScopeID]uint64
	expressions  map[model.ExpressionID]uint64
	dependencies map[model.DependencyID]uint64
	denominators map[model.DenominatorRef][]model.RowID
	accesses     []arrangement.Access
}

type recurrenceFactory struct{}

func (recurrenceFactory) Bind(value signature.Signature) (binding.Binding, bool) {
	if !value.Available() {
		return nil, false
	}
	return recurrenceBinding{value: value}, true
}

type recurrenceBinding struct{ value signature.Signature }

func (value recurrenceBinding) Signature() signature.Signature { return value.value }
func (value recurrenceBinding) NewWorker(binding.Fence) (binding.Worker, bool) {
	return nil, false
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
func (inventory *recurrenceInventory) ResolveExpand(model.ExpandContract) ([]expand.Vector, bool) {
	return nil, false
}
func (inventory *recurrenceInventory) ResolveDenominator(ref model.DenominatorRef) (witness.DenominatorEvidence, bool) {
	rows, ok := inventory.denominators[ref]
	if !ok {
		return witness.DenominatorEvidence{}, false
	}
	evidence, ok := identity.DeriveContentID("relation/mount/witness/recurrence-law/v1", []byte("recurrence-denominator"))
	if !ok {
		return witness.DenominatorEvidence{}, false
	}
	return witness.NewDenominatorEvidence(rows, evidence)
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
		operationA:  issueOperation(t, owner, "recurrence-operation-a"),
		operationB:  issueOperation(t, owner, "recurrence-operation-b"),
	}
	value.columnA = issueColumn(t, value.relationA, "recurrence-column-a")
	value.columnB = issueColumn(t, value.relationB, "recurrence-column-b")
	value.keyA = issueKey(t, value.relationA, "recurrence-key-a")
	value.keyB = issueKey(t, value.relationB, "recurrence-key-b")
	value.rowA = issueRow(t, value.relationA, "recurrence-row-a")
	value.rowB = issueRow(t, value.relationB, "recurrence-row-b")
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

func issueOperation(t *testing.T, owner model.OwnerID, label string) model.OperationID {
	t.Helper()
	value, ok := model.IssueOperationID(owner, content(t, label))
	if !ok {
		t.Fatal("issue operation")
	}
	return value
}

func issueRow(t *testing.T, relation model.RelationID, label string) model.RowID {
	t.Helper()
	value, ok := model.IssueRowID(relation, content(t, label))
	if !ok {
		t.Fatal("issue row")
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
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	outcomes, ok := outcome.NewSet(outcome.Produced, outcome.NoSelection, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	denominatorA, ok := model.NewDenominatorRef(value.relationB, value.keyB)
	if !ok {
		t.Fatal("input denominator A")
	}
	denominatorB, ok := model.NewDenominatorRef(value.relationA, value.keyA)
	if !ok {
		t.Fatal("input denominator B")
	}
	signatureA, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: value.operationA, Version: 1},
		Fence:    signature.Fence{Owner: value.owner, Schema: value.schema},
		Inputs: []signature.Input{{Relation: value.relationB, Column: value.columnB, Type: value.typeID,
			Presence: signature.RequirePresent, Delivery: delivery, Denominator: denominatorA}},
		Outputs:     []signature.Output{{Relation: value.relationA, Column: value.columnA, Type: value.typeID, Presence: signature.ProducePresent, Denominator: mustDenominator(t, value.relationA, value.keyA)}},
		Cardinality: cardinality, Outcomes: outcomes,
	})
	if !ok {
		t.Fatal("signature A")
	}
	signatureB, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: value.operationB, Version: 1},
		Fence:    signature.Fence{Owner: value.owner, Schema: value.schema},
		Inputs: []signature.Input{{Relation: value.relationA, Column: value.columnA, Type: value.typeID,
			Presence: signature.RequirePresent, Delivery: delivery, Denominator: denominatorB}},
		Outputs:     []signature.Output{{Relation: value.relationB, Column: value.columnB, Type: value.typeID, Presence: signature.ProducePresent, Denominator: mustDenominator(t, value.relationB, value.keyB)}},
		Cardinality: cardinality, Outcomes: outcomes,
	})
	if !ok {
		t.Fatal("signature B")
	}
	expressionA := plan.DefineExpressionRef(value.expressionA, algebra.NewPublish(
		algebra.NewApply([]algebra.Expression{algebra.NewInput(value.relationB)}, algebra.NewApplyContract(
			signatureA.Identity(), []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed())),
		algebra.NewPublishContract(value.relationA, value.keyA),
	))
	expressionB := plan.DefineExpressionRef(value.expressionB, algebra.NewPublish(
		algebra.NewApply([]algebra.Expression{algebra.NewInput(value.relationA)}, algebra.NewApplyContract(
			signatureB.Identity(), []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed())),
		algebra.NewPublishContract(value.relationB, value.keyB),
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
		!builder.AddScope(model.DefineScopeSchema(value.scopeA, nil, finite(t, "recurrence-region-a"))) ||
		!builder.AddScope(model.DefineScopeSchema(value.scopeB, nil, finite(t, "recurrence-region-b"))) ||
		!builder.AddExpression(expressionA) || !builder.AddExpression(expressionB) ||
		!builder.AddSignature(signatureA) || !builder.AddSignature(signatureB) ||
		!builder.AddDependency(dependencyA) || !builder.AddDependency(dependencyB) || !builder.AddSCC(component) {
		t.Fatal("recurrence declarations")
	}
	capability, capabilityOK := model.NewAscendingCapability(value.typeID)
	if !capabilityOK || !builder.AddTypeCapability(capability) {
		t.Fatal("recurrence capability")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("recurrence schema build")
	}
	return schema
}

func mustDenominator(t *testing.T, relation model.RelationID, key model.KeyID) model.DenominatorRef {
	t.Helper()
	value, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	return value
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
		denominators: map[model.DenominatorRef][]model.RowID{
			mustDenominator(t, value.relationA, value.keyA): {value.rowA},
			mustDenominator(t, value.relationB, value.keyB): {value.rowB},
		},
	}
	mounted, ok := witness.Specialize(cert, inventory, recurrenceFactory{}, algebraRegistry{algebra: testAlgebra{typeID: value.typeID}}, newLineageFactory(t, value.owner))
	if !ok || !mounted.Available() {
		book, bookOK := address.Bind(cert, inventory)
		arrangementPlan, arrangementOK := arrangement.Derive(cert, book, inventory, expand.EmptyCatalog(), []binding.PartitionDirectory{})
		t.Fatalf("positive recurrence mount refused: book=%v arrangement=%v valid=%v", bookOK, arrangementOK, arrangementPlan.ValidFor(book))
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
	schemas := mounted.Columns()
	if len(schemas) != len(columns) {
		t.Fatalf("mounted schema catalogue length = %d, want %d", len(schemas), len(columns))
	}
	for index, schema := range schemas {
		if !schema.Available() || schema.ID() != columns[index] || schema.Relation() != columns[index].Relation() || schema.Type() != value.typeID {
			t.Fatalf("mounted schema catalogue[%d] = %#v, want column %v with type %v", index, schema, columns[index], value.typeID)
		}
	}
	digest := mounted.Digest()
	declarations := mounted.Columns()
	if len(declarations) != 2 || declarations[0].ID() != columns[0] || declarations[1].ID() != columns[1] || !declarations[0].Available() || !declarations[1].Available() {
		t.Fatalf("mounted column declarations = %#v", declarations)
	}
	declarations[0] = model.ColumnSchema{}
	untouchedDeclarations := mounted.Columns()
	if len(untouchedDeclarations) != 2 || !untouchedDeclarations[0].Available() || mounted.Digest() != digest {
		t.Fatal("column declarations exposed mutable storage")
	}
	repeatedColumns := mounted.ColumnIDs()
	if len(repeatedColumns) != len(columns) || repeatedColumns[0] != columns[0] || repeatedColumns[1] != columns[1] {
		t.Fatal("mounted column catalogue was not deterministic")
	}
	columns[0] = model.ColumnID{}
	untouchedColumns := mounted.ColumnIDs()
	if len(untouchedColumns) != 2 || !untouchedColumns[0].Available() || mounted.Digest() != digest {
		t.Fatal("column catalogue exposed mutable storage")
	}
	schemas[0] = model.ColumnSchema{}
	untouchedSchemas := mounted.Columns()
	if len(untouchedSchemas) != 2 || !untouchedSchemas[0].Available() || mounted.Digest() != digest {
		t.Fatal("schema catalogue exposed mutable storage")
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
