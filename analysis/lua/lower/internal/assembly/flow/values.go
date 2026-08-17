package flow

import (
	programflow "github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

type valuesRows struct {
	rows  []programflow.Value
	terms []keyspace.Term
}

func (r *Rows) AppendValue(row programflow.Value, terms []keyspace.Term) (programflow.Range, bool) {
	if r == nil {
		return programflow.Range{}, false
	}
	span, ok := rangeFor(len(r.values.terms), len(terms))
	if !ok {
		return programflow.Range{}, false
	}
	r.values.terms = append(r.values.terms, terms...)
	r.values.rows = append(r.values.rows, row)
	return span, true
}

func (r *Rows) ValueAt(index int) (programflow.Value, bool) {
	if r == nil || index < 0 || index >= len(r.values.rows) {
		return programflow.Value{}, false
	}
	return r.values.rows[index], true
}

func (r *Rows) ValueTermAt(index int) (keyspace.Term, bool) {
	if r == nil || index < 0 || index >= len(r.values.terms) {
		return 0, false
	}
	return r.values.terms[index], true
}
