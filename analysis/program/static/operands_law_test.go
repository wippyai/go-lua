package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

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
		Types: TypesInput{Primitive: []Primitive{
			{Kind: PrimitiveNumber}, {Kind: PrimitiveString}, {Kind: PrimitiveFunction},
		}},
		References: ReferencesInput{TypeRef: []TypeRef{{
			Resolution: TypeRefDeclaration, Source: []keyspace.Key{1},
			Target: keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1),
		}}},
		Declarations: DeclarationsInput{Alias: []TypeAlias{{
			Owner:  keyspace.MakeTerm(keyspace.FamilyBody, 1),
			Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2),
			Name:   1, NameCoordinate: coordinate,
		}}},
		Operands: OperandsInput{
			// Deliberately supplied out of ordinal order: semantic Claims.At is
			// canonical by term, not builder iteration.
			Claim: []ClaimTarget{{
				Claim:  keyspace.MakeTerm(keyspace.FamilyValueClaim, 1),
				Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1),
			}},
			TypeValue: []TypeValueTarget{{Target: keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)}},
			Annotation: []Annotation{
				{Scope: keyspace.MakeTerm(keyspace.FamilyValueClaim, 1), Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1), Name: 2, Values: keyspace.MakeTerm(keyspace.FamilyValues, 1)},
				{Scope: keyspace.MakeTerm(keyspace.FamilyValueClaim, 2), Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1), Name: 3, Values: keyspace.MakeTerm(keyspace.FamilyValues, 1)},
			},
		},
	}
}

func TestOperandsPreserveExactSparseAndDenseRelations(t *testing.T) {
	draft, err := Build(operandsFixture(t))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	operands := component.View().Operands()
	claims := operands.Claims()
	claim1 := keyspace.MakeTerm(keyspace.FamilyValueClaim, 1)
	claim2 := keyspace.MakeTerm(keyspace.FamilyValueClaim, 2)
	if claims.Count() != 1 {
		t.Fatalf("semantic ClaimTarget count = %d, want 1 (not all 2 ValueClaims)", claims.Count())
	}
	if got, ok := claims.At(0); !ok || got != claim1 {
		t.Fatalf("canonical claim target term = %v/%v, want claim 1", got, ok)
	}
	if target, ok := claims.Target(claim1); !ok || target != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) {
		t.Fatalf("claim target = %v/%v", target, ok)
	}
	if target, ok := claims.Target(claim2); ok || target != 0 {
		t.Fatalf("missing sparse target = %v/%v, want zero/false", target, ok)
	}

	typeValues := operands.TypeValues()
	typeValue := keyspace.MakeTerm(keyspace.FamilyTypeValue, 1)
	if typeValues.Count() != 1 {
		t.Fatalf("TypeValue target count = %d, want 1", typeValues.Count())
	}
	if target, ok := typeValues.Target(typeValue); !ok || target != keyspace.MakeTerm(keyspace.FamilyTypeRef, 1) {
		t.Fatalf("TypeValue target = %v/%v", target, ok)
	}

	annotations := operands.Annotations()
	primitive := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	if count, ok := annotations.ForCount(primitive); !ok || count != 2 {
		t.Fatalf("annotation CSR count = %d/%v, want 2", count, ok)
	}
	for index := range 2 {
		term, ok := annotations.ForAt(primitive, index)
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyAnnotation, uint32(index+1)) {
			t.Fatalf("annotation CSR term[%d] = %v/%v", index, term, ok)
		}
	}
	if count, ok := annotations.ForCount(keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2)); !ok || count != 0 {
		t.Fatalf("valid unannotated target count = %d/%v, want 0/true", count, ok)
	}
	if _, ok := annotations.ForCount(keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)); ok {
		t.Fatal("annotation query accepted non-static anchor")
	}
}

func TestOperandsRejectInvalidTargetsAndDenominators(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Input)
	}{
		{"duplicate claim", func(input *Input) {
			input.Operands.Claim = append(input.Operands.Claim, input.Operands.Claim[0])
		}},
		{"invalid claim family", func(input *Input) {
			input.Operands.Claim[0].Claim = keyspace.MakeTerm(keyspace.FamilyTypeValue, 1)
		}},
		{"invalid claim static target", func(input *Input) {
			input.Operands.Claim[0].Target = keyspace.MakeTerm(keyspace.FamilyValues, 1)
		}},
		{"type value static-only primitive", func(input *Input) {
			input.Operands.TypeValue[0].Target = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 3)
		}},
		{"type value unresolved reference", func(input *Input) {
			input.References.TypeRef[0].Resolution = TypeRefUnresolved
			input.References.TypeRef[0].Target = 0
		}},
		{"type value wrong dense count", func(input *Input) {
			input.Operands.TypeValue = nil
		}},
		{"annotation wrong dense count", func(input *Input) {
			input.Operands.Annotation = input.Operands.Annotation[:1]
		}},
		{"annotation invalid values", func(input *Input) {
			input.Operands.Annotation[0].Values = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
		}},
		{"annotation invalid anchor", func(input *Input) {
			input.Operands.Annotation[0].Target = keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := operandsFixture(t)
			test.edit(&input)
			if _, err := Build(input); err == nil {
				t.Fatal("Build() accepted invalid operand relation")
			}
		})
	}
}

func TestOperandsCanonicalizeSparseClaimOrder(t *testing.T) {
	input := operandsFixture(t)
	input.Counts[keyspace.FamilyValueClaim] = 3
	input.Counts[keyspace.FamilyTypePrimitive] = 4
	input.Types.Primitive = append(input.Types.Primitive, Primitive{Kind: PrimitiveBoolean})
	claim1 := keyspace.MakeTerm(keyspace.FamilyValueClaim, 1)
	claim3 := keyspace.MakeTerm(keyspace.FamilyValueClaim, 3)
	input.Operands.Claim = []ClaimTarget{
		{Claim: claim3, Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 4)},
		{Claim: claim1, Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)},
	}
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	claims := component.View().Operands().Claims()
	for index, want := range []keyspace.Term{claim1, claim3} {
		got, ok := claims.At(index)
		if !ok || got != want {
			t.Fatalf("canonical Claims.At(%d) = %v/%v, want %v", index, got, ok, want)
		}
	}
}

func TestOperandsRejectCrossOwnerConcreteTargetSharing(t *testing.T) {
	input := operandsFixture(t)
	// The same concrete type cannot have two external canonical parents.
	input.Operands.TypeValue[0].Target = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	if _, err := Build(input); err == nil {
		t.Fatal("Build() accepted a static target shared by Claim and TypeValue")
	}
}

func TestOperandsCopyFenceBoundsAndQueriesDoNotAllocate(t *testing.T) {
	input := operandsFixture(t)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	input.Operands.Claim[0].Target = 0
	input.Operands.TypeValue[0].Target = 0
	input.Operands.Annotation[0].Name = 99
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	operands := component.View().Operands()
	claim := keyspace.MakeTerm(keyspace.FamilyValueClaim, 1)
	typeValue := keyspace.MakeTerm(keyspace.FamilyTypeValue, 1)
	annotationTarget := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	if target, ok := operands.Claims().Target(claim); !ok || target == 0 {
		t.Fatalf("claim copy fence = %v/%v", target, ok)
	}
	if target, ok := operands.TypeValues().Target(typeValue); !ok || target == 0 {
		t.Fatalf("TypeValue copy fence = %v/%v", target, ok)
	}
	if row, ok := operands.Annotations().Get(keyspace.MakeTerm(keyspace.FamilyAnnotation, 1)); !ok || row.Name != 2 {
		t.Fatalf("annotation copy fence = %+v/%v", row, ok)
	}
	if _, ok := operands.Claims().At(1); ok {
		t.Fatal("Claims.At accepted sparse out-of-bounds index")
	}
	if _, ok := operands.TypeValues().Target(keyspace.MakeTerm(keyspace.FamilyTypeValue, 2)); ok {
		t.Fatal("TypeValues.Target accepted unknown term")
	}
	if _, ok := operands.Annotations().ForAt(annotationTarget, 2); ok {
		t.Fatal("Annotations.ForAt accepted out-of-bounds index")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		operands.Claims().Count()
		operands.Claims().At(0)
		operands.Claims().Target(claim)
		operands.TypeValues().Target(typeValue)
		operands.Annotations().Get(keyspace.MakeTerm(keyspace.FamilyAnnotation, 1))
		operands.Annotations().ForCount(annotationTarget)
		operands.Annotations().ForAt(annotationTarget, 1)
	}); allocations != 0 {
		t.Fatalf("operand queries allocated %.2f times", allocations)
	}
}

// The closed authored-static family set and its immutable owner stores must
// move together. This is intentionally a local query law: it prevents a new
// type form from being accepted by Build yet becoming unqueryable as an
// annotation anchor after compaction.
func TestAnnotationAnchorCoversEveryStaticTypeFamily(t *testing.T) {
	component := &Component{types: typeStore{
		primitive:    make([]Primitive, 1),
		literal:      make([]Literal, 1),
		optional:     make([]Optional, 1),
		union:        make([]poolRange, 1),
		intersection: make([]poolRange, 1),
		generic:      make([]genericRow, 1),
		array:        make([]Array, 1),
		mapType:      make([]Map, 1),
		record:       make([]recordRow, 1),
		field:        make([]Field, 1),
	}, references: referenceStore{rows: make([]typeRefRow, 1)},
		signatures: signatureStore{
			functions:  make([]typeFunctionRow, 1),
			assertions: make([]TypeAsserts, 1),
		}, operators: operatorsStore{
			typeOf:      make([]TypeOf, 1),
			keyOf:       make([]KeyOf, 1),
			indexAccess: make([]IndexAccess, 1),
			conditional: make([]Conditional, 1),
		},
	}
	families := []keyspace.Family{
		keyspace.FamilyTypePrimitive, keyspace.FamilyTypeLiteral,
		keyspace.FamilyTypeOptional, keyspace.FamilyTypeUnion,
		keyspace.FamilyTypeIntersection, keyspace.FamilyTypeRef,
		keyspace.FamilyTypeGeneric, keyspace.FamilyTypeArray,
		keyspace.FamilyTypeMap, keyspace.FamilyTypeRecord,
		keyspace.FamilyTypeField, keyspace.FamilyTypeFunction,
		keyspace.FamilyTypeAsserts, keyspace.FamilyTypeOf,
		keyspace.FamilyTypeKeyOf, keyspace.FamilyTypeIndexAccess,
		keyspace.FamilyTypeConditional,
	}
	for _, family := range families {
		if !annotationTargetPresent(component, keyspace.MakeTerm(family, 1)) {
			t.Fatalf("annotation anchor rejected static family %v", family)
		}
	}
	if annotationTargetPresent(component, keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)) {
		t.Fatal("annotation anchor accepted declaration rather than a type occurrence")
	}
}
