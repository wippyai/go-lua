package publications

import "github.com/wippyai/go-lua/analysis/program/keyspace"

// View is the immutable query surface over a sealed publication table. It
// holds the sealed table by value: the enclosing owner checks its publication
// fence once when it mints the view. A zero View is permanently unavailable.
type View struct {
	table     Table
	available bool
}

// View returns a query surface over this sealed table.
func (table Table) View() View { return View{table: table, available: true} }

// Available reports whether this view resolves to a sealed relation.
func (view View) Available() bool { return view.available }

// Count is the authored publication denominator.
func (view View) Count() int {
	if !view.available {
		return 0
	}
	return view.table.Count()
}

// At returns the canonical term of one dense index.
func (view View) At(index int) (keyspace.Term, bool) {
	if !view.available || index < 0 || index >= view.table.Count() {
		return 0, false
	}
	return keyspace.MakeTerm(keyspace.FamilyTypePublication, uint32(index+1)), true
}

// Get returns the exact authored relation. It never reconstructs a dotted
// publication path: References owns spelling, Flow owns Assign geometry, and
// Link later derives the export namespace.
func (view View) Get(term keyspace.Term) (keyspace.Term, uint32, keyspace.Term, bool) {
	if !view.available {
		return 0, 0, 0, false
	}
	row, ok := view.table.publication.Row(term)
	return row.Assign, row.Pair, row.Target, ok
}
