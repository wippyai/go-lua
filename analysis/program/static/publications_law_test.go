package static

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

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
		Types:  TypesInput{Primitive: []Primitive{{Kind: PrimitiveNumber}}},
		References: ReferencesInput{TypeRef: []TypeRef{
			{Resolution: TypeRefDeclaration, Target: keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1), Source: []keyspace.Key{1}},
			{Resolution: TypeRefCanonicalPath, Source: []keyspace.Key{2}, Canonical: []keyspace.Key{9}},
		}},
		Declarations: DeclarationsInput{Alias: []TypeAlias{{
			Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1),
			Name: 1, NameCoordinate: coordinate,
		}}},
		Publications: PublicationsInput{Type: []Publication{{
			Assign: keyspace.MakeTerm(keyspace.FamilyAssign, 1), Pair: 0, Target: keyspace.MakeTerm(keyspace.FamilyTypeRef, 1),
		}}},
	}
}

func TestPublicationsPreserveExactDenseRelation(t *testing.T) {
	draft, err := Build(publicationFixture(t))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	publications := component.View().Publications()
	term, ok := publications.At(0)
	if !ok || term != keyspace.MakeTerm(keyspace.FamilyTypePublication, 1) {
		t.Fatalf("Publications.At(0) = %v/%v, want publication 1", term, ok)
	}
	assign, pair, target, ok := publications.Get(term)
	if !ok || assign != keyspace.MakeTerm(keyspace.FamilyAssign, 1) || pair != 0 || target != keyspace.MakeTerm(keyspace.FamilyTypeRef, 1) {
		t.Fatalf("Publications.Get() = (%v, %d, %v, %v), want exact authored row", assign, pair, target, ok)
	}
}

func TestPublicationsAcceptResolvedTargetsAndDistinctPairs(t *testing.T) {
	input := publicationFixture(t)
	input.Counts[keyspace.FamilyTypePublication] = 2
	input.Publications.Type = append(input.Publications.Type, Publication{
		Assign: keyspace.MakeTerm(keyspace.FamilyAssign, 1), Pair: math.MaxUint32, Target: keyspace.MakeTerm(keyspace.FamilyTypeRef, 2),
	})
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() rejected Declaration/CanonicalPath targets or maximum uint32 pair: %v", err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	publications := component.View().Publications()
	if publications.Count() != 2 {
		t.Fatalf("Publications.Count() = %d, want 2", publications.Count())
	}
	_, pair, target, ok := publications.Get(keyspace.MakeTerm(keyspace.FamilyTypePublication, 2))
	if !ok || pair != math.MaxUint32 || target != keyspace.MakeTerm(keyspace.FamilyTypeRef, 2) {
		t.Fatalf("second publication = (%d, %v, %v), want maximum pair/canonical target", pair, target, ok)
	}
}

func TestPublicationsRejectInvalidRowsAndLocalForestDefects(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Input)
	}{
		{"missing publication count", func(input *Input) { input.Counts[keyspace.FamilyTypePublication] = 0 }},
		{"extra publication count", func(input *Input) { input.Counts[keyspace.FamilyTypePublication] = 2 }},
		{"foreign assign", func(input *Input) { input.Publications.Type[0].Assign = keyspace.MakeTerm(keyspace.FamilyAssign, 2) }},
		{"wrong assign family", func(input *Input) { input.Publications.Type[0].Assign = keyspace.MakeTerm(keyspace.FamilyBody, 1) }},
		{"wrong target family", func(input *Input) {
			input.Publications.Type[0].Target = keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)
		}},
		{"foreign target", func(input *Input) { input.Publications.Type[0].Target = keyspace.MakeTerm(keyspace.FamilyTypeRef, 3) }},
		{"unresolved target", func(input *Input) {
			input.References.TypeRef[0] = TypeRef{Resolution: TypeRefUnresolved, Source: []keyspace.Key{1}}
		}},
		{"duplicate assign pair", func(input *Input) {
			input.Counts[keyspace.FamilyTypePublication] = 2
			input.Publications.Type = append(input.Publications.Type, Publication{
				Assign: keyspace.MakeTerm(keyspace.FamilyAssign, 1), Pair: 0, Target: keyspace.MakeTerm(keyspace.FamilyTypeRef, 2),
			})
		}},
		{"target shared between publications", func(input *Input) {
			input.Counts[keyspace.FamilyAssign] = 2
			input.Counts[keyspace.FamilyTypePublication] = 2
			input.Publications.Type = append(input.Publications.Type, Publication{
				Assign: keyspace.MakeTerm(keyspace.FamilyAssign, 2), Pair: 0, Target: keyspace.MakeTerm(keyspace.FamilyTypeRef, 1),
			})
		}},
		{"target shared with type value", func(input *Input) {
			input.Counts[keyspace.FamilyTypeValue] = 1
			input.Operands.TypeValue = []TypeValueTarget{{Target: keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)}}
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := publicationFixture(t)
			test.edit(&input)
			if _, err := Build(input); err == nil {
				t.Fatal("Build() accepted invalid publication relation")
			}
		})
	}
}

func TestPublicationsCopyFenceBoundsAndQueriesDoNotAllocate(t *testing.T) {
	input := publicationFixture(t)
	draft, err := Build(input)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	input.Publications.Type[0] = Publication{}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	publications := component.View().Publications()
	term := keyspace.MakeTerm(keyspace.FamilyTypePublication, 1)
	assign, pair, target, ok := publications.Get(term)
	if !ok || assign == 0 || pair != 0 || target == 0 {
		t.Fatalf("publication copy fence = (%v, %d, %v, %v)", assign, pair, target, ok)
	}
	if _, ok := publications.At(1); ok {
		t.Fatal("Publications.At accepted out-of-bounds index")
	}
	if _, _, _, ok := publications.Get(0); ok {
		t.Fatal("Publications.Get accepted zero term")
	}
	if _, _, _, ok := publications.Get(keyspace.MakeTerm(keyspace.FamilyTypePublication, 2)); ok {
		t.Fatal("Publications.Get accepted foreign ordinal")
	}
	if _, _, _, ok := publications.Get(keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)); ok {
		t.Fatal("Publications.Get accepted foreign family")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		publications.Count()
		publications.At(0)
		publications.Get(term)
	}); allocations != 0 {
		t.Fatalf("publication queries allocated %.2f times", allocations)
	}
}
