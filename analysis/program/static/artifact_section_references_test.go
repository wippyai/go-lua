package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestArtifactReferencesDecoderRetainsAuthoredDispositions(t *testing.T) {
	counts := [keyspace.FamilyCount]uint32{
		keyspace.FamilyTypeRef:   3,
		keyspace.FamilyTypeAlias: 1,
		keyspace.FamilyCell:      1,
	}
	input := referenceInput(counts, ReferencesInput{TypeRef: []TypeRef{
		{Resolution: TypeRefDeclaration, Source: []keyspace.Key{1}, Target: keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1)},
		{Resolution: TypeRefCanonicalPath, Source: []keyspace.Key{2, 3}, Root: keyspace.MakeTerm(keyspace.FamilyCell, 1), Canonical: []keyspace.Key{7, 8}},
		{Resolution: TypeRefUnresolved, Source: []keyspace.Key{4, 5}, Root: keyspace.MakeTerm(keyspace.FamilyCell, 1)},
	}})
	decoded := decodeStaticArtifactInputForTest(t, input)
	if len(decoded.References.TypeRef) != 3 {
		t.Fatalf("decoded reference count = %d, want 3", len(decoded.References.TypeRef))
	}
	if decoded.References.TypeRef[0].Resolution != TypeRefDeclaration ||
		decoded.References.TypeRef[0].Target != keyspace.MakeTerm(keyspace.FamilyTypeAlias, 1) {
		t.Fatalf("decoded declaration reference = %+v", decoded.References.TypeRef[0])
	}
	canonical := decoded.References.TypeRef[1]
	if canonical.Resolution != TypeRefCanonicalPath || canonical.Root != keyspace.MakeTerm(keyspace.FamilyCell, 1) ||
		len(canonical.Source) != 2 || canonical.Source[1] != 3 || len(canonical.Canonical) != 2 || canonical.Canonical[0] != 7 {
		t.Fatalf("decoded canonical reference = %+v", canonical)
	}
	if decoded.References.TypeRef[2].Resolution != TypeRefUnresolved ||
		decoded.References.TypeRef[2].Target != 0 || len(decoded.References.TypeRef[2].Canonical) != 0 {
		t.Fatalf("decoded unresolved reference = %+v", decoded.References.TypeRef[2])
	}
}
