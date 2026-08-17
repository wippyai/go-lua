package static

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"testing"
)

func TestStaticTypeEnumerationIsCompleteAndOrdered(t *testing.T) {
	component := staticContentComponent(t, staticTypeDenominatorInput(t))
	wantFamilies := [...]keyspace.Family{
		keyspace.FamilyTypeAlias,
		keyspace.FamilyTypeInterface,
		keyspace.FamilyTypeParam,
		keyspace.FamilyTypePrimitive,
		keyspace.FamilyTypeLiteral,
		keyspace.FamilyTypeOptional,
		keyspace.FamilyTypeUnion,
		keyspace.FamilyTypeIntersection,
		keyspace.FamilyTypeRef,
		keyspace.FamilyTypeGeneric,
		keyspace.FamilyTypeArray,
		keyspace.FamilyTypeMap,
		keyspace.FamilyTypeRecord,
		keyspace.FamilyTypeFunction,
		keyspace.FamilyTypeAsserts,
		keyspace.FamilyTypeOf,
		keyspace.FamilyTypeKeyOf,
		keyspace.FamilyTypeIndexAccess,
		keyspace.FamilyTypeConditional,
	}
	want := make([]keyspace.Term, 0, len(wantFamilies)+19)
	for _, family := range wantFamilies {
		count := 1
		if family == keyspace.FamilyTypePrimitive {
			count = 20
		}
		for ordinal := 1; ordinal <= count; ordinal++ {
			want = append(want, keyspace.MakeTerm(family, uint32(ordinal)))
		}
	}
	if got := component.StaticTypeTermCount(); got != len(want) {
		t.Fatalf("StaticTypeTermCount = %d, want %d", got, len(want))
	}
	seen := make(map[keyspace.Family]int, len(wantFamilies))
	for index, expected := range want {
		term, ok := component.StaticTypeTermAt(index)
		if !ok || term != expected {
			t.Fatalf("StaticTypeTermAt(%d) = %v/%v, want %v", index, term, ok, expected)
		}
		if !component.StaticTypeTerm(term) {
			t.Fatalf("enumerated term %v is not a static type", term)
		}
		seen[keyspace.TermFamily(term)]++
	}
	for _, family := range wantFamilies {
		if seen[family] == 0 {
			t.Fatalf("static type family %d has no enumerated row", family)
		}
	}
	if _, ok := component.StaticTypeTermAt(-1); ok {
		t.Fatal("StaticTypeTermAt accepted a negative index")
	}
	if _, ok := component.StaticTypeTermAt(len(want)); ok {
		t.Fatal("StaticTypeTermAt accepted an out-of-range index")
	}
	if component.StaticTypeTerm(keyspace.MakeTerm(keyspace.FamilyRead, 1)) {
		t.Fatal("non-static Flow term entered the static authority")
	}

}

func TestStaticTypeQueriesDoNotAllocate(t *testing.T) {
	component := staticContentComponent(t, staticTypeDenominatorInput(t))
	term := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	if allocations := testing.AllocsPerRun(1000, func() {
		component.StaticTypeTermCount()
		component.StaticTypeTermAt(0)
		component.StaticTypeTerm(term)
	}); allocations != 0 {
		t.Fatalf("static type queries allocated %.2f times", allocations)
	}
}

func TestStaticTypeIndexIsExcludedFromAuthoredContentID(t *testing.T) {
	component := staticContentComponent(t, staticTypeDenominatorInput(t))
	want := component.ContentID()
	component.staticTypes.prefix[1]++
	if got := contentID(component); got != want {
		t.Fatalf("derived static type prefix changed authored ContentID: %x != %x", got, want)
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
	primitives := make([]Primitive, 20)
	for index := range primitives {
		primitives[index] = Primitive{Kind: PrimitiveAny}
	}
	return Input{
		Counts: counts,
		Types: TypesInput{
			Primitive:    primitives,
			Literal:      []Literal{{Kind: keyspace.LiteralString, Exact: 1}},
			Optional:     []Optional{{Inner: primitive(1)}},
			Union:        []Union{{Members: []keyspace.Term{primitive(2), primitive(3)}}},
			Intersection: []Intersection{{Members: []keyspace.Term{primitive(4), primitive(5)}}},
			Generic: []Generic{{
				Base: term(keyspace.FamilyTypeRef, 1), Args: []keyspace.Term{primitive(6)},
			}},
			Array:  []Array{{Element: primitive(7)}},
			Map:    []Map{{Key: primitive(8), Value: primitive(9)}},
			Field:  []Field{{Key: 2, Type: primitive(10)}},
			Record: []Record{{Fields: []keyspace.Term{term(keyspace.FamilyTypeField, 1)}}},
		},
		References: ReferencesInput{TypeRef: []TypeRef{{
			Resolution: TypeRefUnresolved, Source: []keyspace.Key{3},
		}}},
		Declarations: DeclarationsInput{
			Alias: []TypeAlias{{
				Owner: term(keyspace.FamilyBody, 1), Target: primitive(11), Name: 4,
				NameCoordinate: coordinate, Params: []keyspace.Term{term(keyspace.FamilyTypeParam, 1)},
			}},
			TypeParam: []TypeParam{{
				Owner: term(keyspace.FamilyTypeAlias, 1), Name: 5, Constraint: primitive(12),
			}},
			Interface: []Interface{{
				Owner: term(keyspace.FamilyBody, 2), Name: 6, NameCoordinate: coordinate,
			}},
		},
		Signatures: SignaturesInput{
			TypeFunction: []TypeFunction{{
				Scope: term(keyspace.FamilyCell, 1), ReturnsKnown: true,
			}},
			TypeAsserts: []TypeAsserts{{
				Name: 7, ParamCoordinate: coordinate, Narrow: primitive(13),
			}},
		},
		Operators: OperatorsInput{
			TypeOf: []TypeOf{{
				Scope: term(keyspace.FamilyCell, 1), Operand: term(keyspace.FamilyRead, 1),
			}},
			KeyOf:       []KeyOf{{Inner: primitive(14)}},
			IndexAccess: []IndexAccess{{Object: primitive(15), Index: primitive(16)}},
			Conditional: []Conditional{{
				Check: primitive(17), Extends: primitive(18), Then: primitive(19), Else: primitive(20),
			}},
		},
	}
}

func TestStaticTypesViewUsesPublishedForestOrder(t *testing.T) {
	component := staticContentComponent(t, staticTypeDenominatorInput(t))
	types := component.View().StaticTypes()

	wantFamilies := [...]keyspace.Family{
		keyspace.FamilyTypeAlias,
		keyspace.FamilyTypeInterface,
		keyspace.FamilyTypeParam,
		keyspace.FamilyTypePrimitive,
		keyspace.FamilyTypeLiteral,
		keyspace.FamilyTypeOptional,
		keyspace.FamilyTypeUnion,
		keyspace.FamilyTypeIntersection,
		keyspace.FamilyTypeRef,
		keyspace.FamilyTypeGeneric,
		keyspace.FamilyTypeArray,
		keyspace.FamilyTypeMap,
		keyspace.FamilyTypeRecord,
		keyspace.FamilyTypeFunction,
		keyspace.FamilyTypeAsserts,
		keyspace.FamilyTypeOf,
		keyspace.FamilyTypeKeyOf,
		keyspace.FamilyTypeIndexAccess,
		keyspace.FamilyTypeConditional,
	}
	want := make([]keyspace.Term, 0, component.StaticTypeTermCount())
	for _, family := range wantFamilies {
		count := 1
		if family == keyspace.FamilyTypePrimitive {
			count = 20
		}
		for ordinal := 1; ordinal <= count; ordinal++ {
			want = append(want, keyspace.MakeTerm(family, uint32(ordinal)))
		}
	}
	if got := types.Count(); got != len(want) {
		t.Fatalf("StaticTypes.Count() = %d, want %d", got, len(want))
	}
	for index, expected := range want {
		ref, ok := types.At(index)
		if !ok || ref.Term() != expected {
			t.Fatalf("StaticTypes.At(%d) = %v/%v, want %v", index, ref.Term(), ok, expected)
		}
		bound, ok := types.Ref(expected)
		if !ok || bound.Term() != expected {
			t.Fatalf("StaticTypes.Ref(%v) = %v/%v", expected, bound.Term(), ok)
		}
	}
}

func TestStaticTypesViewRejectsNilAndMalformedTerms(t *testing.T) {
	var nilComponent *Component
	zero := nilComponent.View().StaticTypes()
	if zero.Count() != 0 {
		t.Fatal("nil Component StaticTypes view exposed rows")
	}
	if _, ok := zero.At(0); ok {
		t.Fatal("nil Component StaticTypes.At succeeded")
	}
	if _, ok := zero.Ref(keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)); ok {
		t.Fatal("nil Component StaticTypes.Ref succeeded")
	}

	component := staticContentComponent(t, staticTypeDenominatorInput(t))
	types := component.View().StaticTypes()
	bad := []keyspace.Term{
		0,
		keyspace.MakeTerm(keyspace.FamilyRead, 1),
		keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 0),
		keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 21),
	}
	for _, term := range bad {
		if ref, ok := types.Ref(term); ok || ref.Term() != 0 {
			t.Fatalf("StaticTypes.Ref(%v) = %v/%v, want zero/false", term, ref.Term(), ok)
		}
	}
}

func TestStaticTypesRawTermsRebindLocally(t *testing.T) {
	component := staticContentComponent(t, staticTypeDenominatorInput(t))
	foreign := staticContentComponent(t, Input{
		Counts: [keyspace.FamilyCount]uint32{keyspace.FamilyTypePrimitive: 1},
		Types:  TypesInput{Primitive: []Primitive{{Kind: PrimitiveAny}}},
	})

	foreignRef, ok := foreign.View().StaticTypes().At(0)
	if !ok {
		t.Fatal("foreign StaticTypes.At(0) failed")
	}
	raw := foreignRef.Term()
	if raw != keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1) {
		t.Fatalf("foreign StaticTypeRef.Term() = %v, want primitive/1", raw)
	}

	bound, ok := component.View().StaticTypes().Ref(raw)
	if !ok || bound.Term() != raw {
		t.Fatalf("raw term failed to rebind locally: %v/%v", bound.Term(), ok)
	}
	if foreignRef.component != foreign {
		t.Fatal("foreign ref lost its owner component")
	}
	if bound.component != component {
		t.Fatal("rebound ref did not carry the receiving owner component")
	}
	if bound.component == foreignRef.component {
		t.Fatal("rebound ref retained the foreign owner component")
	}
}

func TestStaticTypesConstructionViewCannotLeakReferences(t *testing.T) {
	draft := primitiveDraft(t)
	finalizer, err := draft.Finalizer()
	if err != nil {
		t.Fatalf("Finalizer() error = %v", err)
	}
	construction := finalizer.View().StaticTypes()
	if construction.Count() != 0 {
		t.Fatal("claimed construction View exposed post-commit StaticTypes")
	}
	if _, ok := construction.At(0); ok {
		t.Fatal("claimed construction View minted a StaticTypeRef")
	}

	component, err := finalizer.Commit(CommitInput{})
	if err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if construction.Count() != 0 {
		t.Fatal("expired construction StaticTypes view regained rows")
	}
	if _, ok := construction.Ref(keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)); ok {
		t.Fatal("expired construction StaticTypes view minted a ref")
	}
	ref, ok := component.View().StaticTypes().Ref(keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1))
	if !ok || ref.Term() == 0 {
		t.Fatal("published Component StaticTypes failed to mint a ref")
	}
}

func TestStaticTypesHotQueriesDoNotAllocate(t *testing.T) {
	component := staticContentComponent(t, staticTypeDenominatorInput(t))
	types := component.View().StaticTypes()
	term := keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
	ref, ok := types.Ref(term)
	if !ok {
		t.Fatal("StaticTypes.Ref failed for allocation fixture")
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		_ = types.Count()
		_, _ = types.At(0)
		_, _ = types.Ref(term)
		_ = ref.Term()
	}); allocations != 0 {
		t.Fatalf("StaticTypes hot queries allocated %.2f times", allocations)
	}
}
