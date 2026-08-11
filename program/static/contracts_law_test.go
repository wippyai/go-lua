package static

import (
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

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
		Types: TypesInput{Primitive: []Primitive{
			{Kind: PrimitiveAny}, {Kind: PrimitiveNumber}, {Kind: PrimitiveString}, {Kind: PrimitiveBoolean},
		}},
		Declarations: DeclarationsInput{TypeParam: []TypeParam{{
			Owner: keyspace.MakeTerm(keyspace.FamilyFunction, 1), Name: 3,
			Constraint: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1),
		}}},
		Signatures: SignaturesInput{TypeAsserts: []TypeAsserts{{
			Name: 7, ParamCoordinate: coordinate, Bound: true, Param: 0,
			Narrow: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 4),
		}}},
		Contracts: ContractsInput{
			Function: []FunctionContract{{
				TypeParams:   []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeParam, 1)},
				ReturnsKnown: true,
				Returns:      []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1)},
			}},
			Call: []CallContract{{TypeArguments: []keyspace.Term{
				keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2), keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3),
			}}},
		},
	}
}

func TestContractsPreserveDenseTypedSidecars(t *testing.T) {
	draft, err := Build(contractsFixture(t))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	contracts := component.View().Contracts()
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	if contracts.Functions().Count() != 1 || contracts.Calls().Count() != 1 {
		t.Fatalf("dense counts = (%d, %d)", contracts.Functions().Count(), contracts.Calls().Count())
	}
	if known, ok := contracts.Functions().Get(function); !ok || !known {
		t.Fatalf("function header = (%v, %v)", known, ok)
	}
	if count, ok := contracts.Functions().TypeParamCount(function); !ok || count != 1 {
		t.Fatalf("function type parameter count = (%d, %v)", count, ok)
	}
	if got, ok := contracts.Functions().TypeParamAt(function, 0); !ok || got != keyspace.MakeTerm(keyspace.FamilyTypeParam, 1) {
		t.Fatalf("function type parameter = (%v, %v)", got, ok)
	}
	if count, ok := contracts.Functions().ReturnCount(function); !ok || count != 1 {
		t.Fatalf("function return count = (%d, %v)", count, ok)
	}
	if got, ok := contracts.Functions().ReturnAt(function, 0); !ok || got != keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1) {
		t.Fatalf("function return = (%v, %v)", got, ok)
	}
	if count, ok := contracts.Calls().TypeArgumentCount(call); !ok || count != 2 {
		t.Fatalf("call argument count = (%d, %v)", count, ok)
	}
	if got, ok := contracts.Calls().TypeArgumentAt(call, 1); !ok || got != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3) {
		t.Fatalf("call argument = (%v, %v)", got, ok)
	}
}

func TestContractsPreserveOmittedAndKnownEmptyReturns(t *testing.T) {
	for _, known := range []bool{false, true} {
		input := contractsFixture(t)
		input.Contracts.Function[0].ReturnsKnown = known
		input.Contracts.Function[0].Returns = nil
		input.Contracts.Call[0].TypeArguments = nil
		input.Signatures.TypeAsserts[0].Bound = false
		input.Signatures.TypeAsserts[0].Narrow = 0
		draft, err := Build(input)
		if err != nil {
			t.Fatalf("Build(known=%v) error = %v", known, err)
		}
		component, err := commitStaticDraft(t, draft)
		if err != nil {
			t.Fatalf("take() error = %v", err)
		}
		got, ok := component.View().Contracts().Functions().Get(keyspace.MakeTerm(keyspace.FamilyFunction, 1))
		if !ok || got != known {
			t.Fatalf("ReturnsKnown = (%v, %v), want (%v, true)", got, ok, known)
		}
		if count, ok := component.View().Contracts().Calls().TypeArgumentCount(keyspace.MakeTerm(keyspace.FamilyCall, 1)); !ok || count != 0 {
			t.Fatalf("empty call type arguments = (%d, %v)", count, ok)
		}
	}
}

func TestContractsRejectCoverageOwnershipAndForestDefects(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Input)
	}{
		{"missing function contract", func(input *Input) { input.Contracts.Function = nil }},
		{"extra call contract", func(input *Input) { input.Contracts.Call = append(input.Contracts.Call, CallContract{}) }},
		{"omitted returns with child", func(input *Input) { input.Contracts.Function[0].ReturnsKnown = false }},
		{"orphan function type parameter", func(input *Input) { input.Contracts.Function[0].TypeParams = nil }},
		{"duplicate function type parameter", func(input *Input) {
			input.Counts[keyspace.FamilyTypeParam] = 2
			input.Declarations.TypeParam = append(input.Declarations.TypeParam, TypeParam{Owner: keyspace.MakeTerm(keyspace.FamilyFunction, 1), Name: 4})
			input.Contracts.Function[0].TypeParams = []keyspace.Term{
				keyspace.MakeTerm(keyspace.FamilyTypeParam, 1), keyspace.MakeTerm(keyspace.FamilyTypeParam, 1),
			}
		}},
		{"wrong function type parameter owner", func(input *Input) {
			input.Declarations.TypeParam[0].Owner = keyspace.MakeTerm(keyspace.FamilyFunction, 0)
		}},
		{"shared return type argument child", func(input *Input) {
			input.Contracts.Call[0].TypeArguments[0] = keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1)
		}},
		{"bound assertion as call argument", func(input *Input) {
			input.Contracts.Function[0].Returns = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2)}
			input.Contracts.Call[0].TypeArguments[0] = keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1)
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Build(func() Input { input := contractsFixture(t); test.edit(&input); return input }()); err == nil {
				t.Fatal("Build() accepted invalid contract relation")
			}
		})
	}
}

func TestContractsRejectNestedAssertionAndCrossOwnerDuplicate(t *testing.T) {
	input := contractsFixture(t)
	input.Counts[keyspace.FamilyTypeGeneric] = 1
	input.Types.Generic = []Generic{{
		Base: keyspace.MakeTerm(keyspace.FamilyTypeRef, 1), Args: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeAsserts, 1)},
	}}
	input.Counts[keyspace.FamilyTypeRef] = 1
	input.References.TypeRef = []TypeRef{{Resolution: TypeRefUnresolved, Source: []keyspace.Key{1}}}
	input.Contracts.Function[0].Returns = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeGeneric, 1)}
	if _, err := Build(input); err == nil {
		t.Fatal("Build() accepted bound assertion nested in a function return")
	}

	input = contractsFixture(t)
	input.Counts[keyspace.FamilyTypeAlias] = 1
	input.Counts[keyspace.FamilyBody] = 1
	input.Declarations.Alias = []TypeAlias{{
		Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2),
		Name: 11, NameCoordinate: signatureCoordinate(t), Params: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeParam, 1)},
	}}
	if _, err := Build(input); err == nil {
		t.Fatal("Build() accepted a type parameter claimed by Alias and Function")
	}

	input = contractsFixture(t)
	input.Counts[keyspace.FamilyCell] = 1
	input.Counts[keyspace.FamilyTypeAsserts] = 0
	input.Signatures.TypeAsserts = nil
	input.Counts[keyspace.FamilyTypeFunction] = 1
	input.Counts[keyspace.FamilyTypeOptional] = 1
	input.Types.Optional = []Optional{{Inner: keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1)}}
	input.Signatures.TypeFunction = []TypeFunction{{
		Scope:        keyspace.MakeTerm(keyspace.FamilyCell, 1),
		ReturnsKnown: true, Returns: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeOptional, 1)},
	}}
	input.Contracts.Function[0].Returns = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2)}
	input.Contracts.Call[0].TypeArguments = nil
	if _, err := Build(input); err == nil {
		t.Fatal("Build() accepted a cycle through an existing TypeFunction")
	}

	input = contractsFixture(t)
	input.Counts[keyspace.FamilyTypeKeyOf] = 1
	input.Operators.KeyOf = []KeyOf{{Inner: keyspace.MakeTerm(keyspace.FamilyTypeKeyOf, 1)}}
	input.Contracts.Call[0].TypeArguments = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeKeyOf, 1)}
	if _, err := Build(input); err == nil {
		t.Fatal("Build() accepted a cycle through an existing static operator")
	}
}

func TestContractsCopyFencesBoundsAndQueriesDoNotAllocate(t *testing.T) {
	input := contractsFixture(t)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	input.Contracts.Function[0].TypeParams[0] = 0
	input.Contracts.Function[0].Returns[0] = 0
	input.Contracts.Call[0].TypeArguments[0] = 0
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	contracts := component.View().Contracts()
	function := keyspace.MakeTerm(keyspace.FamilyFunction, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	if got, ok := contracts.Functions().TypeParamAt(function, 0); !ok || got == 0 {
		t.Fatalf("type parameter copy fence = (%v, %v)", got, ok)
	}
	if got, ok := contracts.Functions().ReturnAt(function, 0); !ok || got == 0 {
		t.Fatalf("return copy fence = (%v, %v)", got, ok)
	}
	if got, ok := contracts.Calls().TypeArgumentAt(call, 0); !ok || got == 0 {
		t.Fatalf("type argument copy fence = (%v, %v)", got, ok)
	}
	if _, ok := contracts.Functions().ReturnAt(function, -1); ok {
		t.Fatal("ReturnAt accepted negative index")
	}
	if _, ok := contracts.Calls().TypeArgumentAt(call, 2); ok {
		t.Fatal("TypeArgumentAt accepted out-of-range index")
	}
	if _, ok := contracts.Functions().Get(keyspace.MakeTerm(keyspace.FamilyFunction, 2)); ok {
		t.Fatal("Functions.Get accepted unknown term")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		contracts.Functions().Get(function)
		contracts.Functions().TypeParamAt(function, 0)
		contracts.Functions().ReturnAt(function, 0)
		contracts.Calls().TypeArgumentAt(call, 0)
	}); allocations != 0 {
		t.Fatalf("contract queries allocated %.2f times", allocations)
	}
}
