package signatures

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// View is the immutable query surface over a sealed signature table. It holds
// the sealed table by value: the enclosing owner checks its publication fence
// once when it mints the view. A zero View is permanently unavailable.
type View struct {
	table     Table
	available bool
}

// View returns a query surface over this sealed table.
func (table Table) View() View { return View{table: table, available: true} }

// Available reports whether this view resolves to a sealed signature set.
func (view View) Available() bool { return view.available }

type TypeFunctions struct{ view View }
type Assertions struct{ view View }

func (view View) TypeFunctions() TypeFunctions { return TypeFunctions{view: view} }
func (view View) Assertions() Assertions       { return Assertions{view: view} }

func (view TypeFunctions) Count() int { return view.view.count(keyspace.FamilyTypeFunction) }
func (view Assertions) Count() int    { return view.view.count(keyspace.FamilyTypeAsserts) }

func (view View) count(family keyspace.Family) int {
	if !view.available {
		return 0
	}
	return view.table.Count(family)
}

func (view TypeFunctions) At(index int) (keyspace.Term, bool) {
	return view.view.term(keyspace.FamilyTypeFunction, index)
}
func (view Assertions) At(index int) (keyspace.Term, bool) {
	return view.view.term(keyspace.FamilyTypeAsserts, index)
}

func (view View) term(family keyspace.Family, index int) (keyspace.Term, bool) {
	if !view.available || index < 0 || index >= view.table.Count(family) {
		return 0, false
	}
	return keyspace.MakeTerm(family, uint32(index+1)), true
}

// Get returns the scalar shape of one callable: its scope, whether a return
// clause was written, and the variadic tail with its coordinate.
func (view TypeFunctions) Get(term keyspace.Term) (keyspace.Term, keyspace.Term, source.Coordinate, bool, bool) {
	row, ok := view.row(term)
	return row.Scope, row.Variadic, row.VariadicCoordinate, row.ReturnsKnown, ok
}

// row resolves one callable behind the availability fence. Every column read
// below goes through it, so a term this table does not name fails closed
// before any column is touched.
func (view TypeFunctions) row(term keyspace.Term) (TypeFunctionRow, bool) {
	if !view.view.available {
		return TypeFunctionRow{}, false
	}
	return view.view.table.function.Row(term)
}

func (view TypeFunctions) TypeParamCount(term keyspace.Term) (int, bool) {
	row, ok := view.row(term)
	if !ok {
		return 0, false
	}
	return view.view.table.terms.Count(row.TypeParams), true
}

func (view TypeFunctions) TypeParamAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	row, ok := view.row(term)
	if !ok {
		return 0, false
	}
	return view.view.table.terms.At(row.TypeParams, index)
}

func (view TypeFunctions) ParameterCount(term keyspace.Term) (int, bool) {
	row, ok := view.row(term)
	if !ok {
		return 0, false
	}
	return view.view.table.parameters.Count(row.Parameters), true
}

func (view TypeFunctions) ParameterAt(term keyspace.Term, index int) (Parameter, bool) {
	row, ok := view.row(term)
	if !ok {
		return Parameter{}, false
	}
	return view.view.table.parameters.At(row.Parameters, index)
}

func (view TypeFunctions) ReturnCount(term keyspace.Term) (int, bool) {
	row, ok := view.row(term)
	if !ok {
		return 0, false
	}
	return view.view.table.terms.Count(row.Returns), true
}

func (view TypeFunctions) ReturnAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	row, ok := view.row(term)
	if !ok {
		return 0, false
	}
	return view.view.table.terms.At(row.Returns, index)
}

// Get returns the authored assertion row fields in their canonical order.
func (view Assertions) Get(term keyspace.Term) (keyspace.Key, source.Coordinate, bool, uint32, keyspace.Term, bool) {
	if !view.view.available {
		return 0, source.Coordinate{}, false, 0, 0, false
	}
	row, ok := view.view.table.assert.Row(term)
	return row.Name, row.ParamCoordinate, row.Bound, row.Param, row.Narrow, ok
}
