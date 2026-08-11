package static

import "github.com/wippyai/go-lua/program/keyspace"

// DeclaredTypes exposes the authored Cell-to-static-type relation. It is a
// child of Declarations because it is authored declaration syntax, not a
// ninth competing Static top-level view.
func (view Declarations) DeclaredTypes() DeclaredTypes {
	return DeclaredTypes{component: view.component, state: view.state}
}

func (view DeclaredTypes) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.declarations.declaredTypes)
}

func (view DeclaredTypes) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyDeclaredType, index, view.Count())
}

func (view DeclaredTypes) Get(term keyspace.Term) (keyspace.Term, keyspace.Term, bool) {
	component := view.componentOf()
	row, ok := declaredTypeRowAt(component, term)
	return row.cell, row.target, ok
}

// ForCell is O(1) over the dense canonical Cell ordinal space. A false result
// means this locally valid Cell has no authored declared type; it says nothing
// about the Cell's later lexical role, which Static does not own.
func (view DeclaredTypes) ForCell(cell keyspace.Term) (keyspace.Term, bool) {
	component := view.componentOf()
	if component == nil || !keyspace.ValidTerm(cell, keyspace.FamilyCell, len(component.declarations.declaredByCell)) {
		return 0, false
	}
	term := component.declarations.declaredByCell[keyspace.TermOrdinal(cell)-1]
	return term, term != 0
}

func declaredTypeRowAt(component *Component, term keyspace.Term) (declaredTypeRow, bool) {
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyDeclaredType, len(component.declarations.declaredTypes)) {
		return declaredTypeRow{}, false
	}
	return component.declarations.declaredTypes[keyspace.TermOrdinal(term)-1], true
}
