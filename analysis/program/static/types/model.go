// Package types owns the authored static type forest: ten distinct typed
// relations, their two child columns, and the sealed table that holds them.
//
// The package is independent of the enclosing Static component. It validates
// and seals its own rows, exposes immutable queries, and hands the resulting
// table back to Static as a value. It resolves no reference, infers nothing,
// and deliberately owns no TypeRef row: the References vertical owns that
// exact relation and appears here only as an opaque base term.
package types

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/internal/rows"
)

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

// Valid reports membership in the closed primitive vocabulary.
func (kind PrimitiveKind) Valid() bool { return kind >= PrimitiveNil && kind <= PrimitiveSelf }

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

// Input is the full authored Types denominator. Build copies every slice and
// retains none of them.
type Input struct {
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

// MembersRow is the sealed form of a Union or an Intersection: its members
// live in the shared term column and the row keeps only their window.
type MembersRow struct{ Members rows.Span }

// GenericRow is the sealed form of a Generic.
type GenericRow struct {
	Base keyspace.Term
	Args rows.Span
}

// RecordRow is the sealed form of a Record.
type RecordRow struct {
	Fields   rows.Span
	ReadOnly bool
}

// Table is the sealed immutable type forest. Every relation is its own dense
// table numbered by its canonical family, and variable-width children select
// windows from one shared term column.
type Table struct {
	primitive    rows.Table[Primitive]
	literal      rows.Table[Literal]
	optional     rows.Table[Optional]
	union        rows.Table[MembersRow]
	intersection rows.Table[MembersRow]
	generic      rows.Table[GenericRow]
	array        rows.Table[Array]
	mapType      rows.Table[Map]
	record       rows.Table[RecordRow]
	field        rows.Table[Field]
	terms        rows.Pool[keyspace.Term]
}

// Primitive returns the authored primitive one canonical term names. It is
// the read a sibling vertical uses to decide whether a runtime type target is
// loadable, without reaching into this owner's storage.
func (table Table) Primitive(term keyspace.Term) (Primitive, bool) {
	return table.primitive.Row(term)
}

// Count reports the sealed row denominator of one type family.
func (table Table) Count(family keyspace.Family) int {
	switch family {
	case keyspace.FamilyTypePrimitive:
		return table.primitive.Count()
	case keyspace.FamilyTypeLiteral:
		return table.literal.Count()
	case keyspace.FamilyTypeOptional:
		return table.optional.Count()
	case keyspace.FamilyTypeUnion:
		return table.union.Count()
	case keyspace.FamilyTypeIntersection:
		return table.intersection.Count()
	case keyspace.FamilyTypeGeneric:
		return table.generic.Count()
	case keyspace.FamilyTypeArray:
		return table.array.Count()
	case keyspace.FamilyTypeMap:
		return table.mapType.Count()
	case keyspace.FamilyTypeRecord:
		return table.record.Count()
	case keyspace.FamilyTypeField:
		return table.field.Count()
	default:
		return 0
	}
}

// CountsMatch reports whether this owner published exactly the dense native
// rows assigned to it. Static supplies the already-sealed family column; the
// typed owner, not the enclosing component, decides which of its rows
// contribute to that column.
func (table Table) CountsMatch(counts [keyspace.FamilyCount]uint32) bool {
	return table.Count(keyspace.FamilyTypePrimitive) == int(counts[keyspace.FamilyTypePrimitive]) &&
		table.Count(keyspace.FamilyTypeLiteral) == int(counts[keyspace.FamilyTypeLiteral]) &&
		table.Count(keyspace.FamilyTypeOptional) == int(counts[keyspace.FamilyTypeOptional]) &&
		table.Count(keyspace.FamilyTypeUnion) == int(counts[keyspace.FamilyTypeUnion]) &&
		table.Count(keyspace.FamilyTypeIntersection) == int(counts[keyspace.FamilyTypeIntersection]) &&
		table.Count(keyspace.FamilyTypeGeneric) == int(counts[keyspace.FamilyTypeGeneric]) &&
		table.Count(keyspace.FamilyTypeArray) == int(counts[keyspace.FamilyTypeArray]) &&
		table.Count(keyspace.FamilyTypeMap) == int(counts[keyspace.FamilyTypeMap]) &&
		table.Count(keyspace.FamilyTypeRecord) == int(counts[keyspace.FamilyTypeRecord]) &&
		table.Count(keyspace.FamilyTypeField) == int(counts[keyspace.FamilyTypeField])
}

// CountRows publishes this typed owner's contribution to the generated
// ProgramStatic denominator. The primary row is intentionally partial: the
// enclosing Static owner sums the native contributions from all typed child
// owners under the one generated identity.
func (table Table) CountRows() (denominator.CountRows, bool) {
	value := table.Count(keyspace.FamilyTypePrimitive) +
		table.Count(keyspace.FamilyTypeLiteral) +
		table.Count(keyspace.FamilyTypeOptional) +
		table.Count(keyspace.FamilyTypeUnion) +
		table.Count(keyspace.FamilyTypeIntersection) +
		table.Count(keyspace.FamilyTypeGeneric) +
		table.Count(keyspace.FamilyTypeArray) +
		table.Count(keyspace.FamilyTypeMap) +
		table.Count(keyspace.FamilyTypeRecord)
	return programStaticCountRows(value)
}

func programStaticCountRows(value int) (denominator.CountRows, bool) {
	if !keyspace.TermOrdinalFits(value) {
		return denominator.CountRows{}, false
	}
	id := denominator.GeneratedProgramStaticIDs().ProgramStatic
	row, ok := denominator.NewCountRow(id, uint64(value))
	if !ok {
		return denominator.CountRows{}, false
	}
	return denominator.NewCountRows([]denominator.CountRow{row})
}
