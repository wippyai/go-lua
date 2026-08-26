package relation

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/typestate/statecell"
)

// SealStateCellSpace seals the state-cell space one link's typestate columns
// are addressed over.
//
// The space is the axis's own denominator: it is issued against the link, it
// owns every cell it hands out, and it normalizes a cell to the dense position
// its own seal assigned. The mount reaches it here so the columns of one link
// are addressed over exactly one space and never over a second construction of
// the same one.
func SealStateCellSpace(link identity.ContentID, allocations, protocols int) (statecell.Space, bool) {
	return statecell.Seal(link, allocations, protocols)
}
