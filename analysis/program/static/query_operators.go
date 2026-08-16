package static

import "github.com/wippyai/go-lua/analysis/program/keyspace"

func (view View) Operators() Operators {
	return Operators{component: view.component, state: view.state}
}
func (view Operators) TypeOfs() TypeOfs { return TypeOfs{component: view.component, state: view.state} }
func (view Operators) KeyOfs() KeyOfs   { return KeyOfs{component: view.component, state: view.state} }
func (view Operators) IndexAccesses() IndexAccesses {
	return IndexAccesses{component: view.component, state: view.state}
}
func (view Operators) Conditionals() Conditionals {
	return Conditionals{component: view.component, state: view.state}
}

func (view TypeOfs) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.operators.typeOf)
}
func (view KeyOfs) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.operators.keyOf)
}
func (view IndexAccesses) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.operators.indexAccess)
}
func (view Conditionals) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.operators.conditional)
}

func (view TypeOfs) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyTypeOf, index, view.Count())
}
func (view KeyOfs) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyTypeKeyOf, index, view.Count())
}
func (view IndexAccesses) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyTypeIndexAccess, index, view.Count())
}
func (view Conditionals) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyTypeConditional, index, view.Count())
}

func (view TypeOfs) Get(term keyspace.Term) (keyspace.Term, keyspace.Term, bool) {
	component := view.componentOf()
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeOf, len(component.operators.typeOf)) {
		return 0, 0, false
	}
	row := component.operators.typeOf[keyspace.TermOrdinal(term)-1]
	return row.Scope, row.Operand, true
}
func (view KeyOfs) Get(term keyspace.Term) (keyspace.Term, bool) {
	component := view.componentOf()
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeKeyOf, len(component.operators.keyOf)) {
		return 0, false
	}
	return component.operators.keyOf[keyspace.TermOrdinal(term)-1].Inner, true
}
func (view IndexAccesses) Get(term keyspace.Term) (keyspace.Term, keyspace.Term, bool) {
	component := view.componentOf()
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeIndexAccess, len(component.operators.indexAccess)) {
		return 0, 0, false
	}
	row := component.operators.indexAccess[keyspace.TermOrdinal(term)-1]
	return row.Object, row.Index, true
}
func (view Conditionals) Get(term keyspace.Term) (keyspace.Term, keyspace.Term, keyspace.Term, keyspace.Term, bool) {
	component := view.componentOf()
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeConditional, len(component.operators.conditional)) {
		return 0, 0, 0, 0, false
	}
	row := component.operators.conditional[keyspace.TermOrdinal(term)-1]
	return row.Check, row.Extends, row.Then, row.Else, true
}
