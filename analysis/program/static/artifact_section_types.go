package static

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
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
