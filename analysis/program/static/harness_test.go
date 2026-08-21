package static

import (
	"testing"

	staticoperands "github.com/wippyai/go-lua/analysis/program/static/operands"

	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"

	staticsig "github.com/wippyai/go-lua/analysis/program/static/signatures"

	staticpubs "github.com/wippyai/go-lua/analysis/program/static/publications"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"

	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"

	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static/operators"
)

func staticFixture(t *testing.T) Input {
	t.Helper()
	input := publicationFixture(t)
	input.Counts[keyspace.FamilyCell] = 1
	input.Counts[keyspace.FamilyRead] = 1
	input.Counts[keyspace.FamilyTypeOf] = 2
	input.Counts[keyspace.FamilyValues] = 1
	input.Counts[keyspace.FamilyValueClaim] = 2
	input.Counts[keyspace.FamilyAnnotation] = 2
	input.Operators.TypeOf = []operators.TypeOf{
		{Scope: keyspace.MakeTerm(keyspace.FamilyCell, 1), Operand: keyspace.MakeTerm(keyspace.FamilyRead, 1)},
		{Scope: keyspace.MakeTerm(keyspace.FamilyCell, 1), Operand: keyspace.MakeTerm(keyspace.FamilyRead, 1)},
	}
	input.Operands.Annotation = []staticoperands.Annotation{
		{Scope: keyspace.MakeTerm(keyspace.FamilyValueClaim, 1), Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1), Name: 1, Values: keyspace.MakeTerm(keyspace.FamilyValues, 1)},
		{Scope: keyspace.MakeTerm(keyspace.FamilyValueClaim, 2), Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1), Name: 2, Values: keyspace.MakeTerm(keyspace.FamilyValues, 1)},
	}
	return input
}

func primitiveComponent(t *testing.T) *Component {
	t.Helper()
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypePrimitive] = 1
	component, _, err := Build(Input{
		Counts: counts,
		Types:  statictypes.Input{Primitive: []statictypes.Primitive{{Kind: statictypes.PrimitiveAny}}},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return component
}

// referenceInput supplies complete declaration rows whenever a TypeRef test
// reserves a declaration family. The static boundary rejects counted-but-
// absent rows; tests that exercise a target must therefore carry its owner.
func referenceInput(counts [keyspace.FamilyCount]uint32, refs staticrefs.Input) Input {
	input := Input{Counts: counts, References: refs}
	if counts[keyspace.FamilyTypeAlias] != 0 {
		input.Counts[keyspace.FamilyBody] = 1
		// Keep the declaration target distinct from any relation under test so
		// the structural forest test does not hide the Reference assertion.
		input.Counts[keyspace.FamilyTypePrimitive] = 2
		input.Types.Primitive = []statictypes.Primitive{{Kind: statictypes.PrimitiveAny}, {Kind: statictypes.PrimitiveNever}}
		coordinate, _ := source.CoordinateFromParts(1, 1, 1, 2)
		params := []keyspace.Term(nil)
		if counts[keyspace.FamilyTypeParam] != 0 {
			params = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeParam, 1)}
			input.Declarations.TypeParam = []staticdecl.TypeParam{{
				Owner: keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1), Name: 1,
			}}
		}
		input.Declarations.Alias = []staticdecl.TypeAlias{{
			Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2),
			Name: 1, NameCoordinate: coordinate, Params: params,
		}}
	}
	if counts[keyspace.FamilyTypeInterface] != 0 {
		input.Counts[keyspace.FamilyBody] = 1
		coordinate, _ := source.CoordinateFromParts(1, 1, 1, 2)
		input.Declarations.Interface = []staticdecl.Interface{{
			Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Name: 2, NameCoordinate: coordinate,
		}}
	}
	return input
}

func staticContentComponent(t *testing.T, input Input) *Component {
	t.Helper()
	component, _, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	return component
}

func contractsFixture(t *testing.T) Input {
	t.Helper()
	coordinate := signatureCoordinate(t)
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypePrimitive] = 4
	counts[keyspace.FamilyTypeParam] = 1
	counts[keyspace.FamilyTypeAsserts] = 1
	counts[keyspace.FamilyFunction] = 1
	counts[keyspace.FamilyCall] = 1
	return Input{
		Counts: counts,
		Types: statictypes.Input{Primitive: []statictypes.Primitive{
			{Kind: statictypes.PrimitiveAny}, {Kind: statictypes.PrimitiveNumber}, {Kind: statictypes.PrimitiveString}, {Kind: statictypes.PrimitiveBoolean},
		}},
		Declarations: staticdecl.Input{TypeParam: []staticdecl.TypeParam{{
			Owner: keyspace.MakeTerm(keyspace.FamilyFunction, 1), Name: 3,
			Constraint: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1),
		}}},
		Signatures: staticsig.Input{TypeAsserts: []staticsig.TypeAsserts{{
			Name: 7, ParamCoordinate: coordinate, Bound: true, Param: 0,
			Narrow: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 4),
		}}},
		Contracts: staticcontracts.Input{
			Function: []staticcontracts.FunctionContract{{
				TypeParams:   []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeParam, 1)},
				ReturnsKnown: true,
				Returns:      []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1)},
			}},
			Call: []staticcontracts.CallContract{{TypeArguments: []keyspace.Term{
				keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2), keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3),
			}}},
		},
	}
}

func declarationFixture(t *testing.T) Input {
	t.Helper()
	coordinate, ok := source.CoordinateFromParts(2, 3, 2, 7)
	if !ok {
		t.Fatal("CoordinateFromParts() rejected fixture")
	}
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyBody] = 2
	counts[keyspace.FamilyTypeAlias] = 1
	counts[keyspace.FamilyTypeParam] = 1
	counts[keyspace.FamilyTypePrimitive] = 3
	counts[keyspace.FamilyTypeRef] = 1
	counts[keyspace.FamilyTypeField] = 1
	counts[keyspace.FamilyTypeFunction] = 1
	counts[keyspace.FamilyTypeInterface] = 1
	return Input{Counts: counts,
		Types: statictypes.Input{
			Primitive: []statictypes.Primitive{{Kind: statictypes.PrimitiveAny}, {Kind: statictypes.PrimitiveNumber}, {Kind: statictypes.PrimitiveString}},
			Field:     []statictypes.Field{{Key: 4, Type: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3)}},
		},
		References: staticrefs.Input{TypeRef: []staticrefs.TypeRef{{
			Resolution: staticrefs.Unresolved, Source: []keyspace.Key{5},
		}}},
		Declarations: staticdecl.Input{
			Alias: []staticdecl.TypeAlias{{
				Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1),
				Name: 1, NameCoordinate: coordinate, Params: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeParam, 1)},
			}},
			TypeParam: []staticdecl.TypeParam{{
				Owner: keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1), Name: 2, Constraint: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2),
			}},
			Interface: []staticdecl.Interface{{
				Owner: keyspace.MakeTerm(keyspace.FamilyBody, 2), Name: 3, NameCoordinate: coordinate,
				Extends: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)},
				Members: []staticdecl.InterfaceMember{
					{Kind: staticdecl.InterfaceField, Field: keyspace.MakeTerm(keyspace.FamilyTypeField, 1)},
					{Kind: staticdecl.InterfaceMethod, Name: 6, NameCoordinate: coordinate, Signature: keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1)},
				},
			}},
		},
		Signatures: staticsig.Input{TypeFunction: []staticsig.TypeFunction{{
			Scope: keyspace.MakeTerm(keyspace.FamilyTypeInterface, 1),
		}}},
	}
}

func operandsFixture(t *testing.T) Input {
	t.Helper()
	coordinate, ok := source.CoordinateFromParts(1, 1, 1, 2)
	if !ok {
		t.Fatal("CoordinateFromParts() rejected fixture")
	}
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyValues] = 1
	counts[keyspace.FamilyValueClaim] = 2
	counts[keyspace.FamilyTypeValue] = 1
	counts[keyspace.FamilyAnnotation] = 2
	counts[keyspace.FamilyTypePrimitive] = 3
	counts[keyspace.FamilyTypeAlias] = 1
	counts[keyspace.FamilyTypeRef] = 1
	return Input{
		Counts: counts,
		Types: statictypes.Input{Primitive: []statictypes.Primitive{
			{Kind: statictypes.PrimitiveNumber}, {Kind: statictypes.PrimitiveString}, {Kind: statictypes.PrimitiveFunction},
		}},
		References: staticrefs.Input{TypeRef: []staticrefs.TypeRef{{
			Resolution: staticrefs.Declaration, Source: []keyspace.Key{1},
			Target: keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1),
		}}},
		Declarations: staticdecl.Input{Alias: []staticdecl.TypeAlias{{
			Owner:  keyspace.MakeTerm(keyspace.FamilyBody, 1),
			Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2),
			Name:   1, NameCoordinate: coordinate,
		}}},
		Operands: staticoperands.Input{
			// Deliberately supplied out of ordinal order: semantic Claims.At is
			// canonical by term, not builder iteration.
			Claim: []staticoperands.ClaimTarget{{
				Claim:  keyspace.MakeTerm(keyspace.FamilyValueClaim, 1),
				Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1),
			}},
			TypeValue: []staticoperands.TypeValueTarget{{Target: keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)}},
			Annotation: []staticoperands.Annotation{
				{Scope: keyspace.MakeTerm(keyspace.FamilyValueClaim, 1), Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1), Name: 2, Values: keyspace.MakeTerm(keyspace.FamilyValues, 1)},
				{Scope: keyspace.MakeTerm(keyspace.FamilyValueClaim, 2), Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1), Name: 3, Values: keyspace.MakeTerm(keyspace.FamilyValues, 1)},
			},
		},
	}
}

func operatorFixture() Input {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyCell] = 1
	counts[keyspace.FamilyRead] = 1
	counts[keyspace.FamilyTypePrimitive] = 6
	counts[keyspace.FamilyTypeOf] = 2
	counts[keyspace.FamilyTypeKeyOf] = 1
	counts[keyspace.FamilyTypeIndexAccess] = 1
	counts[keyspace.FamilyTypeConditional] = 1
	primitive := func(ordinal uint32) keyspace.Term {
		return keyspace.MakeTerm(keyspace.FamilyTypePrimitive, ordinal)
	}
	return Input{Counts: counts,
		Types: statictypes.Input{Primitive: []statictypes.Primitive{
			{Kind: statictypes.PrimitiveNil}, {Kind: statictypes.PrimitiveBoolean}, {Kind: statictypes.PrimitiveNumber},
			{Kind: statictypes.PrimitiveInteger}, {Kind: statictypes.PrimitiveString}, {Kind: statictypes.PrimitiveAny},
		}},
		Operators: operators.Input{
			TypeOf: []operators.TypeOf{
				{Scope: keyspace.MakeTerm(keyspace.FamilyCell, 1), Operand: keyspace.MakeTerm(keyspace.FamilyRead, 1)},
				{Scope: keyspace.MakeTerm(keyspace.FamilyCell, 1), Operand: keyspace.MakeTerm(keyspace.FamilyRead, 1)},
			},
			KeyOf:       []operators.KeyOf{{Inner: keyspace.MakeTerm(keyspace.FamilyTypeOf, 1)}},
			IndexAccess: []operators.IndexAccess{{Object: primitive(1), Index: primitive(2)}},
			Conditional: []operators.Conditional{{Check: primitive(3), Extends: primitive(4), Then: primitive(5), Else: primitive(6)}},
		},
	}
}

func publicationFixture(t *testing.T) Input {
	t.Helper()
	coordinate, ok := source.CoordinateFromParts(1, 1, 1, 2)
	if !ok {
		t.Fatal("CoordinateFromParts() rejected fixture")
	}
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyAssign] = 1
	counts[keyspace.FamilyTypePrimitive] = 1
	counts[keyspace.FamilyTypeAlias] = 1
	counts[keyspace.FamilyTypeRef] = 2
	counts[keyspace.FamilyTypePublication] = 1
	return Input{
		Counts: counts,
		Types:  statictypes.Input{Primitive: []statictypes.Primitive{{Kind: statictypes.PrimitiveNumber}}},
		References: staticrefs.Input{TypeRef: []staticrefs.TypeRef{
			{Resolution: staticrefs.Declaration, Target: keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1), Source: []keyspace.Key{1}},
			{Resolution: staticrefs.CanonicalPath, Source: []keyspace.Key{2}, Canonical: []keyspace.Key{9}},
		}},
		Declarations: staticdecl.Input{Alias: []staticdecl.TypeAlias{{
			Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1),
			Name: 1, NameCoordinate: coordinate,
		}}},
		Publications: staticpubs.Input{Type: []staticpubs.Publication{{
			Assign: keyspace.MakeTerm(keyspace.FamilyAssign, 1), Pair: 0, Target: keyspace.MakeTerm(keyspace.FamilyTypeRef, 1),
		}}},
	}
}

func declaredTypeFixture(t *testing.T) Input {
	t.Helper()
	input := declarationFixture(t)
	input.Counts[keyspace.FamilyCell] = 2
	input.Counts[keyspace.FamilyDeclaredType] = 1
	input.Counts[keyspace.FamilyTypePrimitive] = 4
	input.Types.Primitive = append(input.Types.Primitive, statictypes.Primitive{Kind: statictypes.PrimitiveBoolean})
	input.Declarations.DeclaredType = []staticdecl.DeclaredType{{
		Cell:   keyspace.MakeTerm(keyspace.FamilyCell, 1),
		Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 4),
	}}
	return input
}

func signatureCoordinate(t *testing.T) source.Coordinate {
	t.Helper()
	value, ok := source.CoordinateFromParts(3, 4, 3, 8)
	if !ok {
		t.Fatal("CoordinateFromParts() rejected fixture")
	}
	return value
}

func signatureFixture(t *testing.T) Input {
	t.Helper()
	coordinate := signatureCoordinate(t)
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyCell] = 1
	counts[keyspace.FamilyTypePrimitive] = 3
	counts[keyspace.FamilyTypeFunction] = 1
	counts[keyspace.FamilyTypeAsserts] = 1
	counts[keyspace.FamilyTypeParam] = 1
	return Input{Counts: counts,
		Types: statictypes.Input{Primitive: []statictypes.Primitive{
			{Kind: statictypes.PrimitiveNumber}, {Kind: statictypes.PrimitiveString}, {Kind: statictypes.PrimitiveBoolean},
		}},
		Declarations: staticdecl.Input{TypeParam: []staticdecl.TypeParam{{
			Owner: keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1), Name: 7,
		}}},
		Signatures: staticsig.Input{
			TypeFunction: []staticsig.TypeFunction{{
				Scope:      keyspace.MakeTerm(keyspace.FamilyCell, 1),
				TypeParams: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeParam, 1)},
				Parameters: []staticsig.Parameter{{
					Name: 9, NameCoordinate: coordinate, Type: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1),
				}},
				Variadic:           keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2),
				VariadicCoordinate: coordinate,
				ReturnsKnown:       true,
				Returns:            []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1)},
			}},
			TypeAsserts: []staticsig.TypeAsserts{{
				Name: 9, ParamCoordinate: coordinate, Bound: true, Param: 0,
				Narrow: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3),
			}},
		},
	}
}

func staticTypeDenominatorInput(t *testing.T) Input {
	t.Helper()
	coordinate, ok := source.CoordinateFromParts(1, 1, 1, 2)
	if !ok {
		t.Fatal("CoordinateFromParts rejected static denominator fixture")
	}
	counts := [keyspace.FamilyCount]uint32{}
	for _, family := range []keyspace.Family{
		keyspace.FamilyTypeAlias,
		keyspace.FamilyTypeInterface,
		keyspace.FamilyTypeParam,
		keyspace.FamilyTypeLiteral,
		keyspace.FamilyTypeOptional,
		keyspace.FamilyTypeUnion,
		keyspace.FamilyTypeIntersection,
		keyspace.FamilyTypeRef,
		keyspace.FamilyTypeGeneric,
		keyspace.FamilyTypeArray,
		keyspace.FamilyTypeMap,
		keyspace.FamilyTypeRecord,
		keyspace.FamilyTypeField,
		keyspace.FamilyTypeFunction,
		keyspace.FamilyTypeAsserts,
		keyspace.FamilyTypeOf,
		keyspace.FamilyTypeKeyOf,
		keyspace.FamilyTypeIndexAccess,
		keyspace.FamilyTypeConditional,
	} {
		counts[family] = 1
	}
	counts[keyspace.FamilyTypePrimitive] = 20
	counts[keyspace.FamilyBody] = 2
	counts[keyspace.FamilyCell] = 1
	counts[keyspace.FamilyRead] = 1

	term := func(family keyspace.Family, ordinal uint32) keyspace.Term {
		return keyspace.MakeTerm(family, ordinal)
	}
	primitive := func(ordinal uint32) keyspace.Term {
		return term(keyspace.FamilyTypePrimitive, ordinal)
	}
	primitives := make([]statictypes.Primitive, 20)
	for index := range primitives {
		primitives[index] = statictypes.Primitive{Kind: statictypes.PrimitiveAny}
	}
	return Input{
		Counts: counts,
		Types: statictypes.Input{
			Primitive:    primitives,
			Literal:      []statictypes.Literal{{Kind: keyspace.LiteralString, Exact: 1}},
			Optional:     []statictypes.Optional{{Inner: primitive(1)}},
			Union:        []statictypes.Union{{Members: []keyspace.Term{primitive(2), primitive(3)}}},
			Intersection: []statictypes.Intersection{{Members: []keyspace.Term{primitive(4), primitive(5)}}},
			Generic: []statictypes.Generic{{
				Base: term(keyspace.FamilyTypeRef, 1), Args: []keyspace.Term{primitive(6)},
			}},
			Array:  []statictypes.Array{{Element: primitive(7)}},
			Map:    []statictypes.Map{{Key: primitive(8), Value: primitive(9)}},
			Field:  []statictypes.Field{{Key: 2, Type: primitive(10)}},
			Record: []statictypes.Record{{Fields: []keyspace.Term{term(keyspace.FamilyTypeField, 1)}}},
		},
		References: staticrefs.Input{TypeRef: []staticrefs.TypeRef{{
			Resolution: staticrefs.Unresolved, Source: []keyspace.Key{3},
		}}},
		Declarations: staticdecl.Input{
			Alias: []staticdecl.TypeAlias{{
				Owner: term(keyspace.FamilyBody, 1), Target: primitive(11), Name: 4,
				NameCoordinate: coordinate, Params: []keyspace.Term{term(keyspace.FamilyTypeParam, 1)},
			}},
			TypeParam: []staticdecl.TypeParam{{
				Owner: term(keyspace.FamilyTypeAlias, 1), Name: 5, Constraint: primitive(12),
			}},
			Interface: []staticdecl.Interface{{
				Owner: term(keyspace.FamilyBody, 2), Name: 6, NameCoordinate: coordinate,
			}},
		},
		Signatures: staticsig.Input{
			TypeFunction: []staticsig.TypeFunction{{
				Scope: term(keyspace.FamilyCell, 1), ReturnsKnown: true,
			}},
			TypeAsserts: []staticsig.TypeAsserts{{
				Name: 7, ParamCoordinate: coordinate, Narrow: primitive(13),
			}},
		},
		Operators: operators.Input{
			TypeOf: []operators.TypeOf{{
				Scope: term(keyspace.FamilyCell, 1), Operand: term(keyspace.FamilyRead, 1),
			}},
			KeyOf:       []operators.KeyOf{{Inner: primitive(14)}},
			IndexAccess: []operators.IndexAccess{{Object: primitive(15), Index: primitive(16)}},
			Conditional: []operators.Conditional{{
				Check: primitive(17), Extends: primitive(18), Then: primitive(19), Else: primitive(20),
			}},
		},
	}
}
