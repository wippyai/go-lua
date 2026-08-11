package static

import (
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

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
