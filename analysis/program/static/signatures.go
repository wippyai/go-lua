package static

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
	"github.com/wippyai/go-lua/analysis/program/source"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
)

// compactSignatures owns the complete authored TypeFunction and TypeAsserts
// denominator. It retains direct typed rows and source-order pools only; no
// generic static node or child-edge representation is introduced here.
func compactSignatures(component *Component, counts [keyspace.FamilyCount]uint32, input SignaturesInput) error {
	store := &component.signatures
	for _, row := range input.TypeFunction {
		if !validSignature(counts, row) {
			return errors.New("program/static: invalid type function")
		}
		params, ok := appendTerms(&store.params, row.TypeParams)
		if !ok {
			return errors.New("program/static: oversized type function parameters")
		}
		fixed, ok := appendParameters(&store.fixed, row.Parameters)
		if !ok {
			return errors.New("program/static: oversized type function fixed parameters")
		}
		returns, ok := appendTerms(&store.returns, row.Returns)
		if !ok {
			return errors.New("program/static: oversized type function returns")
		}
		store.functions = append(store.functions, typeFunctionRow{
			scope: row.Scope, typeParams: params, parameters: fixed,
			variadic: row.Variadic, variadicCoord: row.VariadicCoordinate,
			returnsKnown: row.ReturnsKnown, returns: returns,
		})
	}
	for _, row := range input.TypeAsserts {
		if !validAssertion(counts, row) {
			return errors.New("program/static: invalid type assertion")
		}
		store.assertions = append(store.assertions, row)
	}
	if !interfaceMethodScopes(component, counts) {
		return errors.New("program/static: interface method signature scope mismatch")
	}
	return nil
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

func appendParameters(pool *[]parameterRow, values []Parameter) (poolRange, bool) {
	start := len(*pool)
	if uint64(start)+uint64(len(values)) > uint64(^uint32(0)) {
		return poolRange{}, false
	}
	for _, value := range values {
		*pool = append(*pool, parameterRow{name: value.Name, coordinate: value.NameCoordinate, typ: value.Type})
	}
	return poolRange{Start: uint32(start), End: uint32(len(*pool))}, true
}

func validAssertion(counts [keyspace.FamilyCount]uint32, row TypeAsserts) bool {
	if row.Name == 0 || !validCoordinate(row.ParamCoordinate) ||
		(!row.Bound && row.Param != 0) {
		return false
	}
	return row.Narrow == 0 || staticrole.Node(counts, row.Narrow)
}

func interfaceMethodScopes(component *Component, counts [keyspace.FamilyCount]uint32) bool {
	for index, iface := range component.declarations.interfaces {
		owner := keyspace.MakeTerm(keyspace.FamilyTypeInterface, uint32(index+1))
		for _, member := range component.declarations.members[iface.members.Start:iface.members.End] {
			if member.kind != InterfaceMethod {
				continue
			}
			if !hasFamily(counts, member.signature, keyspace.FamilyTypeFunction) ||
				component.signatures.functions[keyspace.TermOrdinal(member.signature)-1].scope != owner {
				return false
			}
		}
	}
	return true
}

// emitSignaturesContainment owns the typed children of TypeFunction and
// TypeAsserts. Scope stays an opaque cross-owner anchor.
func emitSignaturesContainment(component *Component, check *containment) bool {
	store := &component.signatures
	for index, row := range store.functions {
		parent := keyspace.MakeTerm(keyspace.FamilyTypeFunction, uint32(index+1))
		for _, parameter := range store.fixed[row.parameters.Start:row.parameters.End] {
			if !check.attach(parent, parameter.typ) {
				return false
			}
		}
		if row.variadic != 0 && !check.attach(parent, row.variadic) {
			return false
		}
		for _, child := range store.returns[row.returns.Start:row.returns.End] {
			if !check.attach(parent, child) || !check.markIfAssertionReturn(parent, child) {
				return false
			}
		}
	}
	for index, row := range store.assertions {
		if row.Narrow != 0 && !check.attach(keyspace.MakeTerm(keyspace.FamilyTypeAsserts, uint32(index+1)), row.Narrow) {
			return false
		}
	}
	return true
}

func (check *containment) markIfAssertionReturn(parent, child keyspace.Term) bool {
	if keyspace.TermFamily(child) != keyspace.FamilyTypeAsserts {
		return true
	}
	return check.markDirectReturn(parent, child)
}

// validBoundAssertions is a signature law rather than generic containment:
// only a direct return may bind a parameter, and a TypeFunction binding uses
// its binder-last formal-name rule.
func validBoundAssertions(component *Component, check *containment) bool {
	for index, row := range component.signatures.assertions {
		if !row.Bound {
			continue
		}
		assertion := keyspace.MakeTerm(keyspace.FamilyTypeAsserts, uint32(index+1))
		parent := check.parentOf(assertion)
		directReturns := check.directReturns[keyspace.FamilyTypeAsserts]
		ordinal := keyspace.TermOrdinal(assertion)
		if parent == 0 || ordinal == 0 || uint64(ordinal) > uint64(len(directReturns)) || directReturns[ordinal-1] != parent {
			return false
		}
		switch keyspace.TermFamily(parent) {
		case keyspace.FamilyTypeFunction:
			ordinal := keyspace.TermOrdinal(parent)
			if ordinal == 0 || int(ordinal) > len(component.signatures.functions) {
				return false
			}
			function := component.signatures.functions[ordinal-1]
			if row.Param >= function.parameters.End-function.parameters.Start {
				return false
			}
			parameter := component.signatures.fixed[function.parameters.Start+row.Param]
			if parameter.name == 0 || parameter.name != row.Name {
				return false
			}
			for _, later := range component.signatures.fixed[function.parameters.Start+row.Param+1 : function.parameters.End] {
				if later.name == row.Name {
					return false
				}
			}
		case keyspace.FamilyFunction:
			ordinal := keyspace.TermOrdinal(parent)
			if ordinal == 0 || int(ordinal) > len(component.contracts.functions) {
				return false
			}
		default:
			return false
		}
	}
	return true
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
