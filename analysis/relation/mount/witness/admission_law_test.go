package witness_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

type countedInventory struct {
	*evidenceInventory
	scopeCalls       int
	denominatorCalls int
}

func (inventory *countedInventory) ScopeRegion(id model.ScopeID) (witness.Region, bool) {
	inventory.scopeCalls++
	return inventory.evidenceInventory.ScopeRegion(id)
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

func TestSpecializeResolvesEachCertifiedObligationExactlyOnce(t *testing.T) {
	fixture := newSemanticAdmissionFixture(t)
	inventory := &countedInventory{evidenceInventory: &fixture.evidence}
	factory := &countedFactory{value: fixture.operation}
	registry := &countedAlgebraRegistry{algebra: testAlgebra{typeID: fixture.typeID}}
	mounted, ok := witness.Specialize(fixture.cert, inventory, factory, registry)
	if !ok || !mounted.Available() {
		t.Fatal("counted semantic mount refused")
	}
	if inventory.scopeCalls != 1 {
		t.Fatalf("ScopeRegion calls = %d, want exactly one", inventory.scopeCalls)
	}
	if inventory.denominatorCalls != 1 {
		t.Fatalf("ResolveDenominator calls = %d, want exactly one", inventory.denominatorCalls)
	}
	if factory.calls != 1 {
		t.Fatalf("Factory.Bind calls = %d, want exactly one", factory.calls)
	}
	if registry.calls != 1 {
		t.Fatalf("AlgebraRegistry.Resolve calls = %d, want exactly one", registry.calls)
	}
}

func TestMountedDigestIsDeterministicAndGenerationFenced(t *testing.T) {
	fixture := newSemanticAdmissionFixture(t)
	first, ok := witness.Specialize(fixture.cert, &fixture.evidence, operationFactory{value: fixture.operation}, algebraRegistry{algebra: testAlgebra{typeID: fixture.typeID}})
	if !ok || !first.Available() {
		t.Fatal("first semantic mount refused")
	}
	second, ok := witness.Specialize(fixture.cert, &fixture.evidence, operationFactory{value: fixture.operation}, algebraRegistry{algebra: testAlgebra{typeID: fixture.typeID}})
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
	altered, ok := witness.Specialize(fixture.cert, &alteredEvidence, operationFactory{value: fixture.operation}, algebraRegistry{algebra: testAlgebra{typeID: fixture.typeID}})
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
	if mounted, ok := witness.Specialize(fixture.cert, &missingEvidence, operationFactory{value: fixture.operation}, algebraRegistry{algebra: testAlgebra{typeID: fixture.typeID}}); ok || mounted.Available() {
		t.Fatal("unavailable denominator evidence accepted")
	}
	nilRows := fixture.evidence
	nilRows.rows = nil
	if mounted, ok := witness.Specialize(fixture.cert, &nilRows, operationFactory{value: fixture.operation}, algebraRegistry{algebra: testAlgebra{typeID: fixture.typeID}}); ok || mounted.Available() {
		t.Fatal("unstable/nil denominator row evidence accepted")
	}
}
