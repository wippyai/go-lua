package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestNamespaceIDsAreDeterministicAndInputSensitive(t *testing.T) {
	var targetID, programID, moduleID identity.ContentID
	targetID[0], programID[0], moduleID[0] = 1, 2, 3
	first := namespaceID(targetID, programID, moduleID, 1)
	if !first.Available() {
		t.Fatal("namespace ID unavailable")
	}
	second := namespaceID(targetID, programID, moduleID, 1)
	if second != first {
		t.Fatal("namespace ID was not deterministic")
	}
	if changed := namespaceID(targetID, programID, moduleID, 2); changed == first {
		t.Fatal("namespace ordinal did not affect identity")
	}
	if got := namespaceID(identity.ContentID{}, programID, moduleID, 1); got.Available() {
		t.Fatal("unavailable target acquired namespace ID")
	}
	plane := namespacePlaneID(targetID, []identity.ContentID{first})
	if !plane.Available() {
		t.Fatal("namespace plane ID unavailable")
	}
	if changed := namespacePlaneID(targetID, []identity.ContentID{first, second}); changed == plane {
		t.Fatal("namespace schema delta did not affect plane identity")
	}
}
