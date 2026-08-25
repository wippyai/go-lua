package lineage

import (
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

func testContent(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("analysis/relation/semantic/lineage/law/v1", []byte(label))
	if !ok {
		t.Fatalf("derive content %q", label)
	}
	return value
}

func testOwner(t *testing.T, label string) model.OwnerID {
	t.Helper()
	owner, ok := model.IssueOwnerID(testContent(t, "owner/"+label))
	if !ok {
		t.Fatalf("issue owner %q", label)
	}
	return owner
}

func testLineage(t *testing.T, owner model.OwnerID, label string) model.LineageRef {
	t.Helper()
	ref, ok := model.IssueLineageRef(owner, testContent(t, "lineage/"+label))
	if !ok {
		t.Fatalf("issue lineage %q", label)
	}
	return ref
}

func testFence(t *testing.T, label string, mount byte, generation identity.Generation) binding.Fence {
	t.Helper()
	owner := testOwner(t, "schema/"+label)
	schema, ok := model.IssueSchemaID(owner, testContent(t, "schema/"+label))
	if !ok {
		t.Fatalf("issue schema")
	}
	var mountID identity.MountID
	mountID[0] = mount
	fence, ok := binding.NewFence(schema, mountID, generation)
	if !ok {
		t.Fatalf("bind fence")
	}
	return fence
}

func newTestAuthority(t *testing.T, owner model.OwnerID, fence binding.Fence) Authority {
	t.Helper()
	factory, ok := NewFactory(owner)
	if !ok {
		t.Fatalf("construct factory")
	}
	authority, ok := factory.Bind(fence)
	if !ok || authority == nil {
		t.Fatalf("bind authority")
	}
	return authority
}

func TestAuthorityJoinsAreACI(t *testing.T) {
	lineageOwner := testOwner(t, "authority")
	left := testLineage(t, testOwner(t, "left"), "left")
	middle := testLineage(t, testOwner(t, "middle"), "middle")
	right := testLineage(t, testOwner(t, "right"), "right")
	authority := newTestAuthority(t, lineageOwner, testFence(t, "aci", 1, 1))

	leftMiddle, ok := authority.Join(left, middle)
	if !ok {
		t.Fatal("left-middle join")
	}
	middleLeft, ok := authority.Join(middle, left)
	if !ok || middleLeft != leftMiddle {
		t.Fatal("commutativity")
	}
	if !authority.Validate(leftMiddle) {
		t.Fatal("derived ref was not admitted")
	}

	leftMiddleRight, ok := authority.Join(leftMiddle, right)
	if !ok {
		t.Fatal("nested left association")
	}
	middleRight, ok := authority.Join(middle, right)
	if !ok {
		t.Fatal("middle-right join")
	}
	rightAssociated, ok := authority.Join(left, middleRight)
	if !ok || rightAssociated != leftMiddleRight {
		t.Fatal("associativity")
	}

	duplicate, ok := authority.Join(leftMiddle, left)
	if !ok || duplicate != leftMiddle {
		t.Fatal("idempotence/absorption")
	}
	single, ok := authority.Join(left, left)
	if !ok || single != left {
		t.Fatal("atomic idempotence")
	}
}

func TestAuthorityIdentityIgnoresFenceAndOrder(t *testing.T) {
	authorityOwner := testOwner(t, "stable-authority")
	left := testLineage(t, testOwner(t, "stable-left"), "left")
	right := testLineage(t, testOwner(t, "stable-right"), "right")

	first := newTestAuthority(t, authorityOwner, testFence(t, "first", 1, 1))
	second := newTestAuthority(t, authorityOwner, testFence(t, "second", 2, 7))
	if first.Identity() != second.Identity() {
		t.Fatal("authority identity depends on runtime fence")
	}
	firstResult, ok := first.Join(left, right)
	if !ok {
		t.Fatal("first join")
	}
	secondResult, ok := second.Join(right, left)
	if !ok {
		t.Fatal("second join")
	}
	if firstResult != secondResult {
		t.Fatal("join identity depends on fence or input order")
	}
}

func TestAuthorityRejectsUnknownOwnedReferences(t *testing.T) {
	authorityOwner := testOwner(t, "owned")
	authority := newTestAuthority(t, authorityOwner, testFence(t, "owned", 1, 1))
	unknown := testLineage(t, authorityOwner, "fabricated")
	foreign := testLineage(t, testOwner(t, "foreign"), "foreign")
	if authority.Validate(unknown) {
		t.Fatal("unknown authority-owned ref accepted")
	}
	if _, ok := authority.Join(unknown, foreign); ok {
		t.Fatal("unknown authority-owned ref joined")
	}
	if !authority.Validate(foreign) {
		t.Fatal("valid foreign atom rejected")
	}
	if authority.Validate(model.LineageRef{}) {
		t.Fatal("zero ref accepted")
	}
	if _, ok := authority.Join(model.LineageRef{}, foreign); ok {
		t.Fatal("zero ref joined")
	}
}

func TestAuthorityRejectsOwnedRefFromAnotherFence(t *testing.T) {
	authorityOwner := testOwner(t, "fence-owner")
	factory, ok := NewFactory(authorityOwner)
	if !ok {
		t.Fatal("construct factory")
	}
	first, ok := factory.Bind(testFence(t, "fence-first", 1, 1))
	if !ok {
		t.Fatal("bind first")
	}
	second, ok := factory.Bind(testFence(t, "fence-second", 2, 1))
	if !ok {
		t.Fatal("bind second")
	}
	foreign := testLineage(t, testOwner(t, "fence-input"), "input")
	owned, ok := first.Join(foreign, testLineage(t, testOwner(t, "fence-input-2"), "input-2"))
	if !ok {
		t.Fatal("derive first owned ref")
	}
	if second.Validate(owned) {
		t.Fatal("owned ref crossed runtime authority")
	}
	if _, ok := second.Join(owned, foreign); ok {
		t.Fatal("owned ref crossed runtime authority through join")
	}
}

func TestAuthorityConcurrentHashConsing(t *testing.T) {
	authorityOwner := testOwner(t, "concurrent-authority")
	authority := newTestAuthority(t, authorityOwner, testFence(t, "concurrent", 1, 1))
	left := testLineage(t, testOwner(t, "concurrent-left"), "left")
	right := testLineage(t, testOwner(t, "concurrent-right"), "right")

	const workers = 32
	results := make(chan model.LineageRef, workers)
	var wait sync.WaitGroup
	wait.Add(workers)
	for index := 0; index < workers; index++ {
		go func(index int) {
			defer wait.Done()
			var result model.LineageRef
			var ok bool
			if index%2 == 0 {
				result, ok = authority.Join(left, right)
			} else {
				result, ok = authority.Join(right, left)
			}
			if !ok {
				t.Errorf("concurrent join %d refused", index)
				return
			}
			results <- result
		}(index)
	}
	wait.Wait()
	close(results)
	var expected model.LineageRef
	for result := range results {
		if !expected.Available() {
			expected = result
			continue
		}
		if result != expected {
			t.Fatal("concurrent joins produced different identities")
		}
	}
	if !expected.Available() || !authority.Validate(expected) {
		t.Fatal("concurrent result was not retained in authority arena")
	}
}

func TestAuthorityRejectsInconsistentArenaEntries(t *testing.T) {
	authorityOwner := testOwner(t, "inconsistent-authority")
	first := testLineage(t, testOwner(t, "inconsistent-first"), "first")
	second := testLineage(t, testOwner(t, "inconsistent-second"), "second")
	third := testLineage(t, testOwner(t, "inconsistent-third"), "third")
	authorityValue := newTestAuthority(t, authorityOwner, testFence(t, "inconsistent", 1, 1))
	derived, ok := authorityValue.Join(first, second)
	if !ok {
		t.Fatal("derive authority-owned ref")
	}

	concrete, ok := authorityValue.(*authority)
	if !ok {
		t.Fatal("authority implementation type changed")
	}
	concrete.mu.Lock()
	concrete.nodes[derived.Content()] = node{atoms: []atom{
		mustAtom(t, first),
		mustAtom(t, third),
	}}
	concrete.mu.Unlock()
	if authorityValue.Validate(derived) {
		t.Fatal("inconsistent cache entry validated")
	}
	if _, ok := authorityValue.Join(derived, third); ok {
		t.Fatal("inconsistent cache entry joined")
	}
}

func TestFactoryRejectsUnavailableInputs(t *testing.T) {
	if factory, ok := NewFactory(model.OwnerID{}); ok || factory != nil {
		t.Fatal("unavailable owner accepted")
	}
	owner := testOwner(t, "factory")
	factory, ok := NewFactory(owner)
	if !ok {
		t.Fatal("construct factory")
	}
	if authority, ok := factory.Bind(binding.Fence{}); ok || authority != nil {
		t.Fatal("unavailable fence accepted")
	}
}

func mustAtom(t *testing.T, ref model.LineageRef) atom {
	t.Helper()
	value, ok := newAtom(ref)
	if !ok {
		t.Fatalf("construct atom")
	}
	return value
}
