package static

import (
	"fmt"
	"math"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	programstatic "github.com/wippyai/go-lua/analysis/program/static"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
)

// validCountedTerm closes a child edge against the frozen Collector census.
// Admission normally enforces this earlier; the freeze law repeats it for
// private Flow-owned sidecars and for hostile row mutation tests.
func validCountedTerm(counts [keyspace.FamilyCount]uint32, term keyspace.Term) bool {
	family := keyspace.TermFamily(term)
	return family != keyspace.FamilyInvalid && family != keyspace.FamilyImport && family < keyspace.FamilyCount &&
		keyspace.TermOrdinal(term) <= counts[family]
}

func validStaticNodeTerm(counts [keyspace.FamilyCount]uint32, term keyspace.Term) bool {
	return staticrole.Node(counts, term)
}

func validStaticScopeTerm(counts [keyspace.FamilyCount]uint32, term keyspace.Term) bool {
	return staticrole.ScopeHandle(counts, term)
}

func validRawKeyShape(raw staticRawKey) bool {
	return raw.present && validStaticRawLiteral(raw.value)
}

func validRawName(raw staticRawKey) bool {
	return validRawKeyShape(raw) && raw.value.Kind == keyspace.LiteralString && raw.value.String != ""
}

func validRequiredCoordinate(coordinate source.Coordinate) bool {
	return coordinate != (source.Coordinate{}) && validCoordinateOrZero(coordinate)
}

// validateStaticRowsTerms ensures freeze never returns an Input containing a
// zero child, foreign family, missing fill, malformed payload, or future
// ordinal. The static package still owns its deeper cycle/containment proof.
func validateStaticRowsTerms(rows *staticRows, counts [keyspace.FamilyCount]uint32) error {
	if rows == nil {
		return fmt.Errorf("program/lower/collector: nil Static rows")
	}
	term := func(value keyspace.Term, label string) error {
		if !validStaticNodeTerm(counts, value) {
			return fmt.Errorf("program/lower/collector: invalid Static %s term", label)
		}
		return nil
	}
	family := func(value keyspace.Term, want keyspace.Family, label string) error {
		if !validCountedTerm(counts, value) || keyspace.TermFamily(value) != want {
			return fmt.Errorf("program/lower/collector: invalid Static %s family", label)
		}
		return nil
	}
	for index, row := range rows.primitive {
		if !kindValid(row.Kind) {
			return fmt.Errorf("program/lower/collector: invalid primitive %d", index)
		}
	}
	for index, row := range rows.literal {
		switch row.kind {
		case keyspace.LiteralBool, keyspace.LiteralInteger, keyspace.LiteralString:
			if !validRawKeyShape(row.exact) || row.floatBits != 0 {
				return fmt.Errorf("program/lower/collector: invalid literal %d", index)
			}
		case keyspace.LiteralFloat:
			if row.exact.present || math.IsNaN(math.Float64frombits(row.floatBits)) {
				return fmt.Errorf("program/lower/collector: invalid float literal %d", index)
			}
		default:
			return fmt.Errorf("program/lower/collector: invalid literal kind %d", index)
		}
	}
	for index, row := range rows.optional {
		if err := term(row.Inner, fmt.Sprintf("optional %d child", index)); err != nil {
			return err
		}
	}
	for index, row := range rows.union {
		if len(row.Members) < 2 {
			return fmt.Errorf("program/lower/collector: union %d has insufficient members", index)
		}
		for child, value := range row.Members {
			if err := term(value, fmt.Sprintf("union %d member %d", index, child)); err != nil {
				return err
			}
		}
	}
	for index, row := range rows.intersection {
		if len(row.Members) < 2 {
			return fmt.Errorf("program/lower/collector: intersection %d has insufficient members", index)
		}
		for child, value := range row.Members {
			if err := term(value, fmt.Sprintf("intersection %d member %d", index, child)); err != nil {
				return err
			}
		}
	}
	for index, row := range rows.generic {
		if err := family(row.base, keyspace.FamilyTypeRef, fmt.Sprintf("generic %d base", index)); err != nil {
			return err
		}
		if len(row.args) == 0 {
			return fmt.Errorf("program/lower/collector: generic %d has no arguments", index)
		}
		for child, value := range row.args {
			if err := term(value, fmt.Sprintf("generic %d argument %d", index, child)); err != nil {
				return err
			}
		}
	}
	for index, row := range rows.array {
		if err := term(row.Element, fmt.Sprintf("array %d element", index)); err != nil {
			return err
		}
	}
	for index, row := range rows.mapType {
		if err := term(row.Key, fmt.Sprintf("map %d key", index)); err != nil {
			return err
		}
		if err := term(row.Value, fmt.Sprintf("map %d value", index)); err != nil {
			return err
		}
	}
	for index, row := range rows.field {
		if !validRawName(row.key) {
			return fmt.Errorf("program/lower/collector: invalid field %d key", index)
		}
		if err := term(row.typ, fmt.Sprintf("field %d type", index)); err != nil {
			return err
		}
	}
	for index, row := range rows.record {
		for child, value := range row.Fields {
			if err := family(value, keyspace.FamilyTypeField, fmt.Sprintf("record %d field %d", index, child)); err != nil {
				return err
			}
		}
	}
	for index, row := range rows.references {
		if len(row.source) == 0 {
			return fmt.Errorf("program/lower/collector: TypeRef %d has no source path", index)
		}
		for child, value := range row.source {
			if !validRawName(value) {
				return fmt.Errorf("program/lower/collector: invalid TypeRef %d source component %d", index, child)
			}
		}
		for child, value := range row.canonical {
			if !validRawName(value) {
				return fmt.Errorf("program/lower/collector: invalid TypeRef %d canonical component %d", index, child)
			}
		}
		if len(row.source) == 1 {
			if row.root != 0 {
				return fmt.Errorf("program/lower/collector: TypeRef %d unexpected root", index)
			}
		} else if err := family(row.root, keyspace.FamilyCell, fmt.Sprintf("TypeRef %d root", index)); err != nil {
			return err
		}
		switch row.resolution {
		case programstatic.TypeRefUnresolved:
			if row.target != 0 || len(row.canonical) != 0 {
				return fmt.Errorf("program/lower/collector: invalid unresolved TypeRef %d", index)
			}
		case programstatic.TypeRefDeclaration:
			if len(row.canonical) != 0 || !validCountedTerm(counts, row.target) {
				return fmt.Errorf("program/lower/collector: invalid declaration TypeRef %d", index)
			}
			if !staticrole.TypeReferenceTarget(counts, row.target) {
				return fmt.Errorf("program/lower/collector: invalid declaration TypeRef target %d", index)
			}
		case programstatic.TypeRefCanonicalPath:
			if row.target != 0 || len(row.canonical) == 0 {
				return fmt.Errorf("program/lower/collector: invalid canonical TypeRef %d", index)
			}
		default:
			return fmt.Errorf("program/lower/collector: invalid TypeRef resolution %d", index)
		}
	}
	for index, row := range rows.aliases {
		if err := family(row.owner, keyspace.FamilyBody, fmt.Sprintf("alias %d owner", index)); err != nil || !validRawName(row.name) || !validRequiredCoordinate(row.coordinate) || !row.targetSet || !row.paramsSet {
			if err != nil {
				return err
			}
			return fmt.Errorf("program/lower/collector: incomplete alias %d", index)
		}
		if err := term(row.target, fmt.Sprintf("alias %d target", index)); err != nil {
			return err
		}
		for child, value := range row.params {
			if err := family(value, keyspace.FamilyTypeParam, fmt.Sprintf("alias %d parameter %d", index, child)); err != nil {
				return err
			}
		}
	}
	for index, row := range rows.params {
		if !validRawName(row.name) || !staticrole.TypeParameterOwner(counts, row.owner) || !row.filled || (row.constraint != 0 && !validStaticNodeTerm(counts, row.constraint)) {
			return fmt.Errorf("program/lower/collector: invalid type parameter %d", index)
		}
	}
	for index, row := range rows.interfaces {
		if err := family(row.owner, keyspace.FamilyBody, fmt.Sprintf("interface %d owner", index)); err != nil {
			return err
		}
		if !validRawName(row.name) || !validRequiredCoordinate(row.coordinate) || !row.extendsSet || !row.membersSet {
			return fmt.Errorf("program/lower/collector: incomplete interface %d", index)
		}
		for child, value := range row.extends {
			if err := family(value, keyspace.FamilyTypeRef, fmt.Sprintf("interface %d extension %d", index, child)); err != nil {
				return err
			}
		}
		for child, member := range row.membersRaw {
			switch member.kind {
			case programstatic.InterfaceField:
				if err := family(member.field, keyspace.FamilyTypeField, fmt.Sprintf("interface %d field %d", index, child)); err != nil || member.name.present || member.signature != 0 || member.coordinate != (source.Coordinate{}) {
					if err != nil {
						return err
					}
					return fmt.Errorf("program/lower/collector: malformed interface field %d", child)
				}
			case programstatic.InterfaceMethod:
				if !validRawName(member.name) || !validRequiredCoordinate(member.coordinate) {
					return fmt.Errorf("program/lower/collector: malformed interface method %d", child)
				}
				if err := family(member.signature, keyspace.FamilyTypeFunction, fmt.Sprintf("interface %d method %d signature", index, child)); err != nil {
					return err
				}
			default:
				return fmt.Errorf("program/lower/collector: invalid interface member %d", child)
			}
		}
	}
	for index, row := range rows.declared {
		if err := family(row.Cell, keyspace.FamilyCell, fmt.Sprintf("declared type %d cell", index)); err != nil {
			return err
		}
		if err := term(row.Target, fmt.Sprintf("declared type %d target", index)); err != nil {
			return err
		}
	}
	for index, row := range rows.typeFunctions {
		if !validStaticScopeTerm(counts, row.scope) || !row.typeParamsSet || !row.parametersSet || !row.variadicSet || !row.returnsSet {
			return fmt.Errorf("program/lower/collector: incomplete TypeFunction %d", index)
		}
		for child, value := range row.typeParams {
			if err := family(value, keyspace.FamilyTypeParam, fmt.Sprintf("TypeFunction %d generic %d", index, child)); err != nil {
				return err
			}
		}
		for child, parameter := range row.parameters {
			if !validCoordinateOrZero(parameter.coordinate) || parameter.name.present != (parameter.coordinate != (source.Coordinate{})) || (parameter.name.present && !validRawName(parameter.name)) {
				return fmt.Errorf("program/lower/collector: invalid TypeFunction %d parameter %d", index, child)
			}
			if err := term(parameter.typ, fmt.Sprintf("TypeFunction %d parameter %d type", index, child)); err != nil {
				return err
			}
		}
		if row.variadic != 0 {
			if err := term(row.variadic, fmt.Sprintf("TypeFunction %d variadic", index)); err != nil || !validRequiredCoordinate(row.variadicCoordinate) {
				if err != nil {
					return err
				}
				return fmt.Errorf("program/lower/collector: invalid TypeFunction %d variadic coordinate", index)
			}
		} else if row.variadicCoordinate != (source.Coordinate{}) {
			return fmt.Errorf("program/lower/collector: absent TypeFunction %d variadic has coordinate", index)
		}
		if !row.returnsKnown && len(row.returns) != 0 {
			return fmt.Errorf("program/lower/collector: omitted TypeFunction %d returns have children", index)
		}
		for child, value := range row.returns {
			if err := term(value, fmt.Sprintf("TypeFunction %d return %d", index, child)); err != nil {
				return err
			}
		}
	}
	for index, row := range rows.assertions {
		if !validRawName(row.name) || !validRequiredCoordinate(row.coordinate) || (!row.bound && row.param != 0) || !row.narrowSet || (row.narrow != 0 && !validStaticNodeTerm(counts, row.narrow)) {
			return fmt.Errorf("program/lower/collector: invalid TypeAsserts %d", index)
		}
	}
	for index, row := range rows.functionContracts {
		if !row.typeParamsSet || !row.returnsSet || (!row.returnsKnown && len(row.returns) != 0) {
			return fmt.Errorf("program/lower/collector: incomplete FunctionContract %d", index)
		}
		for child, value := range row.typeParams {
			if err := family(value, keyspace.FamilyTypeParam, fmt.Sprintf("FunctionContract %d generic %d", index, child)); err != nil {
				return err
			}
		}
		for child, value := range row.returns {
			if err := term(value, fmt.Sprintf("FunctionContract %d return %d", index, child)); err != nil {
				return err
			}
		}
	}
	for index, row := range rows.callContracts {
		if !row.filled {
			return fmt.Errorf("program/lower/collector: incomplete CallContract %d", index)
		}
		for child, value := range row.arguments {
			if err := term(value, fmt.Sprintf("CallContract %d argument %d", index, child)); err != nil {
				return err
			}
		}
	}
	for index, row := range rows.typeOf {
		if !validStaticScopeTerm(counts, row.Scope) || !validCountedTerm(counts, row.Operand) {
			return fmt.Errorf("program/lower/collector: invalid TypeOf %d", index)
		}
	}
	for index, row := range rows.keyOf {
		if err := term(row.Inner, fmt.Sprintf("KeyOf %d child", index)); err != nil {
			return err
		}
	}
	for index, row := range rows.indexAccess {
		if err := term(row.Object, fmt.Sprintf("IndexAccess %d object", index)); err != nil {
			return err
		}
		if err := term(row.Index, fmt.Sprintf("IndexAccess %d index", index)); err != nil {
			return err
		}
	}
	for index, row := range rows.conditional {
		if err := term(row.Check, fmt.Sprintf("Conditional %d check", index)); err != nil {
			return err
		}
		if err := term(row.Extends, fmt.Sprintf("Conditional %d extends", index)); err != nil {
			return err
		}
		if err := term(row.Then, fmt.Sprintf("Conditional %d then", index)); err != nil {
			return err
		}
		if err := term(row.Else, fmt.Sprintf("Conditional %d else", index)); err != nil {
			return err
		}
	}
	for index, row := range rows.annotations {
		if !validStaticScopeTerm(counts, row.scope) || !validRawName(row.name) || !row.filled || !validCountedTerm(counts, row.values) || keyspace.TermFamily(row.values) != keyspace.FamilyValues {
			return fmt.Errorf("program/lower/collector: invalid Annotation %d", index)
		}
		if !staticrole.AnnotationTarget(counts, row.target) {
			return fmt.Errorf("program/lower/collector: invalid Annotation %d target", index)
		}
	}
	for index, row := range rows.typeValues {
		if !validStaticTypeValueTarget(rows, counts, row.Target) {
			return fmt.Errorf("program/lower/collector: invalid TypeValue %d target", index)
		}
	}
	for index, row := range rows.claims {
		if err := family(row.Claim, keyspace.FamilyValueClaim, fmt.Sprintf("Claim %d", index)); err != nil {
			return err
		}
		if err := term(row.Target, fmt.Sprintf("Claim %d target", index)); err != nil {
			return err
		}
	}
	for index, row := range rows.publications {
		if err := family(row.Assign, keyspace.FamilyAssign, fmt.Sprintf("publication %d assign", index)); err != nil {
			return err
		}
		if err := family(row.Target, keyspace.FamilyTypeRef, fmt.Sprintf("publication %d target", index)); err != nil {
			return err
		}
		ref := rows.references[keyspace.TermOrdinal(row.Target)-1]
		if ref.resolution != programstatic.TypeRefDeclaration && ref.resolution != programstatic.TypeRefCanonicalPath {
			return fmt.Errorf("program/lower/collector: publication %d target is unresolved", index)
		}
	}
	return nil
}

// validStaticTypeValueTarget is deliberately local to the TypeValue
// sidecar. Runtime-loadable primitive and resolved alias/interface TypeRef
// targets are a row/value law, not the general Static Node role.
func validStaticTypeValueTarget(rows *staticRows, counts [keyspace.FamilyCount]uint32, target keyspace.Term) bool {
	if rows == nil || !validCountedTerm(counts, target) {
		return false
	}
	switch keyspace.TermFamily(target) {
	case keyspace.FamilyTypePrimitive:
		ordinal := keyspace.TermOrdinal(target)
		return ordinal != 0 && uint64(ordinal) <= uint64(len(rows.primitive)) && rows.primitive[ordinal-1].Kind.RuntimeLoadable()
	case keyspace.FamilyTypeRef:
		ordinal := keyspace.TermOrdinal(target)
		if ordinal == 0 || uint64(ordinal) > uint64(len(rows.references)) {
			return false
		}
		ref := rows.references[ordinal-1]
		return ref.resolution == programstatic.TypeRefDeclaration &&
			(ref.target != 0 && (keyspace.TermFamily(ref.target) == keyspace.FamilyTypeAlias || keyspace.TermFamily(ref.target) == keyspace.FamilyTypeInterface)) &&
			validCountedTerm(counts, ref.target)
	default:
		return false
	}
}
