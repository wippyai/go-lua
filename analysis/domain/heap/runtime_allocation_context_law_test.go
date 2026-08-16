package heap_test

import (
	"crypto/sha256"
	"testing"

	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/identity"
)

func contextTestID(name string) identity.ContentID {
	return identity.ContentID(sha256.Sum256([]byte(name)))
}

// TestRuntimeAllocationContextsFenceReuseAndRevocation keeps physical runtime
// ownership out of ProgramArtifact while proving that a reusable allocation
// requirement can be bound independently in process, actor, shared, and
// thread contexts. It deliberately asks for no placement class: that requires
// the separate semantic escape/lifetime query.
func TestRuntimeAllocationContextsFenceReuseAndRevocation(t *testing.T) {
	_, schema, _ := compactHeapFixture(t, "runtime_allocation_context", compactHeapSource, nil)
	key := compactAllocationKeys(t, schema, 1)[0]
	requirement, requirementOK := schema.AllocationRequirementForKey(key)
	if !requirementOK || !requirement.Valid() || requirement.AllocationID() == (identity.ContentID{}) || requirement.MountID() == (identity.ContentID{}) {
		t.Fatal("allocation requirement")
	}

	authority, authorityOK := schema.BeginRuntimeAllocationContexts(contextTestID("policy/v1"))
	if !authorityOK {
		t.Fatal("runtime context authority")
	}
	processOwner, processOwnerOK := authority.ProcessOwner(contextTestID("process/main"))
	actorOwner, actorOwnerOK := authority.ActorOwner(contextTestID("actor/one"))
	threadOwner, threadOwnerOK := authority.ThreadOwner(contextTestID("thread/one"))
	sharedOwner, sharedOwnerOK := authority.SharedOwner(contextTestID("shared/pool"))
	process, processOK := authority.Process(processOwner)
	actor, actorOK := authority.Actor(actorOwner)
	thread, threadOK := authority.Thread(threadOwner)
	sharedAuthorization, authorized := authority.AuthorizeShared(contextTestID("share/sealed-v1"))
	shared, sharedOK := authority.Shared(sharedOwner, sharedAuthorization)
	if !processOwnerOK || !actorOwnerOK || !threadOwnerOK || !sharedOwnerOK || !processOK || !actorOK || !threadOK || !authorized || !sharedOK ||
		process.Class() != heapdomain.RuntimeAllocationContextProcess || actor.Class() != heapdomain.RuntimeAllocationContextActor ||
		thread.Class() != heapdomain.RuntimeAllocationContextThread || shared.Class() != heapdomain.RuntimeAllocationContextShared ||
		shared.SharedAuthorizationID() != contextTestID("share/sealed-v1") {
		t.Fatal("closed runtime context classes")
	}
	if _, ok := authority.Shared(sharedOwner, heapdomain.SharedAllocationAuthorization{}); ok {
		t.Fatal("shared context accepted without explicit authorization")
	}
	if _, ok := authority.Actor(processOwner); ok {
		t.Fatal("process owner retagged as actor")
	}
	if process.ContextID() == actor.ContextID() || actor.ContextID() == thread.ContextID() || thread.ContextID() == shared.ContextID() {
		t.Fatal("different runtime contexts aliased")
	}

	processMount, processMounted := authority.Mount(requirement, process)
	actorMount, actorMounted := authority.Mount(requirement, actor)
	threadMount, threadMounted := authority.Mount(requirement, thread)
	sharedMount, sharedMounted := authority.Mount(requirement, shared)
	if !processMounted || !actorMounted || !threadMounted || !sharedMounted ||
		processMount.ID() == actorMount.ID() || actorMount.ID() == threadMount.ID() || threadMount.ID() == sharedMount.ID() {
		t.Fatal("same requirement did not remain context-distinct")
	}
	if unavailable, ok := authority.Unavailable(actorMount); !ok || !unavailable.Valid() || unavailable.MountedID() != actorMount.ID() || unavailable.Availability() != heapdomain.PlacementFactorsUnbound {
		t.Fatal("missing explicit unavailable placement outcome")
	}

	otherAuthority, otherOK := schema.BeginRuntimeAllocationContexts(contextTestID("policy/v2"))
	if !otherOK {
		t.Fatal("second runtime authority")
	}
	otherActorOwner, otherActorOwnerOK := otherAuthority.ActorOwner(contextTestID("actor/one"))
	otherActor, otherActorOK := otherAuthority.Actor(otherActorOwner)
	if !otherActorOwnerOK || !otherActorOK {
		t.Fatal("second runtime actor")
	}
	if _, ok := otherAuthority.Actor(actorOwner); ok {
		t.Fatal("owner capability crossed runtime authority")
	}
	if _, ok := authority.Mount(requirement, otherActor); ok {
		t.Fatal("foreign runtime context mounted into authority")
	}
	otherSharedOwner, otherSharedOwnerOK := otherAuthority.SharedOwner(contextTestID("shared/pool"))
	if !otherSharedOwnerOK {
		t.Fatal("second runtime shared owner")
	}
	if _, ok := otherAuthority.Shared(otherSharedOwner, sharedAuthorization); ok {
		t.Fatal("shared authorization crossed runtime authority")
	}

	authority.Close()
	if process.Valid() || actorMount.Valid() || shared.Valid() {
		t.Fatal("closed runtime authority left live contexts or mounts")
	}

	// Recreating an equal scalar policy may intentionally create a new runtime
	// binding for the reusable requirement, but it must never revive or accept
	// a capability issued by the closed binding.
	recreated, recreatedOK := schema.BeginRuntimeAllocationContexts(contextTestID("policy/v1"))
	if !recreatedOK {
		t.Fatal("recreated runtime context authority")
	}
	if _, ok := recreated.Actor(actorOwner); ok {
		t.Fatal("closed authority owner revived under recreated authority")
	}
	if _, ok := recreated.Mount(requirement, actor); ok {
		t.Fatal("closed authority context mounted under recreated authority")
	}
	recreatedActorOwner, recreatedActorOwnerOK := recreated.ActorOwner(contextTestID("actor/one"))
	recreatedActor, recreatedActorOK := recreated.Actor(recreatedActorOwner)
	if !recreatedActorOwnerOK || !recreatedActorOK {
		t.Fatal("recreated runtime actor")
	}
	if _, ok := recreated.Mount(requirement, recreatedActor); !ok {
		t.Fatal("reusable requirement did not bind to new runtime")
	}
}

// TestRuntimeAllocationContextsEqualContentIssuers uses two independently
// sealed real Heap owners, rather than a fabricated test shell. Semantic IDs
// are stable across equal content; private issuer capabilities remain fenced.
func TestRuntimeAllocationContextsEqualContentIssuers(t *testing.T) {
	_, leftSchema, _ := compactHeapFixture(t, "runtime_allocation_equal_content", compactHeapSource, nil)
	_, rightSchema, _ := compactHeapFixture(t, "runtime_allocation_equal_content", compactHeapSource, nil)
	if leftSchema.ContentID() != rightSchema.ContentID() {
		t.Fatal("equal content heap identities")
	}
	leftKey := compactAllocationKeys(t, leftSchema, 1)[0]
	rightKey := compactAllocationKeys(t, rightSchema, 1)[0]
	leftRequirement, leftRequirementOK := leftSchema.AllocationRequirementForKey(leftKey)
	rightRequirement, rightRequirementOK := rightSchema.AllocationRequirementForKey(rightKey)
	if !leftRequirementOK || !rightRequirementOK || leftRequirement.ID() != rightRequirement.ID() {
		t.Fatal("equal content allocation requirements")
	}

	policy := contextTestID("equal-content-policy")
	left, leftOK := leftSchema.BeginRuntimeAllocationContexts(policy)
	right, rightOK := rightSchema.BeginRuntimeAllocationContexts(policy)
	if !leftOK || !rightOK {
		t.Fatal("equal content runtime authorities")
	}
	actorID := contextTestID("equal-content-actor")
	leftOwner, leftOwnerOK := left.ActorOwner(actorID)
	rightOwner, rightOwnerOK := right.ActorOwner(actorID)
	leftContext, leftContextOK := left.Actor(leftOwner)
	rightContext, rightContextOK := right.Actor(rightOwner)
	if !leftOwnerOK || !rightOwnerOK || !leftContextOK || !rightContextOK ||
		leftOwner.ID() != rightOwner.ID() || leftContext.ContextID() != rightContext.ContextID() {
		t.Fatal("semantic identity leaked issuer generation")
	}
	leftMounted, leftMountedOK := left.Mount(leftRequirement, leftContext)
	rightMounted, rightMountedOK := right.Mount(rightRequirement, rightContext)
	if !leftMountedOK || !rightMountedOK || leftMounted.ID() != rightMounted.ID() {
		t.Fatal("mounted semantic identity leaked issuer generation")
	}
	if _, ok := left.Actor(rightOwner); ok {
		t.Fatal("foreign equal-content owner accepted")
	}
	if _, ok := left.Mount(leftRequirement, rightContext); ok {
		t.Fatal("foreign equal-content context accepted")
	}

	leftSharedOwner, leftSharedOwnerOK := left.SharedOwner(contextTestID("equal-content-shared-owner"))
	rightSharedOwner, rightSharedOwnerOK := right.SharedOwner(contextTestID("equal-content-shared-owner"))
	leftAuthorization, leftAuthorizationOK := left.AuthorizeShared(contextTestID("equal-content-sharing-policy"))
	rightAuthorization, rightAuthorizationOK := right.AuthorizeShared(contextTestID("equal-content-sharing-policy"))
	if !leftSharedOwnerOK || !rightSharedOwnerOK || !leftAuthorizationOK || !rightAuthorizationOK {
		t.Fatal("equal-content shared capabilities")
	}
	if _, ok := left.Shared(leftSharedOwner, rightAuthorization); ok {
		t.Fatal("foreign equal-content shared authorization accepted")
	}
	if _, ok := right.Shared(rightSharedOwner, leftAuthorization); ok {
		t.Fatal("foreign equal-content shared authorization accepted")
	}
}
