package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// referenceInput supplies complete declaration rows whenever a TypeRef test
// reserves a declaration family. The static boundary rejects counted-but-
// absent rows; tests that exercise a target must therefore carry its owner.
func referenceInput(counts [keyspace.FamilyCount]uint32, refs ReferencesInput) Input {
	input := Input{Counts: counts, References: refs}
	if counts[keyspace.FamilyTypeAlias] != 0 {
		input.Counts[keyspace.FamilyBody] = 1
		// Keep the declaration target distinct from any relation under test so
		// the structural forest test does not hide the Reference assertion.
		input.Counts[keyspace.FamilyTypePrimitive] = 2
		input.Types.Primitive = []Primitive{{Kind: PrimitiveAny}, {Kind: PrimitiveNever}}
		coordinate, _ := source.CoordinateFromParts(1, 1, 1, 2)
		params := []keyspace.Term(nil)
		if counts[keyspace.FamilyTypeParam] != 0 {
			params = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypeParam, 1)}
			input.Declarations.TypeParam = []TypeParam{{
				Owner: keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1), Name: 1,
			}}
		}
		input.Declarations.Alias = []TypeAlias{{
			Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Target: keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 2),
			Name: 1, NameCoordinate: coordinate, Params: params,
		}}
	}
	if counts[keyspace.FamilyTypeInterface] != 0 {
		input.Counts[keyspace.FamilyBody] = 1
		coordinate, _ := source.CoordinateFromParts(1, 1, 1, 2)
		input.Declarations.Interface = []Interface{{
			Owner: keyspace.MakeTerm(keyspace.FamilyBody, 1), Name: 2, NameCoordinate: coordinate,
		}}
	}
	return input
}

func TestReferencesPreserveAuthoredBinderDisposition(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypeRef] = 3
	counts[keyspace.FamilyTypeAlias] = 1
	counts[keyspace.FamilyCell] = 1
	draft, err := Build(referenceInput(counts, ReferencesInput{TypeRef: []TypeRef{
		{Resolution: TypeRefDeclaration, Source: []keyspace.Key{1}, Target: keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)},
		{Resolution: TypeRefCanonicalPath, Source: []keyspace.Key{2, 3}, Root: keyspace.MakeTerm(keyspace.FamilyCell, 1), Canonical: []keyspace.Key{7, 8}},
		{Resolution: TypeRefUnresolved, Source: []keyspace.Key{4, 5}, Root: keyspace.MakeTerm(keyspace.FamilyCell, 1)},
	}}))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	references := component.View().References()
	if got := references.Count(); got != 3 {
		t.Fatalf("Count() = %d, want 3", got)
	}
	declaration := keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)
	if kind, target, root, ok := references.Get(declaration); !ok || kind != TypeRefDeclaration ||
		target != keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1) || root != 0 {
		t.Fatalf("declaration Get() = (%v, %v, %v, %v)", kind, target, root, ok)
	}
	canonical := keyspace.MakeTerm(keyspace.FamilyTypeRef, 2)
	if source, ok := references.SourceCount(canonical); !ok || source != 2 {
		t.Fatalf("SourceCount() = (%d, %v), want 2", source, ok)
	}
	if key, ok := references.SourceAt(canonical, 1); !ok || key != 3 {
		t.Fatalf("SourceAt() = (%d, %v), want 3", key, ok)
	}
	if canonicalCount, ok := references.CanonicalCount(canonical); !ok || canonicalCount != 2 {
		t.Fatalf("CanonicalCount() = (%d, %v), want 2", canonicalCount, ok)
	}
	if key, ok := references.CanonicalAt(canonical, 0); !ok || key != 7 {
		t.Fatalf("CanonicalAt() = (%d, %v), want 7", key, ok)
	}
	unresolved := keyspace.MakeTerm(keyspace.FamilyTypeRef, 3)
	if kind, target, root, ok := references.Get(unresolved); !ok || kind != TypeRefUnresolved || target != 0 || root != keyspace.MakeTerm(keyspace.FamilyCell, 1) {
		t.Fatalf("unresolved Get() = (%v, %v, %v, %v)", kind, target, root, ok)
	}
	if length, ok := references.CanonicalCount(unresolved); !ok || length != 0 {
		t.Fatalf("unresolved CanonicalCount() = (%d, %v), want 0", length, ok)
	}
}

func TestReferencesRejectInvalidXORRootAndArity(t *testing.T) {
	cell := keyspace.MakeTerm(keyspace.FamilyCell, 1)
	alias := keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)
	interfaceType := keyspace.MakeTerm(keyspace.FamilyTypeInterface, 1)
	param := keyspace.MakeTerm(keyspace.FamilyTypeParam, 1)
	cases := []struct {
		name string
		row  TypeRef
		ok   bool
	}{
		{"unqualified unresolved", TypeRef{Resolution: TypeRefUnresolved, Source: []keyspace.Key{1}}, true},
		{"qualified unresolved", TypeRef{Resolution: TypeRefUnresolved, Source: []keyspace.Key{1, 2}, Root: cell}, true},
		{"alias target", TypeRef{Resolution: TypeRefDeclaration, Source: []keyspace.Key{1}, Target: alias}, true},
		{"interface target", TypeRef{Resolution: TypeRefDeclaration, Source: []keyspace.Key{1}, Target: interfaceType}, true},
		{"param target", TypeRef{Resolution: TypeRefDeclaration, Source: []keyspace.Key{1}, Target: param}, true},
		{"canonical", TypeRef{Resolution: TypeRefCanonicalPath, Source: []keyspace.Key{1, 2}, Root: cell, Canonical: []keyspace.Key{3}}, true},
		{"empty source", TypeRef{Resolution: TypeRefUnresolved}, false},
		{"zero source key", TypeRef{Resolution: TypeRefUnresolved, Source: []keyspace.Key{0}}, false},
		{"unqualified root", TypeRef{Resolution: TypeRefUnresolved, Source: []keyspace.Key{1}, Root: cell}, false},
		{"qualified missing root", TypeRef{Resolution: TypeRefUnresolved, Source: []keyspace.Key{1, 2}}, false},
		{"qualified noncell root", TypeRef{Resolution: TypeRefUnresolved, Source: []keyspace.Key{1, 2}, Root: alias}, false},
		{"unresolved target", TypeRef{Resolution: TypeRefUnresolved, Source: []keyspace.Key{1}, Target: alias}, false},
		{"unresolved canonical", TypeRef{Resolution: TypeRefUnresolved, Source: []keyspace.Key{1}, Canonical: []keyspace.Key{2}}, false},
		{"declaration missing target", TypeRef{Resolution: TypeRefDeclaration, Source: []keyspace.Key{1}}, false},
		{"declaration canonical", TypeRef{Resolution: TypeRefDeclaration, Source: []keyspace.Key{1}, Target: alias, Canonical: []keyspace.Key{2}}, false},
		{"declaration type ref target", TypeRef{Resolution: TypeRefDeclaration, Source: []keyspace.Key{1}, Target: keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)}, false},
		{"canonical target", TypeRef{Resolution: TypeRefCanonicalPath, Source: []keyspace.Key{1}, Target: alias, Canonical: []keyspace.Key{2}}, false},
		{"canonical missing path", TypeRef{Resolution: TypeRefCanonicalPath, Source: []keyspace.Key{1}}, false},
		{"canonical zero path key", TypeRef{Resolution: TypeRefCanonicalPath, Source: []keyspace.Key{1}, Canonical: []keyspace.Key{0}}, false},
		{"unknown disposition", TypeRef{Resolution: 99, Source: []keyspace.Key{1}}, false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			counts := [keyspace.FamilyCount]uint32{}
			counts[keyspace.FamilyTypeRef] = 1
			counts[keyspace.FamilyCell] = 1
			counts[keyspace.FamilyTypeAlias] = 1
			counts[keyspace.FamilyTypeInterface] = 1
			counts[keyspace.FamilyTypeParam] = 1
			_, err := Build(referenceInput(counts, ReferencesInput{TypeRef: []TypeRef{test.row}}))
			if (err == nil) != test.ok {
				t.Fatalf("Build() error = %v, want accepted=%v", err, test.ok)
			}
		})
	}
}

func TestReferencesCopyFenceAndBounds(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypeRef] = 1
	counts[keyspace.FamilyCell] = 1
	source := []keyspace.Key{1, 2}
	canonical := []keyspace.Key{3, 4}
	draft, err := Build(referenceInput(counts, ReferencesInput{TypeRef: []TypeRef{{
		Resolution: TypeRefCanonicalPath,
		Source:     source,
		Root:       keyspace.MakeTerm(keyspace.FamilyCell, 1),
		Canonical:  canonical,
	}}}))
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	source[0], canonical[0] = 99, 99
	component, err := commitStaticDraft(t, draft)
	if err != nil {
		t.Fatalf("take() error = %v", err)
	}
	references := component.View().References()
	term := keyspace.MakeTerm(keyspace.FamilyTypeRef, 1)
	if got, ok := references.SourceAt(term, 0); !ok || got != 1 {
		t.Fatalf("source copy fence = (%d, %v)", got, ok)
	}
	if got, ok := references.CanonicalAt(term, 0); !ok || got != 3 {
		t.Fatalf("canonical copy fence = (%d, %v)", got, ok)
	}
	for _, index := range []int{-1, 2} {
		if _, ok := references.SourceAt(term, index); ok {
			t.Fatalf("SourceAt(%d) accepted out-of-bounds index", index)
		}
		if _, ok := references.CanonicalAt(term, index); ok {
			t.Fatalf("CanonicalAt(%d) accepted out-of-bounds index", index)
		}
	}
	if _, ok := references.At(1); ok {
		t.Fatal("At() accepted out-of-bounds ordinal")
	}
	if _, _, _, ok := references.Get(keyspace.MakeTerm(keyspace.FamilyTypeRef, 2)); ok {
		t.Fatal("Get() accepted an unknown TypeRef")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		references.Get(term)
		references.SourceCount(term)
		references.SourceAt(term, 1)
		references.CanonicalCount(term)
		references.CanonicalAt(term, 1)
	}); allocations != 0 {
		t.Fatalf("Reference queries allocated %.2f times", allocations)
	}
}

func TestGenericAcceptsReferencesOwnedTypeRef(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{}
	counts[keyspace.FamilyTypePrimitive] = 1
	counts[keyspace.FamilyTypeRef] = 1
	counts[keyspace.FamilyTypeAlias] = 1
	counts[keyspace.FamilyTypeGeneric] = 1
	input := referenceInput(counts, ReferencesInput{TypeRef: []TypeRef{{
		Resolution: TypeRefDeclaration,
		Source:     []keyspace.Key{1},
		Target:     keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1),
	}}})
	input.Types.Generic = []Generic{{
		Base: keyspace.MakeTerm(keyspace.FamilyTypeRef, 1),
		Args: []keyspace.Term{keyspace.MakeTerm(keyspace.FamilyTypePrimitive, 1)},
	}}
	_, err := Build(input)
	if err != nil {
		t.Fatalf("Build() rejected Generic TypeRef base: %v", err)
	}
}
