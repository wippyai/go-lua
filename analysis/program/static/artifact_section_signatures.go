package static

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
	"github.com/wippyai/go-lua/internal/framing"
)

func (decoder *staticArtifactDecoder) signatures(output *SignaturesInput) error {
	if !decoder.probing && !decoder.preflighted {
		if err := decoder.preflightSignatures(); err != nil {
			return err
		}
	}
	count, err := decoder.count(staticArtifactTypeFunctionWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.TypeFunction = make([]TypeFunction, count)
	}
	for index := 0; index < count; index++ {
		scope, err := decoder.term()
		if err != nil {
			return err
		}
		if !staticrole.ScopeHandleFamily(keyspace.TermFamily(scope)) {
			return errInvalidArtifactSection
		}
		typeParams, err := decoder.termSequenceConstraint(0, staticArtifactTypeParamTerm)
		if err != nil {
			return err
		}
		parameterCount, err := decoder.count(staticArtifactParameterWireMin)
		if err != nil {
			return err
		}
		var parameters []Parameter
		if !decoder.probing {
			parameters = make([]Parameter, parameterCount)
		}
		for parameterIndex := 0; parameterIndex < parameterCount; parameterIndex++ {
			name, err := decoder.uint32()
			if err != nil {
				return err
			}
			coordinate, err := decoder.coordinate()
			if err != nil {
				return err
			}
			typ, err := decoder.constrainedTerm(staticArtifactStaticNodeTerm)
			if err != nil {
				return err
			}
			if (name == 0) != (coordinate == (source.Coordinate{})) {
				return errInvalidArtifactSection
			}
			if !decoder.probing {
				parameters[parameterIndex] = Parameter{Name: keyspace.Key(name), NameCoordinate: coordinate, Type: typ}
			}
		}
		variadic, err := decoder.term()
		if err != nil {
			return err
		}
		variadicCoordinate, err := decoder.coordinate()
		if err != nil {
			return err
		}
		if (variadic == 0) != (variadicCoordinate == (source.Coordinate{})) {
			return errInvalidArtifactSection
		}
		if variadic != 0 && !validDecodedTerm(variadic, staticArtifactStaticNodeTerm) {
			return errInvalidArtifactSection
		}
		returnsKnown, err := decoder.boolean()
		if err != nil {
			return err
		}
		returns, err := decoder.termSequenceConstraint(0, staticArtifactStaticNodeTerm)
		if err != nil {
			return err
		}
		returnsCount := decoder.lastTermCount
		if !returnsKnown && returnsCount != 0 {
			return errInvalidArtifactSection
		}
		if !decoder.probing {
			output.TypeFunction[index] = TypeFunction{
				Scope: scope, TypeParams: typeParams, Parameters: parameters,
				Variadic: variadic, VariadicCoordinate: variadicCoordinate,
				ReturnsKnown: returnsKnown, Returns: returns,
			}
		}
	}

	count, err = decoder.count(staticArtifactAssertionWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.TypeAsserts = make([]TypeAsserts, count)
	}
	for index := 0; index < count; index++ {
		name, err := decoder.key()
		if err != nil {
			return err
		}
		coordinate, err := decoder.coordinate()
		if err != nil || coordinate == (source.Coordinate{}) {
			if err != nil {
				return err
			}
			return errInvalidArtifactSection
		}
		bound, err := decoder.boolean()
		if err != nil {
			return err
		}
		parameter, err := decoder.uint32()
		if err != nil {
			return err
		}
		if !bound && parameter != 0 {
			return errInvalidArtifactSection
		}
		narrow, err := decoder.term()
		if err != nil {
			return err
		}
		if narrow != 0 && !validDecodedTerm(narrow, staticArtifactStaticNodeTerm) {
			return errInvalidArtifactSection
		}
		if !decoder.probing {
			output.TypeAsserts[index] = TypeAsserts{Name: name, ParamCoordinate: coordinate, Bound: bound, Param: parameter, Narrow: narrow}
		}
	}
	return nil
}

// writeSignaturesContent owns source-only static callable and assertion
// syntax. It expands all local pools; their range offsets are not authored
// semantics.
func writeSignaturesContent(writer *framing.Writer, store signatureStore) error {
	if err := writer.Count(uint64(len(store.functions))); err != nil {
		return err
	}
	for _, row := range store.functions {
		if err := writer.Uint(uint64(row.scope)); err != nil {
			return err
		}
		if err := writeTypeTermsContent(writer, store.params[row.typeParams.Start:row.typeParams.End]); err != nil {
			return err
		}
		parameters := store.fixed[row.parameters.Start:row.parameters.End]
		if err := writer.Count(uint64(len(parameters))); err != nil {
			return err
		}
		for _, parameter := range parameters {
			if err := writer.Uint(uint64(parameter.name)); err != nil {
				return err
			}
			if err := writeCoordinateContent(writer, parameter.coordinate); err != nil {
				return err
			}
			if err := writer.Uint(uint64(parameter.typ)); err != nil {
				return err
			}
		}
		if err := writer.Uint(uint64(row.variadic)); err != nil {
			return err
		}
		if err := writeCoordinateContent(writer, row.variadicCoord); err != nil {
			return err
		}
		if err := writer.Bool(row.returnsKnown); err != nil {
			return err
		}
		if err := writeTypeTermsContent(writer, store.returns[row.returns.Start:row.returns.End]); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(store.assertions))); err != nil {
		return err
	}
	for _, row := range store.assertions {
		if err := writer.Uint(uint64(row.Name)); err != nil {
			return err
		}
		if err := writeCoordinateContent(writer, row.ParamCoordinate); err != nil {
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
