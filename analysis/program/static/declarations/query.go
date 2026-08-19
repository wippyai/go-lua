package declarations

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// View is the immutable query surface over a sealed declaration table. It
// holds the sealed table by value: the enclosing owner checks its publication
// fence once when it mints the view, and the rows a view already holds cannot
// change afterwards. A zero View is permanently unavailable.
type View struct {
	table     Table
	available bool
}

// View returns a query surface over this sealed table.
func (table Table) View() View { return View{table: table, available: true} }

// Available reports whether this view resolves to a sealed declaration set.
func (view View) Available() bool { return view.available }

type Aliases struct{ view View }
type TypeParams struct{ view View }
type Interfaces struct{ view View }
type DeclaredTypes struct{ view View }

func (view View) Aliases() Aliases       { return Aliases{view: view} }
func (view View) TypeParams() TypeParams { return TypeParams{view: view} }
func (view View) Interfaces() Interfaces { return Interfaces{view: view} }

// DeclaredTypes exposes the authored Cell-to-static-type relation. It is a
// child of this vertical because it is authored declaration syntax, not a
// competing Static top-level view.
func (view View) DeclaredTypes() DeclaredTypes { return DeclaredTypes{view: view} }

func (view Aliases) Count() int       { return view.view.count(keyspace.FamilyTypeAlias) }
func (view TypeParams) Count() int    { return view.view.count(keyspace.FamilyTypeParam) }
func (view Interfaces) Count() int    { return view.view.count(keyspace.FamilyTypeInterface) }
func (view DeclaredTypes) Count() int { return view.view.count(keyspace.FamilyDeclaredType) }

func (view View) count(family keyspace.Family) int {
	if !view.available {
		return 0
	}
	return view.table.Count(family)
}

func (view Aliases) At(index int) (keyspace.Term, bool) {
	return view.view.term(keyspace.FamilyTypeAlias, index)
}
func (view TypeParams) At(index int) (keyspace.Term, bool) {
	return view.view.term(keyspace.FamilyTypeParam, index)
}
func (view Interfaces) At(index int) (keyspace.Term, bool) {
	return view.view.term(keyspace.FamilyTypeInterface, index)
}
func (view DeclaredTypes) At(index int) (keyspace.Term, bool) {
	return view.view.term(keyspace.FamilyDeclaredType, index)
}

// term is the one canonical index-to-term projection of this vertical.
func (view View) term(family keyspace.Family, index int) (keyspace.Term, bool) {
	if !view.available || index < 0 || index >= view.table.Count(family) {
		return 0, false
	}
	return keyspace.MakeTerm(family, uint32(index+1)), true
}

func (view Aliases) Get(term keyspace.Term) (keyspace.Term, keyspace.Term, keyspace.Key, source.Coordinate, bool) {
	if !view.view.available {
		return 0, 0, 0, source.Coordinate{}, false
	}
	row, ok := view.view.table.alias.Row(term)
	return row.Owner, row.Target, row.Name, row.NameCoordinate, ok
}

func (view Aliases) ParamCount(term keyspace.Term) (int, bool) {
	if !view.view.available {
		return 0, false
	}
	row, ok := view.view.table.alias.Row(term)
	if !ok {
		return 0, false
	}
	return view.view.table.aliasParams.Count(row.Params), true
}

func (view Aliases) ParamAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	if !view.view.available {
		return 0, false
	}
	row, ok := view.view.table.alias.Row(term)
	if !ok {
		return 0, false
	}
	return view.view.table.aliasParams.At(row.Params, index)
}

func (view TypeParams) Get(term keyspace.Term) (keyspace.Term, keyspace.Key, keyspace.Term, bool) {
	if !view.view.available {
		return 0, 0, 0, false
	}
	row, ok := view.view.table.param.Row(term)
	return row.Owner, row.Name, row.Constraint, ok
}

func (view Interfaces) Get(term keyspace.Term) (keyspace.Term, keyspace.Key, source.Coordinate, bool) {
	if !view.view.available {
		return 0, 0, source.Coordinate{}, false
	}
	row, ok := view.view.table.iface.Row(term)
	return row.Owner, row.Name, row.NameCoordinate, ok
}

func (view Interfaces) ExtendCount(term keyspace.Term) (int, bool) {
	if !view.view.available {
		return 0, false
	}
	row, ok := view.view.table.iface.Row(term)
	if !ok {
		return 0, false
	}
	return view.view.table.interfaceRefs.Count(row.Extends), true
}

func (view Interfaces) ExtendAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	if !view.view.available {
		return 0, false
	}
	row, ok := view.view.table.iface.Row(term)
	if !ok {
		return 0, false
	}
	return view.view.table.interfaceRefs.At(row.Extends, index)
}

func (view Interfaces) MemberCount(term keyspace.Term) (int, bool) {
	if !view.view.available {
		return 0, false
	}
	row, ok := view.view.table.iface.Row(term)
	if !ok {
		return 0, false
	}
	return view.view.table.members.Count(row.Members), true
}

func (view Interfaces) MemberAt(term keyspace.Term, index int) (InterfaceMember, bool) {
	if !view.view.available {
		return InterfaceMember{}, false
	}
	row, ok := view.view.table.iface.Row(term)
	if !ok {
		return InterfaceMember{}, false
	}
	return view.view.table.members.At(row.Members, index)
}

func (view DeclaredTypes) Get(term keyspace.Term) (keyspace.Term, keyspace.Term, bool) {
	if !view.view.available {
		return 0, 0, false
	}
	row, ok := view.view.table.declaredType.Row(term)
	return row.Cell, row.Target, ok
}

// ForCell is O(1) over the dense canonical Cell ordinal space. A false result
// means this locally valid Cell has no authored declared type; it says nothing
// about the Cell's later lexical role, which Static does not own.
func (view DeclaredTypes) ForCell(cell keyspace.Term) (keyspace.Term, bool) {
	if !view.view.available {
		return 0, false
	}
	term, ok := view.view.table.declaredByCell.Row(cell)
	return term, ok && term != 0
}
