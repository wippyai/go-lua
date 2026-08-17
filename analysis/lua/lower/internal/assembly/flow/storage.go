package flow

import (
	"github.com/wippyai/go-lua/analysis/lua/bind"
	programflow "github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

type storageRows struct {
	cells        []programflow.Cell
	globalCensus bind.GlobalCensus
	reads        []programflow.Read
	varargs      []programflow.Vararg
	binds        []programflow.Bind
	assigns      []programflow.Assign
	writes       []programflow.Write
}

func (r *Rows) SetGlobalCensus(census bind.GlobalCensus) {
	if r != nil {
		r.storage.globalCensus = census
	}
}

func (r *Rows) GlobalCensus() bind.GlobalCensus {
	if r == nil {
		return bind.GlobalCensus{}
	}
	return r.storage.globalCensus
}

func (r *Rows) InitGlobalCells(count int) bool {
	if r == nil || count < 0 || uint64(count) > uint64(keyspace.MaxTermOrdinal) {
		return false
	}
	r.storage.cells = make([]programflow.Cell, count)
	return true
}

func (r *Rows) SetCell(index int, row programflow.Cell) bool {
	if r == nil || index < 0 || index >= len(r.storage.cells) {
		return false
	}
	r.storage.cells[index] = row
	return true
}

func (r *Rows) AppendCell(row programflow.Cell) {
	if r != nil {
		r.storage.cells = append(r.storage.cells, row)
	}
}

func (r *Rows) CellAt(index int) (programflow.Cell, bool) {
	if r == nil || index < 0 || index >= len(r.storage.cells) {
		return programflow.Cell{}, false
	}
	return r.storage.cells[index], true
}

func (r *Rows) AppendRead(row programflow.Read) {
	if r != nil {
		r.storage.reads = append(r.storage.reads, row)
	}
}

func (r *Rows) AppendVararg(row programflow.Vararg) {
	if r != nil {
		r.storage.varargs = append(r.storage.varargs, row)
	}
}

func (r *Rows) AppendBind(row programflow.Bind) {
	if r != nil {
		r.storage.binds = append(r.storage.binds, row)
	}
}

func (r *Rows) BindAt(index int) (programflow.Bind, bool) {
	if r == nil || index < 0 || index >= len(r.storage.binds) {
		return programflow.Bind{}, false
	}
	return r.storage.binds[index], true
}

func (r *Rows) AppendAssign(row programflow.Assign) {
	if r != nil {
		r.storage.assigns = append(r.storage.assigns, row)
	}
}

func (r *Rows) AppendWrite(row programflow.Write) {
	if r != nil {
		r.storage.writes = append(r.storage.writes, row)
	}
}
