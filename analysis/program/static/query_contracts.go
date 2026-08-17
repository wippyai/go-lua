package static

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func (view View) Contracts() Contracts {
	return Contracts(view)
}
func (view Contracts) Functions() Functions {
	return Functions(view)
}
func (view Contracts) Calls() Calls { return Calls(view) }

func (view Functions) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.contracts.functions)
}
func (view Calls) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.contracts.calls)
}
func (view Functions) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyFunction, index, view.Count())
}
func (view Calls) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyCall, index, view.Count())
}

// Get exposes the return-clause header. ReturnsKnown distinguishes an omitted
// clause from an authored empty clause; ReturnCount/ReturnAt retain its exact
// source order.
func (view Functions) Get(term keyspace.Term) (returnsKnown bool, ok bool) {
	component := view.componentOf()
	row, ok := functionContractAt(component, term)
	return row.returnsKnown, ok
}
func (view Functions) TypeParamCount(term keyspace.Term) (int, bool) {
	component := view.componentOf()
	row, ok := functionContractAt(component, term)
	return int(row.typeParams.End - row.typeParams.Start), ok
}
func (view Functions) TypeParamAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	component := view.componentOf()
	row, ok := functionContractAt(component, term)
	if !ok || index < 0 || uint32(index) >= row.typeParams.End-row.typeParams.Start {
		return 0, false
	}
	return component.contracts.terms[row.typeParams.Start+uint32(index)], true
}
func (view Functions) ReturnCount(term keyspace.Term) (int, bool) {
	component := view.componentOf()
	row, ok := functionContractAt(component, term)
	return int(row.returns.End - row.returns.Start), ok
}
func (view Functions) ReturnAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	component := view.componentOf()
	row, ok := functionContractAt(component, term)
	if !ok || index < 0 || uint32(index) >= row.returns.End-row.returns.Start {
		return 0, false
	}
	return component.contracts.terms[row.returns.Start+uint32(index)], true
}
func (view Calls) TypeArgumentCount(term keyspace.Term) (int, bool) {
	component := view.componentOf()
	row, ok := callContractAt(component, term)
	return int(row.End - row.Start), ok
}
func (view Calls) TypeArgumentAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	component := view.componentOf()
	row, ok := callContractAt(component, term)
	if !ok || index < 0 || uint32(index) >= row.End-row.Start {
		return 0, false
	}
	return component.contracts.terms[row.Start+uint32(index)], true
}

// TypeArgumentID is the immutable O(1) identity of one authored call
// type-argument column. TypeArgumentAt remains the member query.
func (view Calls) TypeArgumentID(term keyspace.Term) (identity.ContentID, bool) {
	component := view.componentOf()
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyCall, len(component.contracts.calls)) ||
		len(component.contracts.callTypeArgumentIDs) != len(component.contracts.calls) {
		return identity.ContentID{}, false
	}
	id := component.contracts.callTypeArgumentIDs[keyspace.TermOrdinal(term)-1]
	return id, id.Available()
}

func functionContractAt(component *Component, term keyspace.Term) (functionContractRow, bool) {
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyFunction, len(component.contracts.functions)) {
		return functionContractRow{}, false
	}
	return component.contracts.functions[keyspace.TermOrdinal(term)-1], true
}
func callContractAt(component *Component, term keyspace.Term) (poolRange, bool) {
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyCall, len(component.contracts.calls)) {
		return poolRange{}, false
	}
	return component.contracts.calls[keyspace.TermOrdinal(term)-1], true
}
