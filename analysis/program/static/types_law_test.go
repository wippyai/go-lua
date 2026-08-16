package static

import (
	"sync"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestTypesPreserveExactTypedRelations(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypePrimitive] = 5
	counts[keyspace.FamilyTypeLiteral] = 1
	counts[keyspace.FamilyTypeOptional] = 1
	counts[keyspace.FamilyTypeUnion] = 1
	counts[keyspace.FamilyTypeIntersection] = 1
	counts[keyspace.FamilyTypeRef] = 2 // References owns the rows below.
	counts[keyspace.FamilyTypeGeneric] = 1
	counts[keyspace.FamilyTypeArray] = 1
	counts[keyspace.FamilyTypeMap] = 1
	counts[keyspace.FamilyTypeRecord] = 1
	counts[keyspace.FamilyTypeField] = 1
	counts[keyspace.FamilyTypeAlias] = 1
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyCell] = 1

	primitive := func(ordinal uint32) keyspace.Term {
		return keyspace.MakeTerm(keyspace.FamilyTypePrimitive, ordinal)
	}
	optional := keyspace.MakeTerm(keyspace.FamilyTypeOptional, 1)
	union := keyspace.MakeTerm(keyspace.FamilyTypeUnion, 1)
	generic := keyspace.MakeTerm(keyspace.FamilyTypeGeneric, 1)
	array := keyspace.MakeTerm(keyspace.FamilyTypeArray, 1)
	mapType := keyspace.MakeTerm(keyspace.FamilyTypeMap, 1)
	field := keyspace.MakeTerm(keyspace.FamilyTypeField, 1)
	ref := func(ordinal uint32) keyspace.Term {
		return keyspace.MakeTerm(keyspace.FamilyTypeRef, ordinal)
	}

	draft, err := Build(Input{Counts: counts, Types: TypesInput{
		Primitive: []Primitive{
			{Kind: PrimitiveNil}, {Kind: PrimitiveNumber},
			{Kind: PrimitiveString}, {Kind: PrimitiveBoolean}, {Kind: PrimitiveNever},
		},
		Literal:      []Literal{{Kind: keyspace.LiteralString, Exact: 7}},
		Optional:     []Optional{{Inner: primitive(1)}},
		Union:        []Union{{Members: []keyspace.Term{optional, primitive(2)}}},
		Intersection: []Intersection{{Members: []keyspace.Term{primitive(3), primitive(4)}}},
		Generic:      []Generic{{Base: ref(1), Args: []keyspace.Term{union}}},
		Array:        []Array{{Element: generic, ReadOnly: true}},
		Map:          []Map{{Key: ref(2), Value: array}},
		Field:        []Field{{Key: 9, Type: mapType, Optional: true}},
		Record:       []Record{{Fields: []keyspace.Term{field}, ReadOnly: true}},
	}, References: ReferencesInput{TypeRef: []TypeRef{
		{Resolution: TypeRefDeclaration, Source: []keyspace.Key{1}, Target: keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)},
		{Resolution: TypeRefCanonicalPath, Source: []keyspace.Key{2, 3}, Root: keyspace.MakeTerm(keyspace.FamilyCell, 1), Canonical: []keyspace.Key{4}},
	}}, Declarations: DeclarationsInput{Alias: []TypeAlias{{
		Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Target: primitive(5), Name: 10,
		NameCoordinate: func() source.Coordinate { value, _ := source.CoordinateFromParts(1, 1, 1, 2); return value }(),
	}}}})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	types := component.View().Types()
	if got := types.Primitives().Count(); got != 5 {
		t.Fatalf("primitive count = %d, want 5", got)
	}
	if kind, ok := types.Primitives().Get(primitive(2)); !ok || kind != PrimitiveNumber {
		t.Fatalf("primitive relation = (%v, %v), want number", kind, ok)
	}
	if kind, exact, bits, ok := types.Literals().Get(keyspace.MakeTerm(keyspace.FamilyTypeLiteral, 1)); !ok ||
		kind != keyspace.LiteralString || exact != 7 || bits != 0 {
		t.Fatalf("literal relation = (%v, %v, %d, %v)", kind, exact, bits, ok)
	}
	if got, ok := types.Unions().MemberCount(union); !ok || got != 2 {
		t.Fatalf("union length = (%d, %v)", got, ok)
	}
	if got, ok := types.Unions().MemberAt(union, 1); !ok || got != primitive(2) {
		t.Fatalf("union member = (%v, %v)", got, ok)
	}
	if base, arity, ok := types.Generics().Get(generic); !ok || base != ref(1) || arity != 1 {
		t.Fatalf("generic relation = (%v, %d, %v)", base, arity, ok)
	}
	if key, typ, optionalField, ok := types.Fields().Get(field); !ok || key != 9 || typ != mapType || !optionalField {
		t.Fatalf("field relation = (%v, %v, %v, %v)", key, typ, optionalField, ok)
	}
	if readOnly, fields, ok := types.Records().Get(keyspace.MakeTerm(keyspace.FamilyTypeRecord, 1)); !ok || !readOnly || fields != 1 {
		t.Fatalf("record relation = (%v, %d, %v)", readOnly, fields, ok)
	}
}

func TestTypesRejectIncompleteOrAmbiguousLocalShape(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypePrimitive] = 1
	counts[keyspace.FamilyTypeUnion] = 1
	if _, err := Build(Input{Counts: counts, Types: TypesInput{
		Primitive: []Primitive{{Kind: PrimitiveNumber}},
		Union:     []Union{{Members: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)}}},
	}}); err == nil {
		t.Fatal("Build() accepted one-member union")
	}

	counts = [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypeLiteral] = 1
	if _, err := Build(Input{Counts: counts, Types: TypesInput{
		Literal: []Literal{{Kind: keyspace.LiteralString}},
	}}); err == nil {
		t.Fatal("Build() accepted string literal without Source exact key")
	}
}

// This chain is deliberately large enough that the former walk-from-every-
// child algorithm would repeatedly rediscover every suffix. The law asserts
// structure, not a machine-dependent duration: acceptance of the chain and
// rejection after one back-edge exercise the same linear containment seal.
func TestTypesContainmentLongChainAndCycle(t *testing.T) {
	const length = 8192
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypePrimitive] = 1
	counts[keyspace.FamilyTypeOptional] = length
	rows := make([]Optional, length)
	rows[0] = Optional{Inner: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)}
	for index := 1; index < len(rows); index++ {
		rows[index] = Optional{Inner: keyspace.MakeTerm(keyspace.FamilyTypeOptional, uint32(index))}
	}
	input := Input{Counts: counts, Types: TypesInput{
		Primitive: []Primitive{{Kind: PrimitiveAny}},
		Optional:  rows,
	}}
	if _, err := Build(input); err != nil {
		t.Fatalf("Build() rejected acyclic containment chain: %v", err)
	}
	input.Types.Optional[0].Inner = keyspace.MakeTerm(keyspace.FamilyTypeOptional, length)
	if _, err := Build(input); err == nil {
		t.Fatal("Build() accepted a containment cycle through a long chain")
	}
}

func TestTypesDraftIsOneShot(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypePrimitive] = 1
	draft, err := Build(Input{Counts: counts, Types: TypesInput{
		Primitive: []Primitive{{Kind: PrimitiveAny}},
	}})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	copy := *draft
	if _, err := commitStaticDraft(t, draft); err != nil {
		t.Fatalf("first take() error = %v", err)
	}
	if _, err := commitStaticDraft(t, &copy); err == nil {
		t.Fatal("copied Draft acquired a second component")
	}
}

func TestTypesDraftCopiesConsumeOnceUnderContention(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypePrimitive] = 1
	draft, err := Build(Input{Counts: counts, Types: TypesInput{
		Primitive: []Primitive{{Kind: PrimitiveAny}},
	}})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	const contenders = 32
	start := make(chan struct{})
	results := make(chan bool, contenders)
	var group sync.WaitGroup
	group.Add(contenders)
	for range contenders {
		copy := *draft
		go func() {
			defer group.Done()
			<-start
			_, err := commitStaticDraft(t, &copy)
			results <- err == nil
		}()
	}
	close(start)
	group.Wait()
	close(results)

	successes := 0
	for result := range results {
		if result {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("contended Draft takes = %d successes, want exactly 1", successes)
	}
}
