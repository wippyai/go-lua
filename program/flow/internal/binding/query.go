package binding

import (
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

// CellCount reports the exact sealed Cell cardinality.
func (r Result) CellCount() int {
	if !r.available() || len(r.roles) < 2 || len(r.hosts) != len(r.roles) {
		return 0
	}
	return len(r.roles) - 1
}

// Role returns the unique definition role of one sealed Cell.
func (r Result) Role(cell keyspace.Term) (kind.CellRole, bool) {
	if !r.available() {
		return 0, false
	}
	ordinal, ok := r.cellOrdinal(cell)
	if !ok {
		return 0, false
	}
	role := r.roles[ordinal]
	if !validRole(role) {
		return 0, false
	}
	return role, true
}

// Host returns the authored definition host of one sealed Cell. Global Cells
// are Program-scoped and therefore return host zero with ok=true.
func (r Result) Host(cell keyspace.Term) (keyspace.Term, bool) {
	if !r.available() {
		return 0, false
	}
	ordinal, ok := r.cellOrdinal(cell)
	if !ok {
		return 0, false
	}
	if !validRole(r.roles[ordinal]) {
		return 0, false
	}
	return r.hosts[ordinal], true
}

// ChunkVararg returns the optional Cell derived from chunk-scope Vararg rows.
func (r Result) ChunkVararg() (keyspace.Term, bool) {
	if !r.available() || r.chunk == 0 {
		return 0, false
	}
	ordinal, ok := r.cellOrdinal(r.chunk)
	if !ok || r.roles[ordinal] != kind.CellChunkVararg || r.hosts[ordinal] == 0 {
		return 0, false
	}
	return r.chunk, true
}

// FunctionCell returns the unique local Cell whose Source-ordered Bind
// position establishes one Function's lexical identity. It is intentionally a
// one-way, exact projection: Functions without a unique self binding and all
// malformed, nonlocal, or global claims fail closed.
func (r Result) FunctionCell(function keyspace.Term) (keyspace.Term, bool) {
	if !r.available() || keyspace.TermFamily(function) != keyspace.FamilyFunction {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(function)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(r.functionCells)) ||
		len(r.roles) < 2 || len(r.hosts) != len(r.roles) {
		return 0, false
	}
	cell := r.functionCells[ordinal]
	cellOrdinal, ok := r.cellOrdinal(cell)
	if !ok || r.roles[cellOrdinal] != kind.CellLocal ||
		keyspace.TermFamily(r.hosts[cellOrdinal]) != keyspace.FamilyBind ||
		keyspace.TermOrdinal(r.hosts[cellOrdinal]) == 0 {
		return 0, false
	}
	return cell, true
}

func (r Result) cellOrdinal(cell keyspace.Term) (uint32, bool) {
	if keyspace.TermFamily(cell) != keyspace.FamilyCell {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(cell)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(r.roles)) || len(r.hosts) != len(r.roles) {
		return 0, false
	}
	return ordinal, true
}

func validRole(role kind.CellRole) bool {
	return role >= kind.CellGlobal && role <= kind.CellChunkVararg
}
