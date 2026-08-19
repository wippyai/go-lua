package value

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// The list position is a dimension of exactly the two families that fan one
// authored statement out over a value list. A read names a single cell, so a
// read carrying a position names nothing Program ever issues and is not an
// identity; a bind and an assignment write are issued once per position, so
// each of their positions is a distinct identity.
func TestStorageTransferRefAdmitsPositionOnlyForPositionalFamilies(t *testing.T) {
	var link, mount, occurrence identity.ContentID
	link[31], mount[31], occurrence[31] = 1, 2, 3

	for _, testcase := range []struct {
		name     string
		kind     storageTransferKind
		position uint32
		valid    bool
	}{
		{name: "read at the only position it has", kind: storageTransferRead, position: 0, valid: true},
		{name: "read carrying a list position", kind: storageTransferRead, position: 1, valid: false},
		{name: "bind at the first target", kind: storageTransferBind, position: 0, valid: true},
		{name: "bind at a later target", kind: storageTransferBind, position: 3, valid: true},
		{name: "write at the first target", kind: storageTransferWrite, position: 0, valid: true},
		{name: "write at a later target", kind: storageTransferWrite, position: 3, valid: true},
		{name: "no family", kind: storageTransferInvalid, position: 0, valid: false},
	} {
		ref := StorageTransferRef{linkID: link, mount: mount, occurrence: occurrence, kind: testcase.kind, position: testcase.position}
		if got := ref.valid(); got != testcase.valid {
			t.Fatalf("%s: identity valid=%t, want %t", testcase.name, got, testcase.valid)
		}
	}
}

// Each admitted position is its own relation: the content identity separates
// two targets of the same statement, so one target's carried value can never
// replay through the other's receipt.
func TestStorageTransferIdentitySeparatesTargetPositions(t *testing.T) {
	var link, mount, occurrence identity.ContentID
	link[31], mount[31], occurrence[31] = 1, 2, 3

	first := storageTransferIdentity(StorageTransferRef{linkID: link, mount: mount, occurrence: occurrence, kind: storageTransferWrite, position: 0})
	second := storageTransferIdentity(StorageTransferRef{linkID: link, mount: mount, occurrence: occurrence, kind: storageTransferWrite, position: 1})
	if !first.Available() || !second.Available() {
		t.Fatal("positional write identities are unavailable")
	}
	if first == second {
		t.Fatal("two target positions share one content identity")
	}
}
