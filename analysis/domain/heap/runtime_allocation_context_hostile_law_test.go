package heap

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow"
)

func hostileRuntimeID(text string) identity.ContentID {
	return identity.ContentID(sha256.Sum256([]byte(text)))
}

// hostileRuntimeAuthority is intentionally a private-field fixture for
// recomputation tests only. The separately external equal-content law seals
// two real Heap owners through the public artifact-native API; keeping that
// construction out of package heap avoids an import cycle through grammar
// and Call/dispatch.
func hostileRuntimeAuthority(heapID, policyID identity.ContentID) *RuntimeAllocationContextAuthority {
	return &RuntimeAllocationContextAuthority{owner: &schema{id: heapID}, policyID: policyID, generation: newRuntimeAllocationGeneration()}
}

func hostileRequirement(heapID identity.ContentID) AllocationRequirement {
	requirement := AllocationRequirement{
		heapID:     heapID,
		keyID:      hostileRuntimeID("key"),
		artifactID: hostileRuntimeID("artifact"),
		mount:      hostileRuntimeID("mount"),
		program:    hostileRuntimeID("program"),
		allocation: hostileRuntimeID("allocation"),
		localID:    hostileRuntimeID("local"),
		kind:       AllocationTable,
		form:       flow.AllocationFormClosed,
	}
	requirement.id = allocationRequirementID(requirement.heapID, requirement.keyID, requirement.artifactID, requirement.mount, requirement.program, requirement.allocation, requirement.localID, requirement.kind, requirement.form)
	return requirement
}

func TestRuntimeAllocationContextHostileRecomputation(t *testing.T) {
	heapID := hostileRuntimeID("heap")
	authority := hostileRuntimeAuthority(heapID, hostileRuntimeID("policy"))
	actorOwner, ok := authority.ActorOwner(hostileRuntimeID("actor"))
	if !ok {
		t.Fatal("actor owner")
	}
	actor, ok := authority.Actor(actorOwner)
	if !ok {
		t.Fatal("actor context")
	}
	sharedOwner, ok := authority.SharedOwner(hostileRuntimeID("shared-owner"))
	if !ok {
		t.Fatal("shared owner")
	}
	sharedAuthorization, ok := authority.AuthorizeShared(hostileRuntimeID("sharing-policy"))
	if !ok {
		t.Fatal("shared authorization")
	}
	shared, ok := authority.Shared(sharedOwner, sharedAuthorization)
	if !ok {
		t.Fatal("shared context")
	}
	requirement := hostileRequirement(heapID)
	mounted, ok := authority.Mount(requirement, actor)
	if !ok {
		t.Fatal("mounted receipt")
	}

	badOwner := actorOwner
	badOwner.id = hostileRuntimeID("actor/spliced")
	if badOwner.valid() {
		t.Fatal("spliced owner accepted")
	}
	badAuthorization := sharedAuthorization
	badAuthorization.id = hostileRuntimeID("sharing-policy/spliced")
	if badAuthorization.valid() {
		t.Fatal("spliced shared authorization accepted")
	}
	badContext := actor
	badContext.owner = sharedOwner
	if badContext.valid() {
		t.Fatal("spliced context owner accepted")
	}
	badSharedContext := shared
	badSharedContext.sharedBy = hostileRuntimeID("sharing-policy/spliced")
	if badSharedContext.valid() {
		t.Fatal("spliced shared context accepted")
	}
	badRequirement := requirement
	badRequirement.artifactID = hostileRuntimeID("artifact/spliced")
	if badRequirement.valid() {
		t.Fatal("spliced requirement accepted")
	}
	badRequirement = requirement
	badRequirement.localID = hostileRuntimeID("local/spliced")
	if badRequirement.valid() {
		t.Fatal("mutated requirement local identity accepted")
	}
	badMounted := mounted
	badMounted.id = hostileRuntimeID("mount/spliced")
	if badMounted.valid() {
		t.Fatal("spliced mounted receipt accepted")
	}
}
