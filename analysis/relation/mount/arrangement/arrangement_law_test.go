package arrangement_test

import (
	"reflect"
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
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

type fixture struct {
	certificate certificate.Certificate
	relation    model.RelationID
	column      model.ColumnID
	key         model.KeyID
	scope       model.ScopeID
	expression  model.ExpressionID
	dependency  model.DependencyID
}

type addressInventory struct {
	fence        address.Fence
	relations    map[model.RelationID]uint64
	columns      map[model.ColumnID]uint64
	keys         map[model.KeyID]uint64
	scopes       map[model.ScopeID]uint64
	expressions  map[model.ExpressionID]uint64
	dependencies map[model.DependencyID]uint64
}

func (inventory *addressInventory) Fence() address.Fence { return inventory.fence }
func (inventory *addressInventory) ResolveRelation(id model.RelationID) (uint64, bool) {
	value, ok := inventory.relations[id]
	return value, ok
}
func (inventory *addressInventory) ResolveColumn(id model.ColumnID) (uint64, bool) {
	value, ok := inventory.columns[id]
	return value, ok
}
func (inventory *addressInventory) ResolveKey(id model.KeyID) (uint64, bool) {
	value, ok := inventory.keys[id]
	return value, ok
}
func (inventory *addressInventory) ResolveScope(id model.ScopeID) (uint64, bool) {
	value, ok := inventory.scopes[id]
	return value, ok
}
func (inventory *addressInventory) ResolveExpression(id model.ExpressionID) (uint64, bool) {
	value, ok := inventory.expressions[id]
	return value, ok
}
func (inventory *addressInventory) ResolveDependency(id model.DependencyID) (uint64, bool) {
	value, ok := inventory.dependencies[id]
	return value, ok
}

type arrangementInventory struct {
	fence  address.Fence
	slot   uint64
	calls  uint64
	miss   bool
	handle arrangement.Handle
}

func (inventory *arrangementInventory) Fence() address.Fence { return inventory.fence }
func (inventory *arrangementInventory) Resolve(value arrangement.Access) (arrangement.Handle, bool) {
	if inventory.miss {
		return arrangement.Handle{}, false
	}
	if inventory.handle.Available() {
		return inventory.handle, true
	}
	if inventory.slot == 0 {
		return arrangement.Handle{}, false
	}
	slot := inventory.slot + inventory.calls
	inventory.calls++
	return arrangement.NewHandle(inventory.fence, slot)
}

// vectorGateInventory makes the Input row-vector resolution failure explicit.
// It leaves every other sealed physical access alone, so a rejected derive
// proves that Input cannot quietly retain its relation scan or rebuild a
// vector later at runtime.
type vectorGateInventory struct {
	base    *arrangementInventory
	vector  arrangement.Access
	foreign arrangement.Handle
	missing bool
}

func (inventory *vectorGateInventory) Fence() address.Fence { return inventory.base.Fence() }
func (inventory *vectorGateInventory) Resolve(value arrangement.Access) (arrangement.Handle, bool) {
	if value.Equal(inventory.vector) {
		if inventory.missing {
			return arrangement.Handle{}, false
		}
		if inventory.foreign.Available() {
			return inventory.foreign, true
		}
	}
	return inventory.base.Resolve(value)
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	owner, ok := model.IssueOwnerID(token(t, "owner"))
	if !ok {
		t.Fatal("owner")
	}
	relation, ok := model.IssueRelationID(owner, token(t, "relation"))
	if !ok {
		t.Fatal("relation")
	}
	column, ok := model.IssueColumnID(relation, token(t, "column"))
	if !ok {
		t.Fatal("column")
	}
	typeID, ok := model.IssueTypeID(owner, token(t, "type"))
	if !ok {
		t.Fatal("type")
	}
	key, ok := model.IssueKeyID(relation, token(t, "key"))
	if !ok {
		t.Fatal("key")
	}
	scope, ok := model.IssueScopeID(owner, token(t, "scope"))
	if !ok {
		t.Fatal("scope")
	}
	schemaID, ok := model.IssueSchemaID(owner, token(t, "schema"))
	if !ok {
		t.Fatal("schema")
	}
	expression, ok := model.IssueExpressionID(owner, token(t, "expression"))
	if !ok {
		t.Fatal("expression")
	}
	dependency, ok := model.IssueDependencyID(owner, token(t, "dependency"))
	if !ok {
		t.Fatal("dependency")
	}
	relationRef, ok := plan.NewRelationRef(relation)
	if !ok {
		t.Fatal("relation ref")
	}
	dependencyRef := plan.DefineDependencyRef(dependency)
	builder := plan.NewBuilder(schemaID)
	builder.AddRelation(model.DefineRelationSchema(relation, []model.ColumnID{column}, []model.KeyID{key}, scope))
	builder.AddColumn(model.DefineColumnSchema(column, typeID))
	builder.AddKey(model.DefineKeySchema(key, []model.ColumnID{column}))
	builder.AddScope(model.DefineScopeSchema(scope, nil, region.True()))
	builder.AddExpression(plan.DefineExpressionRef(expression, algebra.NewInput(relation)))
	builder.AddDependency(plan.DefineDependency(dependency, expression, []plan.RelationRef{relationRef}, nil, "input"))
	builder.AddSCC(plan.DefineSCC([]plan.DependencyRef{dependencyRef}, nil, plan.DefineRecurrence(plan.Acyclic, nil)))
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("schema")
	}
	checked, refusal := certificate.Check(schema)
	if refusal != nil {
		t.Fatalf("certificate: %v", refusal)
	}
	return fixture{certificate: checked, relation: relation, column: column, key: key, scope: scope, expression: expression, dependency: dependency}
}

func (value fixture) addresses(t *testing.T) *addressInventory {
	t.Helper()
	store, ok := identity.IssueStore()
	if !ok {
		t.Fatal("store")
	}
	fence, ok := address.NewFence(value.certificate.SchemaID(), value.certificate.Digest(), store, identity.MountID{0: 1}, identity.Generation(1))
	if !ok {
		t.Fatal("fence")
	}
	return &addressInventory{
		fence:        fence,
		relations:    map[model.RelationID]uint64{value.relation: 1},
		columns:      map[model.ColumnID]uint64{value.column: 2},
		keys:         map[model.KeyID]uint64{value.key: 3},
		scopes:       map[model.ScopeID]uint64{value.scope: 4},
		expressions:  map[model.ExpressionID]uint64{value.expression: 5},
		dependencies: map[model.DependencyID]uint64{value.dependency: 6},
	}
}

func TestDeriveIsLogicalAcrossPhysicalReorder(t *testing.T) {
	value := newFixture(t)
	addresses := value.addresses(t)
	book, ok := address.Bind(value.certificate, addresses)
	if !ok {
		t.Fatal("address bind")
	}
	firstInventory := &arrangementInventory{fence: book.Fence(), slot: 11}
	first, ok := arrangement.Derive(value.certificate, book, firstInventory, expand.EmptyCatalog(), []binding.PartitionDirectory{})
	if !ok || !first.Available() || !first.ValidFor(book) {
		t.Fatal("first arrangement derive")
	}
	secondInventory := &arrangementInventory{fence: book.Fence(), slot: 12}
	second, ok := arrangement.Derive(value.certificate, book, secondInventory, expand.EmptyCatalog(), []binding.PartitionDirectory{})
	if !ok || first.LogicalDigest() != second.LogicalDigest() || first.Digest() == second.Digest() {
		t.Fatal("physical assignment leaked into logical identity or was not retained")
	}
	accesses := first.Accesses()
	if len(accesses) != 2 {
		t.Fatalf("access count = %d, want scan plus authored vector", len(accesses))
	}
	firstLayout, ok := first.Resolve(accesses[0])
	if !ok || !firstLayout.ValidFor(book.Fence()) || !firstLayout.Handle().ValidFor(book.Fence()) {
		t.Fatal("logical lookup did not return a valid fenced layout")
	}
	secondLayout, ok := second.Resolve(accesses[0])
	if !ok || firstLayout.Digest() == secondLayout.Digest() {
		t.Fatal("physical layout reorder was not retained")
	}
	foreignFence, ok := address.NewFence(value.certificate.SchemaID(), value.certificate.Digest(), addresses.fence.StoreID(), identity.MountID{0: 9}, addresses.fence.Generation())
	if !ok {
		t.Fatal("foreign fence")
	}
	if firstLayout.ValidFor(foreignFence) {
		t.Fatal("foreign layout accepted")
	}
	staleFence, ok := address.NewFence(value.certificate.SchemaID(), value.certificate.Digest(), addresses.fence.StoreID(), addresses.fence.MountID(), identity.Generation(uint64(addresses.fence.Generation())+1))
	if !ok {
		t.Fatal("stale fence")
	}
	if firstLayout.ValidFor(staleFence) {
		t.Fatal("stale layout accepted")
	}
	accesses[0] = arrangement.Access{}
	if len(first.Accesses()) != 2 || !first.HasAccess(first.Accesses()[0]) {
		t.Fatal("access enumeration was not defensive")
	}
}

func TestDeriveRefusesMissingForeignAndDuplicateHandles(t *testing.T) {
	value := newFixture(t)
	addresses := value.addresses(t)
	book, ok := address.Bind(value.certificate, addresses)
	if !ok {
		t.Fatal("address bind")
	}
	missing := &arrangementInventory{fence: book.Fence(), miss: true}
	if plan, ok := arrangement.Derive(value.certificate, book, missing, expand.EmptyCatalog(), []binding.PartitionDirectory{}); ok || plan.Available() {
		t.Fatal("missing arrangement accepted")
	}
	foreignFence, ok := address.NewFence(value.certificate.SchemaID(), value.certificate.Digest(), addresses.fence.StoreID(), identity.MountID{0: 2}, addresses.fence.Generation())
	if !ok {
		t.Fatal("foreign fence")
	}
	foreign := &arrangementInventory{fence: book.Fence()}
	foreignHandle, ok := arrangement.NewHandle(foreignFence, 19)
	if !ok {
		t.Fatal("foreign handle")
	}
	// Resolve's foreign result must be refused even when the inventory fence is
	// otherwise correct.
	foreign.handle = foreignHandle
	if plan, ok := arrangement.Derive(value.certificate, book, foreign, expand.EmptyCatalog(), []binding.PartitionDirectory{}); ok || plan.Available() {
		t.Fatal("foreign arrangement handle accepted")
	}
	if _, ok := arrangement.NewHandle(book.Fence(), 0); ok {
		t.Fatal("zero arrangement handle accepted")
	}
}

func TestInputRefusesMissingOrForeignAuthoredVectorAtMount(t *testing.T) {
	value := newFixture(t)
	addresses := value.addresses(t)
	book, ok := address.Bind(value.certificate, addresses)
	if !ok {
		t.Fatal("address bind")
	}
	vector, ok := arrangement.NewVectorAccess(value.relation, []model.ColumnID{value.column})
	if !ok {
		t.Fatal("Input row vector")
	}
	missing := &vectorGateInventory{
		base:    &arrangementInventory{fence: book.Fence(), slot: 71},
		vector:  vector,
		missing: true,
	}
	if plan, derived := arrangement.Derive(value.certificate, book, missing, expand.EmptyCatalog(), []binding.PartitionDirectory{}); derived || plan.Available() {
		t.Fatal("Input accepted a missing sealed row vector")
	}
	foreignFence, ok := address.NewFence(value.certificate.SchemaID(), value.certificate.Digest(), addresses.fence.StoreID(), identity.MountID{0: 2}, addresses.fence.Generation())
	if !ok {
		t.Fatal("foreign fence")
	}
	foreignHandle, ok := arrangement.NewHandle(foreignFence, 73)
	if !ok {
		t.Fatal("foreign vector handle")
	}
	foreign := &vectorGateInventory{
		base:    &arrangementInventory{fence: book.Fence(), slot: 81},
		vector:  vector,
		foreign: foreignHandle,
	}
	if plan, derived := arrangement.Derive(value.certificate, book, foreign, expand.EmptyCatalog(), []binding.PartitionDirectory{}); derived || plan.Available() {
		t.Fatal("Input accepted a foreign sealed row vector")
	}
}

func TestInputAdmitsASealedZeroColumnRelation(t *testing.T) {
	owner, ok := model.IssueOwnerID(token(t, "zero-input-owner"))
	if !ok {
		t.Fatal("owner")
	}
	schemaID, ok := model.IssueSchemaID(owner, token(t, "zero-input-schema"))
	if !ok {
		t.Fatal("schema")
	}
	relation, ok := model.IssueRelationID(owner, token(t, "zero-input-relation"))
	if !ok {
		t.Fatal("relation")
	}
	scope, ok := model.IssueScopeID(owner, token(t, "zero-input-scope"))
	if !ok {
		t.Fatal("scope")
	}
	expression, ok := model.IssueExpressionID(owner, token(t, "zero-input-expression"))
	if !ok {
		t.Fatal("expression")
	}
	dependency, ok := model.IssueDependencyID(owner, token(t, "zero-input-dependency"))
	if !ok {
		t.Fatal("dependency")
	}
	relationRef, ok := plan.NewRelationRef(relation)
	if !ok {
		t.Fatal("relation ref")
	}
	builder := plan.NewBuilder(schemaID)
	if !builder.AddRelation(model.DefineRelationSchema(relation, nil, nil, scope)) ||
		!builder.AddScope(model.DefineScopeSchema(scope, nil, region.True())) ||
		!builder.AddExpression(plan.DefineExpressionRef(expression, algebra.NewInput(relation))) ||
		!builder.AddDependency(plan.DefineDependency(dependency, expression, []plan.RelationRef{relationRef}, nil, "zero-input")) ||
		!builder.AddSCC(plan.DefineSCC([]plan.DependencyRef{plan.DefineDependencyRef(dependency)}, nil, plan.DefineRecurrence(plan.Acyclic, nil))) {
		t.Fatal("zero-column declarations")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("schema build")
	}
	checked, refusal := certificate.Check(schema)
	if refusal != nil || !checked.Available() {
		t.Fatalf("zero-column certificate = %v", refusal)
	}
	store, ok := identity.IssueStore()
	if !ok {
		t.Fatal("store")
	}
	fence, ok := address.NewFence(schemaID, checked.Digest(), store, identity.MountID{0: 41}, identity.Generation(1))
	if !ok {
		t.Fatal("fence")
	}
	addresses := &addressInventory{
		fence:        fence,
		relations:    map[model.RelationID]uint64{relation: 1},
		columns:      map[model.ColumnID]uint64{},
		keys:         map[model.KeyID]uint64{},
		scopes:       map[model.ScopeID]uint64{scope: 2},
		expressions:  map[model.ExpressionID]uint64{expression: 3},
		dependencies: map[model.DependencyID]uint64{dependency: 4},
	}
	book, ok := address.Bind(checked, addresses)
	if !ok {
		t.Fatal("address bind")
	}
	derived, ok := arrangement.Derive(checked, book, &arrangementInventory{fence: fence, slot: 91}, expand.EmptyCatalog(), []binding.PartitionDirectory{})
	if !ok || !derived.Available() || len(derived.Accesses()) != 1 {
		t.Fatal("zero-column Input did not seal its one scan/vector access")
	}
	node, ok := derived.Execution().Entry(expression)
	if !ok {
		t.Fatal("zero-column Input node")
	}
	binding, ok := node.Input()
	if !ok || !binding.Available() || len(binding.Scan().Columns()) != 0 || len(binding.Values().Columns()) != 0 || !binding.Scan().Equal(binding.Values()) {
		t.Fatal("zero-column Input did not retain the legal empty row vector")
	}
	rangeValue, ok := node.Range()
	if !ok || !rangeValue.Layout().Equal(binding.Scan()) {
		t.Fatal("zero-column Input range did not retain relation cofiber authority")
	}
}

type censusFixture struct {
	certificate  certificate.Certificate
	owner        model.OwnerID
	schema       model.SchemaID
	typeID       model.TypeID
	relationA    model.RelationID
	relationB    model.RelationID
	columnA      model.ColumnID
	columnA2     model.ColumnID
	columnB      model.ColumnID
	keyA         model.KeyID
	keyB         model.KeyID
	scope        model.ScopeID
	denominatorA model.DenominatorRef
	denominatorB model.DenominatorRef
	operations   [3]signature.Identity
	expressions  [9]model.ExpressionID
}

// newCensusFixture is the complete closed operator table used by the checker
// laws: one entry for every algebra expression plus all three delivery shapes.
func newCensusFixture(t *testing.T) censusFixture {
	t.Helper()
	owner, ok := model.IssueOwnerID(token(t, "census-owner"))
	if !ok {
		t.Fatal("owner")
	}
	schema, ok := model.IssueSchemaID(owner, token(t, "census-schema"))
	if !ok {
		t.Fatal("schema")
	}
	typeID, ok := model.IssueTypeID(owner, token(t, "census-type"))
	if !ok {
		t.Fatal("type")
	}
	relationA, ok := model.IssueRelationID(owner, token(t, "census-relation-a"))
	if !ok {
		t.Fatal("relation A")
	}
	relationB, ok := model.IssueRelationID(owner, token(t, "census-relation-b"))
	if !ok {
		t.Fatal("relation B")
	}
	columnA, ok := model.IssueColumnID(relationA, token(t, "census-column-a"))
	if !ok {
		t.Fatal("column A")
	}
	columnA2, ok := model.IssueColumnID(relationA, token(t, "census-column-a2"))
	if !ok {
		t.Fatal("column A2")
	}
	columnB, ok := model.IssueColumnID(relationB, token(t, "census-column-b"))
	if !ok {
		t.Fatal("column B")
	}
	keyA, ok := model.IssueKeyID(relationA, token(t, "census-key-a"))
	if !ok {
		t.Fatal("key A")
	}
	keyB, ok := model.IssueKeyID(relationB, token(t, "census-key-b"))
	if !ok {
		t.Fatal("key B")
	}
	scope, ok := model.IssueScopeID(owner, token(t, "census-scope"))
	if !ok {
		t.Fatal("scope")
	}
	denominatorA, ok := model.NewDenominatorRef(relationA, keyA)
	if !ok {
		t.Fatal("denominator A")
	}
	denominatorB, ok := model.NewDenominatorRef(relationB, keyB)
	if !ok {
		t.Fatal("denominator B")
	}
	var operations [3]signature.Identity
	for index, label := range []string{"scalar", "bounded", "complete"} {
		operation, issueOK := model.IssueOperationID(owner, token(t, "census-operation-"+label))
		if !issueOK {
			t.Fatal("operation")
		}
		operations[index] = signature.Identity{Operation: operation, Version: 1}
	}

	inputA := algebra.NewInput(relationA)
	inputB := algebra.NewInput(relationB)
	expressionValues := []algebra.Expression{
		inputA,
		algebra.NewSelect(inputA, algebra.NewSelectContract(algebra.SelectByScope, scope)),
		algebra.NewProject(inputA, algebra.NewProjectContract(relationB, []algebra.ColumnMapping{algebra.NewColumnMapping(columnA, columnB)}, keyB)),
		algebra.NewJoin(inputA, inputB, algebra.NewJoinContract([]model.ColumnID{columnA}, []model.ColumnID{columnB})),
		algebra.NewMerge([]algebra.Expression{inputA, inputA}, algebra.NewMergeContract(keyA)),
		algebra.NewGroup(inputA, algebra.NewGroupContract(keyA, censusCardinality(t, model.ExactlyOne))),
		algebra.NewComplete(inputA, denominatorA),
		algebra.NewApply([]algebra.Expression{inputA}, algebra.NewApplyContract(operations[0], []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed())),
		algebra.NewPublish(algebra.NewApply([]algebra.Expression{inputA}, algebra.NewApplyContract(operations[0], []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed())), algebra.NewPublishContract(relationB, keyB)),
	}
	var expressions [9]model.ExpressionID
	expressionLabels := []string{"input", "select", "project", "join", "merge", "group", "complete", "apply", "publish"}
	builder := plan.NewBuilder(schema)
	for _, relation := range []model.RelationSchema{
		model.DefineRelationSchema(relationA, []model.ColumnID{columnA, columnA2}, []model.KeyID{keyA}, scope),
		model.DefineRelationSchema(relationB, []model.ColumnID{columnB}, []model.KeyID{keyB}, scope),
	} {
		if !builder.AddRelation(relation) {
			t.Fatal("add relation")
		}
	}
	for _, column := range []model.ColumnSchema{
		model.DefineColumnSchema(columnA, typeID),
		model.DefineColumnSchema(columnA2, typeID),
		model.DefineColumnSchema(columnB, typeID),
	} {
		if !builder.AddColumn(column) {
			t.Fatal("add column")
		}
	}
	for _, key := range []model.KeySchema{
		model.DefineKeySchema(keyA, []model.ColumnID{columnA2, columnA}),
		model.DefineKeySchema(keyB, []model.ColumnID{columnB}),
	} {
		if !builder.AddKey(key) {
			t.Fatal("add key")
		}
	}
	if !builder.AddScope(model.DefineScopeSchema(scope, []model.ColumnID{columnA, columnB}, region.True())) {
		t.Fatal("add scope")
	}
	capability, capabilityOK := model.NewAscendingCapability(typeID)
	if !capabilityOK || !builder.AddTypeCapability(capability) {
		t.Fatal("add ascending type capability")
	}
	for index, expression := range expressionValues {
		expressionID, issueOK := model.IssueExpressionID(owner, token(t, "census-expression-"+expressionLabels[index]))
		if !issueOK || !builder.AddExpression(plan.DefineExpressionRef(expressionID, expression)) {
			t.Fatal("add expression")
		}
		expressions[index] = expressionID
	}
	accepted, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	scalar, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	bounded, ok := signature.NewBoundedSpanDelivery(2, keyB)
	if !ok {
		t.Fatal("bounded delivery")
	}
	complete, ok := signature.NewCompleteSpanDelivery(keyA)
	if !ok {
		t.Fatal("complete delivery")
	}
	inputs := []signature.Input{
		{Relation: relationA, Column: columnA, Type: typeID, Presence: signature.RequirePresent, Delivery: scalar, Denominator: denominatorA},
		{Relation: relationB, Column: columnB, Type: typeID, Presence: signature.RequirePresent, Delivery: bounded, Denominator: denominatorB},
		{Relation: relationA, Column: columnA, Type: typeID, Presence: signature.RequirePresent, Delivery: complete, Denominator: denominatorA},
	}
	deliveries := []signature.Delivery{scalar, bounded, complete}
	for index, operation := range operations {
		value, sealOK := signature.Seal(signature.Spec{
			Identity:    operation,
			Fence:       signature.Fence{Owner: owner, Schema: schema},
			Inputs:      []signature.Input{inputs[index]},
			Outputs:     []signature.Output{{Relation: relationB, Column: columnB, Type: typeID, Presence: signature.ProducePresent, Denominator: denominatorB}},
			Cardinality: censusCardinality(t, model.ExactlyOne), Outcomes: accepted,
		})
		input, inputOK := value.InputAt(0)
		if !sealOK || !value.Available() || !inputOK || !input.Available() || deliveries[index] == (signature.Delivery{}) || !builder.AddSignature(value) {
			t.Fatal("add signature")
		}
	}
	schemaValue, ok := builder.Build()
	if !ok {
		t.Fatal("build schema")
	}
	checked, refusal := certificate.Check(schemaValue)
	if refusal != nil {
		t.Fatalf("census certificate: %v", refusal)
	}
	return censusFixture{certificate: checked, owner: owner, schema: schema, typeID: typeID, relationA: relationA, relationB: relationB, columnA: columnA, columnA2: columnA2, columnB: columnB, keyA: keyA, keyB: keyB, scope: scope, denominatorA: denominatorA, denominatorB: denominatorB, operations: operations, expressions: expressions}
}

func censusCardinality(t *testing.T, kind model.CardinalityKind) model.Cardinality {
	t.Helper()
	value, ok := model.NewCardinality(kind, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	return value
}

func (value censusFixture) addresses(t *testing.T) *addressInventory {
	t.Helper()
	store, ok := identity.IssueStore()
	if !ok {
		t.Fatal("store")
	}
	fence, ok := address.NewFence(value.certificate.SchemaID(), value.certificate.Digest(), store, identity.MountID{0: 7}, identity.Generation(1))
	if !ok {
		t.Fatal("fence")
	}
	return &addressInventory{
		fence:     fence,
		relations: map[model.RelationID]uint64{value.relationA: 1, value.relationB: 2},
		columns:   map[model.ColumnID]uint64{value.columnA: 3, value.columnA2: 4, value.columnB: 5},
		keys:      map[model.KeyID]uint64{value.keyA: 5, value.keyB: 6},
		scopes:    map[model.ScopeID]uint64{value.scope: 7},
		expressions: map[model.ExpressionID]uint64{
			value.expressions[0]: 8, value.expressions[1]: 9, value.expressions[2]: 10,
			value.expressions[3]: 11, value.expressions[4]: 12, value.expressions[5]: 13,
			value.expressions[6]: 14, value.expressions[7]: 15, value.expressions[8]: 16,
		},
	}
}

func TestDeriveCensusCoversAllExpressionsAndDeliveryShapes(t *testing.T) {
	value := newCensusFixture(t)
	addresses := value.addresses(t)
	book, ok := address.Bind(value.certificate, addresses)
	if !ok {
		t.Fatal("address bind")
	}
	mount := &arrangementInventory{fence: book.Fence(), slot: 101}
	derived, ok := arrangement.Derive(value.certificate, book, mount, expand.EmptyCatalog(), []binding.PartitionDirectory{})
	if !ok || !derived.Available() || !derived.ValidFor(book) {
		t.Fatal("census arrangement derive")
	}
	if len(value.expressions) != 9 {
		t.Fatalf("expression census fixture = %d, want 9", len(value.expressions))
	}

	requirements := []arrangement.Access{}
	for _, relation := range []model.RelationID{value.relationA, value.relationB} {
		access, accessOK := arrangement.NewRelationAccess(relation)
		if !accessOK {
			t.Fatal("relation access")
		}
		requirements = append(requirements, access)
	}
	for _, key := range []model.KeyID{value.keyA, value.keyB} {
		access, accessOK := arrangement.NewKeyAccess(key)
		if !accessOK {
			t.Fatal("key access")
		}
		requirements = append(requirements, access)
	}
	for _, vector := range []struct {
		relation model.RelationID
		columns  []model.ColumnID
	}{
		{value.relationA, []model.ColumnID{value.columnA, value.columnA2}},
		{value.relationA, []model.ColumnID{value.columnA}},
		{value.relationB, []model.ColumnID{value.columnB}},
	} {
		access, accessOK := arrangement.NewVectorAccess(vector.relation, vector.columns)
		if !accessOK {
			t.Fatal("vector access")
		}
		requirements = append(requirements, access)
	}
	for _, requirement := range requirements {
		if !derived.HasAccess(requirement) {
			t.Fatalf("required access missing: relation=%v key=%v columns=%v", requirement.Relation(), requirement.Key(), requirement.Columns())
		}
	}
	if mount.calls != 9 {
		t.Fatalf("logical inventory resolves = %d, want 9", mount.calls)
	}
	classes := map[arrangement.CoordinateClass]int{}
	for _, layout := range derived.Layouts() {
		classes[layout.CoordinateClass()]++
	}
	wantClasses := map[arrangement.CoordinateClass]int{
		arrangement.CoordinateClassNone:                 5,
		arrangement.CoordinateClassDeclaredKey:          4,
		arrangement.CoordinateClassStableCorrespondence: 2,
		arrangement.CoordinateClassLookupOnly:           1,
	}
	if len(derived.Layouts()) != 12 || !reflect.DeepEqual(classes, wantClasses) {
		t.Fatalf("physical coordinate census = %d/%v, want 12/%v", len(derived.Layouts()), classes, wantClasses)
	}
	keyAAccess, accessOK := arrangement.NewKeyAccess(value.keyA)
	if !accessOK {
		t.Fatal("composite key access")
	}
	keyALayout, layoutOK := derived.Resolve(keyAAccess)
	if !layoutOK {
		t.Fatal("composite key layout")
	}
	keyColumns := keyALayout.KeyColumns()
	if keyALayout.CoordinateClass() != arrangement.CoordinateClassDeclaredKey || len(keyColumns) != 2 || keyColumns[0] != value.columnA2 || keyColumns[1] != value.columnA || keyALayout.Columns() != nil || len(keyAAccess.Columns()) != 0 {
		t.Fatalf("composite key layout order/identity = %v/%v", keyColumns, keyAAccess.Columns())
	}
	keyBAccess, accessOK := arrangement.NewKeyAccess(value.keyB)
	if !accessOK {
		t.Fatal("single key access")
	}
	keyBLayout, layoutOK := derived.Resolve(keyBAccess)
	if !layoutOK || len(keyBLayout.KeyColumns()) != 1 || keyBLayout.KeyColumns()[0] != value.columnB {
		t.Fatal("single key layout columns")
	}
	scan, scanOK := arrangement.NewRelationAccess(value.relationA)
	emptyVector, vectorOK := arrangement.NewVectorAccess(value.relationA, nil)
	if !scanOK || !vectorOK || !scan.Equal(emptyVector) {
		t.Fatal("relation scan/vector equivalence contract changed")
	}
	scanLayout, scanLayoutOK := derived.Resolve(scan)
	emptyLayout, emptyLayoutOK := derived.Resolve(emptyVector)
	if !scanLayoutOK || !emptyLayoutOK || !scanLayout.Equal(emptyLayout) || scanLayout.CoordinateClass() != arrangement.CoordinateClassNone || scanLayout.KeyColumns() != nil {
		t.Fatal("relation scan layout equivalence")
	}
	vectorAccess, vectorOK := arrangement.NewVectorAccess(value.relationA, []model.ColumnID{value.columnA})
	if !vectorOK {
		t.Fatal("vector access")
	}
	physicalLayout := func(access arrangement.Access, class arrangement.CoordinateClass, keyColumns []model.ColumnID) (arrangement.Layout, bool) {
		var result arrangement.Layout
		for _, candidate := range derived.Layouts() {
			if !candidate.Access().Equal(access) || candidate.CoordinateClass() != class {
				continue
			}
			gotKeys := candidate.KeyColumns()
			if len(gotKeys) != len(keyColumns) {
				continue
			}
			matches := true
			for index := range keyColumns {
				if gotKeys[index] != keyColumns[index] {
					matches = false
					break
				}
			}
			if !matches {
				continue
			}
			if result.Available() {
				return arrangement.Layout{}, false
			}
			result = candidate
		}
		return result, result.Available()
	}
	vectorLayout, vectorLayoutOK := physicalLayout(vectorAccess, arrangement.CoordinateClassStableCorrespondence, []model.ColumnID{value.columnA})
	if _, ambiguous := derived.Resolve(vectorAccess); ambiguous {
		t.Fatal("ambiguous correspondence access was resolved by declaration order")
	}
	if !vectorLayoutOK || vectorLayout.CoordinateClass() != arrangement.CoordinateClassStableCorrespondence || len(vectorLayout.KeyColumns()) != 1 || vectorLayout.KeyColumns()[0] != value.columnA || len(vectorLayout.Columns()) != 1 || vectorLayout.Columns()[0] != value.columnA {
		t.Fatal("correspondence vector layout")
	}
	mergeVectorAccess, mergeVectorOK := arrangement.NewVectorAccess(value.relationA, []model.ColumnID{value.columnA, value.columnA2})
	if !mergeVectorOK {
		t.Fatal("merge vector access")
	}
	mergeVectorLayout, mergeVectorLayoutOK := physicalLayout(mergeVectorAccess, arrangement.CoordinateClassLookupOnly, []model.ColumnID{value.columnA2, value.columnA})
	if _, ambiguous := derived.Resolve(mergeVectorAccess); ambiguous {
		t.Fatal("ambiguous lookup access was resolved by declaration order")
	}
	if !mergeVectorLayoutOK || mergeVectorLayout.CoordinateClass() != arrangement.CoordinateClassLookupOnly || len(mergeVectorLayout.KeyColumns()) != 2 || mergeVectorLayout.KeyColumns()[0] != value.columnA2 || mergeVectorLayout.KeyColumns()[1] != value.columnA {
		t.Fatal("lookup-only Merge vector layout")
	}
	layouts := derived.Layouts()
	planAccesses := derived.Accesses()
	if len(layouts) != len(planAccesses) {
		t.Fatal("layout/access census mismatch")
	}
	for index, layout := range layouts {
		if !layout.Access().Equal(planAccesses[index]) {
			t.Fatal("layout/access duplicate projections diverged")
		}
		gotColumns, wantColumns := layout.Columns(), planAccesses[index].Columns()
		if len(gotColumns) != len(wantColumns) {
			t.Fatal("layout/access column projection diverged")
		}
		for columnIndex := range gotColumns {
			if gotColumns[columnIndex] != wantColumns[columnIndex] {
				t.Fatal("layout/access column projection diverged")
			}
		}
		if len(gotColumns) > 0 {
			gotColumns[0] = model.ColumnID{}
			if fresh := derived.Layouts()[index].Columns(); fresh[0] != wantColumns[0] {
				t.Fatal("layout columns were not defensive")
			}
		}
	}
	for index, layout := range layouts {
		if layout.Access().Equal(keyAAccess) {
			keyValues := layout.KeyColumns()
			keyValues[0] = model.ColumnID{}
			accessValues := layout.Access().Columns()
			if len(accessValues) != 0 {
				accessValues[0] = model.ColumnID{}
			}
			columnValues := layout.Columns()
			if len(columnValues) != 0 {
				columnValues[0] = model.ColumnID{}
			}
			fresh := derived.Layouts()[index]
			freshKeys := fresh.KeyColumns()
			if freshKeys[0] != value.columnA2 || freshKeys[1] != value.columnA {
				t.Fatal("layout key columns were not defensive")
			}
			break
		}
	}

	deliveries := derived.DeliveryRequirements()
	if len(deliveries) != 3 {
		t.Fatalf("delivery requirements = %d, want 3", len(deliveries))
	}
	seenKinds := map[signature.DeliveryKind]bool{}
	var scalarAccess, completeAccess arrangement.Access
	var scalarLayout, completeLayout arrangement.Layout
	expectedPhysical := append([]arrangement.Access(nil), requirements...)
	for _, delivery := range deliveries {
		seenKinds[delivery.Delivery().Kind] = true
		if !delivery.Available() || delivery.Index() != 0 || delivery.Input().Relation != delivery.Relation() || delivery.Input().Column != delivery.Column() {
			t.Fatal("invalid delivery requirement projection")
		}
		if delivery.Delivery().IsSpan() {
			if !derived.HasAccess(func() arrangement.Access {
				access, accessOK := arrangement.NewKeyAccess(delivery.Delivery().OrderKey())
				if !accessOK {
					return arrangement.Access{}
				}
				return access
			}()) {
				t.Fatal("span order key access missing")
			}
		}
		access, accessOK := delivery.Access()
		if !accessOK {
			t.Fatal("delivery physical access")
		}
		known := false
		for _, expected := range expectedPhysical {
			if expected.Equal(access) {
				known = true
				break
			}
		}
		if !known {
			expectedPhysical = append(expectedPhysical, access)
		}
		layout, layoutOK := derived.Resolve(access)
		if !layoutOK {
			t.Fatal("delivery physical layout")
		}
		switch delivery.Delivery().Kind {
		case signature.ScalarDelivery:
			scalarAccess, scalarLayout = access, layout
		case signature.CompleteSpanDelivery:
			completeAccess, completeLayout = access, layout
		}
	}
	if !seenKinds[signature.ScalarDelivery] || !seenKinds[signature.BoundedSpanDelivery] || !seenKinds[signature.CompleteSpanDelivery] {
		t.Fatalf("delivery shapes = %#v", seenKinds)
	}
	logicalAccesses := make([]arrangement.Access, 0, len(derived.Accesses()))
	for _, candidate := range derived.Accesses() {
		seen := false
		for _, prior := range logicalAccesses {
			if prior.Equal(candidate) {
				seen = true
				break
			}
		}
		if !seen {
			logicalAccesses = append(logicalAccesses, candidate)
		}
	}
	if len(expectedPhysical) != len(logicalAccesses) {
		t.Fatalf("exact logical access census = %d, want %d (physical layouts=%d)", len(logicalAccesses), len(expectedPhysical), len(derived.Accesses()))
	}
	for _, candidate := range logicalAccesses {
		found := false
		for _, expected := range expectedPhysical {
			if candidate.Equal(expected) {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("plan contains an undeclared physical access")
		}
	}
	if !scalarAccess.Equal(completeAccess) || !scalarLayout.Equal(completeLayout) {
		t.Fatal("different delivery shapes did not share one physical access and layout")
	}

	// Join vectors are authored left-to-right and remain distinct logical
	// requirements; this catches accidental vector/set normalization.
	joinLeft, _ := arrangement.NewVectorAccess(value.relationA, []model.ColumnID{value.columnA})
	joinRight, _ := arrangement.NewVectorAccess(value.relationB, []model.ColumnID{value.columnB})
	if !derived.HasAccess(joinLeft) || !derived.HasAccess(joinRight) || joinLeft.Equal(joinRight) {
		t.Fatal("authored join orientation was not retained")
	}

	// A resolver returning one physical handle for multiple requirements is
	// rejected rather than silently aliasing runtime storage.
	fixed, fixedOK := arrangement.NewHandle(book.Fence(), 999)
	if !fixedOK {
		t.Fatal("fixed handle")
	}
	if plan, deriveOK := arrangement.Derive(value.certificate, book, &arrangementInventory{fence: book.Fence(), handle: fixed}, expand.EmptyCatalog(), []binding.PartitionDirectory{}); deriveOK || plan.Available() {
		t.Fatal("duplicate physical handles accepted")
	}
}

func token(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("analysis/relation/mount/arrangement/test/v1", []byte(label))
	if !ok {
		t.Fatal("token")
	}
	return value
}
