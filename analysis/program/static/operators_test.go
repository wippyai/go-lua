package static

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"testing"
)

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
		Types: TypesInput{Primitive: []Primitive{
			{Kind: PrimitiveNil}, {Kind: PrimitiveBoolean}, {Kind: PrimitiveNumber},
			{Kind: PrimitiveInteger}, {Kind: PrimitiveString}, {Kind: PrimitiveAny},
		}},
		Operators: OperatorsInput{
			TypeOf: []TypeOf{
				{Scope: keyspace.MakeTerm(keyspace.FamilyCell, 1), Operand: keyspace.MakeTerm(keyspace.FamilyRead, 1)},
				{Scope: keyspace.MakeTerm(keyspace.FamilyCell, 1), Operand: keyspace.MakeTerm(keyspace.FamilyRead, 1)},
			},
			KeyOf:       []KeyOf{{Inner: keyspace.MakeTerm(keyspace.FamilyTypeOf, 1)}},
			IndexAccess: []IndexAccess{{Object: primitive(1), Index: primitive(2)}},
			Conditional: []Conditional{{Check: primitive(3), Extends: primitive(4), Then: primitive(5), Else: primitive(6)}},
		},
	}
}

func TestOperatorsPreserveTypedRelationsAndCrossOwnerLeaves(t *testing.T) {
	draft, err := Build(operatorFixture())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	operators := component.View().Operators()
	typeOf := keyspace.MakeTerm(keyspace.FamilyTypeOf, 1)
	if scope, operand, ok := operators.TypeOfs().Get(typeOf); !ok ||
		scope != keyspace.MakeTerm(keyspace.FamilyCell, 1) || operand != keyspace.MakeTerm(keyspace.FamilyRead, 1) {
		t.Fatalf("typeof relation = (%v, %v, %v)", scope, operand, ok)
	}
	// The same Source/Flow operand can occur in several TypeOf rows.  It is
	// not a concrete type child and therefore must not be rejected as shared.
	if _, operand, ok := operators.TypeOfs().Get(keyspace.MakeTerm(keyspace.FamilyTypeOf, 2)); !ok ||
		operand != keyspace.MakeTerm(keyspace.FamilyRead, 1) {
		t.Fatalf("second typeof cross-owner operand = (%v, %v)", operand, ok)
	}
	if inner, ok := operators.KeyOfs().Get(keyspace.MakeTerm(keyspace.FamilyTypeKeyOf, 1)); !ok || inner != typeOf {
		t.Fatalf("keyof typed child = (%v, %v)", inner, ok)
	}
	if object, index, ok := operators.IndexAccesses().Get(keyspace.MakeTerm(keyspace.FamilyTypeIndexAccess, 1)); !ok ||
		object != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) || index != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2) {
		t.Fatalf("index-access relation = (%v, %v, %v)", object, index, ok)
	}
	if check, extends, then, otherwise, ok := operators.Conditionals().Get(keyspace.MakeTerm(keyspace.FamilyTypeConditional, 1)); !ok ||
		check != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3) || extends != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 4) ||
		then != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 5) || otherwise != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 6) {
		t.Fatalf("conditional relation = (%v, %v, %v, %v, %v)", check, extends, then, otherwise, ok)
	}
}

func TestOperatorsRejectCoverageCrossOwnerAndForestDefects(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Input)
	}{
		{"missing typeof row", func(input *Input) { input.Operators.TypeOf = input.Operators.TypeOf[:1] }},
		{"missing keyof row", func(input *Input) { input.Operators.KeyOf = nil }},
		{"missing indexed access row", func(input *Input) { input.Operators.IndexAccess = nil }},
		{"missing conditional row", func(input *Input) { input.Operators.Conditional = nil }},
		{"extra typeof row", func(input *Input) { input.Counts[keyspace.FamilyTypeOf] = 1 }},
		{"invalid typeof scope family", func(input *Input) {
			input.Counts[keyspace.FamilyBody] = 1
			input.Operators.TypeOf[0].Scope = keyspace.MakeTerm(keyspace.FamilyBody, 1)
		}},
		{"foreign typeof scope", func(input *Input) { input.Operators.TypeOf[0].Scope = keyspace.MakeTerm(keyspace.FamilyCell, 2) }},
		{"zero typeof operand", func(input *Input) { input.Operators.TypeOf[0].Operand = 0 }},
		{"foreign typeof operand", func(input *Input) { input.Operators.TypeOf[0].Operand = keyspace.MakeTerm(keyspace.FamilyRead, 2) }},
		{"Import typeof operand", func(input *Input) {
			input.Counts[keyspace.FamilyImport] = 1
			input.Operators.TypeOf[0].Operand = keyspace.MakeTerm(keyspace.FamilyImport, 1)
		}},
		{"nonstatic keyof child", func(input *Input) { input.Operators.KeyOf[0].Inner = keyspace.MakeTerm(keyspace.FamilyRead, 1) }},
		{"same indexed child twice", func(input *Input) {
			input.Operators.IndexAccess[0].Index = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
		}},
		{"shared operator child", func(input *Input) {
			input.Operators.KeyOf[0].Inner = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
		}},
		{"keyof cycle", func(input *Input) { input.Operators.KeyOf[0].Inner = keyspace.MakeTerm(keyspace.FamilyTypeKeyOf, 1) }},
		{"indexed-access cycle", func(input *Input) {
			input.Operators.IndexAccess[0].Object = keyspace.MakeTerm(keyspace.FamilyTypeIndexAccess, 1)
		}},
		{"conditional cycle", func(input *Input) {
			input.Operators.Conditional[0].Check = keyspace.MakeTerm(keyspace.FamilyTypeConditional, 1)
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := operatorFixture()
			test.edit(&input)
			if _, err := Build(input); err == nil {
				t.Fatal("Build() accepted invalid operator relation")
			}
		})
	}

	// TypeOf's operand is an explicit cross-owner Flow value occurrence. Static
	// rejects a Body handle locally, before containment can be assembled.
	input := operatorFixture()
	input.Counts[keyspace.FamilyBody] = 1
	input.Operators.TypeOf[0].Operand = keyspace.MakeTerm(keyspace.FamilyBody, 1)
	if _, err := Build(input); err == nil {
		t.Fatal("Build() accepted non-Flow TypeOf operand")
	}

	// A concrete type edge from the Types vertical and an operator edge form
	// one local forest; neither owner gets a private cycle exception.
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypeOptional] = 1
	counts[keyspace.FamilyTypeKeyOf] = 1
	if _, err := Build(Input{Counts: counts,
		Types:     TypesInput{Optional: []Optional{{Inner: keyspace.MakeTerm(keyspace.FamilyTypeKeyOf, 1)}}},
		Operators: OperatorsInput{KeyOf: []KeyOf{{Inner: keyspace.MakeTerm(keyspace.FamilyTypeOptional, 1)}}},
	}); err == nil {
		t.Fatal("Build() accepted a cross-vertical static cycle")
	}
}

func TestOperatorsCopyFencesBoundsAndQueriesDoNotAllocate(t *testing.T) {
	input := operatorFixture()
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	input.Operators.TypeOf[0].Operand = 0
	input.Operators.KeyOf[0].Inner = 0
	input.Operators.IndexAccess[0].Object = 0
	input.Operators.Conditional[0].Check = 0
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	operators := component.View().Operators()
	typeOf := keyspace.MakeTerm(keyspace.FamilyTypeOf, 1)
	keyOf := keyspace.MakeTerm(keyspace.FamilyTypeKeyOf, 1)
	indexAccess := keyspace.MakeTerm(keyspace.FamilyTypeIndexAccess, 1)
	conditional := keyspace.MakeTerm(keyspace.FamilyTypeConditional, 1)
	if _, operand, ok := operators.TypeOfs().Get(typeOf); !ok || operand == 0 {
		t.Fatalf("typeof copy fence = (%v, %v)", operand, ok)
	}
	if inner, ok := operators.KeyOfs().Get(keyOf); !ok || inner == 0 {
		t.Fatalf("keyof copy fence = (%v, %v)", inner, ok)
	}
	if object, _, ok := operators.IndexAccesses().Get(indexAccess); !ok || object == 0 {
		t.Fatalf("indexed access copy fence = (%v, %v)", object, ok)
	}
	if check, _, _, _, ok := operators.Conditionals().Get(conditional); !ok || check == 0 {
		t.Fatalf("conditional copy fence = (%v, %v)", check, ok)
	}
	if _, ok := operators.KeyOfs().At(-1); ok {
		t.Fatal("KeyOfs.At accepted negative index")
	}
	if _, _, ok := operators.TypeOfs().Get(keyspace.MakeTerm(keyspace.FamilyTypeOf, 3)); ok {
		t.Fatal("TypeOfs.Get accepted out-of-range term")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		operators.TypeOfs().Get(typeOf)
		operators.KeyOfs().Get(keyOf)
		operators.IndexAccesses().Get(indexAccess)
		operators.Conditionals().Get(conditional)
	}); allocations != 0 {
		t.Fatalf("operator queries allocated %.2f times", allocations)
	}
}
