package flow

import (
	programflow "github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

type tableRows struct {
	rows   []programflow.Table
	fields []programflow.Field
	order  []keyspace.Term
	filled []bool
}

func (r *Rows) AppendTable(row programflow.Table) {
	if r != nil {
		r.tables.rows = append(r.tables.rows, row)
		r.tables.filled = append(r.tables.filled, false)
	}
}

func (r *Rows) SetTableFields(index int, fields programflow.Range) bool {
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

func (r *Rows) AppendTableField(row programflow.Field) {
	if r != nil {
		r.tables.fields = append(r.tables.fields, row)
	}
}

func (r *Rows) TableFieldAt(index int) (programflow.Field, bool) {
	if r == nil || index < 0 || index >= len(r.tables.fields) {
		return programflow.Field{}, false
	}
	return r.tables.fields[index], true
}

func (r *Rows) AppendTableOrder(terms []keyspace.Term) (programflow.Range, bool) {
	if r == nil {
		return programflow.Range{}, false
	}
	rangeValue, ok := rangeFor(len(r.tables.order), len(terms))
	if !ok {
		return programflow.Range{}, false
	}
	r.tables.order = append(r.tables.order, terms...)
	return rangeValue, true
}
