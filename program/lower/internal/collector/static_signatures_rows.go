package collector

import (
	"errors"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// TypeFunction is deliberately split into declare/fill phases. Every fill
// tracks a separate explicit state so omitted returns differ from known-empty.
func (rows *staticRows) TypeFunctionDeclare(term, scope keyspace.Term) error {
	if err := staticDenseAppendTerm(term, keyspace.FamilyTypeFunction, len(rows.typeFunctions)); err != nil {
		return err
	}
	rows.typeFunctions = append(rows.typeFunctions, staticRawTypeFunction{scope: scope})
	return nil
}

func (rows *staticRows) TypeFunctionGenerics(term keyspace.Term, params []keyspace.Term) error {
	index, err := denseOrdinal(term, keyspace.FamilyTypeFunction, len(rows.typeFunctions))
	if err != nil {
		return err
	}
	row := &rows.typeFunctions[index]
	if row.typeParamsSet {
		return errors.New("program/lower/collector: TypeFunction generics filled twice")
	}
	row.typeParams = append([]keyspace.Term(nil), params...)
	row.typeParamsSet = true
	return nil
}

func (rows *staticRows) TypeFunctionParametersRaw(term keyspace.Term, params []staticRawParameter) error {
	index, err := denseOrdinal(term, keyspace.FamilyTypeFunction, len(rows.typeFunctions))
	if err != nil {
		return err
	}
	row := &rows.typeFunctions[index]
	if row.parametersSet {
		return errors.New("program/lower/collector: TypeFunction parameters filled twice")
	}
	for _, parameter := range params {
		if !validCoordinateOrZero(parameter.coordinate) || parameter.typ == 0 {
			return errors.New("program/lower/collector: invalid TypeFunction parameter")
		}
		if parameter.name.present {
			if parameter.name.value.Kind != keyspace.LiteralString || parameter.name.value.String == "" {
				return errors.New("program/lower/collector: invalid TypeFunction parameter name")
			}
			if parameter.coordinate == (source.Coordinate{}) {
				return errors.New("program/lower/collector: named TypeFunction parameter lacks coordinate")
			}
		} else if parameter.coordinate != (source.Coordinate{}) {
			return errors.New("program/lower/collector: unnamed TypeFunction parameter has coordinate")
		}
	}
	row.parameters = append([]staticRawParameter(nil), params...)
	row.parametersSet = true
	return nil
}

func (rows *staticRows) TypeFunctionParameters(term keyspace.Term, names []string, coordinates [][4]uint32, types []keyspace.Term) error {
	if len(names) != len(types) || (len(coordinates) != 0 && len(coordinates) != len(types)) {
		return errors.New("program/lower/collector: TypeFunction parameter shape mismatch")
	}
	params := make([]staticRawParameter, len(types))
	for index, typ := range types {
		var coordinate source.Coordinate
		var err error
		if len(coordinates) != 0 {
			coordinate, err = coordinateFromParts(coordinates[index])
			if err != nil {
				return err
			}
		}
		var name staticRawKey
		if names[index] != "" {
			name, err = rawString(names[index])
			if err != nil {
				return err
			}
		}
		params[index] = staticRawParameter{name: name, coordinate: coordinate, typ: typ}
	}
	return rows.TypeFunctionParametersRaw(term, params)
}

func (rows *staticRows) TypeFunctionVariadic(term, variadic keyspace.Term, coordinate source.Coordinate) error {
	index, err := denseOrdinal(term, keyspace.FamilyTypeFunction, len(rows.typeFunctions))
	if err != nil {
		return err
	}
	row := &rows.typeFunctions[index]
	if row.variadicSet {
		return errors.New("program/lower/collector: TypeFunction variadic filled twice")
	}
	if variadic != 0 && (coordinate == (source.Coordinate{}) || !validCoordinateOrZero(coordinate)) {
		return errors.New("program/lower/collector: invalid variadic coordinate")
	}
	if variadic == 0 && coordinate != (source.Coordinate{}) {
		return errors.New("program/lower/collector: absent variadic has coordinate")
	}
	row.variadic, row.variadicCoordinate, row.variadicSet = variadic, coordinate, true
	return nil
}

func (rows *staticRows) TypeFunctionReturns(term keyspace.Term, known bool, returns []keyspace.Term) error {
	index, err := denseOrdinal(term, keyspace.FamilyTypeFunction, len(rows.typeFunctions))
	if err != nil {
		return err
	}
	row := &rows.typeFunctions[index]
	if row.returnsSet {
		return errors.New("program/lower/collector: TypeFunction returns filled twice")
	}
	if !known && len(returns) != 0 {
		return errors.New("program/lower/collector: omitted returns have children")
	}
	row.returnsKnown, row.returns, row.returnsSet = known, append([]keyspace.Term(nil), returns...), true
	return nil
}

func (rows *staticRows) TypeAsserts(term keyspace.Term, name string, coordinate source.Coordinate, bound bool, param uint32, narrow keyspace.Term) error {
	if err := staticDenseAppendTerm(term, keyspace.FamilyTypeAsserts, len(rows.assertions)); err != nil {
		return err
	}
	key, err := rawString(name)
	if err != nil {
		return err
	}
	if coordinate == (source.Coordinate{}) || !validCoordinateOrZero(coordinate) || (!bound && param != 0) {
		return errors.New("program/lower/collector: invalid TypeAsserts row")
	}
	rows.assertions = append(rows.assertions, staticRawAssertion{name: key, coordinate: coordinate, bound: bound, param: param, narrow: narrow, narrowSet: true})
	return nil
}
