package types

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/rows"
)

// View is the immutable query surface over a sealed type forest. It holds the
// sealed table by value: the enclosing owner checks its publication fence once
// when it mints the view, and the rows a view already holds cannot change
// afterwards. A zero View is permanently unavailable and every cursor derived
// from it is empty.
type View struct {
	table     Table
	available bool
}

// View returns a query surface over this sealed table.
func (table Table) View() View { return View{table: table, available: true} }

// Available reports whether this view resolves to a sealed type forest.
func (view View) Available() bool { return view.available }

// The ten cursors are exact typed relations. Each is a value handle over the
// sealed table; deriving one performs no lookup and retains no owner state.
type Primitives struct{ view View }
type Literals struct{ view View }
type Optionals struct{ view View }
type Unions struct{ view View }
type Intersections struct{ view View }
type Generics struct{ view View }
type Arrays struct{ view View }
type Maps struct{ view View }
type Records struct{ view View }
type Fields struct{ view View }

func (view View) Primitives() Primitives       { return Primitives{view: view} }
func (view View) Literals() Literals           { return Literals{view: view} }
func (view View) Optionals() Optionals         { return Optionals{view: view} }
func (view View) Unions() Unions               { return Unions{view: view} }
func (view View) Intersections() Intersections { return Intersections{view: view} }
func (view View) Generics() Generics           { return Generics{view: view} }
func (view View) Arrays() Arrays               { return Arrays{view: view} }
func (view View) Maps() Maps                   { return Maps{view: view} }
func (view View) Records() Records             { return Records{view: view} }
func (view View) Fields() Fields               { return Fields{view: view} }

func (view Primitives) Count() int    { return view.view.count(keyspace.FamilyTypePrimitive) }
func (view Literals) Count() int      { return view.view.count(keyspace.FamilyTypeLiteral) }
func (view Optionals) Count() int     { return view.view.count(keyspace.FamilyTypeOptional) }
func (view Unions) Count() int        { return view.view.count(keyspace.FamilyTypeUnion) }
func (view Intersections) Count() int { return view.view.count(keyspace.FamilyTypeIntersection) }
func (view Generics) Count() int      { return view.view.count(keyspace.FamilyTypeGeneric) }
func (view Arrays) Count() int        { return view.view.count(keyspace.FamilyTypeArray) }
func (view Maps) Count() int          { return view.view.count(keyspace.FamilyTypeMap) }
func (view Records) Count() int       { return view.view.count(keyspace.FamilyTypeRecord) }
func (view Fields) Count() int        { return view.view.count(keyspace.FamilyTypeField) }

func (view View) count(family keyspace.Family) int {
	if !view.available {
		return 0
	}
	return view.table.Count(family)
}

func (view Optionals) At(index int) (keyspace.Term, bool) {
	return view.view.term(keyspace.FamilyTypeOptional, index)
}
func (view Unions) At(index int) (keyspace.Term, bool) {
	return view.view.term(keyspace.FamilyTypeUnion, index)
}
func (view Intersections) At(index int) (keyspace.Term, bool) {
	return view.view.term(keyspace.FamilyTypeIntersection, index)
}
func (view Generics) At(index int) (keyspace.Term, bool) {
	return view.view.term(keyspace.FamilyTypeGeneric, index)
}
func (view Arrays) At(index int) (keyspace.Term, bool) {
	return view.view.term(keyspace.FamilyTypeArray, index)
}
func (view Maps) At(index int) (keyspace.Term, bool) {
	return view.view.term(keyspace.FamilyTypeMap, index)
}
func (view Records) At(index int) (keyspace.Term, bool) {
	return view.view.term(keyspace.FamilyTypeRecord, index)
}
func (view Fields) At(index int) (keyspace.Term, bool) {
	return view.view.term(keyspace.FamilyTypeField, index)
}

// term is the one canonical index-to-term projection of this vertical. It
// reads the sealed denominator rather than recounting a relation.
func (view View) term(family keyspace.Family, index int) (keyspace.Term, bool) {
	if !view.available || index < 0 || index >= view.table.Count(family) {
		return 0, false
	}
	return keyspace.MakeTerm(family, uint32(index+1)), true
}

func (view Primitives) Get(term keyspace.Term) (PrimitiveKind, bool) {
	if !view.view.available {
		return 0, false
	}
	row, ok := view.view.table.primitive.Row(term)
	return row.Kind, ok
}

func (view Literals) Get(term keyspace.Term) (keyspace.LiteralKind, keyspace.Key, uint64, bool) {
	if !view.view.available {
		return 0, 0, 0, false
	}
	row, ok := view.view.table.literal.Row(term)
	return row.Kind, row.Exact, row.FloatBits, ok
}

func (view Optionals) Get(term keyspace.Term) (keyspace.Term, bool) {
	if !view.view.available {
		return 0, false
	}
	row, ok := view.view.table.optional.Row(term)
	return row.Inner, ok
}

func (view Arrays) Get(term keyspace.Term) (keyspace.Term, bool, bool) {
	if !view.view.available {
		return 0, false, false
	}
	row, ok := view.view.table.array.Row(term)
	return row.Element, row.ReadOnly, ok
}

func (view Maps) Get(term keyspace.Term) (keyspace.Term, keyspace.Term, bool, bool) {
	if !view.view.available {
		return 0, 0, false, false
	}
	row, ok := view.view.table.mapType.Row(term)
	return row.Key, row.Value, row.ReadOnly, ok
}

func (view Fields) Get(term keyspace.Term) (keyspace.Key, keyspace.Term, bool, bool) {
	if !view.view.available {
		return 0, 0, false, false
	}
	row, ok := view.view.table.field.Row(term)
	return row.Key, row.Type, row.Optional, ok
}

func (view Unions) MemberCount(term keyspace.Term) (int, bool) {
	return view.view.spanCount(view.view.table.union, term)
}
func (view Unions) MemberAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	return view.view.spanAt(view.view.table.union, term, index)
}
func (view Intersections) MemberCount(term keyspace.Term) (int, bool) {
	return view.view.spanCount(view.view.table.intersection, term)
}
func (view Intersections) MemberAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	return view.view.spanAt(view.view.table.intersection, term, index)
}

func (view Generics) Get(term keyspace.Term) (keyspace.Term, int, bool) {
	if !view.view.available {
		return 0, 0, false
	}
	row, ok := view.view.table.generic.Row(term)
	if !ok {
		return 0, 0, false
	}
	return row.Base, view.view.table.terms.Count(row.Args), true
}
func (view Generics) ArgAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	if !view.view.available {
		return 0, false
	}
	row, ok := view.view.table.generic.Row(term)
	if !ok {
		return 0, false
	}
	return view.view.table.terms.At(row.Args, index)
}

func (view Records) Get(term keyspace.Term) (bool, int, bool) {
	if !view.view.available {
		return false, 0, false
	}
	row, ok := view.view.table.record.Row(term)
	if !ok {
		return false, 0, false
	}
	return row.ReadOnly, view.view.table.terms.Count(row.Fields), true
}
func (view Records) FieldAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	if !view.view.available {
		return 0, false
	}
	row, ok := view.view.table.record.Row(term)
	if !ok {
		return 0, false
	}
	return view.view.table.terms.At(row.Fields, index)
}

// spanCount and spanAt are the one member read shared by the two compound
// relations. Both resolve the row first, so a term this table does not name
// fails closed before any column is touched.
func (view View) spanCount(compound rows.Table[MembersRow], term keyspace.Term) (int, bool) {
	if !view.available {
		return 0, false
	}
	row, ok := compound.Row(term)
	if !ok {
		return 0, false
	}
	return view.table.terms.Count(row.Members), true
}

func (view View) spanAt(compound rows.Table[MembersRow], term keyspace.Term, index int) (keyspace.Term, bool) {
	if !view.available {
		return 0, false
	}
	row, ok := compound.Row(term)
	if !ok {
		return 0, false
	}
	return view.table.terms.At(row.Members, index)
}
