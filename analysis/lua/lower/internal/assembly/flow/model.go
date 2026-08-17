// Package flow owns the mutable Flow construction rows for the Lua lowerer.
// It is a row owner, not a Collector facade: callers supply already-admitted
// typed rows and this package stores, copies, and freezes them. Cross-owner
// checks stay at the assembly orchestration boundary.
package flow

import (
	programflow "github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Rows is the complete Flow construction plane. Its row groups are private;
// a sibling owner can receive only an explicit copied row or an accessor.
type Rows struct {
	values    valuesRows
	access    accessRows
	storage   storageRows
	tables    tableRows
	functions functionRows
	calls     callRows
	control   controlRows
	operators operatorRows
	operands  operandRows
}

func (r *Rows) Reset() {
	if r != nil {
		*r = Rows{}
	}
}

func rangeFor(poolLen, add int) (programflow.Range, bool) {
	if poolLen < 0 || add < 0 {
		return programflow.Range{}, false
	}
	end := uint64(poolLen) + uint64(add)
	if end > uint64(keyspace.MaxTermOrdinal) {
		return programflow.Range{}, false
	}
	return programflow.Range{Start: uint32(poolLen), End: uint32(end)}, true
}
