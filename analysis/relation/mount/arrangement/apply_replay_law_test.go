package arrangement

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// TestCorrelatedSubtreeConstructorDigestAndCapabilities exercises the generic
// mount certificate directly.  A broadcast Complete is expressed by mounted
// denominator extents, not a child-shape tag or a replay-level directory.
func TestCorrelatedSubtreeConstructorDigestAndCapabilities(t *testing.T) {
	fixture := newCorrelatedSubtreeLawFixture(t)
	correlation := algebra.NewApplyCorrelation(fixture.population, fixture.column, fixture.typeID, [][]model.ColumnID{{}, {}})
	slots := []algebra.SlotSource{algebra.NewSlotSource(0, 0), algebra.NewSlotSource(1, 0)}
	deliveries := fixture.deliveries(t, len(slots))
	first := fixture.subtree(t, 0, fixture.complete, correlation, slots, deliveries)
	second := fixture.subtree(t, 1, fixture.complete, correlation, slots, deliveries)
	replay, ok := newApplyReplay(lawContent(t, "static-apply"), correlation, fixture.driver, []CorrelatedSubtree{first, second})
	if !ok || !replay.Available() {
		t.Fatal("generic static replay was not sealed")
	}
	again, againOK := newApplyReplay(lawContent(t, "static-apply"), correlation, fixture.driver, []CorrelatedSubtree{first, second})
	if !againOK || again.Digest() != replay.Digest() {
		t.Fatal("replay digest was not deterministic")
	}
	if first.Digest() == second.Digest() {
		t.Fatal("subtree ordinal was erased from digest")
	}
	if scope, scopeOK := first.EmptyScope(); !scopeOK || !scope.Available() {
		t.Fatal("complete subtree did not retain its sealed empty scope")
	}
	input, inputOK := first.InputAt(0)
	complete, completeOK := first.CompleteAt(0)
	if !inputOK || !completeOK || input.Node().Digest() == complete.Node().Digest() {
		t.Fatal("complete subtree did not retain exact input and complete extents")
	}
	completePath := complete.Occurrence().Path()
	if completePath == nil {
		t.Fatal("root Complete occurrence lost its exact empty path")
	}
	if found, foundOK := first.CompleteFor(complete.Node(), completePath); !foundOK || found.Occurrence().Digest() != complete.Occurrence().Digest() {
		t.Fatal("root Complete occurrence was not path-addressable")
	}
	for _, source := range []CorrelationExtentSource{input.Source(), complete.Source()} {
		if _, _, driver := source.PopulationDriver(); driver {
			t.Fatal("mounted extent exposed a population driver")
		}
		if _, partition := source.Partition(); partition {
			t.Fatal("broadcast extent exposed a partition directory")
		}
		if denominator, denominatorOK := source.Denominator(); !denominatorOK || denominator != fixture.population {
			t.Fatal("mounted extent lost exact denominator authority")
		}
	}
}

// TestCorrelatedSubtreeDriverIsAnExtent verifies that the old scalar/direct
// case is represented solely by an exact population-driver source on its
// Input occurrence.  It has no compatibility replay arm and no synthetic
// Select scope.
func TestCorrelatedSubtreeDriverIsAnExtent(t *testing.T) {
	fixture := newCorrelatedSubtreeLawFixture(t)
	correlation := algebra.NewApplyCorrelation(fixture.population, fixture.column, fixture.typeID, [][]model.ColumnID{{fixture.column}, {}})
	slots := []algebra.SlotSource{algebra.NewSlotSource(0, 0), algebra.NewSlotSource(1, 0)}
	deliveries := fixture.deliveries(t, len(slots))
	driver := fixture.subtree(t, 0, fixture.populationInput, correlation, slots, deliveries)
	shared := fixture.subtree(t, 1, fixture.complete, correlation, slots, deliveries)
	replay, ok := newApplyReplay(lawContent(t, "driver-apply"), correlation, fixture.driver, []CorrelatedSubtree{driver, shared})
	if !ok || !replay.Available() {
		t.Fatal("driver replay was not sealed")
	}
	if _, scopeOK := driver.EmptyScope(); scopeOK {
		t.Fatal("scalar driver minted a synthetic empty scope")
	}
	extent, extentOK := driver.InputAt(0)
	if !extentOK || driver.CompleteCount() != 0 {
		t.Fatal("scalar driver did not retain exactly one input extent")
	}
	layout, slot, sourceOK := extent.Source().PopulationDriver()
	if !sourceOK || !layout.Equal(fixture.driver) || slot != algebra.NewSlotSource(0, 0) {
		t.Fatal("driver extent did not retain exact vector/cell provenance")
	}
	if _, partition := extent.Source().Partition(); partition {
		t.Fatal("driver extent exposed a partition authority")
	}
}

// TestCorrelatedSubtreeRepeatedInputOccurrenceIsPathAddressed proves that a
// repeated same-relation logical Input cannot be selected by node digest or
// relation alone.  The resolver deliberately interns both Join leaves to the
// same physical node; the sealed root-relative paths remain distinct.
func TestCorrelatedSubtreeRepeatedInputOccurrenceIsPathAddressed(t *testing.T) {
	fixture := newCorrelatedSubtreeLawFixture(t)
	joined := fixture.joinedComplete(t)
	correlation := algebra.NewApplyCorrelation(fixture.population, fixture.column, fixture.typeID, [][]model.ColumnID{{}, {}})
	slots := []algebra.SlotSource{
		algebra.NewSlotSource(0, 0), algebra.NewSlotSource(0, 1),
		algebra.NewSlotSource(1, 0), algebra.NewSlotSource(1, 1),
	}
	deliveries := fixture.deliveries(t, len(slots))
	first := fixture.subtree(t, 0, joined, correlation, slots, deliveries)
	second := fixture.subtree(t, 1, joined, correlation, slots, deliveries)
	if replay, ok := newApplyReplay(lawContent(t, "repeated-input-apply"), correlation, fixture.driver, []CorrelatedSubtree{first, second}); !ok || !replay.Available() {
		t.Fatal("repeated-input replay was not sealed")
	}
	left, leftOK := first.InputAt(0)
	right, rightOK := first.InputAt(1)
	if !leftOK || !rightOK || left.Node().value != right.Node().value {
		t.Fatal("fixture did not retain repeated physical input node")
	}
	leftPath, rightPath := left.Occurrence().Path(), right.Occurrence().Path()
	if sameUint32Path(leftPath, rightPath) {
		t.Fatal("repeated input occurrences lost their distinct paths")
	}
	if found, foundOK := first.InputFor(left.Node(), leftPath); !foundOK || found.Occurrence().Digest() != left.Occurrence().Digest() {
		t.Fatal("left occurrence was not path-addressable")
	}
	if found, foundOK := first.InputFor(left.Node(), rightPath); !foundOK || found.Occurrence().Digest() != right.Occurrence().Digest() {
		t.Fatal("right occurrence was not path-addressable")
	}
	if _, found := first.InputFor(left.Node(), nil); found {
		t.Fatal("node identity alone selected an occurrence")
	}

	// Dropping the right exact cell leaves its occurrence without a source.
	// The cold walker must refuse rather than inherit the carrier denominator.
	if _, hostileOK := sealCorrelatedSubtree(0, joined, correlation, fixture.driver, []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, fixture.deliveries(t, 1), certificate.CorrelationPartition{}, binding.PartitionDirectory{}); hostileOK {
		t.Fatal("missing repeated-input extent was accepted")
	}
}

func sameUint32Path(left, right []uint32) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

type correlatedSubtreeLawFixture struct {
	fence           address.Fence
	relation        model.RelationID
	column          model.ColumnID
	key             model.KeyID
	typeID          model.TypeID
	population      model.DenominatorRef
	driver          Layout
	delivery        Layout
	operation       signature.Identity
	complete        *executionNode
	populationInput *executionNode
	resolver        *executionResolver
	scope           model.ScopeID
}

func newCorrelatedSubtreeLawFixture(t *testing.T) correlatedSubtreeLawFixture {
	t.Helper()
	owner, ok := model.IssueOwnerID(lawContent(t, "owner"))
	if !ok {
		t.Fatal("owner")
	}
	schemaID, ok := model.IssueSchemaID(owner, lawContent(t, "schema"))
	if !ok {
		t.Fatal("schema")
	}
	typeID, ok := model.IssueTypeID(owner, lawContent(t, "type"))
	if !ok {
		t.Fatal("type")
	}
	relation, ok := model.IssueRelationID(owner, lawContent(t, "relation"))
	if !ok {
		t.Fatal("relation")
	}
	column, ok := model.IssueColumnID(relation, lawContent(t, "column"))
	if !ok {
		t.Fatal("column")
	}
	key, ok := model.IssueKeyID(relation, lawContent(t, "key"))
	if !ok {
		t.Fatal("key")
	}
	scope, ok := model.IssueScopeID(owner, lawContent(t, "scope"))
	if !ok {
		t.Fatal("scope")
	}
	expressionID, ok := model.IssueExpressionID(owner, lawContent(t, "expression"))
	if !ok {
		t.Fatal("expression")
	}
	dependencyID, ok := model.IssueDependencyID(owner, lawContent(t, "dependency"))
	if !ok {
		t.Fatal("dependency")
	}
	operationID, ok := model.IssueOperationID(owner, lawContent(t, "operation"))
	if !ok {
		t.Fatal("operation")
	}
	population, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("population")
	}

	relationSchema := model.DefineRelationSchema(relation, []model.ColumnID{column}, []model.KeyID{key}, scope)
	builder := plan.NewBuilder(schemaID)
	relationRef, ok := plan.NewRelationRef(relation)
	if !ok || !builder.AddRelation(relationSchema) || !builder.AddColumn(model.DefineColumnSchema(column, typeID)) || !builder.AddKey(model.DefineKeySchema(key, []model.ColumnID{column})) || !builder.AddScope(model.DefineScopeSchema(scope, nil, region.True())) {
		t.Fatal("schema declarations")
	}
	if !builder.AddExpression(plan.DefineExpressionRef(expressionID, algebra.NewInput(relation))) || !builder.AddDependency(plan.DefineDependency(dependencyID, expressionID, []plan.RelationRef{relationRef}, nil, "correlated-subtree-law")) || !builder.AddSCC(plan.DefineSCC([]plan.DependencyRef{plan.DefineDependencyRef(dependencyID)}, nil, plan.DefineRecurrence(plan.Acyclic, nil))) {
		t.Fatal("schema graph")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("schema build")
	}
	checked, refusal := certificate.Check(schema)
	if refusal != nil || !checked.Available() {
		t.Fatalf("certificate: %v", refusal)
	}
	store, ok := identity.IssueStore()
	if !ok {
		t.Fatal("store")
	}
	fence, ok := address.NewFence(schemaID, checked.Digest(), store, identity.MountID{0: 91}, identity.Generation(1))
	if !ok {
		t.Fatal("address fence")
	}
	addresses := &correlatedSubtreeLawAddressInventory{
		fence:        fence,
		relations:    map[model.RelationID]uint64{relation: 1},
		columns:      map[model.ColumnID]uint64{column: 2},
		keys:         map[model.KeyID]uint64{key: 3},
		scopes:       map[model.ScopeID]uint64{scope: 4},
		expressions:  map[model.ExpressionID]uint64{expressionID: 5},
		dependencies: map[model.DependencyID]uint64{dependencyID: 6},
	}
	book, ok := address.Bind(checked, addresses)
	if !ok {
		t.Fatal("address book")
	}

	relationAccess, ok := NewRelationAccess(relation)
	if !ok {
		t.Fatal("relation access")
	}
	valuesAccess, ok := NewVectorAccess(relation, []model.ColumnID{column})
	if !ok {
		t.Fatal("values access")
	}
	keyAccess, ok := NewKeyAccess(key)
	if !ok {
		t.Fatal("key access")
	}
	deliveryAccess, ok := newAccess(relation, key, []model.ColumnID{column})
	if !ok {
		t.Fatal("delivery access")
	}
	scan, ok := newLayoutWithClass(fence, mustLawHandle(t, fence, 11), relationAccess, nil, CoordinateClassNone)
	if !ok {
		t.Fatal("scan layout")
	}
	values, ok := newLayoutWithClass(fence, mustLawHandle(t, fence, 12), valuesAccess, nil, CoordinateClassNone)
	if !ok {
		t.Fatal("values layout")
	}
	keyLayout, ok := newLayoutWithClass(fence, mustLawHandle(t, fence, 13), keyAccess, []model.ColumnID{column}, CoordinateClassDeclaredKey)
	if !ok {
		t.Fatal("key layout")
	}
	stable, ok := newLayoutWithClass(fence, mustLawHandle(t, fence, 14), valuesAccess, []model.ColumnID{column}, CoordinateClassStableCorrespondence)
	if !ok {
		t.Fatal("stable layout")
	}
	delivery, ok := newLayoutWithClass(fence, mustLawHandle(t, fence, 15), deliveryAccess, []model.ColumnID{column}, CoordinateClassDeclaredKey)
	if !ok {
		t.Fatal("delivery layout")
	}
	resolver := &executionResolver{
		fence:     fence,
		book:      book,
		relations: []model.RelationSchema{relationSchema},
		layouts:   []Layout{scan, values, keyLayout, stable, delivery},
		nodes:     make(map[identity.ContentID]*executionNode),
		visiting:  make(map[identity.ContentID]bool),
	}
	selectContract := algebra.NewSelectContract(algebra.SelectByScope, scope)
	complete, ok := resolver.node(algebra.NewComplete(algebra.NewSelect(algebra.NewInput(relation), selectContract), population))
	if !ok || complete == nil {
		t.Fatal("complete child")
	}
	populationInput, ok := resolver.node(algebra.NewInput(relation))
	if !ok || populationInput == nil {
		t.Fatal("population input")
	}
	return correlatedSubtreeLawFixture{
		fence: fence, relation: relation, column: column, key: key, typeID: typeID,
		population: population, driver: values, delivery: delivery,
		operation: signature.Identity{Operation: operationID, Version: 1},
		complete:  complete, populationInput: populationInput, resolver: resolver, scope: scope,
	}
}

func (fixture correlatedSubtreeLawFixture) deliveries(t *testing.T, count int) []DeliveryBinding {
	t.Helper()
	result := make([]DeliveryBinding, count)
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	for index := range result {
		input, inputOK := signature.NewHomogeneousInput(fixture.relation, fixture.column, fixture.typeID, signature.RequirePresent, delivery, fixture.population)
		requirement, requirementOK := newDeliveryRequirement(fixture.operation, index, input)
		result[index] = DeliveryBinding{requirement: requirement, layout: fixture.delivery}
		if !inputOK || !requirementOK || !result[index].Available() {
			t.Fatalf("delivery %d", index)
		}
	}
	return result
}

func (fixture correlatedSubtreeLawFixture) subtree(t *testing.T, ordinal uint32, root *executionNode, correlation algebra.ApplyCorrelation, slots []algebra.SlotSource, deliveries []DeliveryBinding) CorrelatedSubtree {
	t.Helper()
	value, ok := sealCorrelatedSubtree(ordinal, root, correlation, fixture.driver, slots, deliveries, certificate.CorrelationPartition{}, binding.PartitionDirectory{})
	if !ok || !value.Available() {
		t.Fatalf("subtree %d was not sealed", ordinal)
	}
	return value
}

func (fixture correlatedSubtreeLawFixture) joinedComplete(t *testing.T) *executionNode {
	t.Helper()
	join := algebra.NewJoin(algebra.NewInput(fixture.relation), algebra.NewInput(fixture.relation), algebra.NewJoinContract([]model.ColumnID{fixture.column}, []model.ColumnID{fixture.column}))
	selectContract := algebra.NewSelectContract(algebra.SelectByScope, fixture.scope)
	value, ok := fixture.resolver.node(algebra.NewComplete(algebra.NewSelect(join, selectContract), fixture.population))
	if !ok || value == nil {
		t.Fatal("joined complete child")
	}
	return value
}

type correlatedSubtreeLawAddressInventory struct {
	fence        address.Fence
	relations    map[model.RelationID]uint64
	columns      map[model.ColumnID]uint64
	keys         map[model.KeyID]uint64
	scopes       map[model.ScopeID]uint64
	expressions  map[model.ExpressionID]uint64
	dependencies map[model.DependencyID]uint64
}

func (inventory *correlatedSubtreeLawAddressInventory) Fence() address.Fence { return inventory.fence }
func (inventory *correlatedSubtreeLawAddressInventory) ResolveRelation(id model.RelationID) (uint64, bool) {
	value, ok := inventory.relations[id]
	return value, ok
}
func (inventory *correlatedSubtreeLawAddressInventory) ResolveColumn(id model.ColumnID) (uint64, bool) {
	value, ok := inventory.columns[id]
	return value, ok
}
func (inventory *correlatedSubtreeLawAddressInventory) ResolveKey(id model.KeyID) (uint64, bool) {
	value, ok := inventory.keys[id]
	return value, ok
}
func (inventory *correlatedSubtreeLawAddressInventory) ResolveScope(id model.ScopeID) (uint64, bool) {
	value, ok := inventory.scopes[id]
	return value, ok
}
func (inventory *correlatedSubtreeLawAddressInventory) ResolveExpression(id model.ExpressionID) (uint64, bool) {
	value, ok := inventory.expressions[id]
	return value, ok
}
func (inventory *correlatedSubtreeLawAddressInventory) ResolveDependency(id model.DependencyID) (uint64, bool) {
	value, ok := inventory.dependencies[id]
	return value, ok
}

func lawContent(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("analysis/relation/mount/arrangement/correlated-subtree-law/v1", []byte(label))
	if !ok {
		t.Fatal("law content")
	}
	return value
}

func mustLawHandle(t *testing.T, fence address.Fence, slot uint64) Handle {
	t.Helper()
	value, ok := NewHandle(fence, slot)
	if !ok {
		t.Fatal("law handle")
	}
	return value
}
