package static

import "github.com/wippyai/go-lua/program/keyspace"

// EffectRows exposes Static's sparse authored rows without returning mutable
// pooled slices. Every query is a direct dense Function-ordinal lookup or a
// scalar pool-range read and allocates nothing.
func (view View) EffectRows() EffectRows {
	return EffectRows{component: view.component, state: view.state}
}

func (view EffectRows) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.effectRows.rows)
}

func (view EffectRows) At(index int) (keyspace.Term, bool) {
	component := view.componentOf()
	if component == nil || index < 0 || index >= len(component.effectRows.rows) {
		return 0, false
	}
	return component.effectRows.rows[index].function, true
}

// Get returns one immutable row header. A false result distinguishes an
// omitted Function row from a present row whose RowSpec is explicitly closed.
// The returned Occurrences slice is always empty under the current admitted
// source vocabulary, so no mutable pooled storage escapes this boundary.
func (view EffectRows) Get(function keyspace.Term) (RowSpec, bool) {
	component := view.componentOf()
	index := effectRowIndex(component, function)
	if index < 0 {
		return RowSpec{}, false
	}
	row := component.effectRows.rows[index]
	return RowSpec{
		RowFormals: row.rowFormals,
		Tail:       row.tail,
		Var:        row.variable,
	}, true
}

func (view EffectRows) Tail(function keyspace.Term) (RowTail, bool) {
	row, ok := view.Get(function)
	return row.Tail, ok
}

func (view EffectRows) Variable(function keyspace.Term) (RowVar, bool) {
	row, ok := view.Get(function)
	return row.Var, ok
}

func (view EffectRows) OccurrenceCount(function keyspace.Term) (int, bool) {
	component := view.componentOf()
	index := effectRowIndex(component, function)
	if index < 0 {
		return 0, false
	}
	range_ := component.effectRows.rows[index].occurrences
	return int(range_.End - range_.Start), true
}

func (view EffectRows) OccurrenceAt(function keyspace.Term, index int) (EffectOccurrence, bool) {
	component := view.componentOf()
	rowIndex := effectRowIndex(component, function)
	if rowIndex < 0 || index < 0 {
		return EffectOccurrence{}, false
	}
	range_ := component.effectRows.rows[rowIndex].occurrences
	if uint64(index) >= uint64(range_.End-range_.Start) {
		return EffectOccurrence{}, false
	}
	return component.effectRows.occurrences[range_.Start+uint32(index)], true
}

func effectRowIndex(component *Component, function keyspace.Term) int {
	if component == nil || keyspace.TermFamily(function) != keyspace.FamilyFunction {
		return -1
	}
	ordinal := keyspace.TermOrdinal(function)
	lookup := component.effectRows.byFunction
	if ordinal == 0 || uint64(ordinal) >= uint64(len(lookup)) {
		return -1
	}
	index := lookup[ordinal]
	if index == 0 {
		return -1
	}
	return int(index - 1)
}
