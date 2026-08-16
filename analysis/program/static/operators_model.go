package static

import "github.com/wippyai/go-lua/analysis/program/keyspace"

// TypeOf, KeyOf, IndexAccess, and Conditional are distinct authored static
// operator relations. Their Program meaning is only their written shape;
// selection, lookup, and runtime-type judgments belong to later factors.
// TypeOf's Scope and Operand are cross-owner references, not type children.
type TypeOf struct {
	Scope   keyspace.Term
	Operand keyspace.Term
}

type KeyOf struct{ Inner keyspace.Term }

type IndexAccess struct {
	Object keyspace.Term
	Index  keyspace.Term
}

type Conditional struct {
	Check   keyspace.Term
	Extends keyspace.Term
	Then    keyspace.Term
	Else    keyspace.Term
}

// OperatorsInput is the complete authored static-operator denominator.
// All slices are copied during Build.
type OperatorsInput struct {
	TypeOf      []TypeOf
	KeyOf       []KeyOf
	IndexAccess []IndexAccess
	Conditional []Conditional
}

type operatorsStore struct {
	typeOf      []TypeOf
	keyOf       []KeyOf
	indexAccess []IndexAccess
	conditional []Conditional
}

// Operators exposes the four exact operator relations without a generic
// operator kind or universal child vocabulary.
type Operators struct {
	component *Component
	state     *draftState
}
type TypeOfs struct {
	component *Component
	state     *draftState
}
type KeyOfs struct {
	component *Component
	state     *draftState
}
type IndexAccesses struct {
	component *Component
	state     *draftState
}
type Conditionals struct {
	component *Component
	state     *draftState
}
