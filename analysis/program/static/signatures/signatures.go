// Package signatures owns the authored source-only static callables: the
// TypeFunction relation with its three source-ordered columns, and the
// TypeAsserts relation.
//
// The package is independent of the enclosing Static component. It validates
// and seals its own rows, exposes immutable queries, and hands the resulting
// table back to Static as a value. Scope stays an opaque cross-owner anchor:
// this vertical never reconstructs lexical or body geometry.
package signatures

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/internal/rows"
)

// Parameter is one authored fixed parameter of a TypeFunction. An absent
// source name has both Name and NameCoordinate zero; a named parameter has
// both present. The parameter's Type is a concrete static type child.
type Parameter struct {
	Name           keyspace.Key
	NameCoordinate source.Coordinate
	Type           keyspace.Term
}

// TypeFunction is one source-only static callable. Scope is the existing
// static-scope handle; its eventual lexical/body containment is sealed jointly
// with the owner that owns that geometry. Params, Parameters, and Returns are
// source ordered. ReturnsKnown distinguishes an omitted clause from `-> ()`.
type TypeFunction struct {
	Scope              keyspace.Term
	TypeParams         []keyspace.Term
	Parameters         []Parameter
	Variadic           keyspace.Term
	VariadicCoordinate source.Coordinate
	ReturnsKnown       bool
	Returns            []keyspace.Term
}

// TypeAsserts retains the authored asserted parameter and its immediate
// binder disposition without an overloaded negative sentinel. Narrow zero is
// the authored truthy/non-nil form.
type TypeAsserts struct {
	Name            keyspace.Key
	ParamCoordinate source.Coordinate
	Bound           bool
	Param           uint32
	Narrow          keyspace.Term
}

// Input is the complete authored signature denominator.
type Input struct {
	TypeFunction []TypeFunction
	TypeAsserts  []TypeAsserts
}

// TypeFunctionRow is the sealed form of a TypeFunction: its three ordered
// child sequences live in shared columns and the row keeps their windows.
type TypeFunctionRow struct {
	Scope              keyspace.Term
	TypeParams         rows.Span
	Parameters         rows.Span
	Variadic           keyspace.Term
	VariadicCoordinate source.Coordinate
	ReturnsKnown       bool
	Returns            rows.Span
}

// Table is the sealed immutable signature relation set.
type Table struct {
	function   rows.Table[TypeFunctionRow]
	assert     rows.Table[TypeAsserts]
	terms      rows.Pool[keyspace.Term]
	parameters rows.Pool[Parameter]
}

// Count reports the sealed row denominator of one signature family.
func (table Table) Count(family keyspace.Family) int {
	switch family {
	case keyspace.FamilyTypeFunction:
		return table.function.Count()
	case keyspace.FamilyTypeAsserts:
		return table.assert.Count()
	default:
		return 0
	}
}

// CountsMatch reports the native signature denominators against the enclosing
// sealed family column.
func (table Table) CountsMatch(counts [keyspace.FamilyCount]uint32) bool {
	return table.Count(keyspace.FamilyTypeFunction) == int(counts[keyspace.FamilyTypeFunction]) &&
		table.Count(keyspace.FamilyTypeAsserts) == int(counts[keyspace.FamilyTypeAsserts])
}

// CountRows publishes this typed owner's native signature contribution to the
// generated ProgramStatic denominator.
func (table Table) CountRows() (denominator.CountRows, bool) {
	value := table.Count(keyspace.FamilyTypeFunction) + table.Count(keyspace.FamilyTypeAsserts)
	if !keyspace.TermOrdinalFits(value) {
		return denominator.CountRows{}, false
	}
	row, ok := denominator.NewCountRow(denominator.GeneratedProgramStaticIDs().ProgramStatic, uint64(value))
	if !ok {
		return denominator.CountRows{}, false
	}
	return denominator.NewCountRows([]denominator.CountRow{row})
}

// Scope returns the authored static scope of one TypeFunction. It is the read
// the interface-method scope law consumes, so no sibling reaches into this
// owner's function storage to answer the same question.
func (table Table) Scope(function keyspace.Term) (keyspace.Term, bool) {
	row, ok := table.function.Row(function)
	return row.Scope, ok
}

// Assert returns the authored assertion one canonical term names.
func (table Table) Assert(term keyspace.Term) (TypeAsserts, bool) { return table.assert.Row(term) }

// BindsFormal states this vertical's binder-last formal-name rule: the named
// parameter a bound assertion selects must exist at that index, must carry
// exactly that name, and must be the last parameter of the callable to carry
// it. Publishing the rule rather than the parameter column keeps the joint
// bound-assertion law from re-deriving a signature's own scoping.
func (table Table) BindsFormal(function keyspace.Term, param uint32, name keyspace.Key) bool {
	row, ok := table.function.Row(function)
	if !ok || name == 0 {
		return false
	}
	width := table.parameters.Count(row.Parameters)
	if uint64(param) >= uint64(width) {
		return false
	}
	selected, ok := table.parameters.At(row.Parameters, int(param))
	if !ok || selected.Name != name {
		return false
	}
	for index := int(param) + 1; index < width; index++ {
		later, ok := table.parameters.At(row.Parameters, index)
		if !ok || later.Name == name {
			return false
		}
	}
	return true
}

// VisitFunctionTypeParams emits every TypeFunction-owned type parameter claim
// against the callable that owns it, the exact mirror of the alias column the
// Declarations vertical publishes.
func (table Table) VisitFunctionTypeParams(claim func(owner, param keyspace.Term) bool) bool {
	if claim == nil {
		return false
	}
	for owner, row := range table.function.Terms() {
		for _, param := range table.terms.All(row.TypeParams) {
			if !claim(owner, param) {
				return false
			}
		}
	}
	return true
}

// VisitContainment emits the typed children of TypeFunction and TypeAsserts
// in the canonical relation order. attachReturn carries a return-position
// child, which the enclosing owner also records as direct-return evidence for
// the bound-assertion law; Scope stays an opaque cross-owner anchor and emits
// no edge.
func (table Table) VisitContainment(attach, attachReturn func(parent, child keyspace.Term) bool) bool {
	if attach == nil || attachReturn == nil {
		return false
	}
	for parent, row := range table.function.Terms() {
		for _, parameter := range table.parameters.All(row.Parameters) {
			if !attach(parent, parameter.Type) {
				return false
			}
		}
		if row.Variadic != 0 && !attach(parent, row.Variadic) {
			return false
		}
		for _, child := range table.terms.All(row.Returns) {
			if !attachReturn(parent, child) {
				return false
			}
		}
	}
	for parent, row := range table.assert.Terms() {
		if row.Narrow != 0 && !attach(parent, row.Narrow) {
			return false
		}
	}
	return true
}
