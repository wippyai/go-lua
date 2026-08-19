// Package flow owns the mutable Flow construction rows for the Lua lowerer.
// It is a row owner, not a Collector facade: callers supply already-admitted
// typed rows and this package stores, copies, and freezes them. Cross-owner
// checks stay at the assembly orchestration boundary.
package flow

import (
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
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

func rangeFor(poolLen, add int) (authored.Range, bool) {
	if poolLen < 0 || add < 0 {
		return authored.Range{}, false
	}
	end := uint64(poolLen) + uint64(add)
	if end > uint64(keyspace.MaxTermOrdinal) {
		return authored.Range{}, false
	}
	return authored.Range{Start: uint32(poolLen), End: uint32(end)}, true
}

// The row groups and their append/query operations live with Rows.  They are
// one mutable construction owner; splitting each append-only helper into its
// own file obscured that ownership without creating a separately testable
// behavior surface.
type accessRows struct {
	exact   []authored.ExactLens
	dynamic []authored.DynamicLens
}

func (r *Rows) AppendExactLens(row authored.ExactLens) {
	if r != nil {
		r.access.exact = append(r.access.exact, row)
	}
}

func (r *Rows) AppendDynamicLens(row authored.DynamicLens) {
	if r != nil {
		r.access.dynamic = append(r.access.dynamic, row)
	}
}

type callRows struct{ rows []authored.Call }

func (r *Rows) AppendCall(row authored.Call) {
	if r != nil {
		r.calls.rows = append(r.calls.rows, row)
	}
}

type controlRows struct {
	returns   []authored.Return
	breaks    []authored.Break
	labels    []authored.Label
	gotos     []authored.Goto
	branches  []authored.Branch
	loops     []authored.Loop
	loopCells []keyspace.Term
}

func (r *Rows) AppendReturn(row authored.Return) {
	if r != nil {
		r.control.returns = append(r.control.returns, row)
	}
}

func (r *Rows) AppendBreak(row authored.Break) {
	if r != nil {
		r.control.breaks = append(r.control.breaks, row)
	}
}

func (r *Rows) AppendLabel(row authored.Label) {
	if r != nil {
		r.control.labels = append(r.control.labels, row)
	}
}

func (r *Rows) AppendGoto(row authored.Goto) {
	if r != nil {
		r.control.gotos = append(r.control.gotos, row)
	}
}

func (r *Rows) AppendBranch(row authored.Branch) {
	if r != nil {
		r.control.branches = append(r.control.branches, row)
	}
}

func (r *Rows) AppendLoop(row authored.Loop, cells []keyspace.Term) (authored.Range, bool) {
	if r == nil {
		return authored.Range{}, false
	}
	result, ok := rangeFor(len(r.control.loopCells), len(cells))
	if !ok {
		return authored.Range{}, false
	}
	r.control.loopCells = append(r.control.loopCells, cells...)
	row.Cells = result
	r.control.loops = append(r.control.loops, row)
	return result, true
}

type functionRows struct {
	rows     []authored.Function
	captures []authored.Capture
}

func (r *Rows) AppendFunction(row authored.Function) {
	if r != nil {
		r.functions.rows = append(r.functions.rows, row)
	}
}

func (r *Rows) FunctionAt(index int) (authored.Function, bool) {
	if r == nil || index < 0 || index >= len(r.functions.rows) {
		return authored.Function{}, false
	}
	return r.functions.rows[index], true
}

func (r *Rows) SetFunction(index int, row authored.Function) bool {
	if r == nil || index < 0 || index >= len(r.functions.rows) {
		return false
	}
	r.functions.rows[index] = row
	return true
}

func (r *Rows) AppendCaptures(captures []authored.Capture) (authored.Range, bool) {
	if r == nil {
		return authored.Range{}, false
	}
	result, ok := rangeFor(len(r.functions.captures), len(captures))
	if !ok {
		return authored.Range{}, false
	}
	r.functions.captures = append(r.functions.captures, captures...)
	return result, true
}

type operandRows struct {
	claims     []authored.ValueClaim
	typeValues []authored.TypeValue
}

func (r *Rows) AppendClaim(row authored.ValueClaim) {
	if r != nil {
		r.operands.claims = append(r.operands.claims, row)
	}
}

func (r *Rows) ClaimAt(index int) (authored.ValueClaim, bool) {
	if r == nil || index < 0 || index >= len(r.operands.claims) {
		return authored.ValueClaim{}, false
	}
	return r.operands.claims[index], true
}

func (r *Rows) AppendTypeValue(row authored.TypeValue) {
	if r != nil {
		r.operands.typeValues = append(r.operands.typeValues, row)
	}
}

type operatorRows struct {
	unaries  []authored.Unary
	binaries []authored.Binary
	selects  []authored.Select
}

func (r *Rows) AppendUnary(row authored.Unary) {
	if r != nil {
		r.operators.unaries = append(r.operators.unaries, row)
	}
}

func (r *Rows) UnaryAt(index int) (authored.Unary, bool) {
	if r == nil || index < 0 || index >= len(r.operators.unaries) {
		return authored.Unary{}, false
	}
	return r.operators.unaries[index], true
}

func (r *Rows) AppendBinary(row authored.Binary) {
	if r != nil {
		r.operators.binaries = append(r.operators.binaries, row)
	}
}

func (r *Rows) AppendSelect(row authored.Select) {
	if r != nil {
		r.operators.selects = append(r.operators.selects, row)
	}
}

type storageRows struct {
	cells        []authored.Cell
	globalCensus bind.GlobalCensus
	reads        []authored.Read
	varargs      []authored.Vararg
	binds        []authored.Bind
	assigns      []authored.Assign
	writes       []authored.Write
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
	r.storage.cells = make([]authored.Cell, count)
	return true
}

func (r *Rows) SetCell(index int, row authored.Cell) bool {
	if r == nil || index < 0 || index >= len(r.storage.cells) {
		return false
	}
	r.storage.cells[index] = row
	return true
}

func (r *Rows) AppendCell(row authored.Cell) {
	if r != nil {
		r.storage.cells = append(r.storage.cells, row)
	}
}

func (r *Rows) CellAt(index int) (authored.Cell, bool) {
	if r == nil || index < 0 || index >= len(r.storage.cells) {
		return authored.Cell{}, false
	}
	return r.storage.cells[index], true
}

func (r *Rows) AppendRead(row authored.Read) {
	if r != nil {
		r.storage.reads = append(r.storage.reads, row)
	}
}

func (r *Rows) AppendVararg(row authored.Vararg) {
	if r != nil {
		r.storage.varargs = append(r.storage.varargs, row)
	}
}

func (r *Rows) AppendBind(row authored.Bind) {
	if r != nil {
		r.storage.binds = append(r.storage.binds, row)
	}
}

func (r *Rows) BindAt(index int) (authored.Bind, bool) {
	if r == nil || index < 0 || index >= len(r.storage.binds) {
		return authored.Bind{}, false
	}
	return r.storage.binds[index], true
}

func (r *Rows) AppendAssign(row authored.Assign) {
	if r != nil {
		r.storage.assigns = append(r.storage.assigns, row)
	}
}

func (r *Rows) AppendWrite(row authored.Write) {
	if r != nil {
		r.storage.writes = append(r.storage.writes, row)
	}
}

type tableRows struct {
	rows   []authored.Table
	fields []authored.Field
	order  []keyspace.Term
	filled []bool
}

func (r *Rows) AppendTable(row authored.Table) {
	if r != nil {
		r.tables.rows = append(r.tables.rows, row)
		r.tables.filled = append(r.tables.filled, false)
	}
}

func (r *Rows) SetTableFields(index int, fields authored.Range) bool {
	if r == nil || index < 0 || index >= len(r.tables.rows) {
		return false
	}
	r.tables.rows[index].Fields = fields
	return true
}

func (r *Rows) SetTableFilled(index int, value bool) bool {
	if r == nil || index < 0 || index >= len(r.tables.filled) {
		return false
	}
	r.tables.filled[index] = value
	return true
}

func (r *Rows) AppendTableField(row authored.Field) {
	if r != nil {
		r.tables.fields = append(r.tables.fields, row)
	}
}

func (r *Rows) TableFieldAt(index int) (authored.Field, bool) {
	if r == nil || index < 0 || index >= len(r.tables.fields) {
		return authored.Field{}, false
	}
	return r.tables.fields[index], true
}

func (r *Rows) AppendTableOrder(terms []keyspace.Term) (authored.Range, bool) {
	if r == nil {
		return authored.Range{}, false
	}
	rangeValue, ok := rangeFor(len(r.tables.order), len(terms))
	if !ok {
		return authored.Range{}, false
	}
	r.tables.order = append(r.tables.order, terms...)
	return rangeValue, true
}

type valuesRows struct {
	rows  []authored.Value
	terms []keyspace.Term
}

func (r *Rows) AppendValue(row authored.Value, terms []keyspace.Term) (authored.Range, bool) {
	if r == nil {
		return authored.Range{}, false
	}
	span, ok := rangeFor(len(r.values.terms), len(terms))
	if !ok {
		return authored.Range{}, false
	}
	r.values.terms = append(r.values.terms, terms...)
	r.values.rows = append(r.values.rows, row)
	return span, true
}

func (r *Rows) ValueAt(index int) (authored.Value, bool) {
	if r == nil || index < 0 || index >= len(r.values.rows) {
		return authored.Value{}, false
	}
	return r.values.rows[index], true
}

func (r *Rows) ValueTermAt(index int) (keyspace.Term, bool) {
	if r == nil || index < 0 || index >= len(r.values.terms) {
		return 0, false
	}
	return r.values.terms[index], true
}

// OwnerAt returns the Body owner carried by a direct Flow row. It is a
// read-only cross-owner witness used by Source orchestration; it never hands
// out a mutable row or a sibling store.
func (r *Rows) OwnerAt(family keyspace.Family, index int) (keyspace.Term, bool) {
	if r == nil || index < 0 {
		return 0, false
	}
	switch family {
	case keyspace.FamilyBind:
		if index < len(r.storage.binds) {
			return r.storage.binds[index].Owner, true
		}
	case keyspace.FamilyAssign:
		if index < len(r.storage.assigns) {
			return r.storage.assigns[index].Owner, true
		}
	case keyspace.FamilyCall:
		if index < len(r.calls.rows) {
			return r.calls.rows[index].Owner, true
		}
	case keyspace.FamilyBranch:
		if index < len(r.control.branches) {
			return r.control.branches[index].Owner, true
		}
	case keyspace.FamilyLoop:
		if index < len(r.control.loops) {
			return r.control.loops[index].Owner, true
		}
	case keyspace.FamilyReturn:
		if index < len(r.control.returns) {
			return r.control.returns[index].Owner, true
		}
	case keyspace.FamilyBreak:
		if index < len(r.control.breaks) {
			return r.control.breaks[index].Owner, true
		}
	case keyspace.FamilyGoto:
		if index < len(r.control.gotos) {
			return r.control.gotos[index].Owner, true
		}
	case keyspace.FamilyLabel:
		if index < len(r.control.labels) {
			return r.control.labels[index].Owner, true
		}
	}
	return 0, false
}
