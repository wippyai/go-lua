package references

import "github.com/wippyai/go-lua/analysis/program/keyspace"

// View is the immutable query surface over a sealed TypeRef table. It holds
// the sealed table by value: the enclosing owner checks its publication fence
// once when it mints the view. A zero View is permanently unavailable.
type View struct {
	table     Table
	available bool
}

// View returns a query surface over this sealed table.
func (table Table) View() View { return View{table: table, available: true} }

// Available reports whether this view resolves to a sealed TypeRef relation.
func (view View) Available() bool { return view.available }

// Count is the authored TypeRef denominator.
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
	return keyspace.MakeTerm(keyspace.FamilyTypeRef, uint32(index+1)), true
}

// Get returns the binder disposition and its two exclusive anchors.
func (view View) Get(term keyspace.Term) (Resolution, keyspace.Term, keyspace.Term, bool) {
	if !view.available {
		return 0, 0, 0, false
	}
	row, ok := view.table.ref.Row(term)
	return row.Resolution, row.Target, row.Root, ok
}

func (view View) SourceCount(term keyspace.Term) (int, bool) {
	if !view.available {
		return 0, false
	}
	row, ok := view.table.ref.Row(term)
	if !ok {
		return 0, false
	}
	return view.table.source.Count(row.Source), true
}

func (view View) SourceAt(term keyspace.Term, index int) (keyspace.Key, bool) {
	if !view.available {
		return 0, false
	}
	row, ok := view.table.ref.Row(term)
	if !ok {
		return 0, false
	}
	return view.table.source.At(row.Source, index)
}

func (view View) CanonicalCount(term keyspace.Term) (int, bool) {
	if !view.available {
		return 0, false
	}
	row, ok := view.table.ref.Row(term)
	if !ok {
		return 0, false
	}
	return view.table.canonical.Count(row.Canonical), true
}

func (view View) CanonicalAt(term keyspace.Term, index int) (keyspace.Key, bool) {
	if !view.available {
		return 0, false
	}
	row, ok := view.table.ref.Row(term)
	if !ok {
		return 0, false
	}
	return view.table.canonical.At(row.Canonical, index)
}
