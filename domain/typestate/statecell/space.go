// Package statecell owns the coordinate space typestate facts are solved
// over: one cell per resource per protocol.
//
// A typestate fact is about a resource, and a resource is a heap allocation
// root - the one identity the analyzer already carries for "this object,
// wherever it is referenced from". A protocol is a separate state machine over
// that same resource, so a resource governed by two protocols has two
// independent states and needs two cells. The space is therefore the product
// of Heap's dense allocation-root directory with the sealed protocol
// directory, and it borrows both rather than minting a third identity for a
// resource or a fourth for a protocol.
//
// The space carries no program point, and that is deliberate: no axis in the
// analyzer declares one. Value is keyed per canonical value, Heap, Placement
// and Heap-context per allocation root, Effect per body; flow sensitivity is
// the engine's own dimension, which carries one point state per execution
// context and point over whatever key space an axis declares. A cell is
// therefore "the state of this resource under this protocol", and the engine
// answers it per point, exactly as it answers Value's narrowed type per point
// over a key space that has no point in it either.
package statecell

import (
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// spaceDomain separates this space's preimage from every other identity
// derived under the same Link.
const spaceDomain = "wippy.analysis/domain/typestate/statecell/v1"

// Space is the sealed typestate coordinate space of one Link: the dense
// product of that Link's allocation roots with its declared protocols.
//
// The zero value is unsealed and answers nothing. A sealed space is immutable
// and is a function of the two counts and the Link identity alone, so two
// seals over one program produce one identical space and one identical
// identity.
type Space struct {
	allocations uint32
	protocols   uint32
	id          identity.ContentID
}

// Cell is one owner-fenced coordinate of a sealed space. The dense index is
// meaningful only to the space that issued it, so a cell from a separately
// sealed Link cannot be rebound here.
type Cell struct {
	owner identity.ContentID
	index uint32
}

// Seal issues the space for one Link. A program with no allocation root or no
// declared protocol seals an empty space: the absence of resources is a sealed
// fact about the program, not an unsealed table.
//
// The product is checked before publication rather than after: a space whose
// cell count does not fit its own dense coordinate is refused, so no consumer
// can be handed a coordinate the carrier cannot address.
func Seal(linkID identity.ContentID, allocations, protocols int) (Space, bool) {
	if !linkID.Available() || allocations < 0 || protocols < 0 {
		return Space{}, false
	}
	if uint64(allocations) > uint64(^uint32(0)) || uint64(protocols) > uint64(^uint32(0)) {
		return Space{}, false
	}
	cells := uint64(allocations) * uint64(protocols)
	if cells > uint64(^uint32(0)) {
		return Space{}, false
	}
	var image [12]byte
	binary.BigEndian.PutUint32(image[0:4], uint32(allocations))
	binary.BigEndian.PutUint32(image[4:8], uint32(protocols))
	binary.BigEndian.PutUint32(image[8:12], uint32(cells))
	id, ok := identity.DeriveContentID(spaceDomain, linkID[:], image[:])
	if !ok {
		return Space{}, false
	}
	return Space{allocations: uint32(allocations), protocols: uint32(protocols), id: id}, true
}

// Available reports whether the space was sealed.
func (space Space) Available() bool { return space.id.Available() }

// ContentID is the space's identity: the digest of the Link and the two
// directory sizes it is the product of.
func (space Space) ContentID() (identity.ContentID, bool) {
	if !space.Available() {
		return identity.ContentID{}, false
	}
	return space.id, true
}

// AllocationCount is the size of the borrowed Heap allocation-root directory.
func (space Space) AllocationCount() int {
	if !space.Available() {
		return 0
	}
	return int(space.allocations)
}

// ProtocolCount is the size of the borrowed protocol directory.
func (space Space) ProtocolCount() int {
	if !space.Available() {
		return 0
	}
	return int(space.protocols)
}

// CellCount is the dense coordinate count the carrier is sized by.
func (space Space) CellCount() int {
	if !space.Available() {
		return 0
	}
	return int(space.allocations) * int(space.protocols)
}

// Cell projects one allocation root and one protocol onto their coordinate.
// The allocation is Heap's own dense ordinal and the protocol is the sealed
// one-based protocol handle; neither is renumbered here.
//
// The layout is allocation-major: the cells of one resource are contiguous, so
// a consumer that judges one resource against every protocol it participates
// in walks one run.
func (space Space) Cell(allocation int, protocol vocabulary.Protocol) (Cell, bool) {
	if !space.Available() || allocation < 0 || allocation >= int(space.allocations) {
		return Cell{}, false
	}
	if protocol == 0 || uint64(protocol) > uint64(space.protocols) {
		return Cell{}, false
	}
	index := uint32(allocation)*space.protocols + uint32(protocol) - 1
	return Cell{owner: space.id, index: index}, true
}

// CellAt issues the cell at one dense coordinate.
func (space Space) CellAt(index int) (Cell, bool) {
	if !space.Available() || index < 0 || index >= space.CellCount() {
		return Cell{}, false
	}
	return Cell{owner: space.id, index: uint32(index)}, true
}

// Owns reports whether this space issued the cell.
func (space Space) Owns(cell Cell) bool {
	return space.Available() && cell.owner == space.id && int(cell.index) < space.CellCount()
}

// Available reports whether the cell was issued by a sealed space.
func (cell Cell) Available() bool { return cell.owner.Available() }

// Index is the cell's dense coordinate.
func (cell Cell) Index() uint32 { return cell.index }

// Allocation is the Heap allocation-root ordinal this cell holds the state of.
func (space Space) Allocation(cell Cell) (int, bool) {
	if !space.Owns(cell) || space.protocols == 0 {
		return 0, false
	}
	return int(cell.index / space.protocols), true
}

// Protocol is the protocol handle this cell holds the state under.
func (space Space) Protocol(cell Cell) (vocabulary.Protocol, bool) {
	if !space.Owns(cell) || space.protocols == 0 {
		return 0, false
	}
	return vocabulary.Protocol(cell.index%space.protocols) + 1, true
}

// DenseIndex normalizes one cell into the dense coordinate the engine
// addresses this axis's Factor by.
//
// It is the axis's one key normalization, and it is owner-fenced: a cell a
// separately sealed space issued has no coordinate here, so a coordinate the
// engine is handed is one this space minted.
func (space Space) DenseIndex(cell Cell) (uint32, bool) {
	if !space.Owns(cell) {
		return 0, false
	}
	return cell.index, true
}
