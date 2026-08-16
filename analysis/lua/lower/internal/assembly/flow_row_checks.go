package assembly

import (
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// localCellAdmission is the Collector-side row proof for a lexical Cell.
// Role predicates deliberately stop at FamilyCell; storage ownership is a
// relation between the Cell row and its Body and therefore stays here.
func localCellAdmission(c *Collector, cell keyspace.Term) bool {
	if c == nil || !validFamilyTerm(c, cell, keyspace.FamilyCell) {
		return false
	}
	ordinal := keyspace.TermOrdinal(cell)
	row, ok := c.flow.CellAt(int(ordinal - 1))
	return ordinal != 0 && ok && row.Kind == flow.CellLocal
}

func localCellInBodyAdmission(c *Collector, cell, body keyspace.Term) bool {
	if !localCellAdmission(c, cell) {
		return false
	}
	row, ok := c.flow.CellAt(int(keyspace.TermOrdinal(cell) - 1))
	return ok && row.Body == body
}
