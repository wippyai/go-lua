package evaluation

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// assignTarget validates one ordered assignment address. Cells have no
// expression evaluation; Lens targets evaluate their address in the same
// left-to-right order in which Assigns.WriteAt exposes them.
func (walker *Session) assignTarget(target, owner keyspace.Term) (bool, error) {
	if !walker.validTerm(target) {
		return false, errors.New("program/flow/evaluation: invalid Assign target")
	}
	switch keyspace.TermFamily(target) {
	case keyspace.FamilyCell:
		cellKind, _, _, ok := walker.view.Storage().Cells().Get(target)
		if !ok || !cellKindValid(cellKind) {
			return false, errors.New("program/flow/evaluation: Assign Cell target is unavailable")
		}
		// Local Cells may be captured by nested lexical Bodies.  Canonical
		// containment validates that ancestry before this traversal starts;
		// this Session has no independent Body graph and must not reimpose
		// same-Body equality here.
		return false, nil
	case keyspace.FamilyLensExact, keyspace.FamilyLensKey:
		targetOwner, hasOwner, err := walker.owner(target)
		if err != nil || !hasOwner || targetOwner != owner {
			return false, errors.New("program/flow/evaluation: Assign Lens crosses Body owner")
		}
		return true, nil
	default:
		return false, errors.New("program/flow/evaluation: Assign target is not writable")
	}
}

func cellKindValid(value authored.CellKind) bool {
	return value == authored.CellLocal || value == authored.CellGlobal
}
