package static

import "github.com/wippyai/go-lua/analysis/program/keyspace"

func (view Publications) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.publications)
}

func (view Publications) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyTypePublication, index, view.Count())
}

// Get returns the exact authored relation. It never reconstructs a dotted
// publication path: Reference owns spelling, Flow owns Assign geometry, and
// Link later derives the export namespace.
func (view Publications) Get(term keyspace.Term) (keyspace.Term, uint32, keyspace.Term, bool) {
	component := view.componentOf()
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypePublication, len(component.publications)) {
		return 0, 0, 0, false
	}
	row := component.publications[keyspace.TermOrdinal(term)-1]
	return row.assign, row.pair, row.target, true
}
