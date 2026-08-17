package publication

import (
	"encoding/hex"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// branchValueObservationFormulaFence is the row-address formula of a branch
// value observation. The formula is the address: branchValueObservationAttachmentID
// re-derives it in valid() and compares, so an attachment whose id was minted
// under a different tag, a different argument order, or a different role name is
// refused. That makes the formula part of the published contract rather than an
// implementation detail, and the construction cut must not move it.
const branchValueObservationFormulaFence = "analysis/branch-value-observation/v1"

// branchValueObservationFenceHex is the address the formula derives for the
// fixed (mount, point) pair below.
const branchValueObservationFenceHex = "ef64ff7da3c7a52b3e63dadf769b85f41f8aac5ad417a6b7898db64ed60a94ff"

func rowAddressFenceID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0] = value
	return id
}

// TestBranchValueObservationRowAddressIsFenced pins the production formula's
// output through the production function, so the pin cannot drift from the
// formula the way a re-spelled literal in a test can.
func TestBranchValueObservationRowAddressIsFenced(t *testing.T) {
	mount, point := rowAddressFenceID(0x21), rowAddressFenceID(0x22)
	id, ok := branchValueObservationAttachmentID(mount, point)
	if !ok || !id.Available() {
		t.Fatal("the fenced branch value observation address no longer derives")
	}
	if got := hex.EncodeToString(id[:]); got != branchValueObservationFenceHex {
		t.Fatalf("branch value observation row address is %s, the fence pins %s; the formula is the published address", got, branchValueObservationFenceHex)
	}
	expected, expectedOK := identity.DeriveContentID(branchValueObservationFormulaFence, mount[:], point[:], []byte("value-summary"))
	if !expectedOK || expected != id {
		t.Fatalf("the production formula no longer derives %s from (%q, mount, point, \"value-summary\")", branchValueObservationFenceHex, branchValueObservationFormulaFence)
	}
}

// TestBranchValueObservationRowAddressIsPositional records that every argument
// of the formula reaches the address, so the pin above fences all of them:
// swapping mount for point, or dropping the role name, mints a different row.
func TestBranchValueObservationRowAddressIsPositional(t *testing.T) {
	mount, point := rowAddressFenceID(0x21), rowAddressFenceID(0x22)
	forward, forwardOK := branchValueObservationAttachmentID(mount, point)
	reverse, reverseOK := branchValueObservationAttachmentID(point, mount)
	if !forwardOK || !reverseOK {
		t.Fatal("the fenced branch value observation address no longer derives")
	}
	if forward == reverse {
		t.Fatal("mount and point are interchangeable in the row address")
	}
	roleless, rolelessOK := identity.DeriveContentID(branchValueObservationFormulaFence, mount[:], point[:])
	if !rolelessOK || roleless == forward {
		t.Fatal("the role name does not reach the row address")
	}
	foreign, foreignOK := identity.DeriveContentID(branchValueObservationFormulaFence+"x", mount[:], point[:], []byte("value-summary"))
	if !foreignOK || foreign == forward {
		t.Fatal("the formula tag does not reach the row address")
	}
	// An unavailable coordinate mints nothing: the address cannot be derived
	// from a partially known attachment.
	if _, ok := branchValueObservationAttachmentID(identity.ContentID{}, point); ok {
		t.Fatal("the row address was derived without a mount")
	}
	if _, ok := branchValueObservationAttachmentID(mount, identity.ContentID{}); ok {
		t.Fatal("the row address was derived without a point")
	}
}
