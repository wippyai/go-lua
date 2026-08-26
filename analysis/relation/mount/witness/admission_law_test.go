package witness_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

type countedInventory struct {
	*evidenceInventory
	denominatorCalls int
}

func (inventory *countedInventory) ResolveDenominator(ref model.DenominatorRef) (witness.DenominatorEvidence, bool) {
	inventory.denominatorCalls++
	return inventory.evidenceInventory.ResolveDenominator(ref)
}

type countedFactory struct {
	value signature.Signature
	calls int
}

func (factory *countedFactory) Bind(value signature.Signature) (binding.Binding, bool) {
	factory.calls++
	return operationFactory{value: factory.value}.Bind(value)
}

type countedAlgebraRegistry struct {
	algebra testAlgebra
	calls   int
}

func (registry *countedAlgebraRegistry) Resolve(typeID model.TypeID) (binding.ValueAlgebra, bool) {
	registry.calls++
	return registry.algebra, registry.algebra.Type() == typeID
}

type testValueEquality struct{ typeID model.TypeID }

func (equality testValueEquality) Type() model.TypeID { return equality.typeID }

func (equality testValueEquality) Equal(left, right binding.ValueToken) bool {
	return left.Available() && right.Available() && left.Type() == equality.typeID && right.Type() == equality.typeID && left.Same(right)
}

type countedEqualityRegistry struct {
	algebra       testAlgebra
	equality      testValueEquality
	algebraCalls  int
	equalityCalls int
}

func (registry *countedEqualityRegistry) Resolve(typeID model.TypeID) (binding.ValueAlgebra, bool) {
	registry.algebraCalls++
	return registry.algebra, registry.algebra.Type() == typeID
}

func (registry *countedEqualityRegistry) ResolveEquality(typeID model.TypeID) (binding.ValueEquality, bool) {
	registry.equalityCalls++
	return registry.equality, registry.equality.Type() == typeID
}

func TestSpecializeResolvesEachCertifiedObligationExactlyOnce(t *testing.T) {
	fixture := newTerminalPresentSemanticAdmissionFixture(t)
	inventory := &countedInventory{evidenceInventory: &fixture.evidence}
	factory := &countedFactory{value: fixture.operation}
	registry := &countedAlgebraRegistry{algebra: testAlgebra{typeID: fixture.typeID}}
	lineageFactory := &countedLineageFactory{owner: fixture.owner, identity: content(t, "counted-lineage"), ok: true}
	mounted, ok := witness.Specialize(fixture.cert, inventory, factory, registry, lineageFactory)
	if !ok || !mounted.Available() {
		t.Fatal("counted semantic mount refused")
	}
	if inventory.denominatorCalls != 1 {
		t.Fatalf("ResolveDenominator calls = %d, want exactly one", inventory.denominatorCalls)
	}
	if factory.calls != 1 {
		t.Fatalf("Factory.Bind calls = %d, want exactly one", factory.calls)
	}
	if registry.calls != 0 {
		t.Fatalf("AlgebraRegistry.Resolve calls = %d, want zero for terminal Apply signature", registry.calls)
	}
	if lineageFactory.calls != 1 {
		t.Fatalf("LineageFactory.Bind calls = %d, want exactly one", lineageFactory.calls)
	}
	lineageAuthority, lineageOK := mounted.Lineage()
	if !lineageOK || lineageAuthority == nil || lineageAuthority.Fence() != mounted.RuntimeFence() || lineageAuthority.Owner() != fixture.owner || lineageAuthority.Identity() != lineageFactory.identity {
		t.Fatal("mounted lineage authority was not exact")
	}
	if _, lineageOK = mounted.Lineage(); !lineageOK || lineageFactory.calls != 1 {
		t.Fatal("lineage lookup was not defensive")
	}
}

// A key comparison is an equality use, not a latent ascent. Even when the
// owner chose Ascending for the key TypeID, a read-only Group must obtain its
// narrow ValueEquality authority rather than resolving a ValueAlgebra that
// the certificate did not require for any Publish or Merge ascent.
func TestSpecializeKeyEqualityDoesNotResolveFreshAlgebra(t *testing.T) {
	owner := issueOwner(t, "equality-owner")
	schemaID := issueSchema(t, owner, "equality-schema")
	relation := issueRelation(t, owner, "equality-relation")
	column := issueColumn(t, relation, "equality-column")
	key := issueKey(t, relation, "equality-key")
	scope := issueScope(t, owner, "equality-scope")
	typeID := issueType(t, owner, "equality-type")
	expression := issueExpression(t, owner, "equality-group")
	cardinality, cardinalityOK := model.NewCardinality(model.ExactlyOne, 0)
	if !cardinalityOK {
		t.Fatal("group cardinality")
	}
	capability, capabilityOK := model.NewAscendingCapability(typeID)
	if !capabilityOK {
		t.Fatal("ascending key capability")
	}
	builder := plan.NewBuilder(schemaID)
	if !builder.AddRelation(model.DefineRelationSchema(relation, []model.ColumnID{column}, []model.KeyID{key}, scope)) ||
		!builder.AddColumn(model.DefineColumnSchema(column, typeID)) ||
		!builder.AddKey(model.DefineKeySchema(key, []model.ColumnID{column})) ||
		!builder.AddScope(model.DefineScopeSchema(scope, nil, finite(t, "equality-scope"))) ||
		!builder.AddTypeCapability(capability) ||
		!builder.AddExpression(plan.DefineExpressionRef(expression, algebra.NewGroup(algebra.NewInput(relation), algebra.NewGroupContract(key, cardinality)))) {
		t.Fatal("equality-only schema declarations")
	}
	schema, schemaOK := builder.Build()
	if !schemaOK {
		t.Fatal("equality-only schema")
	}
	cert, refusal := certificate.Check(schema)
	if refusal != nil || !cert.Available() {
		t.Fatalf("equality-only certificate: %v", refusal)
	}
	if requirements := cert.AlgebraRequirements(); len(requirements) != 0 {
		t.Fatalf("read-only key requested algebra: %+v", requirements)
	}
	if requirements := cert.EqualityRequirements(); len(requirements) != 1 || requirements[0] != typeID {
		t.Fatalf("key equality requirements = %+v, want %v", requirements, typeID)
	}
	store, storeOK := identity.IssueStore()
	if !storeOK {
		t.Fatal("equality store")
	}
	fence, fenceOK := address.NewFence(schemaID, cert.Digest(), store, identity.MountID{0x66}, identity.Generation(1))
	if !fenceOK {
		t.Fatal("equality fence")
	}
	inventory := &mountInventory{fence: fence, relation: relation, column: column, key: key, scope: scope, expression: expression, typeID: typeID}
	registry := &countedEqualityRegistry{algebra: testAlgebra{typeID: typeID}, equality: testValueEquality{typeID: typeID}}
	mounted, mountedOK := witness.Specialize(cert, inventory, nil, registry, newLineageFactory(t, owner))
	if !mountedOK || !mounted.Available() {
		t.Fatal("equality-only mount refused")
	}
	if registry.algebraCalls != 0 {
		t.Fatalf("equality-only mount resolved algebra %d times, want zero", registry.algebraCalls)
	}
	if registry.equalityCalls != 1 {
		t.Fatalf("equality-only mount resolved equality %d times, want once", registry.equalityCalls)
	}
	if equality, equalityOK := mounted.Equality(typeID); !equalityOK || equality == nil || equality.Type() != typeID {
		t.Fatal("mounted equality authority missing")
	}
	if _, algebraOK := mounted.Algebra(typeID); algebraOK {
		t.Fatal("equality-only mount admitted an algebra")
	}
	if node, nodeOK := mounted.Arrangement().Execution().Entry(expression); !nodeOK || !node.Available() || node.Kind() != algebra.KindGroup {
		t.Fatal("equality group arrangement missing")
	}
}

func TestMountedDigestIsDeterministicAndGenerationFenced(t *testing.T) {
	fixture := newSemanticAdmissionFixture(t)
	first, ok := witness.Specialize(fixture.cert, &fixture.evidence, operationFactory{value: fixture.operation}, algebraRegistry{algebra: testAlgebra{typeID: fixture.typeID}}, newLineageFactory(t, fixture.owner))
	if !ok || !first.Available() {
		t.Fatal("first semantic mount refused")
	}
	second, ok := witness.Specialize(fixture.cert, &fixture.evidence, operationFactory{value: fixture.operation}, algebraRegistry{algebra: testAlgebra{typeID: fixture.typeID}}, newLineageFactory(t, fixture.owner))
	if !ok || !second.Available() {
		t.Fatal("second semantic mount refused")
	}
	if first.Digest() != second.Digest() || !first.Same(second) {
		t.Fatal("stable admission was not deterministic")
	}

	alteredInventory := *fixture.evidence.mountInventory
	alteredInventory.fence, ok = address.NewFence(fixture.schema, fixture.cert.Digest(), fixture.evidence.mountInventory.fence.StoreID(), fixture.evidence.mountInventory.fence.MountID(), identity.Generation(2))
	if !ok {
		t.Fatal("altered fence")
	}
	alteredEvidence := fixture.evidence
	alteredEvidence.mountInventory = &alteredInventory
	altered, ok := witness.Specialize(fixture.cert, &alteredEvidence, operationFactory{value: fixture.operation}, algebraRegistry{algebra: testAlgebra{typeID: fixture.typeID}}, newLineageFactory(t, fixture.owner))
	if !ok || !altered.Available() {
		t.Fatal("altered-generation mount refused unexpectedly")
	}
	if first.Same(altered) || first.Digest() == altered.Digest() {
		t.Fatal("generation mutation did not change mounted identity")
	}
	currentScope, currentScopeOK := first.Scope(fixture.scope)
	oldScope, oldScopeOK := altered.Scope(fixture.scope)
	if !currentScopeOK || !oldScopeOK {
		t.Fatal("scope lookup")
	}
	if oldScope.ValidFor(first.RuntimeFence()) {
		t.Fatal("stale scope crossed generation fence")
	}
	if _, ok := first.ConjoinScopes(currentScope, oldScope); ok {
		t.Fatal("stale scope was accepted by current mount")
	}
}

func TestSpecializeRefusesUnavailableDenominatorEvidence(t *testing.T) {
	fixture := newSemanticAdmissionFixture(t)
	missingEvidence := fixture.evidence
	missingEvidence.evidence = identity.ContentID{}
	if mounted, ok := witness.Specialize(fixture.cert, &missingEvidence, operationFactory{value: fixture.operation}, algebraRegistry{algebra: testAlgebra{typeID: fixture.typeID}}, newLineageFactory(t, fixture.owner)); ok || mounted.Available() {
		t.Fatal("unavailable denominator evidence accepted")
	}
	nilRows := fixture.evidence
	nilRows.rows = nil
	if mounted, ok := witness.Specialize(fixture.cert, &nilRows, operationFactory{value: fixture.operation}, algebraRegistry{algebra: testAlgebra{typeID: fixture.typeID}}, newLineageFactory(t, fixture.owner)); ok || mounted.Available() {
		t.Fatal("unstable/nil denominator row evidence accepted")
	}
}
