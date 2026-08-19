package contracts

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// View is the immutable query surface over a sealed contract table. It holds
// the sealed table by value: the enclosing owner checks its publication fence
// once when it mints the view. A zero View is permanently unavailable.
type View struct {
	table     Table
	available bool
}

// View returns a query surface over this sealed table.
func (table Table) View() View { return View{table: table, available: true} }

// Available reports whether this view resolves to a sealed contract set.
func (view View) Available() bool { return view.available }

// Functions and Calls are exact canonical-family views. Neither becomes a
// second Flow graph.
type Functions struct{ view View }
type Calls struct{ view View }

func (view View) Functions() Functions { return Functions{view: view} }
func (view View) Calls() Calls         { return Calls{view: view} }

func (view Functions) Count() int { return view.view.count(keyspace.FamilyFunction) }
func (view Calls) Count() int     { return view.view.count(keyspace.FamilyCall) }

func (view View) count(family keyspace.Family) int {
	if !view.available {
		return 0
	}
	return view.table.Count(family)
}

func (view Functions) At(index int) (keyspace.Term, bool) {
	return view.view.term(keyspace.FamilyFunction, index)
}
func (view Calls) At(index int) (keyspace.Term, bool) {
	return view.view.term(keyspace.FamilyCall, index)
}

func (view View) term(family keyspace.Family, index int) (keyspace.Term, bool) {
	if !view.available || index < 0 || index >= view.table.Count(family) {
		return 0, false
	}
	return keyspace.MakeTerm(family, uint32(index+1)), true
}

// row resolves one function contract behind the availability fence.
func (view Functions) row(term keyspace.Term) (FunctionContractRow, bool) {
	if !view.view.available {
		return FunctionContractRow{}, false
	}
	return view.view.table.function.Row(term)
}

// Get reports whether the callable wrote a return clause at all.
func (view Functions) Get(term keyspace.Term) (returnsKnown bool, ok bool) {
	row, ok := view.row(term)
	return row.ReturnsKnown, ok
}

func (view Functions) TypeParamCount(term keyspace.Term) (int, bool) {
	row, ok := view.row(term)
	if !ok {
		return 0, false
	}
	return view.view.table.terms.Count(row.TypeParams), true
}

func (view Functions) TypeParamAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	row, ok := view.row(term)
	if !ok {
		return 0, false
	}
	return view.view.table.terms.At(row.TypeParams, index)
}

func (view Functions) ReturnCount(term keyspace.Term) (int, bool) {
	row, ok := view.row(term)
	if !ok {
		return 0, false
	}
	return view.view.table.terms.Count(row.Returns), true
}

func (view Functions) ReturnAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	row, ok := view.row(term)
	if !ok {
		return 0, false
	}
	return view.view.table.terms.At(row.Returns, index)
}

// row resolves one call contract behind the availability fence.
func (view Calls) row(term keyspace.Term) (CallContractRow, bool) {
	if !view.view.available {
		return CallContractRow{}, false
	}
	return view.view.table.call.Row(term)
}

func (view Calls) TypeArgumentCount(term keyspace.Term) (int, bool) {
	row, ok := view.row(term)
	if !ok {
		return 0, false
	}
	return view.view.table.terms.Count(row.TypeArguments), true
}

func (view Calls) TypeArgumentAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	row, ok := view.row(term)
	if !ok {
		return 0, false
	}
	return view.view.table.terms.At(row.TypeArguments, index)
}

// TypeArgumentID returns the sealed content identity of one call's authored
// type-argument sequence.
func (view Calls) TypeArgumentID(term keyspace.Term) (identity.ContentID, bool) {
	if !view.view.available {
		return identity.ContentID{}, false
	}
	return view.view.table.callArgumentID.Row(term)
}
