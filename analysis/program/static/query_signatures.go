package static

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func (view View) Signatures() Signatures {
	return Signatures{component: view.component, state: view.state}
}
func (view Signatures) TypeFunctions() TypeFunctions {
	return TypeFunctions{component: view.component, state: view.state}
}
func (view Signatures) Assertions() Assertions {
	return Assertions{component: view.component, state: view.state}
}

func (view TypeFunctions) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.signatures.functions)
}
func (view Assertions) Count() int {
	component := view.componentOf()
	if component == nil {
		return 0
	}
	return len(component.signatures.assertions)
}
func (view TypeFunctions) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyTypeFunction, index, view.Count())
}
func (view Assertions) At(index int) (keyspace.Term, bool) {
	return at(keyspace.FamilyTypeAsserts, index, view.Count())
}

// Get preserves the source-only callable header. VariadicCoordinate is zero
// exactly when Variadic is absent; ReturnsKnown distinguishes omission from an
// authored empty return list.
func (view TypeFunctions) Get(term keyspace.Term) (keyspace.Term, keyspace.Term, source.Coordinate, bool, bool) {
	component := view.componentOf()
	row, ok := functionRowAt(component, term)
	return row.scope, row.variadic, row.variadicCoord, row.returnsKnown, ok
}
func (view TypeFunctions) TypeParamCount(term keyspace.Term) (int, bool) {
	component := view.componentOf()
	row, ok := functionRowAt(component, term)
	return int(row.typeParams.End - row.typeParams.Start), ok
}
func (view TypeFunctions) TypeParamAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	component := view.componentOf()
	row, ok := functionRowAt(component, term)
	if !ok || index < 0 || uint32(index) >= row.typeParams.End-row.typeParams.Start {
		return 0, false
	}
	return component.signatures.params[row.typeParams.Start+uint32(index)], true
}
func (view TypeFunctions) ParameterCount(term keyspace.Term) (int, bool) {
	component := view.componentOf()
	row, ok := functionRowAt(component, term)
	return int(row.parameters.End - row.parameters.Start), ok
}
func (view TypeFunctions) ParameterAt(term keyspace.Term, index int) (Parameter, bool) {
	component := view.componentOf()
	row, ok := functionRowAt(component, term)
	if !ok || index < 0 || uint32(index) >= row.parameters.End-row.parameters.Start {
		return Parameter{}, false
	}
	param := component.signatures.fixed[row.parameters.Start+uint32(index)]
	return Parameter{Name: param.name, NameCoordinate: param.coordinate, Type: param.typ}, true
}
func (view TypeFunctions) ReturnCount(term keyspace.Term) (int, bool) {
	component := view.componentOf()
	row, ok := functionRowAt(component, term)
	return int(row.returns.End - row.returns.Start), ok
}
func (view TypeFunctions) ReturnAt(term keyspace.Term, index int) (keyspace.Term, bool) {
	component := view.componentOf()
	row, ok := functionRowAt(component, term)
	if !ok || index < 0 || uint32(index) >= row.returns.End-row.returns.Start {
		return 0, false
	}
	return component.signatures.returns[row.returns.Start+uint32(index)], true
}

func (view Assertions) Get(term keyspace.Term) (keyspace.Key, source.Coordinate, bool, uint32, keyspace.Term, bool) {
	component := view.componentOf()
	row, ok := assertionRowAt(component, term)
	return row.Name, row.ParamCoordinate, row.Bound, row.Param, row.Narrow, ok
}

func functionRowAt(component *Component, term keyspace.Term) (typeFunctionRow, bool) {
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeFunction, len(component.signatures.functions)) {
		return typeFunctionRow{}, false
	}
	return component.signatures.functions[keyspace.TermOrdinal(term)-1], true
}
func assertionRowAt(component *Component, term keyspace.Term) (TypeAsserts, bool) {
	if component == nil || !keyspace.ValidTerm(term, keyspace.FamilyTypeAsserts, len(component.signatures.assertions)) {
		return TypeAsserts{}, false
	}
	return component.signatures.assertions[keyspace.TermOrdinal(term)-1], true
}
