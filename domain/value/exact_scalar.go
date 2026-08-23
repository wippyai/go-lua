package value

import "github.com/wippyai/go-lua/analysis/program/keyspace"

// ExactScalarKind is Value's closed classification of one singleton scalar.
// It is intentionally narrower than RuntimeKinds: an opaque scalar or a
// relation with multiple alternatives is not an exact constant.
type ExactScalarKind uint8

const (
	ExactScalarInvalid ExactScalarKind = iota
	ExactScalarNil
	ExactScalarBoolean
	ExactScalarLiteral
)

// ExactScalar is a detached scalar projection. Literal is available only for
// sealed Boolean/Integer/Float/String atoms; nil retains its distinct Lua
// identity and therefore has no fabricated keyspace literal. Computed
// literals remain absent from Atom.ExactKey, which is the authored key
// authority.
type ExactScalar struct {
	kind    ExactScalarKind
	literal keyspace.LiteralValue
}

func (scalar ExactScalar) Kind() ExactScalarKind { return scalar.kind }

func (scalar ExactScalar) Literal() (keyspace.LiteralValue, bool) {
	if scalar.kind != ExactScalarBoolean && scalar.kind != ExactScalarLiteral {
		return keyspace.LiteralValue{}, false
	}
	return scalar.literal, true
}

// ExactScalar classifies only a singleton, non-Top Value owned by this exact
// Schema. This is the sole public scalar-constant projection; Analysis does
// not inspect Value's private atom representation.
func (schema *Schema) ExactScalar(value Value) (ExactScalar, bool) {
	if schema == nil || !schema.owns(value) || value.top || len(value.image) != schema.stride() {
		return ExactScalar{}, false
	}
	row := schema.atoms[value.image[0]-1]
	switch row.kind {
	case atomNil:
		return ExactScalar{kind: ExactScalarNil}, true
	case atomFalse:
		return ExactScalar{kind: ExactScalarBoolean, literal: keyspace.LiteralValue{Kind: keyspace.LiteralBool}}, true
	case atomTrue:
		return ExactScalar{kind: ExactScalarBoolean, literal: keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: true}}, true
	case atomLiteral, atomComputedLiteral:
		if row.kind == atomLiteral && !row.hasKey || row.key.Kind < keyspace.LiteralBool || row.key.Kind > keyspace.LiteralString {
			return ExactScalar{}, false
		}
		return ExactScalar{kind: ExactScalarLiteral, literal: row.key}, true
	default:
		return ExactScalar{}, false
	}
}

// Nil returns Lua's sole exact nil Value from this Schema. Missing result
// positions use this owner-issued atom; callers never manufacture an Unknown
// or infer the private atom ordinal.
func (schema *Schema) Nil() (Value, bool) {
	if schema == nil {
		return Value{}, false
	}
	id := schema.atomByRow[atomRow{kind: atomNil}]
	if id == 0 {
		return Value{}, false
	}
	return schema.Singleton(Atom{schema: schema, id: id})
}
