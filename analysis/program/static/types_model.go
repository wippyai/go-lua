package static

import "github.com/wippyai/go-lua/analysis/program/keyspace"

// PrimitiveKind is the complete parser-authored primitive type vocabulary.
// User-defined spellings are owned by the References vertical, not open
// primitive names.
type PrimitiveKind uint8

const (
	PrimitiveNil PrimitiveKind = iota + 1
	PrimitiveBoolean
	PrimitiveNumber
	PrimitiveInteger
	PrimitiveString
	PrimitiveFunction
	PrimitiveAny
	PrimitiveUnknown
	PrimitiveNever
	PrimitiveSelf
)

func (kind PrimitiveKind) valid() bool { return kind >= PrimitiveNil && kind <= PrimitiveSelf }

// RuntimeLoadable is the exact primitive subset represented by a runtime
// type singleton. Function and Self are static-only forms.
func (kind PrimitiveKind) RuntimeLoadable() bool {
	switch kind {
	case PrimitiveNil, PrimitiveBoolean, PrimitiveNumber, PrimitiveInteger,
		PrimitiveString, PrimitiveAny, PrimitiveUnknown, PrimitiveNever:
		return true
	default:
		return false
	}
}

// PrimitiveKindForName is the only spelling-to-primitive conversion. It is
// intentionally closed so parser changes make this boundary fail closed.
func PrimitiveKindForName(name string) (PrimitiveKind, bool) {
	switch name {
	case "nil":
		return PrimitiveNil, true
	case "boolean":
		return PrimitiveBoolean, true
	case "number":
		return PrimitiveNumber, true
	case "integer":
		return PrimitiveInteger, true
	case "string":
		return PrimitiveString, true
	case "function":
		return PrimitiveFunction, true
	case "any":
		return PrimitiveAny, true
	case "unknown":
		return PrimitiveUnknown, true
	case "never":
		return PrimitiveNever, true
	case "self":
		return PrimitiveSelf, true
	default:
		return 0, false
	}
}

// Primitive, Literal, Optional, Union, Intersection, Generic, Array, Map,
// Record, and Field are separate typed relations. There is deliberately
// no universal node kind or generic child edge.
type Primitive struct{ Kind PrimitiveKind }

// Literal carries no duplicate atom payload. Bool, Integer, and String use a
// Source/keyspace-owned Exact handle; Float retains its authored IEEE payload.
type Literal struct {
	Kind      keyspace.LiteralKind
	Exact     keyspace.Key
	FloatBits uint64
}
type Optional struct{ Inner keyspace.Term }
type Union struct{ Members []keyspace.Term }
type Intersection struct{ Members []keyspace.Term }

// Generic has a TypeRef base owned by the References vertical; its
// arguments are source-ordered authored type occurrences.
type Generic struct {
	Base keyspace.Term
	Args []keyspace.Term
}

type Array struct {
	Element  keyspace.Term
	ReadOnly bool
}

type Map struct {
	Key, Value keyspace.Term
	ReadOnly   bool
}

// Field is later claimed exactly once by a Record or an Interface member.
// This local Types vertical records neither cross-vertical ownership choice.
type Field struct {
	Key      keyspace.Key
	Type     keyspace.Term
	Optional bool
}

type Record struct {
	Fields   []keyspace.Term
	ReadOnly bool
}

// TypesInput is the full authored Types denominator. Counts allocates and
// validates canonical Term identities but is never retained after Build.
type TypesInput struct {
	Primitive    []Primitive
	Literal      []Literal
	Optional     []Optional
	Union        []Union
	Intersection []Intersection
	Generic      []Generic
	Array        []Array
	Map          []Map
	Record       []Record
	Field        []Field
}

type typeStore struct {
	primitive    []Primitive
	literal      []Literal
	optional     []Optional
	union        []poolRange
	intersection []poolRange
	generic      []genericRow
	array        []Array
	mapType      []Map
	record       []recordRow
	field        []Field
	terms        []keyspace.Term
	fields       []keyspace.Term
}

type genericRow struct {
	base keyspace.Term
	args poolRange
}

type recordRow struct {
	fields   poolRange
	readOnly bool
}

type Types struct {
	component *Component
	state     *draftState
}
type Primitives struct {
	component *Component
	state     *draftState
}
type Literals struct {
	component *Component
	state     *draftState
}
type Optionals struct {
	component *Component
	state     *draftState
}
type Unions struct {
	component *Component
	state     *draftState
}
type Intersections struct {
	component *Component
	state     *draftState
}
type Generics struct {
	component *Component
	state     *draftState
}
type Arrays struct {
	component *Component
	state     *draftState
}
type Maps struct {
	component *Component
	state     *draftState
}
type Records struct {
	component *Component
	state     *draftState
}
type Fields struct {
	component *Component
	state     *draftState
}
