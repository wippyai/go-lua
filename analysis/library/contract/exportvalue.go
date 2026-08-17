package contract

import (
	"bytes"

	"github.com/wippyai/go-lua/analysis/internal/framing"
)

// The export-value payload format. It is the third payload this package owns,
// and it owns it for the reason the other two are owned here rather than by the
// type layer: nothing it carries is a TYPE.
//
// The declared form states three things about a non-callable export - the
// aggregate it is, the constant it is, and the mutability it is published with -
// and none of the three is a proposition about which values inhabit a set. A
// type says what a value is; whether the exported slot may be rewritten is a
// publication policy over one value, and whether an export is an aggregate whose
// own members the contract addresses further, or a constant that terminates the
// path, is a fact about the export graph this package already owns the addresses
// of. A format whose payload carries a type stays with the layer that owns the
// type; this one has no type in it at all.
//
// The published mutability is the LIBRARY's own statement about its export. It
// is deliberately not the initial environment's: whether a boot root is
// published frozen, and whether a host denies an entry outright, are the
// boot-root and denied-entry forms' business, which only the environment class
// may declare. A library that could seal its own aggregate by publishing it
// frozen would be stating an environment fact from inside a library contract.
const (
	exportValueDomain  = "analysis/library/contract/export-value"
	exportValueVersion = 1
)

// ValueShape is which of the two non-callable value shapes an export publishes.
// The distinction is structural rather than descriptive: an aggregate is a value
// the contract can keep addressing THROUGH, so a member path may continue past
// it, and a constant terminates the path.
type ValueShape uint8

const (
	ValueShapeInvalid ValueShape = iota
	// ValueShapeAggregate is an exported aggregate: the value a library root is,
	// and any exported table reached from it.
	ValueShapeAggregate
	// ValueShapeConstant is an exported constant: one value from the closed
	// literal domain of the language.
	ValueShapeConstant
	valueShapeLimit
)

func (shape ValueShape) Available() bool {
	return shape > ValueShapeInvalid && shape < valueShapeLimit
}

// Mutability is the policy one export is published under. Both spellings are
// writable so that a sealed export is a stated fact rather than an unspellable
// one; a library that publishes nothing sealed still has to say so.
type Mutability uint8

const (
	MutabilityInvalid Mutability = iota
	// MutabilityMutable publishes a value a consumer may rewrite. Every table
	// of the Lua standard library is published this way: the language itself
	// places no seal on them.
	MutabilityMutable
	// MutabilitySealed publishes a value a consumer may not rewrite.
	MutabilitySealed
	mutabilityLimit
)

func (mutability Mutability) Available() bool {
	return mutability > MutabilityInvalid && mutability < mutabilityLimit
}

// ConstantKind is the closed literal domain an exported constant is drawn from.
// It is the language's own value domain and never a Go type: no textual literal
// codec participates, so a float is written as its exact bits and a string as
// its exact bytes.
type ConstantKind uint8

const (
	ConstantInvalid ConstantKind = iota
	ConstantNil
	ConstantBoolean
	ConstantInteger
	ConstantFloat
	ConstantString
	constantKindLimit
)

func (kind ConstantKind) Available() bool {
	return kind > ConstantInvalid && kind < constantKindLimit
}

// Constant is one published constant value. Exactly the field the kind selects
// is meaningful, and every other field is zero: two spellings of one value would
// be two contract identities for one contract, so the unselected fields are a
// law rather than a convention.
type Constant struct {
	Kind    ConstantKind
	Boolean bool
	Integer int64
	// FloatBits is the exact IEEE-754 representation of a published float, as
	// math.Float64bits writes it, so a value no decimal spelling reproduces
	// still round-trips.
	FloatBits uint64
	String    string
}

func (constant Constant) Available() bool {
	if !constant.Kind.Available() {
		return false
	}
	selected := Constant{Kind: constant.Kind}
	switch constant.Kind {
	case ConstantBoolean:
		selected.Boolean = constant.Boolean
	case ConstantInteger:
		selected.Integer = constant.Integer
	case ConstantFloat:
		selected.FloatBits = constant.FloatBits
	case ConstantString:
		selected.String = constant.String
	}
	return selected == constant
}

// ExportValue is one export-value payload: what the export is, and the
// mutability it is published with.
type ExportValue struct {
	Shape      ValueShape
	Mutability Mutability
	// Constant is meaningful only for a constant export. An aggregate carries
	// none, because there is no constant an aggregate could be.
	Constant Constant
}

func (value ExportValue) Available() bool {
	if !value.Shape.Available() || !value.Mutability.Available() {
		return false
	}
	switch value.Shape {
	case ValueShapeAggregate:
		return value.Constant == Constant{}
	case ValueShapeConstant:
		return value.Constant.Available()
	default:
		return false
	}
}

// Aggregate is the common export value of a library root: the aggregate a
// mounted library is, published under one mutability.
func Aggregate(mutability Mutability) ExportValue {
	return ExportValue{Shape: ValueShapeAggregate, Mutability: mutability}
}

// EncodeExportValue writes one export value as a member payload body. The
// result is a complete framed stream: a payload is decodable on its own, so a
// reader that holds a member and its declared format never needs the enclosing
// instance to interpret it.
func EncodeExportValue(value ExportValue) ([]byte, error) {
	if !value.Available() {
		return nil, ErrMalformed
	}
	var buffer bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&buffer, exportValueDomain, exportValueVersion); err != nil {
		return nil, err
	}
	if err := writer.Uint(uint64(value.Shape)); err != nil {
		return nil, err
	}
	if err := writer.Uint(uint64(value.Mutability)); err != nil {
		return nil, err
	}
	if value.Shape == ValueShapeConstant {
		if err := writeConstant(&writer, value.Constant); err != nil {
			return nil, err
		}
	}
	if err := writer.Finish(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// writeConstant writes only the field the kind selects. A constant is written as
// the one value it is, never as a row of every value it is not.
func writeConstant(writer *framing.Writer, constant Constant) error {
	if err := writer.Record(recordConstant); err != nil {
		return err
	}
	if err := writer.Uint(uint64(constant.Kind)); err != nil {
		return err
	}
	switch constant.Kind {
	case ConstantBoolean:
		return writer.Bool(constant.Boolean)
	case ConstantInteger:
		return writer.Uint(uint64(constant.Integer))
	case ConstantFloat:
		return writer.Uint(constant.FloatBits)
	case ConstantString:
		return writer.String(constant.String)
	}
	return nil
}

// DecodeExportValue reads one export-value payload body.
func DecodeExportValue(data []byte) (ExportValue, error) {
	reader, err := framing.NewReader(data, len(data))
	if err != nil {
		return ExportValue{}, ErrMalformed
	}
	if err := reader.Header(exportValueDomain, exportValueVersion); err != nil {
		return ExportValue{}, ErrMalformed
	}
	shape, err := reader.Uint()
	if err != nil || shape > uint64(^uint8(0)) {
		return ExportValue{}, ErrMalformed
	}
	mutability, err := reader.Uint()
	if err != nil || mutability > uint64(^uint8(0)) {
		return ExportValue{}, ErrMalformed
	}
	value := ExportValue{Shape: ValueShape(shape), Mutability: Mutability(mutability)}
	if value.Shape == ValueShapeConstant {
		constant, err := readConstant(reader)
		if err != nil {
			return ExportValue{}, err
		}
		value.Constant = constant
	}
	if err := reader.Finish(); err != nil {
		return ExportValue{}, ErrMalformed
	}
	if !value.Available() {
		return ExportValue{}, ErrMalformed
	}
	return value, nil
}

func readConstant(reader *framing.Reader) (Constant, error) {
	if record, err := reader.Record(); err != nil || record != recordConstant {
		return Constant{}, ErrMalformed
	}
	kind, err := reader.Uint()
	if err != nil || kind > uint64(^uint8(0)) {
		return Constant{}, ErrMalformed
	}
	constant := Constant{Kind: ConstantKind(kind)}
	switch constant.Kind {
	case ConstantBoolean:
		constant.Boolean, err = reader.Bool()
	case ConstantInteger:
		var raw uint64
		raw, err = reader.Uint()
		constant.Integer = int64(raw)
	case ConstantFloat:
		constant.FloatBits, err = reader.Uint()
	case ConstantString:
		constant.String, err = reader.String(maxKey)
	}
	if err != nil {
		return Constant{}, ErrMalformed
	}
	return constant, nil
}
