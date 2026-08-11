package ownership

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	proglink "github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestOwnershipRequiresExactLiveHeapLink(t *testing.T) {
	left := ownershipLiveHeapFixture(t, "ownership_live_heap_fence")
	right := ownershipLiveHeapFixture(t, "ownership_live_heap_fence")
	if left == right || left.ContentID() != right.ContentID() {
		t.Fatal("fixture must provide distinct same-content Link authorities")
	}
	leftHeap, leftHeapOK := heap.Seal(left)
	rightHeap, rightHeapOK := heap.Seal(right)
	if !leftHeapOK || !rightHeapOK || leftHeap.Link() != left || rightHeap.Link() != right || leftHeap.LinkContentID() != rightHeap.LinkContentID() {
		t.Fatal("fixture Heap authorities")
	}
	if _, ok := NewSchema(left, rightHeap); ok {
		t.Fatal("Ownership accepted a same-content foreign Heap authority")
	}
	schema, ok := NewSchema(left, leftHeap)
	if !ok || !schema.Valid() || schema.Link() != left {
		t.Fatal("Ownership rejected its local Heap authority")
	}
	reboundHeap, ok := leftHeap.Rebind(right)
	if !ok || !reboundHeap.Valid() || reboundHeap.Link() != right || reboundHeap.LinkContentID() != leftHeap.LinkContentID() {
		t.Fatal("Heap cold rebind did not produce the right live Link authority")
	}
	reboundSchema, ok := NewSchema(right, reboundHeap)
	if !ok || !reboundSchema.Valid() || reboundSchema.Link() != right {
		t.Fatal("Ownership rejected a lawful cold-rebound Heap authority")
	}
	origin, ok := schema.OriginAt(0)
	if !ok {
		t.Fatal("Ownership origin")
	}
	originRef, ok := schema.OriginRef(origin)
	if !ok {
		t.Fatal("Ownership origin reference")
	}
	reboundOrigin, ok := reboundSchema.RebindOrigin(origin)
	if !ok || !reboundOrigin.Valid() || reboundOrigin == origin || reboundOrigin.Kind() != origin.Kind() {
		t.Fatal("Ownership origin cold rebind")
	}
	reboundRef, ok := reboundSchema.OriginRef(reboundOrigin)
	if !ok || reboundRef != originRef {
		t.Fatal("Ownership origin reference changed across cold rebind")
	}
	broken := *schema.owner
	broken.heap = rightHeap
	if (Schema{owner: &broken}).Valid() {
		t.Fatal("Ownership.Valid accepted a privately inconsistent Heap/Link pair")
	}
}

func ownershipLiveHeapFixture(t *testing.T, name string) *proglink.Link {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: name + ".lua", Text: []byte(`
local root = {}
local function retained() return root end
root.child = retained
return root
`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := proglink.Seal(&proglink.Spec{Target: contract, Modules: []linkproject.Module{{Name: name, Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	return source
}
