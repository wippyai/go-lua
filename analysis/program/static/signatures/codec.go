package signatures

import (
	"github.com/wippyai/go-lua/analysis/program/internal/wire"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
	"github.com/wippyai/go-lua/internal/framing"
)

const (
	// scope, three sequence counts, variadic, coordinate, the returns flag.
	typeFunctionWireMin = wire.UintWireMin * 10
	parameterWireMin    = wire.UintWireMin * 6
	assertionWireMin    = wire.UintWireMin * 8
)

// WriteContent emits source-only static callable and assertion syntax. It
// expands every column window; their offsets are not authored semantics.
func WriteContent(writer *framing.Writer, table Table) error {
	if writer == nil {
		return framing.ErrNilDestination
	}
	if err := writer.Count(uint64(table.function.Count())); err != nil {
		return err
	}
	for _, row := range table.function.All() {
		if err := writer.Uint(uint64(row.Scope)); err != nil {
			return err
		}
		if err := wire.WriteTermSpan(writer, table.terms, row.TypeParams); err != nil {
			return err
		}
		if err := writer.Count(uint64(table.parameters.Count(row.Parameters))); err != nil {
			return err
		}
		for _, parameter := range table.parameters.All(row.Parameters) {
			if err := writer.Uint(uint64(parameter.Name)); err != nil {
				return err
			}
			if err := wire.WriteCoordinate(writer, parameter.NameCoordinate); err != nil {
				return err
			}
			if err := writer.Uint(uint64(parameter.Type)); err != nil {
				return err
			}
		}
		if err := writer.Uint(uint64(row.Variadic)); err != nil {
			return err
		}
		if err := wire.WriteCoordinate(writer, row.VariadicCoordinate); err != nil {
			return err
		}
		if err := writer.Bool(row.ReturnsKnown); err != nil {
			return err
		}
		if err := wire.WriteTermSpan(writer, table.terms, row.Returns); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(table.assert.Count())); err != nil {
		return err
	}
	for _, row := range table.assert.All() {
		if err := writer.Uint(uint64(row.Name)); err != nil {
			return err
		}
		if err := wire.WriteCoordinate(writer, row.ParamCoordinate); err != nil {
			return err
		}
		if err := writer.Bool(row.Bound); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Param)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Narrow)); err != nil {
			return err
		}
	}
	return nil
}

// Scan validates and consumes one Signatures vertical without allocating row
// slices. It is the allocation-free preflight half of Decode.
func Scan(reader *framing.Reader) error {
	_, err := decode(reader, false)
	return err
}

// Decode consumes one Signatures vertical and returns owned authored rows.
func Decode(reader *framing.Reader) (Input, error) {
	return decode(reader, true)
}

func decode(reader *framing.Reader, retain bool) (Input, error) {
	if reader == nil {
		return Input{}, framing.ErrMalformed
	}
	var input Input
	count, err := wire.Count(reader, typeFunctionWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.TypeFunction = make([]TypeFunction, count)
	}
	for index := 0; index < count; index++ {
		row, err := decodeFunction(reader, retain)
		if err != nil {
			return Input{}, err
		}
		if retain {
			input.TypeFunction[index] = row
		}
	}

	count, err = wire.Count(reader, assertionWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.TypeAsserts = make([]TypeAsserts, count)
	}
	for index := 0; index < count; index++ {
		name, err := wire.Key(reader)
		if err != nil {
			return Input{}, err
		}
		coordinate, err := wire.Coordinate(reader)
		if err != nil {
			return Input{}, err
		}
		if coordinate == (source.Coordinate{}) {
			return Input{}, framing.ErrMalformed
		}
		bound, err := wire.Bool(reader)
		if err != nil {
			return Input{}, err
		}
		parameter, err := wire.Uint32(reader)
		if err != nil {
			return Input{}, err
		}
		if !bound && parameter != 0 {
			return Input{}, framing.ErrMalformed
		}
		narrow, err := wire.Term(reader)
		if err != nil {
			return Input{}, err
		}
		if narrow != 0 && !staticrole.NodeFamily(keyspace.TermFamily(narrow)) {
			return Input{}, framing.ErrMalformed
		}
		if retain {
			input.TypeAsserts[index] = TypeAsserts{
				Name: name, ParamCoordinate: coordinate,
				Bound: bound, Param: parameter, Narrow: narrow,
			}
		}
	}
	return input, nil
}

func decodeFunction(reader *framing.Reader, retain bool) (TypeFunction, error) {
	scope, err := wire.Term(reader)
	if err != nil {
		return TypeFunction{}, err
	}
	if !staticrole.ScopeHandleFamily(keyspace.TermFamily(scope)) {
		return TypeFunction{}, framing.ErrMalformed
	}
	typeParams, _, err := wire.TermSequence(reader, 0, retain, typeParamFamily)
	if err != nil {
		return TypeFunction{}, err
	}
	parameters, err := decodeParameters(reader, retain)
	if err != nil {
		return TypeFunction{}, err
	}
	variadic, err := wire.Term(reader)
	if err != nil {
		return TypeFunction{}, err
	}
	variadicCoordinate, err := wire.Coordinate(reader)
	if err != nil {
		return TypeFunction{}, err
	}
	if (variadic == 0) != (variadicCoordinate == (source.Coordinate{})) {
		return TypeFunction{}, framing.ErrMalformed
	}
	if variadic != 0 && !staticrole.NodeFamily(keyspace.TermFamily(variadic)) {
		return TypeFunction{}, framing.ErrMalformed
	}
	returnsKnown, err := wire.Bool(reader)
	if err != nil {
		return TypeFunction{}, err
	}
	returns, returnCount, err := wire.TermSequence(reader, 0, retain, staticrole.NodeFamily)
	if err != nil {
		return TypeFunction{}, err
	}
	if !returnsKnown && returnCount != 0 {
		return TypeFunction{}, framing.ErrMalformed
	}
	return TypeFunction{
		Scope: scope, TypeParams: typeParams, Parameters: parameters,
		Variadic: variadic, VariadicCoordinate: variadicCoordinate,
		ReturnsKnown: returnsKnown, Returns: returns,
	}, nil
}

func decodeParameters(reader *framing.Reader, retain bool) ([]Parameter, error) {
	count, err := wire.Count(reader, parameterWireMin)
	if err != nil {
		return nil, err
	}
	var parameters []Parameter
	if retain {
		parameters = make([]Parameter, count)
	}
	for index := 0; index < count; index++ {
		name, err := wire.Uint32(reader)
		if err != nil {
			return nil, err
		}
		coordinate, err := wire.Coordinate(reader)
		if err != nil {
			return nil, err
		}
		typ, err := wire.ConstrainedTerm(reader, staticrole.NodeFamily)
		if err != nil {
			return nil, err
		}
		if (name == 0) != (coordinate == (source.Coordinate{})) {
			return nil, framing.ErrMalformed
		}
		if retain {
			parameters[index] = Parameter{Name: keyspace.Key(name), NameCoordinate: coordinate, Type: typ}
		}
	}
	return parameters, nil
}

func typeParamFamily(family keyspace.Family) bool { return family == keyspace.FamilyTypeParam }
