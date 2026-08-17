package static

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"testing"
)

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
		Types: TypesInput{Primitive: []Primitive{
			{Kind: PrimitiveNumber}, {Kind: PrimitiveString}, {Kind: PrimitiveBoolean},
		}},
		Declarations: DeclarationsInput{TypeParam: []TypeParam{{
			Owner: keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1), Name: 7,
		}}},
		Signatures: SignaturesInput{
			TypeFunction: []TypeFunction{{
				Scope:      keyspace.MakeTerm(keyspace.FamilyCell, 1),
				TypeParams: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeParam, 1)},
				Parameters: []Parameter{{
					Name: 9, NameCoordinate: coordinate, Type: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1),
				}},
				Variadic:           keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2),
				VariadicCoordinate: coordinate,
				ReturnsKnown:       true,
				Returns:            []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1)},
			}},
			TypeAsserts: []TypeAsserts{{
				Name: 9, ParamCoordinate: coordinate, Bound: true, Param: 0,
				Narrow: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3),
			}},
		},
	}
}

func TestSignaturesPreserveTypedRelations(t *testing.T) {
	draft, err := Build(signatureFixture(t))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	function := keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1)
	assertion := keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1)
	signatures := component.View().Signatures()
	if scope, variadic, coordinate, known, ok := signatures.TypeFunctions().Get(function); !ok ||
		scope != keyspace.MakeTerm(keyspace.FamilyCell, 1) ||
		variadic != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2) || coordinate == (source.Coordinate{}) || !known {
		t.Fatalf("function header = (%v, %v, %v, %v, %v)", scope, variadic, coordinate, known, ok)
	}
	if count, ok := signatures.TypeFunctions().TypeParamCount(function); !ok || count != 1 {
		t.Fatalf("type parameter count = (%d, %v)", count, ok)
	}
	if param, ok := signatures.TypeFunctions().TypeParamAt(function, 0); !ok || param != keyspace.MakeTerm(keyspace.FamilyTypeParam, 1) {
		t.Fatalf("type parameter = (%v, %v)", param, ok)
	}
	if parameter, ok := signatures.TypeFunctions().ParameterAt(function, 0); !ok || parameter.Name != 9 ||
		parameter.NameCoordinate == (source.Coordinate{}) || parameter.Type != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) {
		t.Fatalf("fixed parameter = (%+v, %v)", parameter, ok)
	}
	if result, ok := signatures.TypeFunctions().ReturnAt(function, 0); !ok || result != assertion {
		t.Fatalf("return = (%v, %v)", result, ok)
	}
	if name, coordinate, bound, ordinal, narrow, ok := signatures.Assertions().Get(assertion); !ok || name != 9 ||
		coordinate == (source.Coordinate{}) || !bound || ordinal != 0 || narrow != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3) {
		t.Fatalf("assertion = (%v, %v, %v, %d, %v, %v)", name, coordinate, bound, ordinal, narrow, ok)
	}
}

func TestSignaturesReturnsAndAssertionEncoding(t *testing.T) {
	input := signatureFixture(t)
	input.Signatures.TypeFunction[0].Parameters = nil
	input.Signatures.TypeFunction[0].Variadic = 0
	input.Signatures.TypeFunction[0].VariadicCoordinate = source.Coordinate{}
	input.Signatures.TypeFunction[0].Returns = nil
	input.Signatures.TypeFunction[0].ReturnsKnown = false
	input.Signatures.TypeAsserts[0].Bound = false
	input.Signatures.TypeAsserts[0].Param = 0
	input.Signatures.TypeAsserts[0].Narrow = 0
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() omitted return/error assertion = %v", err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	function := keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1)
	if _, _, _, known, ok := component.View().Signatures().TypeFunctions().Get(function); !ok || known {
		t.Fatalf("omitted returns = (%v, %v)", known, ok)
	}

	input = signatureFixture(t)
	input.Signatures.TypeFunction[0].Parameters = nil
	input.Signatures.TypeFunction[0].Variadic = 0
	input.Signatures.TypeFunction[0].VariadicCoordinate = source.Coordinate{}
	input.Signatures.TypeFunction[0].Returns = nil
	input.Signatures.TypeFunction[0].ReturnsKnown = true
	input.Signatures.TypeAsserts = nil
	input.Counts[keyspace.FamilyTypeAsserts] = 0
	draft, err = Build(input)
	if err != nil {
		t.Fatalf("Build() explicit empty returns = %v", err)
	}
	component, err = commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	if _, _, _, known, ok := component.View().Signatures().TypeFunctions().Get(function); !ok || !known {
		t.Fatalf("explicit empty returns = (%v, %v)", known, ok)
	}

	input = signatureFixture(t)
	input.Signatures.TypeAsserts[0].Bound = false
	input.Signatures.TypeAsserts[0].Param = 1
	if _, err := Build(input); err == nil {
		t.Fatal("Build() accepted unbound assertion ordinal")
	}
}

func TestSignaturesRejectCoverageOwnershipScopeAndForestDefects(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Input)
	}{
		{"missing signature row", func(input *Input) { input.Signatures.TypeFunction = nil }},
		{"missing assertion row", func(input *Input) { input.Signatures.TypeAsserts = nil }},
		{"anonymous parameter coordinate", func(input *Input) {
			input.Signatures.TypeFunction[0].Parameters[0].Name = 0
		}},
		{"named parameter missing coordinate", func(input *Input) {
			input.Signatures.TypeFunction[0].Parameters[0].NameCoordinate = source.Coordinate{}
		}},
		{"variadic missing coordinate", func(input *Input) {
			input.Signatures.TypeFunction[0].VariadicCoordinate = source.Coordinate{}
		}},
		{"absent variadic coordinate", func(input *Input) {
			input.Signatures.TypeFunction[0].Variadic = 0
		}},
		{"invalid static scope", func(input *Input) {
			input.Counts[keyspace.FamilyBody] = 1
			input.Signatures.TypeFunction[0].Scope = keyspace.MakeTerm(keyspace.FamilyBody, 1)
		}},
		{"orphan type parameter", func(input *Input) {
			input.Signatures.TypeFunction[0].TypeParams = nil
		}},
		{"wrong type parameter owner", func(input *Input) {
			input.Counts[keyspace.FamilyFunction] = 1
			input.Contracts.Function = []FunctionContract{{}}
			input.Declarations.TypeParam[0].Owner = keyspace.MakeTerm(keyspace.FamilyFunction, 1)
		}},
		{"bound assertion wrong name", func(input *Input) {
			input.Signatures.TypeAsserts[0].Name = 10
		}},
		{"bound assertion wrong ordinal", func(input *Input) {
			input.Signatures.TypeAsserts[0].Param = 1
		}},
		{"bound assertion fixed parameter", func(input *Input) {
			input.Signatures.TypeFunction[0].Parameters[0].Type = keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1)
			input.Signatures.TypeFunction[0].Variadic = 0
			input.Signatures.TypeFunction[0].VariadicCoordinate = source.Coordinate{}
			input.Signatures.TypeFunction[0].Returns = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2)}
		}},
		{"bound assertion variadic", func(input *Input) {
			input.Signatures.TypeFunction[0].Variadic = keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1)
			input.Signatures.TypeFunction[0].Returns = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2)}
		}},
		{"bound assertion earlier duplicate name", func(input *Input) {
			coordinate := signatureCoordinate(t)
			input.Signatures.TypeFunction[0].Parameters = append(input.Signatures.TypeFunction[0].Parameters, Parameter{
				Name: 9, NameCoordinate: coordinate, Type: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2),
			})
			input.Signatures.TypeFunction[0].Variadic = 0
			input.Signatures.TypeFunction[0].VariadicCoordinate = source.Coordinate{}
		}},
		{"signature assertion cycle", func(input *Input) {
			input.Signatures.TypeAsserts[0].Narrow = keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1)
		}},
		{"shared signature child", func(input *Input) {
			input.Signatures.TypeFunction[0].Returns = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)}
			input.Counts[keyspace.FamilyTypeAsserts] = 0
			input.Signatures.TypeAsserts = nil
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := signatureFixture(t)
			test.edit(&input)
			if _, err := Build(input); err == nil {
				t.Fatal("Build() accepted invalid signature relation")
			}
		})
	}

	input := declarationFixture(t)
	input.Signatures.TypeFunction[0].Scope = keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)
	if _, err := Build(input); err == nil {
		t.Fatal("Build() accepted interface method with non-interface signature scope")
	}

	input = signatureFixture(t)
	input.Counts[keyspace.FamilyTypeParam] = 2
	input.Declarations.TypeParam = append(input.Declarations.TypeParam, TypeParam{
		Owner: keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1), Name: 11,
	})
	input.Signatures.TypeFunction[0].TypeParams = []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilyTypeParam, 1), keyspace.MakeTerm(keyspace.FamilyTypeParam, 1),
	}
	if _, err := Build(input); err == nil {
		t.Fatal("Build() accepted duplicate function type parameter")
	}
}

func TestSignatureCopyFencesBoundsAndQueriesDoNotAllocate(t *testing.T) {
	input := signatureFixture(t)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	input.Signatures.TypeFunction[0].TypeParams[0] = 0
	input.Signatures.TypeFunction[0].Parameters[0].Name = 99
	input.Signatures.TypeFunction[0].Returns[0] = 0
	input.Signatures.TypeAsserts[0].Name = 99
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	function := keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1)
	assertion := keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1)
	signatures := component.View().Signatures()
	if got, ok := signatures.TypeFunctions().TypeParamAt(function, 0); !ok || got == 0 {
		t.Fatalf("type parameter copy fence = (%v, %v)", got, ok)
	}
	if got, ok := signatures.TypeFunctions().ParameterAt(function, 0); !ok || got.Name != 9 {
		t.Fatalf("parameter copy fence = (%+v, %v)", got, ok)
	}
	if got, ok := signatures.TypeFunctions().ReturnAt(function, 0); !ok || got != assertion {
		t.Fatalf("return copy fence = (%v, %v)", got, ok)
	}
	if name, _, _, _, _, ok := signatures.Assertions().Get(assertion); !ok || name != 9 {
		t.Fatalf("assertion copy fence = (%v, %v)", name, ok)
	}
	if _, ok := signatures.TypeFunctions().ParameterAt(function, -1); ok {
		t.Fatal("ParameterAt accepted negative index")
	}
	if _, ok := signatures.TypeFunctions().ReturnAt(function, 1); ok {
		t.Fatal("ReturnAt accepted out-of-range index")
	}
	if _, _, _, _, ok := signatures.TypeFunctions().Get(keyspace.MakeTerm(keyspace.FamilyTypeFunction, 2)); ok {
		t.Fatal("Functions.Get accepted unknown term")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		signatures.TypeFunctions().Get(function)
		signatures.TypeFunctions().TypeParamAt(function, 0)
		signatures.TypeFunctions().ParameterAt(function, 0)
		signatures.TypeFunctions().ReturnAt(function, 0)
		signatures.Assertions().Get(assertion)
	}); allocations != 0 {
		t.Fatalf("signature queries allocated %.2f times", allocations)
	}
}
