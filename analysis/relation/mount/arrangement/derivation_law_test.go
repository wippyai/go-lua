package arrangement_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/derivation"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Every mounted root exposes its already-sealed occurrence derivative through
// the named child package. The test intentionally redeems by occurrence only;
// no expression or inventory is available on this surface.
func TestExecutionRedeemsSealedOccurrenceDerivation(t *testing.T) {
	value := newCensusFixture(t)
	addresses := value.addresses(t)
	book, ok := address.Bind(value.certificate, addresses)
	if !ok {
		t.Fatal("address bind")
	}
	plan, ok := arrangement.Derive(value.certificate, book, &arrangementInventory{fence: book.Fence(), slot: 801}, expand.EmptyCatalog(), []binding.PartitionDirectory{})
	if !ok || !plan.Available() {
		t.Fatal("derive")
	}
	execution := plan.Execution()
	for _, id := range execution.ExpressionIDs() {
		derivative, found := execution.Derivation(id)
		if !found || !derivative.Available() || derivative.Root() != id {
			t.Fatalf("missing derivative for %v", id)
		}
		for index := 0; index < derivative.Len(); index++ {
			path, pathOK := derivative.PathAt(index)
			if !pathOK || !path.Available() || path.Root() != id || path.Occurrence() != uint32(index) || !path.Digest().Available() {
				t.Fatalf("invalid path %d for %v", index, id)
			}
			for frameIndex := 0; frameIndex < path.FrameCount(); frameIndex++ {
				frame, frameOK := path.FrameAt(frameIndex)
				if !frameOK || !frame.Available() {
					t.Fatalf("invalid frame %d/%d for %v", frameIndex, index, id)
				}
				for siblingIndex := 0; siblingIndex < frame.SiblingCount(); siblingIndex++ {
					sibling, siblingOK := frame.SiblingAt(siblingIndex)
					if !siblingOK || !sibling.Available() || !sibling.Physical().Available() {
						t.Fatalf("invalid sibling %d/%d/%d for %v", siblingIndex, frameIndex, index, id)
					}
				}
			}
			if !path.Leaf().Available() || !path.Leaf().Physical().Available() {
				t.Fatalf("invalid leaf %d for %v", index, id)
			}
		}
	}
	var zero derivation.Plan
	if zero.Available() {
		t.Fatal("zero derivative redeemed")
	}
}
