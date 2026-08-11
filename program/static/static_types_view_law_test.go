package static

import (
	"testing"

	"github.com/wippyai/go-lua/program/keyspace"
)

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
