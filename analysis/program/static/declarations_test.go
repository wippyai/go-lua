package static

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"testing"
)

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
		Types: TypesInput{
			Primitive: []Primitive{{Kind: PrimitiveAny}, {Kind: PrimitiveNumber}, {Kind: PrimitiveString}},
			Field:     []Field{{Key: 4, Type: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3)}},
		},
		References: ReferencesInput{TypeRef: []TypeRef{{
			Resolution: TypeRefUnresolved, Source: []keyspace.Key{5},
		}}},
		Declarations: DeclarationsInput{
			Alias: []TypeAlias{{
				Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1),
				Name: 1, NameCoordinate: coordinate, Params: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeParam, 1)},
			}},
			TypeParam: []TypeParam{{
				Owner: keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1), Name: 2, Constraint: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2),
			}},
			Interface: []Interface{{
				Owner: keyspace.MakeTerm(keyspace.FamilyBody, 2), Name: 3, NameCoordinate: coordinate,
				Extends: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)},
				Members: []InterfaceMember{
					{Kind: InterfaceField, Field: keyspace.MakeTerm(keyspace.FamilyTypeField, 1)},
					{Kind: InterfaceMethod, Name: 6, NameCoordinate: coordinate, Signature: keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1)},
				},
			}},
		},
		Signatures: SignaturesInput{TypeFunction: []TypeFunction{{
			Scope: keyspace.MakeTerm(keyspace.FamilyTypeInterface, 1),
		}}},
	}
}

func TestDeclarationsPreserveTypedOwnershipAndOrder(t *testing.T) {
	draft, err := Build(declarationFixture(t))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	declarations := component.View().Declarations()
	alias := keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)
	if owner, target, name, coordinate, ok := declarations.Aliases().Get(alias); !ok ||
		owner != keyspace.MakeTerm(keyspace.FamilyBody, 1) || target != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) ||
		name != 1 || coordinate == (source.Coordinate{}) {
		t.Fatalf("alias relation = (%v, %v, %v, %v, %v)", owner, target, name, coordinate, ok)
	}
	param := keyspace.MakeTerm(keyspace.FamilyTypeParam, 1)
	if count, ok := declarations.Aliases().ParamCount(alias); !ok || count != 1 {
		t.Fatalf("alias parameter count = (%d, %v)", count, ok)
	}
	if got, ok := declarations.Aliases().ParamAt(alias, 0); !ok || got != param {
		t.Fatalf("alias parameter = (%v, %v)", got, ok)
	}
	if owner, name, constraint, ok := declarations.TypeParams().Get(param); !ok || owner != alias || name != 2 ||
		constraint != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2) {
		t.Fatalf("type parameter = (%v, %v, %v, %v)", owner, name, constraint, ok)
	}
	iface := keyspace.MakeTerm(keyspace.FamilyTypeInterface, 1)
	if count, ok := declarations.Interfaces().ExtendCount(iface); !ok || count != 1 {
		t.Fatalf("interface extends = (%d, %v)", count, ok)
	}
	if member, ok := declarations.Interfaces().MemberAt(iface, 1); !ok || member.Kind != InterfaceMethod ||
		member.Field != 0 || member.Name != 6 || member.Signature != keyspace.MakeTerm(keyspace.FamilyTypeFunction, 1) {
		t.Fatalf("method member = (%+v, %v)", member, ok)
	}
}

func TestDeclarationsRejectTotalityXORCoordinatesAndForestDefects(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Input)
	}{
		{"missing alias parameter", func(input *Input) { input.Declarations.Alias[0].Params = nil }},
		{"orphan field", func(input *Input) {
			input.Declarations.Interface[0].Members = input.Declarations.Interface[0].Members[1:]
		}},
		{"field method xor", func(input *Input) { input.Declarations.Interface[0].Members[0].Name = 9 }},
		{"method missing coordinate", func(input *Input) { input.Declarations.Interface[0].Members[1].NameCoordinate = source.Coordinate{} }},
		{"alias absent coordinate", func(input *Input) { input.Declarations.Alias[0].NameCoordinate = source.Coordinate{} }},
		{"interface non-reference extends", func(input *Input) {
			input.Declarations.Interface[0].Extends[0] = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
		}},
		{"shared concrete child", func(input *Input) {
			input.Declarations.TypeParam[0].Constraint = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := declarationFixture(t)
			test.edit(&input)
			if _, err := Build(input); err == nil {
				t.Fatal("Build() accepted invalid declaration relation")
			}
		})
	}
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypeOptional] = 1
	if _, err := Build(Input{Counts: counts, Types: TypesInput{
		Optional: []Optional{{Inner: keyspace.MakeTerm(keyspace.FamilyTypeOptional, 1)}},
	}}); err == nil {
		t.Fatal("Build() accepted cyclic static type forest")
	}

	// Record membership is not a generic type-child edge, but it still closes
	// the concrete containment walk. A field cannot point back to its record.
	input := declarationFixture(t)
	input.Counts[keyspace.FamilyTypeRecord] = 1
	input.Types.Record = []Record{{Fields: []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilyTypeField, 1),
	}}}
	input.Types.Field[0].Type = keyspace.MakeTerm(keyspace.FamilyTypeRecord, 1)
	input.Declarations.Interface[0].Members = input.Declarations.Interface[0].Members[1:]
	if _, err := Build(input); err == nil {
		t.Fatal("Build() accepted a field type cycle through its record owner")
	}
}

func TestDeclarationsCopyFencesBoundsAndQueriesDoNotAllocate(t *testing.T) {
	input := declarationFixture(t)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	input.Declarations.Alias[0].Params[0] = 0
	input.Declarations.Interface[0].Extends[0] = 0
	input.Declarations.Interface[0].Members[1].Name = 99
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	declarations := component.View().Declarations()
	alias := keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)
	iface := keyspace.MakeTerm(keyspace.FamilyTypeInterface, 1)
	if got, ok := declarations.Aliases().ParamAt(alias, 0); !ok || got == 0 {
		t.Fatalf("alias copy fence = (%v, %v)", got, ok)
	}
	if got, ok := declarations.Interfaces().ExtendAt(iface, 0); !ok || got == 0 {
		t.Fatalf("interface extension copy fence = (%v, %v)", got, ok)
	}
	if got, ok := declarations.Interfaces().MemberAt(iface, 1); !ok || got.Name != 6 {
		t.Fatalf("interface member copy fence = (%+v, %v)", got, ok)
	}
	if _, ok := declarations.Aliases().ParamAt(alias, -1); ok {
		t.Fatal("ParamAt accepted negative index")
	}
	if _, ok := declarations.Interfaces().MemberAt(iface, 2); ok {
		t.Fatal("MemberAt accepted out-of-range index")
	}
	if _, _, _, _, ok := declarations.Aliases().Get(keyspace.MakeTerm(keyspace.FamilyTypeAlias, 2)); ok {
		t.Fatal("Aliases.Get accepted unknown term")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		declarations.Aliases().Get(alias)
		declarations.Aliases().ParamCount(alias)
		declarations.Aliases().ParamAt(alias, 0)
		declarations.TypeParams().Get(keyspace.MakeTerm(keyspace.FamilyTypeParam, 1))
		declarations.Interfaces().Get(iface)
		declarations.Interfaces().ExtendAt(iface, 0)
		declarations.Interfaces().MemberAt(iface, 1)
	}); allocations != 0 {
		t.Fatalf("declaration queries allocated %.2f times", allocations)
	}
}
