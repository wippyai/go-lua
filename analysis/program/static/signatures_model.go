package static

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Parameter is one authored fixed parameter of a TypeFunction. An absent
// source name has both Name and NameCoordinate zero; a named parameter has
// both present. The parameter's Type is a concrete static type child.
type Parameter struct {
	Name           keyspace.Key
	NameCoordinate source.Coordinate
	Type           keyspace.Term
}

// TypeFunction is one source-only static callable. Scope is the existing
// static-scope handle; its eventual lexical/body containment is sealed jointly
// with the owner that owns that geometry. Params, Parameters, and Returns are
// source ordered. ReturnsKnown distinguishes an omitted clause from `-> ()`.
type TypeFunction struct {
	Scope              keyspace.Term
	TypeParams         []keyspace.Term
	Parameters         []Parameter
	Variadic           keyspace.Term
	VariadicCoordinate source.Coordinate
	ReturnsKnown       bool
	Returns            []keyspace.Term
}

// TypeAsserts retains the authored asserted parameter and its immediate
// binder disposition without an overloaded negative sentinel. Narrow zero is
// the authored truthy/non-nil form.
type TypeAsserts struct {
	Name            keyspace.Key
	ParamCoordinate source.Coordinate
	Bound           bool
	Param           uint32
	Narrow          keyspace.Term
}

type SignaturesInput struct {
	TypeFunction []TypeFunction
	TypeAsserts  []TypeAsserts
}

type signatureStore struct {
	functions  []typeFunctionRow
	assertions []TypeAsserts
	params     []keyspace.Term
	fixed      []parameterRow
	returns    []keyspace.Term
}

type typeFunctionRow struct {
	scope         keyspace.Term
	typeParams    poolRange
	parameters    poolRange
	variadic      keyspace.Term
	variadicCoord source.Coordinate
	returnsKnown  bool
	returns       poolRange
}

type parameterRow struct {
	name       keyspace.Key
	coordinate source.Coordinate
	typ        keyspace.Term
}

type Signatures struct {
	component *Component
	state     *draftState
}
type TypeFunctions struct {
	component *Component
	state     *draftState
}
type Assertions struct {
	component *Component
	state     *draftState
}
