package operators

import (
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/program/internal/rows"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// View is the immutable query surface over a sealed operator table.  When a
// Static finalizer lends a construction view, live points to its lifecycle
// cell; publication views leave live nil and remain permanently available.
type View struct {
	table *Table
	live  *uint32
}

// NewView creates a lifecycle-bound operator view for the enclosing Static
// owner. The owner supplies only the one-shot lifetime cell, never row data.
func NewView(table *Table, live *uint32) View { return View{table: table, live: live} }

// View returns a permanently available view over a published table.
func (table *Table) View() View { return View{table: table} }

func (view View) available() bool {
	return view.table != nil && (view.live == nil || atomic.LoadUint32(view.live) != 0)
}

// Available reports whether the table can currently be queried.
func (view View) Available() bool { return view.available() }

// The four cursors hold their sealed relation by value. Deriving one is the
// single lifetime check: a cursor minted from an expired view is empty, and a
// cursor already held observes rows that can no longer change.
type TypeOfs struct{ table rows.Table[TypeOf] }
type KeyOfs struct{ table rows.Table[KeyOf] }
type IndexAccesses struct{ table rows.Table[IndexAccess] }
type Conditionals struct{ table rows.Table[Conditional] }

func (view View) TypeOfs() TypeOfs {
	if !view.available() {
		return TypeOfs{}
	}
	return TypeOfs{table: view.table.typeOf}
}

func (view View) KeyOfs() KeyOfs {
	if !view.available() {
		return KeyOfs{}
	}
	return KeyOfs{table: view.table.keyOf}
}

func (view View) IndexAccesses() IndexAccesses {
	if !view.available() {
		return IndexAccesses{}
	}
	return IndexAccesses{table: view.table.indexAccess}
}

func (view View) Conditionals() Conditionals {
	if !view.available() {
		return Conditionals{}
	}
	return Conditionals{table: view.table.conditional}
}

func (view TypeOfs) Count() int       { return view.table.Count() }
func (view KeyOfs) Count() int        { return view.table.Count() }
func (view IndexAccesses) Count() int { return view.table.Count() }
func (view Conditionals) Count() int  { return view.table.Count() }

func (view TypeOfs) At(index int) (keyspace.Term, bool)       { return view.table.Term(index) }
func (view KeyOfs) At(index int) (keyspace.Term, bool)        { return view.table.Term(index) }
func (view IndexAccesses) At(index int) (keyspace.Term, bool) { return view.table.Term(index) }
func (view Conditionals) At(index int) (keyspace.Term, bool)  { return view.table.Term(index) }

func (view TypeOfs) Get(term keyspace.Term) (keyspace.Term, keyspace.Term, bool) {
	row, ok := view.table.Row(term)
	return row.Scope, row.Operand, ok
}

func (view KeyOfs) Get(term keyspace.Term) (keyspace.Term, bool) {
	row, ok := view.table.Row(term)
	return row.Inner, ok
}

func (view IndexAccesses) Get(term keyspace.Term) (keyspace.Term, keyspace.Term, bool) {
	row, ok := view.table.Row(term)
	return row.Object, row.Index, ok
}

func (view Conditionals) Get(term keyspace.Term) (keyspace.Term, keyspace.Term, keyspace.Term, keyspace.Term, bool) {
	row, ok := view.table.Row(term)
	return row.Check, row.Extends, row.Then, row.Else, ok
}
