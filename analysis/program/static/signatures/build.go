package signatures

import (
	"errors"

	"github.com/wippyai/go-lua/internal/rows"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
)

// Build validates and seals the complete authored TypeFunction and TypeAsserts
// denominator. It retains direct typed rows and source-order columns only; no
// generic static node or child-edge representation is introduced here.
func Build(input Input, counts [keyspace.FamilyCount]uint32) (Table, error) {
	var terms rows.PoolBuilder[keyspace.Term]
	var parameters rows.PoolBuilder[Parameter]
	function := rows.NewTableBuilder[TypeFunctionRow](keyspace.FamilyTypeFunction)
	for _, row := range input.TypeFunction {
		if !validSignature(counts, row) {
			return Table{}, errors.New("program/static/signatures: invalid type function")
		}
		params, ok := terms.Append(row.TypeParams)
		if !ok {
			return Table{}, errors.New("program/static/signatures: oversized type function parameters")
		}
		fixed, ok := parameters.Append(row.Parameters)
		if !ok {
			return Table{}, errors.New("program/static/signatures: oversized type function fixed parameters")
		}
		results, ok := terms.Append(row.Returns)
		if !ok {
			return Table{}, errors.New("program/static/signatures: oversized type function returns")
		}
		sealed := TypeFunctionRow{
			Scope: row.Scope, TypeParams: params, Parameters: fixed,
			Variadic: row.Variadic, VariadicCoordinate: row.VariadicCoordinate,
			ReturnsKnown: row.ReturnsKnown, Returns: results,
		}
		if _, ok := function.Append(sealed); !ok {
			return Table{}, errors.New("program/static/signatures: oversized type function table")
		}
	}
	assert := rows.NewTableBuilder[TypeAsserts](keyspace.FamilyTypeAsserts)
	for _, row := range input.TypeAsserts {
		if !validAssertion(counts, row) {
			return Table{}, errors.New("program/static/signatures: invalid type assertion")
		}
		if _, ok := assert.Append(row); !ok {
			return Table{}, errors.New("program/static/signatures: oversized type assertion table")
		}
	}
	return Table{
		function: function.Seal(), assert: assert.Seal(),
		terms: terms.Seal(), parameters: parameters.Seal(),
	}, nil
}

func validSignature(counts [keyspace.FamilyCount]uint32, row TypeFunction) bool {
	if !staticrole.ScopeHandle(counts, row.Scope) ||
		(!row.ReturnsKnown && len(row.Returns) != 0) ||
		(row.Variadic == 0) != (row.VariadicCoordinate == (source.Coordinate{})) {
		return false
	}
	if row.Variadic != 0 && (!staticrole.Node(counts, row.Variadic) || !validCoordinate(row.VariadicCoordinate)) {
		return false
	}
	for _, param := range row.Parameters {
		if !staticrole.Node(counts, param.Type) || (param.Name == 0) != (param.NameCoordinate == (source.Coordinate{})) {
			return false
		}
		if param.Name != 0 && !validCoordinate(param.NameCoordinate) {
			return false
		}
	}
	for _, result := range row.Returns {
		if !staticrole.Node(counts, result) {
			return false
		}
	}
	return true
}

func validAssertion(counts [keyspace.FamilyCount]uint32, row TypeAsserts) bool {
	if row.Name == 0 || !validCoordinate(row.ParamCoordinate) || (!row.Bound && row.Param != 0) {
		return false
	}
	return row.Narrow == 0 || staticrole.Node(counts, row.Narrow)
}

// validCoordinate admits only a present, well-formed authored span.
func validCoordinate(value source.Coordinate) bool {
	if value == (source.Coordinate{}) {
		return false
	}
	startLine, startColumn, endLine, endColumn := value.Parts()
	rebuilt, ok := source.CoordinateFromParts(startLine, startColumn, endLine, endColumn)
	return ok && rebuilt == value
}
