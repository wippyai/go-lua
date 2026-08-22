package publication

import (
	"encoding/hex"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

// branchValueObservationFormulaFence is the row-address formula of a branch
// value observation. The formula is the address: BranchValueObservationID
// re-derives it in valid() and compares, so an attachment whose id was minted
// under a different tag, a different argument order, or a different role name is
// refused. That makes the formula part of the published contract rather than an
// implementation detail, and the construction cut must not move it.
const branchValueObservationFormulaFence = "analysis/branch-value-observation/v2"

// branchValueObservationFenceHex is the address the formula derives for the
// fixed (mount, point, context) tuple below.
const branchValueObservationFenceHex = "efe39d1899a27df811f4d763841c46b854eafafc3a7cc300f9d06d89d46cdbd3"

// branchValueObservationFenceProducer is the preimage byte spelling the
// fenced formula pins. Construction reads that spelling from the sealed
// observation table; the fence names the bytes themselves.
const branchValueObservationFenceProducer schema.Key = "value-summary"

func rowAddressFenceID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0] = value
	return id
}

func rowAddressFenceContext(mount identity.ContentID) executioncontext.Context {
	context, _ := executioncontext.NewContext(
		rowAddressFenceID(0x23), mount, rowAddressFenceID(0x24), rowAddressFenceID(0x25),
	)
	return context
}

// TestBranchValueObservationRowAddressIsFenced pins the production formula's
// output through the production function, so the pin cannot drift from the
// formula the way a re-spelled literal in a test can.
func TestBranchValueObservationRowAddressIsFenced(t *testing.T) {
	mount, point := rowAddressFenceID(0x21), rowAddressFenceID(0x22)
	context := rowAddressFenceContext(mount)
	id, ok := BranchValueObservationID(mount, point, branchValueObservationFenceProducer, context)
	if !ok || !id.Available() {
		t.Fatal("the fenced branch value observation address no longer derives")
	}
	if got := hex.EncodeToString(id[:]); got != branchValueObservationFenceHex {
		t.Fatalf("branch value observation row address is %s, the fence pins %s; the formula is the published address", got, branchValueObservationFenceHex)
	}
	contextID := context.ID()
	expected, expectedOK := identity.DeriveContentID(branchValueObservationFormulaFence, mount[:], point[:], contextID[:], []byte(branchValueObservationFenceProducer))
	if !expectedOK || expected != id {
		t.Fatalf("the production formula no longer derives %s from (%q, mount, point, \"value-summary\")", branchValueObservationFenceHex, branchValueObservationFormulaFence)
	}
}

// TestBranchValueObservationRowAddressIsPositional records that every
// context-qualified argument reaches the address, so the pin above fences all
// of them: swapping a module-qualified mount, or dropping the role name, cannot
// reuse the published row.
func TestBranchValueObservationRowAddressIsPositional(t *testing.T) {
	mount, point := rowAddressFenceID(0x21), rowAddressFenceID(0x22)
	context := rowAddressFenceContext(mount)
	forward, forwardOK := BranchValueObservationID(mount, point, branchValueObservationFenceProducer, context)
	// The context is module-qualified; a swapped mount must therefore fail at
	// the admission boundary rather than minting an address for the wrong lane.
	reverse, reverseOK := BranchValueObservationID(point, mount, branchValueObservationFenceProducer, context)
	if !forwardOK || reverseOK {
		t.Fatal("the context-qualified branch value observation address accepted a foreign module")
	}
	if reverse.Available() && forward == reverse {
		t.Fatal("mount and point are interchangeable in the row address")
	}
	contextID := context.ID()
	roleless, rolelessOK := identity.DeriveContentID(branchValueObservationFormulaFence, mount[:], point[:], contextID[:])
	if !rolelessOK || roleless == forward {
		t.Fatal("the role name does not reach the row address")
	}
	foreign, foreignOK := identity.DeriveContentID(branchValueObservationFormulaFence+"x", mount[:], point[:], contextID[:], []byte(branchValueObservationFenceProducer))
	if !foreignOK || foreign == forward {
		t.Fatal("the formula tag does not reach the row address")
	}
	// An unavailable coordinate mints nothing: the address cannot be derived
	// from a partially known attachment.
	if _, ok := BranchValueObservationID(identity.ContentID{}, point, branchValueObservationFenceProducer, context); ok {
		t.Fatal("the row address was derived without a mount")
	}
	if _, ok := BranchValueObservationID(mount, identity.ContentID{}, branchValueObservationFenceProducer, context); ok {
		t.Fatal("the row address was derived without a point")
	}
	if _, ok := BranchValueObservationID(mount, point, "", context); ok {
		t.Fatal("the row address was derived without a producer")
	}
}
