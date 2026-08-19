package static

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	programstatic "github.com/wippyai/go-lua/analysis/program/static"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	staticoperands "github.com/wippyai/go-lua/analysis/program/static/operands"
	staticoperators "github.com/wippyai/go-lua/analysis/program/static/operators"
	staticpubs "github.com/wippyai/go-lua/analysis/program/static/publications"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	staticsig "github.com/wippyai/go-lua/analysis/program/static/signatures"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
)

// resolveStaticKey is the only place a Static raw payload becomes a Key. The
// live Source preimage owns quotienting, equality, and final numeric handles.
func resolveStaticKey(keys source.Keys, raw staticRawKey, optional bool) (keyspace.Key, error) {
	if !raw.present {
		if optional {
			return 0, nil
		}
		return 0, errors.New("program/lower/collector: missing Static exact key payload")
	}
	key, ok := keys.Find(raw.value)
	if !ok || key == 0 {
		return 0, errors.New("program/lower/collector: Static exact payload is absent from Source preimage")
	}
	return key, nil
}

func resolveStaticPath(keys source.Keys, path []staticRawKey) ([]keyspace.Key, error) {
	if len(path) == 0 {
		return nil, errors.New("program/lower/collector: empty Static path")
	}
	result := make([]keyspace.Key, len(path))
	for index, raw := range path {
		key, err := resolveStaticKey(keys, raw, false)
		if err != nil {
			return nil, err
		}
		result[index] = key
	}
	return result, nil
}

func staticCount(counts [keyspace.FamilyCount]uint32, family keyspace.Family) (int, error) {
	count := counts[family]
	if count > keyspace.MaxTermOrdinal {
		return 0, fmt.Errorf("program/lower/collector: family %d count exceeds Term ordinal", family)
	}
	return int(count), nil
}

func staticRowsCountsMatch(rows *staticRows, counts [keyspace.FamilyCount]uint32) error {
	if rows == nil {
		return errors.New("program/lower/collector: nil Static rows")
	}
	if counts[keyspace.FamilyInvalid] != 0 || counts[keyspace.FamilyOutcome] != 0 {
		return errors.New("program/lower/collector: invalid Static count boundary")
	}
	check := func(family keyspace.Family, length int) error {
		want, err := staticCount(counts, family)
		if err != nil {
			return err
		}
		if want != length {
			return fmt.Errorf("program/lower/collector: family %d count %d does not match rows %d", family, want, length)
		}
		return nil
	}
	checks := []struct {
		family keyspace.Family
		length int
	}{
		{keyspace.FamilyTypePrimitive, len(rows.primitive)},
		{keyspace.FamilyTypeLiteral, len(rows.literal)},
		{keyspace.FamilyTypeOptional, len(rows.optional)},
		{keyspace.FamilyTypeUnion, len(rows.union)},
		{keyspace.FamilyTypeIntersection, len(rows.intersection)},
		{keyspace.FamilyTypeRef, len(rows.references)},
		{keyspace.FamilyTypeGeneric, len(rows.generic)},
		{keyspace.FamilyTypeArray, len(rows.array)},
		{keyspace.FamilyTypeMap, len(rows.mapType)},
		{keyspace.FamilyTypeRecord, len(rows.record)},
		{keyspace.FamilyTypeField, len(rows.field)},
		{keyspace.FamilyTypeAlias, len(rows.aliases)},
		{keyspace.FamilyTypeParam, len(rows.params)},
		{keyspace.FamilyTypeInterface, len(rows.interfaces)},
		{keyspace.FamilyDeclaredType, len(rows.declared)},
		{keyspace.FamilyTypeFunction, len(rows.typeFunctions)},
		{keyspace.FamilyTypeAsserts, len(rows.assertions)},
		{keyspace.FamilyFunction, len(rows.functionContracts)},
		{keyspace.FamilyCall, len(rows.callContracts)},
		{keyspace.FamilyTypePublication, len(rows.publications)},
		{keyspace.FamilyTypeValue, len(rows.typeValues)},
		{keyspace.FamilyAnnotation, len(rows.annotations)},
		{keyspace.FamilyTypeOf, len(rows.typeOf)},
		{keyspace.FamilyTypeKeyOf, len(rows.keyOf)},
		{keyspace.FamilyTypeIndexAccess, len(rows.indexAccess)},
		{keyspace.FamilyTypeConditional, len(rows.conditional)},
	}
	for _, item := range checks {
		if err := check(item.family, item.length); err != nil {
			return err
		}
	}
	if len(rows.claims) > int(counts[keyspace.FamilyValueClaim]) {
		return errors.New("program/lower/collector: too many sparse Claim targets")
	}
	return nil
}

// freeze resolves every raw key through Source and deep-materializes a fresh
// static.Input. No Source preimage, raw payload map, or provisional Key is
// retained by the returned owner input.
func (rows *staticRows) freeze(preimage source.Preimage, counts [keyspace.FamilyCount]uint32) (programstatic.Input, error) {
	if err := staticRowsCountsMatch(rows, counts); err != nil {
		return programstatic.Input{}, err
	}
	if err := validateStaticRowsTerms(rows, counts); err != nil {
		return programstatic.Input{}, err
	}
	if preimage.Identity().TermCount() == 0 {
		return programstatic.Input{}, errors.New("program/lower/collector: Static freeze requires a live Source preimage")
	}
	keys := preimage.Keys()
	input := programstatic.Input{Counts: counts}

	input.Types.Primitive = append([]statictypes.Primitive(nil), rows.primitive...)
	input.Types.Optional = append([]statictypes.Optional(nil), rows.optional...)
	input.Types.Union = make([]statictypes.Union, len(rows.union))
	for index, row := range rows.union {
		input.Types.Union[index] = statictypes.Union{Members: append([]keyspace.Term(nil), row.Members...)}
	}
	input.Types.Intersection = make([]statictypes.Intersection, len(rows.intersection))
	for index, row := range rows.intersection {
		input.Types.Intersection[index] = statictypes.Intersection{Members: append([]keyspace.Term(nil), row.Members...)}
	}
	input.Types.Generic = make([]statictypes.Generic, len(rows.generic))
	for index, row := range rows.generic {
		input.Types.Generic[index] = statictypes.Generic{Base: row.base, Args: append([]keyspace.Term(nil), row.args...)}
	}
	input.Types.Array = append([]statictypes.Array(nil), rows.array...)
	input.Types.Map = append([]statictypes.Map(nil), rows.mapType...)
	input.Types.Record = make([]statictypes.Record, len(rows.record))
	for index, row := range rows.record {
		input.Types.Record[index] = statictypes.Record{Fields: append([]keyspace.Term(nil), row.Fields...), ReadOnly: row.ReadOnly}
	}
	input.Types.Field = make([]statictypes.Field, len(rows.field))
	for index, row := range rows.field {
		key, err := resolveStaticKey(keys, row.key, false)
		if err != nil {
			return programstatic.Input{}, fmt.Errorf("field %d: %w", index, err)
		}
		input.Types.Field[index] = statictypes.Field{Key: key, Type: row.typ, Optional: row.optional}
	}
	input.Types.Literal = make([]statictypes.Literal, len(rows.literal))
	for index, row := range rows.literal {
		literal := statictypes.Literal{Kind: row.kind, FloatBits: row.floatBits}
		if row.kind != keyspace.LiteralFloat {
			key, err := resolveStaticKey(keys, row.exact, false)
			if err != nil {
				return programstatic.Input{}, fmt.Errorf("literal %d: %w", index, err)
			}
			literal.Exact = key
		}
		input.Types.Literal[index] = literal
	}

	input.References.TypeRef = make([]staticrefs.TypeRef, len(rows.references))
	for index, row := range rows.references {
		path, err := resolveStaticPath(keys, row.source)
		if err != nil {
			return programstatic.Input{}, fmt.Errorf("TypeRef %d source: %w", index, err)
		}
		canonical, err := resolveOptionalStaticPath(keys, row.canonical)
		if err != nil {
			return programstatic.Input{}, fmt.Errorf("TypeRef %d canonical: %w", index, err)
		}
		input.References.TypeRef[index] = staticrefs.TypeRef{Resolution: row.resolution, Target: row.target, Root: row.root, Source: path, Canonical: canonical}
	}

	input.Declarations.Alias = make([]staticdecl.TypeAlias, len(rows.aliases))
	for index, row := range rows.aliases {
		name, err := resolveStaticKey(keys, row.name, false)
		if err != nil {
			return programstatic.Input{}, fmt.Errorf("alias %d name: %w", index, err)
		}
		input.Declarations.Alias[index] = staticdecl.TypeAlias{Owner: row.owner, Target: row.target, Name: name, NameCoordinate: row.coordinate, Params: append([]keyspace.Term(nil), row.params...)}
	}
	input.Declarations.TypeParam = make([]staticdecl.TypeParam, len(rows.params))
	for index, row := range rows.params {
		name, err := resolveStaticKey(keys, row.name, false)
		if err != nil {
			return programstatic.Input{}, fmt.Errorf("type parameter %d name: %w", index, err)
		}
		input.Declarations.TypeParam[index] = staticdecl.TypeParam{Owner: row.owner, Name: name, Constraint: row.constraint}
	}
	input.Declarations.Interface = make([]staticdecl.Interface, len(rows.interfaces))
	for index, row := range rows.interfaces {
		name, err := resolveStaticKey(keys, row.name, false)
		if err != nil {
			return programstatic.Input{}, fmt.Errorf("interface %d name: %w", index, err)
		}
		members := make([]staticdecl.InterfaceMember, len(row.membersRaw))
		for memberIndex, member := range row.membersRaw {
			result := staticdecl.InterfaceMember{Kind: member.kind, Field: member.field, NameCoordinate: member.coordinate, Signature: member.signature}
			if member.kind == staticdecl.InterfaceMethod {
				resolved, err := resolveStaticKey(keys, member.name, false)
				if err != nil {
					return programstatic.Input{}, fmt.Errorf("interface %d member %d name: %w", index, memberIndex, err)
				}
				result.Name = resolved
			}
			members[memberIndex] = result
		}
		input.Declarations.Interface[index] = staticdecl.Interface{Owner: row.owner, Name: name, NameCoordinate: row.coordinate, Extends: append([]keyspace.Term(nil), row.extends...), Members: members}
	}
	input.Declarations.DeclaredType = append([]staticdecl.DeclaredType(nil), rows.declared...)

	input.Signatures.TypeFunction = make([]staticsig.TypeFunction, len(rows.typeFunctions))
	for index, row := range rows.typeFunctions {
		parameters := make([]staticsig.Parameter, len(row.parameters))
		for parameterIndex, parameter := range row.parameters {
			name, err := resolveStaticKey(keys, parameter.name, true)
			if err != nil {
				return programstatic.Input{}, fmt.Errorf("TypeFunction %d parameter %d name: %w", index, parameterIndex, err)
			}
			parameters[parameterIndex] = staticsig.Parameter{Name: name, NameCoordinate: parameter.coordinate, Type: parameter.typ}
		}
		input.Signatures.TypeFunction[index] = staticsig.TypeFunction{Scope: row.scope, TypeParams: append([]keyspace.Term(nil), row.typeParams...), Parameters: parameters, Variadic: row.variadic, VariadicCoordinate: row.variadicCoordinate, ReturnsKnown: row.returnsKnown, Returns: append([]keyspace.Term(nil), row.returns...)}
	}
	input.Signatures.TypeAsserts = make([]staticsig.TypeAsserts, len(rows.assertions))
	for index, row := range rows.assertions {
		name, err := resolveStaticKey(keys, row.name, false)
		if err != nil {
			return programstatic.Input{}, fmt.Errorf("TypeAsserts %d name: %w", index, err)
		}
		input.Signatures.TypeAsserts[index] = staticsig.TypeAsserts{Name: name, ParamCoordinate: row.coordinate, Bound: row.bound, Param: row.param, Narrow: row.narrow}
	}

	input.Contracts.Function = make([]staticcontracts.FunctionContract, len(rows.functionContracts))
	for index, row := range rows.functionContracts {
		input.Contracts.Function[index] = staticcontracts.FunctionContract{TypeParams: append([]keyspace.Term(nil), row.typeParams...), ReturnsKnown: row.returnsKnown, Returns: append([]keyspace.Term(nil), row.returns...)}
	}
	input.Contracts.Call = make([]staticcontracts.CallContract, len(rows.callContracts))
	for index, row := range rows.callContracts {
		input.Contracts.Call[index] = staticcontracts.CallContract{TypeArguments: append([]keyspace.Term(nil), row.arguments...)}
	}

	input.Operators.TypeOf = append([]staticoperators.TypeOf(nil), rows.typeOf...)
	input.Operators.KeyOf = append([]staticoperators.KeyOf(nil), rows.keyOf...)
	input.Operators.IndexAccess = append([]staticoperators.IndexAccess(nil), rows.indexAccess...)
	input.Operators.Conditional = append([]staticoperators.Conditional(nil), rows.conditional...)
	input.Publications.Type = append([]staticpubs.Publication(nil), rows.publications...)

	input.Operands.Claim = append([]staticoperands.ClaimTarget(nil), rows.claims...)
	input.Operands.TypeValue = append([]staticoperands.TypeValueTarget(nil), rows.typeValues...)
	input.Operands.Annotation = make([]staticoperands.Annotation, len(rows.annotations))
	for index, row := range rows.annotations {
		name, err := resolveStaticKey(keys, row.name, false)
		if err != nil {
			return programstatic.Input{}, fmt.Errorf("annotation %d name: %w", index, err)
		}
		input.Operands.Annotation[index] = staticoperands.Annotation{Scope: row.scope, Target: row.target, Name: name, Values: row.values}
	}
	if err := validateSparseClaims(input.Operands.Claim, counts[keyspace.FamilyValueClaim]); err != nil {
		return programstatic.Input{}, err
	}
	return input, nil
}

func resolveOptionalStaticPath(keys source.Keys, path []staticRawKey) ([]keyspace.Key, error) {
	if len(path) == 0 {
		return nil, nil
	}
	return resolveStaticPath(keys, path)
}

func validateSparseClaims(rows []staticoperands.ClaimTarget, count uint32) error {
	var previous uint32
	for index, row := range rows {
		if keyspace.TermFamily(row.Claim) != keyspace.FamilyValueClaim || keyspace.TermOrdinal(row.Claim) == 0 || keyspace.TermOrdinal(row.Claim) > count || row.Target == 0 {
			return errors.New("program/lower/collector: invalid sparse Claim target")
		}
		ordinal := keyspace.TermOrdinal(row.Claim)
		if index != 0 && ordinal <= previous {
			return errors.New("program/lower/collector: Claim targets are not canonical")
		}
		previous = ordinal
	}
	return nil
}
