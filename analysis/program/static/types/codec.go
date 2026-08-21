package types

import (
	"github.com/wippyai/go-lua/internal/rows"
	"github.com/wippyai/go-lua/analysis/program/internal/wire"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
	"github.com/wippyai/go-lua/internal/framing"
)

// The per-row wire minima are the adversarial-arity floor for each relation:
// the smallest number of stream bytes one row of that relation can occupy.
const (
	primitiveWireMin = wire.UintWireMin
	literalWireMin   = wire.UintWireMin * 3
	optionalWireMin  = wire.UintWireMin
	compoundWireMin  = wire.UintWireMin * 3 // count plus two members
	genericWireMin   = wire.UintWireMin * 3 // base, count, one argument
	arrayWireMin     = wire.UintWireMin + wire.BoolWireMin
	mapWireMin       = wire.UintWireMin*2 + wire.BoolWireMin
	recordWireMin    = wire.BoolWireMin + wire.UintWireMin
	fieldWireMin     = wire.UintWireMin*2 + wire.BoolWireMin
)

func staticNodeFamily(family keyspace.Family) bool { return staticrole.NodeFamily(family) }
func typeRefFamily(family keyspace.Family) bool    { return family == keyspace.FamilyTypeRef }
func typeFieldFamily(family keyspace.Family) bool  { return family == keyspace.FamilyTypeField }

// WriteContent emits the exact authored scalar order of the Types vertical.
// Record framing is owned by the enclosing Static stream. Column windows are
// storage layout and are deliberately re-expanded into their semantic
// sequences here, so the wire carries relations rather than spans.
func WriteContent(writer *framing.Writer, table Table) error {
	if writer == nil {
		return framing.ErrNilDestination
	}
	if err := writer.Count(uint64(table.primitive.Count())); err != nil {
		return err
	}
	for _, row := range table.primitive.All() {
		if err := writer.Uint(uint64(row.Kind)); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(table.literal.Count())); err != nil {
		return err
	}
	for _, row := range table.literal.All() {
		if err := writer.Uint(uint64(row.Kind)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Exact)); err != nil {
			return err
		}
		if err := writer.Uint(row.FloatBits); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(table.optional.Count())); err != nil {
		return err
	}
	for _, row := range table.optional.All() {
		if err := writer.Uint(uint64(row.Inner)); err != nil {
			return err
		}
	}
	if err := writeCompound(writer, table.union, table.terms); err != nil {
		return err
	}
	if err := writeCompound(writer, table.intersection, table.terms); err != nil {
		return err
	}
	if err := writer.Count(uint64(table.generic.Count())); err != nil {
		return err
	}
	for _, row := range table.generic.All() {
		if err := writer.Uint(uint64(row.Base)); err != nil {
			return err
		}
		if err := wire.WriteTermSpan(writer, table.terms, row.Args); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(table.array.Count())); err != nil {
		return err
	}
	for _, row := range table.array.All() {
		if err := writer.Uint(uint64(row.Element)); err != nil {
			return err
		}
		if err := writer.Bool(row.ReadOnly); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(table.mapType.Count())); err != nil {
		return err
	}
	for _, row := range table.mapType.All() {
		if err := writer.Uint(uint64(row.Key)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Value)); err != nil {
			return err
		}
		if err := writer.Bool(row.ReadOnly); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(table.record.Count())); err != nil {
		return err
	}
	for _, row := range table.record.All() {
		if err := writer.Bool(row.ReadOnly); err != nil {
			return err
		}
		if err := wire.WriteTermSpan(writer, table.terms, row.Fields); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(table.field.Count())); err != nil {
		return err
	}
	for _, row := range table.field.All() {
		if err := writer.Uint(uint64(row.Key)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Type)); err != nil {
			return err
		}
		if err := writer.Bool(row.Optional); err != nil {
			return err
		}
	}
	return nil
}

// Scan validates and consumes one Types vertical without allocating row
// slices. It is the allocation-free preflight half of Decode.
func Scan(reader *framing.Reader) error {
	_, err := decode(reader, false)
	return err
}

// Decode consumes one Types vertical and returns owned authored input rows.
func Decode(reader *framing.Reader) (Input, error) {
	return decode(reader, true)
}

func decode(reader *framing.Reader, retain bool) (Input, error) {
	if reader == nil {
		return Input{}, framing.ErrMalformed
	}
	var input Input

	count, err := wire.Count(reader, primitiveWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.Primitive = make([]Primitive, count)
	}
	for index := 0; index < count; index++ {
		kind, err := wire.Enum(reader, uint64(PrimitiveSelf))
		if err != nil {
			return Input{}, err
		}
		if retain {
			input.Primitive[index] = Primitive{Kind: PrimitiveKind(kind)}
		}
	}

	count, err = wire.Count(reader, literalWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.Literal = make([]Literal, count)
	}
	for index := 0; index < count; index++ {
		kind, err := wire.Enum(reader, uint64(keyspace.LiteralString))
		if err != nil {
			return Input{}, err
		}
		exact, err := wire.Uint32(reader)
		if err != nil {
			return Input{}, err
		}
		floatBits, err := wire.Uint(reader)
		if err != nil {
			return Input{}, err
		}
		switch keyspace.LiteralKind(kind) {
		case keyspace.LiteralBool, keyspace.LiteralInteger, keyspace.LiteralString:
			if exact == 0 || floatBits != 0 {
				return Input{}, framing.ErrMalformed
			}
		case keyspace.LiteralFloat:
			if exact != 0 {
				return Input{}, framing.ErrMalformed
			}
		default:
			return Input{}, framing.ErrMalformed
		}
		if retain {
			input.Literal[index] = Literal{
				Kind:      keyspace.LiteralKind(kind),
				Exact:     keyspace.Key(exact),
				FloatBits: floatBits,
			}
		}
	}

	count, err = wire.Count(reader, optionalWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.Optional = make([]Optional, count)
	}
	for index := 0; index < count; index++ {
		inner, err := wire.ConstrainedTerm(reader, staticNodeFamily)
		if err != nil {
			return Input{}, err
		}
		if retain {
			input.Optional[index] = Optional{Inner: inner}
		}
	}

	count, err = wire.Count(reader, compoundWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.Union = make([]Union, count)
	}
	for index := 0; index < count; index++ {
		members, _, err := wire.TermSequence(reader, 2, retain, staticNodeFamily)
		if err != nil {
			return Input{}, err
		}
		if retain {
			input.Union[index] = Union{Members: members}
		}
	}

	count, err = wire.Count(reader, compoundWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.Intersection = make([]Intersection, count)
	}
	for index := 0; index < count; index++ {
		members, _, err := wire.TermSequence(reader, 2, retain, staticNodeFamily)
		if err != nil {
			return Input{}, err
		}
		if retain {
			input.Intersection[index] = Intersection{Members: members}
		}
	}

	count, err = wire.Count(reader, genericWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.Generic = make([]Generic, count)
	}
	for index := 0; index < count; index++ {
		base, err := wire.ConstrainedTerm(reader, typeRefFamily)
		if err != nil {
			return Input{}, err
		}
		args, _, err := wire.TermSequence(reader, 1, retain, staticNodeFamily)
		if err != nil {
			return Input{}, err
		}
		if retain {
			input.Generic[index] = Generic{Base: base, Args: args}
		}
	}

	count, err = wire.Count(reader, arrayWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.Array = make([]Array, count)
	}
	for index := 0; index < count; index++ {
		element, err := wire.ConstrainedTerm(reader, staticNodeFamily)
		if err != nil {
			return Input{}, err
		}
		readOnly, err := wire.Bool(reader)
		if err != nil {
			return Input{}, err
		}
		if retain {
			input.Array[index] = Array{Element: element, ReadOnly: readOnly}
		}
	}

	count, err = wire.Count(reader, mapWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.Map = make([]Map, count)
	}
	for index := 0; index < count; index++ {
		key, err := wire.ConstrainedTerm(reader, staticNodeFamily)
		if err != nil {
			return Input{}, err
		}
		value, err := wire.ConstrainedTerm(reader, staticNodeFamily)
		if err != nil {
			return Input{}, err
		}
		readOnly, err := wire.Bool(reader)
		if err != nil {
			return Input{}, err
		}
		if retain {
			input.Map[index] = Map{Key: key, Value: value, ReadOnly: readOnly}
		}
	}

	count, err = wire.Count(reader, recordWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.Record = make([]Record, count)
	}
	for index := 0; index < count; index++ {
		readOnly, err := wire.Bool(reader)
		if err != nil {
			return Input{}, err
		}
		fields, _, err := wire.TermSequence(reader, 0, retain, typeFieldFamily)
		if err != nil {
			return Input{}, err
		}
		if retain {
			input.Record[index] = Record{Fields: fields, ReadOnly: readOnly}
		}
	}

	count, err = wire.Count(reader, fieldWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.Field = make([]Field, count)
	}
	for index := 0; index < count; index++ {
		key, err := wire.Key(reader)
		if err != nil {
			return Input{}, err
		}
		typ, err := wire.ConstrainedTerm(reader, staticNodeFamily)
		if err != nil {
			return Input{}, err
		}
		optional, err := wire.Bool(reader)
		if err != nil {
			return Input{}, err
		}
		if retain {
			input.Field[index] = Field{Key: key, Type: typ, Optional: optional}
		}
	}
	return input, nil
}

// writeCompound emits one compound relation: its row count followed by each
// row's member sequence.
func writeCompound(writer *framing.Writer, compound rows.Table[MembersRow], terms rows.Pool[keyspace.Term]) error {
	if err := writer.Count(uint64(compound.Count())); err != nil {
		return err
	}
	for _, row := range compound.All() {
		if err := wire.WriteTermSpan(writer, terms, row.Members); err != nil {
			return err
		}
	}
	return nil
}
