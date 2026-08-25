package witness_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
)

// newLineageFactory supplies the production lineage ABI to witness fixtures.
// Its owner is deliberately explicit: mounted witness admission must not
// infer or source lineage ownership from Inventory.
func newLineageFactory(t *testing.T, owner model.OwnerID) lineage.Factory {
	t.Helper()
	factory, ok := lineage.NewFactory(owner)
	if !ok {
		t.Fatal("lineage factory")
	}
	return factory
}

type countedLineageFactory struct {
	owner    model.OwnerID
	identity identity.ContentID
	calls    int
	result   lineage.Authority
	ok       bool
}

func (factory *countedLineageFactory) Bind(fence binding.Fence) (lineage.Authority, bool) {
	factory.calls++
	if factory.result != nil || !factory.ok {
		return factory.result, factory.ok
	}
	return countedLineageAuthority{fence: fence, owner: factory.owner, identity: factory.identity}, true
}

type countedLineageAuthority struct {
	fence    binding.Fence
	owner    model.OwnerID
	identity identity.ContentID
}

func (authority countedLineageAuthority) Fence() binding.Fence         { return authority.fence }
func (authority countedLineageAuthority) Owner() model.OwnerID         { return authority.owner }
func (authority countedLineageAuthority) Identity() identity.ContentID { return authority.identity }
func (authority countedLineageAuthority) Validate(model.LineageRef) bool {
	return authority.fence.Available() && authority.owner.Available() && authority.identity.Available()
}
func (authority countedLineageAuthority) Join(left, right model.LineageRef) (model.LineageRef, bool) {
	if !authority.Validate(left) || !authority.Validate(right) {
		return model.LineageRef{}, false
	}
	return left, true
}

func brokenLineageFactory(authority lineage.Authority) lineage.Factory {
	return brokenFactory{authority: authority}
}

type brokenFactory struct{ authority lineage.Authority }

func (factory brokenFactory) Bind(binding.Fence) (lineage.Authority, bool) {
	return factory.authority, true
}

func TestSpecializeRejectsMissingForeignAndBrokenLineageAuthority(t *testing.T) {
	fixture := newSemanticAdmissionFixture(t)
	registry := algebraRegistry{algebra: testAlgebra{typeID: fixture.typeID}}
	bind := func(factory lineage.Factory) (witness.Mounted, bool) {
		return witness.Specialize(fixture.cert, &fixture.evidence, operationFactory{value: fixture.operation}, registry, factory)
	}

	if mounted, ok := bind(nil); ok || mounted.Available() {
		t.Fatal("nil lineage factory accepted")
	}
	if mounted, ok := bind(brokenLineageFactory(nil)); ok || mounted.Available() {
		t.Fatal("nil lineage authority accepted")
	}

	foreignFence, ok := binding.NewFence(fixture.schema, fixture.evidence.mountInventory.fence.MountID(), identity.Generation(2))
	if !ok {
		t.Fatal("foreign runtime fence")
	}
	foreign := countedLineageAuthority{fence: foreignFence, owner: fixture.owner, identity: content(t, "foreign-lineage")}
	if mounted, ok := bind(brokenLineageFactory(foreign)); ok || mounted.Available() {
		t.Fatal("foreign lineage fence accepted")
	}
	broken := countedLineageAuthority{fence: binding.Fence{}, owner: fixture.owner, identity: content(t, "broken-lineage")}
	if mounted, ok := bind(brokenLineageFactory(broken)); ok || mounted.Available() {
		t.Fatal("broken lineage fence accepted")
	}

	zeroOwner := countedLineageAuthority{fence: fixtureRuntimeFence(t, fixture), identity: content(t, "zero-owner-lineage")}
	if mounted, ok := bind(brokenLineageFactory(zeroOwner)); ok || mounted.Available() {
		t.Fatal("zero lineage owner accepted")
	}
	zeroIdentity := countedLineageAuthority{fence: fixtureRuntimeFence(t, fixture), owner: fixture.owner}
	if mounted, ok := bind(brokenLineageFactory(zeroIdentity)); ok || mounted.Available() {
		t.Fatal("zero lineage identity accepted")
	}
}

func TestMountedDigestCoversLineageOwnerAndIdentity(t *testing.T) {
	fixture := newSemanticAdmissionFixture(t)
	registry := algebraRegistry{algebra: testAlgebra{typeID: fixture.typeID}}
	newMount := func(factory lineage.Factory) witness.Mounted {
		mounted, ok := witness.Specialize(fixture.cert, &fixture.evidence, operationFactory{value: fixture.operation}, registry, factory)
		if !ok || !mounted.Available() {
			t.Fatal("lineage digest fixture refused")
		}
		return mounted
	}

	ownerA := fixture.owner
	identityA := content(t, "lineage-digest-a")
	first := newMount(&countedLineageFactory{owner: ownerA, identity: identityA, ok: true})
	second := newMount(&countedLineageFactory{owner: ownerA, identity: identityA, ok: true})
	if first.Digest() != second.Digest() || !first.Same(second) {
		t.Fatal("same lineage authority was not deterministic")
	}
	identityB := content(t, "lineage-digest-b")
	if changed := newMount(&countedLineageFactory{owner: ownerA, identity: identityB, ok: true}); changed.Digest() == first.Digest() {
		t.Fatal("lineage identity was absent from mounted digest")
	}
	ownerB := issueOwner(t, "lineage-digest-owner-b")
	if changed := newMount(&countedLineageFactory{owner: ownerB, identity: identityA, ok: true}); changed.Digest() == first.Digest() {
		t.Fatal("lineage owner was absent from mounted digest")
	}
}

func TestZeroMountedLineageLookupIsUnavailable(t *testing.T) {
	var mounted witness.Mounted
	if authority, ok := mounted.Lineage(); ok || authority != nil {
		t.Fatal("zero mounted exposed lineage authority")
	}
}

func fixtureRuntimeFence(t *testing.T, fixture semanticAdmissionFixture) binding.Fence {
	t.Helper()
	value, ok := binding.NewFence(fixture.schema, fixture.evidence.mountInventory.fence.MountID(), fixture.evidence.mountInventory.fence.Generation())
	if !ok {
		t.Fatal("fixture runtime fence")
	}
	return value
}
