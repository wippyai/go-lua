package relation_test

import (
	"testing"

	"github.com/wippyai/go-lua/domain/relationfixture"
	"github.com/wippyai/go-lua/domain/typestate/relation"
)

// TestTheStateCellSpaceIsOneSealPerLink states the axis mount: one link's
// typestate columns are addressed over exactly one space, that space owns
// every cell it hands out, and it normalizes a cell to the dense position its
// own seal assigned rather than to a second numbering.
func TestTheStateCellSpaceIsOneSealPerLink(t *testing.T) {
	fixture := relationfixture.New(t)
	const allocations, protocols = 3, 2
	link := fixture.Values.LinkOwner().ContentID()
	if !link.Available() {
		t.Fatal("the fixture's value schema names no link")
	}
	space, sealed := relation.SealStateCellSpace(link, allocations, protocols)
	if !sealed || !space.Available() {
		t.Fatal("seal the state-cell space")
	}
	if space.CellCount() == 0 {
		t.Fatal("the sealed space holds no cell")
	}

	seen := map[uint32]bool{}
	for index := 0; index < space.CellCount(); index++ {
		cell, ok := space.CellAt(index)
		if !ok {
			t.Fatalf("cell %d is not issued", index)
		}
		if !space.Owns(cell) {
			t.Fatalf("cell %d is not owned by the space that issued it", index)
		}
		dense, normalized := space.DenseIndex(cell)
		if !normalized {
			t.Fatalf("cell %d normalizes to no dense position", index)
		}
		if seen[dense] {
			t.Fatalf("two cells normalize to dense position %d", dense)
		}
		seen[dense] = true
	}
	if len(seen) != space.CellCount() {
		t.Fatalf("the space holds %d cells and %d dense positions", space.CellCount(), len(seen))
	}

	// A second seal of the same link is the same space, and a cell of one
	// link's space is not owned by another's: the columns of one link are
	// addressed over exactly one space.
	again, resealed := relation.SealStateCellSpace(link, allocations, protocols)
	if !resealed {
		t.Fatal("reseal the state-cell space")
	}
	first, firstOK := space.ContentID()
	second, secondOK := again.ContentID()
	if !firstOK || !secondOK || first != second {
		t.Fatal("two seals of one link named two spaces")
	}
	other, otherOK := fixture.Root.ContentID()
	if !otherOK {
		t.Fatal("the fixture root names no content")
	}
	foreign, foreignOK := relation.SealStateCellSpace(other, allocations, protocols)
	if !foreignOK {
		t.Fatal("seal a second link's space")
	}
	cell, cellOK := space.CellAt(0)
	if !cellOK {
		t.Fatal("cell 0 is not issued")
	}
	if foreign.Owns(cell) {
		t.Fatal("one link's space owns another link's cell")
	}
}
