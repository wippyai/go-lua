// Package operators owns authored static operator rows.
//
// The package is deliberately independent of the enclosing Static component:
// it validates and seals its own four row families, exposes only immutable
// queries, and hands the resulting table back to Static as a value.  It does
// not evaluate an operator or infer a type.
package operators

import (
	"github.com/wippyai/go-lua/internal/rows"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// TypeOf is an authored request to derive the type of a Flow value occurrence
// in a lexical scope.  Scope and Operand remain opaque canonical terms; their
// owners validate the terms' row-local meaning.
type TypeOf struct {
	Scope   keyspace.Term
	Operand keyspace.Term
}

// KeyOf is an authored static key projection.
type KeyOf struct{ Inner keyspace.Term }

// IndexAccess is an authored static indexed access.
type IndexAccess struct {
	Object keyspace.Term
	Index  keyspace.Term
}

// Conditional is an authored static conditional expression.
type Conditional struct {
	Check   keyspace.Term
	Extends keyspace.Term
	Then    keyspace.Term
	Else    keyspace.Term
}

// Input is the complete authored operator input. Build validates every row
// into the storage it seals, so an authored input and a decoded one take the
// same single-copy path and no caller slice reaches the sealed table.
type Input struct {
	TypeOf      []TypeOf
	KeyOf       []KeyOf
	IndexAccess []IndexAccess
	Conditional []Conditional
}

// Table is the sealed immutable operator table. Each relation is its own
// dense table numbered by its canonical family, so a term read is
// self-checking and no row storage escapes this owner.
type Table struct {
	typeOf      rows.Table[TypeOf]
	keyOf       rows.Table[KeyOf]
	indexAccess rows.Table[IndexAccess]
	conditional rows.Table[Conditional]
}
