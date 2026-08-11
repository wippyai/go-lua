package collector

import (
	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/keyspace"
)

// localCellAdmission is the Collector-side row proof for a lexical Cell.
// Role predicates deliberately stop at FamilyCell; storage ownership is a
// relation between the Cell row and its Body and therefore stays here.
func localCellAdmission(c *Collector, cell keyspace.Term) bool {
	if c == nil || !validFamilyTerm(c, cell, keyspace.FamilyCell) {
		return false
	}
	ordinal := keyspace.TermOrdinal(cell)
	return ordinal != 0 && uint64(ordinal) <= uint64(len(c.flow.storage.cells)) &&
		c.flow.storage.cells[ordinal-1].Kind == flow.CellLocal
}

func localCellInBodyAdmission(c *Collector, cell, body keyspace.Term) bool {
	if !localCellAdmission(c, cell) {
		return false
	}
	return c.flow.storage.cells[keyspace.TermOrdinal(cell)-1].Body == body
}

func globalCellAdmission(c *Collector, cell keyspace.Term) bool {
	if c == nil || !validFamilyTerm(c, cell, keyspace.FamilyCell) {
		return false
	}
	ordinal := keyspace.TermOrdinal(cell)
	return ordinal != 0 && uint64(ordinal) <= uint64(len(c.flow.storage.cells)) &&
		c.flow.storage.cells[ordinal-1].Kind == flow.CellGlobal
}

func valuesRowAdmission(c *Collector, values keyspace.Term) (flow.Value, bool) {
	if c == nil || !validFamilyTerm(c, values, keyspace.FamilyValues) {
		return flow.Value{}, false
	}
	ordinal := keyspace.TermOrdinal(values)
	if ordinal == 0 || uint64(ordinal) > uint64(len(c.flow.values.values)) {
		return flow.Value{}, false
	}
	return c.flow.values.values[ordinal-1], true
}
