package statecell

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

func testLinkID(t *testing.T, seed string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("go-lua/typestate-statecell-law", []byte(seed))
	if !ok {
		t.Fatal("link identity unavailable")
	}
	return id
}

// The space is the product of the two directories it borrows, and every cell
// of that product is addressable in both directions. A coordinate that names
// no resource-protocol pair, or a pair outside either directory, is not a
// coordinate.
func TestSpaceIsTheCompleteResourceProtocolProduct(t *testing.T) {
	space, ok := Seal(testLinkID(t, "product"), 3, 2)
	if !ok {
		t.Fatal("space rejected")
	}
	if space.CellCount() != 6 {
		t.Fatalf("cell count = %d, want 3 roots x 2 protocols", space.CellCount())
	}
	seen := make(map[uint32]struct{}, space.CellCount())
	for allocation := 0; allocation < space.AllocationCount(); allocation++ {
		for protocol := 1; protocol <= space.ProtocolCount(); protocol++ {
			cell, cellOK := space.Cell(allocation, vocabulary.Protocol(protocol))
			if !cellOK {
				t.Fatalf("cell (%d, %d) is unavailable", allocation, protocol)
			}
			if _, duplicate := seen[cell.Index()]; duplicate {
				t.Fatalf("cell (%d, %d) reuses coordinate %d", allocation, protocol, cell.Index())
			}
			seen[cell.Index()] = struct{}{}
			back, backOK := space.Allocation(cell)
			if !backOK || back != allocation {
				t.Fatalf("cell %d resolves to allocation %d/%v, want %d", cell.Index(), back, backOK, allocation)
			}
			owner, ownerOK := space.Protocol(cell)
			if !ownerOK || owner != vocabulary.Protocol(protocol) {
				t.Fatalf("cell %d resolves to protocol %d/%v, want %d", cell.Index(), owner, ownerOK, protocol)
			}
		}
	}
	if len(seen) != space.CellCount() {
		t.Fatalf("the product covered %d of %d coordinates", len(seen), space.CellCount())
	}
	for _, absent := range []struct {
		allocation int
		protocol   vocabulary.Protocol
	}{{-1, 1}, {3, 1}, {0, 0}, {0, 3}} {
		if _, cellOK := space.Cell(absent.allocation, absent.protocol); cellOK {
			t.Fatalf("(%d, %d) outside the two directories issued a cell", absent.allocation, absent.protocol)
		}
	}
}

// The layout is allocation-major, so one resource's protocols occupy one
// contiguous run. A consumer that judges a resource against every protocol it
// participates in therefore walks a run rather than a stride.
func TestOneResourcesProtocolsAreContiguous(t *testing.T) {
	space, ok := Seal(testLinkID(t, "layout"), 4, 3)
	if !ok {
		t.Fatal("space rejected")
	}
	for allocation := 0; allocation < space.AllocationCount(); allocation++ {
		first, firstOK := space.Cell(allocation, 1)
		last, lastOK := space.Cell(allocation, vocabulary.Protocol(space.ProtocolCount()))
		if !firstOK || !lastOK {
			t.Fatalf("resource %d has no run", allocation)
		}
		if int(last.Index())-int(first.Index()) != space.ProtocolCount()-1 {
			t.Fatalf("resource %d spans %d coordinates, want %d contiguous", allocation, last.Index()-first.Index()+1, space.ProtocolCount())
		}
	}
}

// A cell is owner-fenced: the space that issued it is part of the handle, so a
// coordinate minted under one Link cannot be read as a coordinate of another.
// Two Links with identical directory sizes are still two Links.
func TestCellsAreFencedToTheSpaceThatIssuedThem(t *testing.T) {
	left, leftOK := Seal(testLinkID(t, "left"), 2, 2)
	right, rightOK := Seal(testLinkID(t, "right"), 2, 2)
	if !leftOK || !rightOK {
		t.Fatal("space rejected")
	}
	leftID, leftIDOK := left.ContentID()
	rightID, rightIDOK := right.ContentID()
	if !leftIDOK || !rightIDOK || leftID == rightID {
		t.Fatalf("two Links sealed one identity: %v/%v", leftIDOK, rightIDOK)
	}
	cell, cellOK := left.Cell(1, 2)
	if !cellOK {
		t.Fatal("left space issued no cell")
	}
	if !left.Owns(cell) {
		t.Fatal("the issuing space disowned its own cell")
	}
	if right.Owns(cell) {
		t.Fatal("a foreign space adopted a cell it did not issue")
	}
	if _, ok := right.Allocation(cell); ok {
		t.Fatal("a foreign space resolved a cell it did not issue")
	}
	if _, ok := right.Protocol(cell); ok {
		t.Fatal("a foreign space resolved a cell it did not issue")
	}
}

// The identity is a function of the Link and the two directory sizes alone, so
// two seals over one program agree, and a program whose resource or protocol
// inventory changed is a different space.
func TestSpaceIdentityIsAFunctionOfTheSealedDirectories(t *testing.T) {
	link := testLinkID(t, "identity")
	first, firstOK := Seal(link, 5, 2)
	second, secondOK := Seal(link, 5, 2)
	if !firstOK || !secondOK {
		t.Fatal("space rejected")
	}
	firstID, _ := first.ContentID()
	secondID, _ := second.ContentID()
	if firstID != secondID {
		t.Fatal("two seals over one program disagreed on the space identity")
	}
	for _, other := range []struct{ allocations, protocols int }{{4, 2}, {5, 3}, {2, 5}} {
		changed, changedOK := Seal(link, other.allocations, other.protocols)
		if !changedOK {
			t.Fatalf("space (%d, %d) rejected", other.allocations, other.protocols)
		}
		changedID, _ := changed.ContentID()
		if changedID == firstID {
			t.Fatalf("directories (%d, %d) sealed the identity of (5, 2)", other.allocations, other.protocols)
		}
	}
}

// A program with no resource or no declared protocol has an empty space, and
// an empty space is sealed. The absence of resources is a fact about the
// program; an unsealed space is the absence of an answer.
func TestEmptyDirectoriesSealAnEmptySpace(t *testing.T) {
	for _, empty := range []struct{ allocations, protocols int }{{0, 2}, {3, 0}, {0, 0}} {
		space, ok := Seal(testLinkID(t, "empty"), empty.allocations, empty.protocols)
		if !ok {
			t.Fatalf("space (%d, %d) was refused rather than sealed empty", empty.allocations, empty.protocols)
		}
		if !space.Available() || space.CellCount() != 0 {
			t.Fatalf("space (%d, %d) has %d cells", empty.allocations, empty.protocols, space.CellCount())
		}
		if _, cellOK := space.CellAt(0); cellOK {
			t.Fatalf("empty space (%d, %d) issued a coordinate", empty.allocations, empty.protocols)
		}
	}
}

// The zero space is not a space. It answers no count and issues no coordinate,
// so a consumer cannot mistake an unsealed owner for a program with no
// resources.
func TestUnsealedSpaceAnswersNothing(t *testing.T) {
	var space Space
	if space.Available() {
		t.Fatal("the zero space reports itself sealed")
	}
	if _, ok := space.ContentID(); ok {
		t.Fatal("the zero space published an identity")
	}
	if space.CellCount() != 0 || space.AllocationCount() != 0 || space.ProtocolCount() != 0 {
		t.Fatal("the zero space published a directory size")
	}
	if _, ok := space.Cell(0, 1); ok {
		t.Fatal("the zero space issued a coordinate")
	}
	if _, ok := Seal(identity.ContentID{}, 1, 1); ok {
		t.Fatal("a space sealed without a Link identity")
	}
}

// Reading a coordinate is a hot-path operation of the rule that writes the
// axis, so projecting a resource and protocol onto a cell allocates nothing.
func TestCellProjectionDoesNotAllocate(t *testing.T) {
	space, ok := Seal(testLinkID(t, "allocs"), 16, 4)
	if !ok {
		t.Fatal("space rejected")
	}
	allocations := testing.AllocsPerRun(64, func() {
		for allocation := 0; allocation < space.AllocationCount(); allocation++ {
			cell, cellOK := space.Cell(allocation, 2)
			if !cellOK {
				panic("cell unavailable")
			}
			if _, backOK := space.Allocation(cell); !backOK {
				panic("cell did not resolve")
			}
		}
	})
	if allocations != 0 {
		t.Fatalf("cell projection allocated %.0f times", allocations)
	}
}
