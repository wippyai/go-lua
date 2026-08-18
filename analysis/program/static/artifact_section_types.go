package static

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

func (decoder *staticArtifactDecoder) types(output *TypesInput) error {
	if !decoder.probing && !decoder.preflighted {
		if err := decoder.preflightTypes(); err != nil {
			return err
		}
	}
	count, err := decoder.count(staticArtifactPrimitiveWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Primitive = make([]Primitive, count)
	}
	for index := 0; index < count; index++ {
		kind, err := decoder.enum(uint64(PrimitiveSelf))
		if err != nil {
			return err
		}
		if !decoder.probing {
			output.Primitive[index].Kind = PrimitiveKind(kind)
		}
	}

	count, err = decoder.count(staticArtifactLiteralWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Literal = make([]Literal, count)
	}
	for index := 0; index < count; index++ {
		kind, err := decoder.enum(uint64(keyspace.LiteralString))
		if err != nil {
			return err
		}
		exact, err := decoder.uint32()
		if err != nil {
			return err
		}
		floatBits, err := decoder.reader.Uint()
		if err != nil {
			return err
		}
		switch keyspace.LiteralKind(kind) {
		case keyspace.LiteralBool, keyspace.LiteralInteger, keyspace.LiteralString:
			if exact == 0 || floatBits != 0 {
				return errInvalidArtifactSection
			}
		case keyspace.LiteralFloat:
			if exact != 0 {
				return errInvalidArtifactSection
			}
		default:
			return errInvalidArtifactSection
		}
		if !decoder.probing {
			output.Literal[index] = Literal{
				Kind:      keyspace.LiteralKind(kind),
				Exact:     keyspace.Key(exact),
				FloatBits: floatBits,
			}
		}
	}

	count, err = decoder.count(staticArtifactOptionalWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Optional = make([]Optional, count)
	}
	for index := 0; index < count; index++ {
		inner, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		if !decoder.probing {
			output.Optional[index] = Optional{Inner: inner}
		}
	}

	union, err := decoder.unions()
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Union = union
	}
	intersection, err := decoder.intersections()
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Intersection = intersection
	}

	count, err = decoder.count(staticArtifactGenericWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Generic = make([]Generic, count)
	}
	for index := 0; index < count; index++ {
		base, err := decoder.constrainedTerm(staticArtifactTypeRefTerm)
		if err != nil {
			return err
		}
		args, err := decoder.termSequenceConstraint(1, staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		if !decoder.probing {
			output.Generic[index] = Generic{Base: base, Args: args}
		}
	}

	count, err = decoder.count(staticArtifactArrayWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Array = make([]Array, count)
	}
	for index := 0; index < count; index++ {
		element, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		readOnly, err := decoder.boolean()
		if err != nil {
			return err
		}
		if !decoder.probing {
			output.Array[index] = Array{Element: element, ReadOnly: readOnly}
		}
	}

	count, err = decoder.count(staticArtifactMapWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Map = make([]Map, count)
	}
	for index := 0; index < count; index++ {
		key, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		value, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		readOnly, err := decoder.boolean()
		if err != nil {
			return err
		}
		if !decoder.probing {
			output.Map[index] = Map{Key: key, Value: value, ReadOnly: readOnly}
		}
	}

	count, err = decoder.count(staticArtifactRecordWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Record = make([]Record, count)
	}
	for index := 0; index < count; index++ {
		readOnly, err := decoder.boolean()
		if err != nil {
			return err
		}
		fields, err := decoder.termSequenceConstraint(0, staticArtifactTypeFieldTerm)
		if err != nil {
			return err
		}
		if !decoder.probing {
			output.Record[index] = Record{Fields: fields, ReadOnly: readOnly}
		}
	}

	count, err = decoder.count(staticArtifactFieldWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Field = make([]Field, count)
	}
	for index := 0; index < count; index++ {
		key, err := decoder.key()
		if err != nil {
			return err
		}
		typ, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		optional, err := decoder.boolean()
		if err != nil {
			return err
		}
		if !decoder.probing {
			output.Field[index] = Field{Key: key, Type: typ, Optional: optional}
		}
	}
	return nil
}

func (decoder *staticArtifactDecoder) unions() ([]Union, error) {
	count, err := decoder.count(staticArtifactUnionWireMin)
	if err != nil {
		return nil, err
	}
	var rows []Union
	if !decoder.probing {
		rows = make([]Union, count)
	}
	for index := 0; index < count; index++ {
		members, err := decoder.termSequenceConstraint(2, staticArtifactStaticNodeTerm)
		if err != nil {
			return nil, err
		}
		if !decoder.probing {
			rows[index] = Union{Members: members}
		}
	}
	return rows, nil
}

func (decoder *staticArtifactDecoder) intersections() ([]Intersection, error) {
	count, err := decoder.count(staticArtifactUnionWireMin)
	if err != nil {
		return nil, err
	}
	var rows []Intersection
	if !decoder.probing {
		rows = make([]Intersection, count)
	}
	for index := 0; index < count; index++ {
		members, err := decoder.termSequenceConstraint(2, staticArtifactStaticNodeTerm)
		if err != nil {
			return nil, err
		}
		if !decoder.probing {
			rows[index] = Intersection{Members: members}
		}
	}
	return rows, nil
}

// writeTypesContent owns the exact authored scalar order of the Types
// vertical. Pool ranges are storage layout and are intentionally re-expanded
// into their semantic sequences here.
func writeTypesContent(writer *framing.Writer, store typeStore) error {
	if err := writer.Count(uint64(len(store.primitive))); err != nil {
		return err
	}
	for _, row := range store.primitive {
		if err := writer.Uint(uint64(row.Kind)); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(store.literal))); err != nil {
		return err
	}
	for _, row := range store.literal {
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
	if err := writer.Count(uint64(len(store.optional))); err != nil {
		return err
	}
	for _, row := range store.optional {
		if err := writer.Uint(uint64(row.Inner)); err != nil {
			return err
		}
	}
	if err := writeTypeTermRangesContent(writer, store.union, store.terms); err != nil {
		return err
	}
	if err := writeTypeTermRangesContent(writer, store.intersection, store.terms); err != nil {
		return err
	}
	if err := writer.Count(uint64(len(store.generic))); err != nil {
		return err
	}
	for _, row := range store.generic {
		if err := writer.Uint(uint64(row.base)); err != nil {
			return err
		}
		if err := writeTypeTermsContent(writer, store.terms[row.args.Start:row.args.End]); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(store.array))); err != nil {
		return err
	}
	for _, row := range store.array {
		if err := writer.Uint(uint64(row.Element)); err != nil {
			return err
		}
		if err := writer.Bool(row.ReadOnly); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(store.mapType))); err != nil {
		return err
	}
	for _, row := range store.mapType {
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
	if err := writer.Count(uint64(len(store.record))); err != nil {
		return err
	}
	for _, row := range store.record {
		if err := writer.Bool(row.readOnly); err != nil {
			return err
		}
		if err := writeTypeTermsContent(writer, store.fields[row.fields.Start:row.fields.End]); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(store.field))); err != nil {
		return err
	}
	for _, row := range store.field {
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

func writeTypeTermRangesContent(writer *framing.Writer, rows []poolRange, terms []keyspace.Term) error {
	if err := writer.Count(uint64(len(rows))); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writeTypeTermsContent(writer, terms[row.Start:row.End]); err != nil {
			return err
		}
	}
	return nil
}
