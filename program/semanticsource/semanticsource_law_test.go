package semanticsource

import (
	"errors"
	"testing"
)

func TestGeneratedSchemaPreservesZeroRowsAndCanonicalOrder(t *testing.T) {
	primary := generatedDefinition(t, OriginProgramFlowOperators, 0)
	facet := generatedDefinition(t, OriginProgramFlowOperators, FacetProgramFlowUnaryNumeric)
	other := generatedDefinition(t, OriginLinkBoundary, 0)
	publisher, err := NewPublisher(CatalogSchema())
	if err != nil {
		t.Fatal(err)
	}
	for _, publication := range []Publication{
		mustPublication(t, other, 0),
		mustPublication(t, facet, 0),
		mustPublication(t, primary, 7),
	} {
		if err := publisher.Accept(publication); err != nil {
			t.Fatal(err)
		}
	}
	for _, definition := range CatalogSchema().Definitions() {
		if definition == primary || definition == facet || definition == other {
			continue
		}
		if err := publisher.Accept(mustPublication(t, definition, 0)); err != nil {
			t.Fatal(err)
		}
	}
	publications, err := publisher.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if publications.Count() != CatalogSchema().Count() {
		t.Fatalf("publication count = %d, want %d", publications.Count(), CatalogSchema().Count())
	}
	for index := 0; index < publications.Count(); index++ {
		measure, ok := publications.At(index)
		if !ok {
			t.Fatalf("missing measure %d", index)
		}
		if index != 0 {
			previous, _ := publications.At(index - 1)
			if compareToken(previous.Token(), measure.Token()) >= 0 {
				t.Fatalf("measures are not canonical at %d", index)
			}
		}
		if measure.Token() == facet.Token() && measure.Count() != 0 {
			t.Fatalf("facet zero count = %d, want 0", measure.Count())
		}
	}
}

func TestPublicationRejectsDuplicateMissingForeignAndInvalidValues(t *testing.T) {
	primary := generatedDefinition(t, OriginProgramSourceProvenance, 0)
	publisher, err := NewPublisher(CatalogSchema())
	if err != nil {
		t.Fatal(err)
	}
	emptyPrimary := mustPublication(t, primary, 0)
	if err := publisher.Accept(emptyPrimary); err != nil {
		t.Fatal(err)
	}
	if err := publisher.Accept(emptyPrimary); !errors.Is(err, ErrDuplicatePublication) {
		t.Fatalf("duplicate error = %v, want %v", err, ErrDuplicatePublication)
	}
	if _, err := publisher.Seal(); !errors.Is(err, ErrMissingPublication) {
		t.Fatalf("missing error = %v, want %v", err, ErrMissingPublication)
	}
	if err := publisher.Accept(Publication{}); !errors.Is(err, ErrInvalidPublication) {
		t.Fatalf("zero publication error = %v, want %v", err, ErrInvalidPublication)
	}
	if _, err := SealPublication(primary, -1); !errors.Is(err, ErrInvalidPublication) {
		t.Fatalf("negative count error = %v, want %v", err, ErrInvalidPublication)
	}
	foreign := issuedDefinition(issuedToken(0x0100_00FF, 0, 1))
	fresh, err := NewPublisher(CatalogSchema())
	if err != nil {
		t.Fatal(err)
	}
	if err := fresh.Accept(mustPublication(t, foreign, 0)); !errors.Is(err, ErrUnexpectedPublication) {
		t.Fatalf("foreign error = %v, want %v", err, ErrUnexpectedPublication)
	}
}

func TestSchemaRejectsFacetRevisionAndDetachedMutation(t *testing.T) {
	facet := generatedDefinition(t, OriginProgramFlowOperators, FacetProgramFlowUnaryNumeric)
	if _, err := NewPublisher(issuedSchema(facet)); !errors.Is(err, ErrMissingFacetParent) {
		t.Fatalf("orphan facet error = %v, want %v", err, ErrMissingFacetParent)
	}
	primary := issuedDefinition(issuedToken(0x0100_0001, 0, 1))
	otherRevision := issuedDefinition(issuedToken(0x0100_0001, 0, 2))
	if _, err := NewPublisher(issuedSchema(primary, otherRevision)); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("revision error = %v, want %v", err, ErrRevisionConflict)
	}
	schema := CatalogSchema()
	definitions := schema.Definitions()
	definitions[0] = RelationDef{}
	definition, ok := schema.DefinitionAt(0)
	if !ok || !definition.valid() {
		t.Fatal("schema changed through detached definitions")
	}
	publisher, err := NewPublisher(schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range schema.Definitions() {
		if err := publisher.Accept(mustPublication(t, definition, 0)); err != nil {
			t.Fatal(err)
		}
	}
	publications, err := publisher.Seal()
	if err != nil {
		t.Fatal(err)
	}
	measures := publications.Measures()
	measures[0] = Measure{}
	measure, ok := publications.At(0)
	if !ok || !measure.Token().valid() {
		t.Fatal("publications changed through detached measures")
	}
}

func TestTokenIdentityAndCatalogDefinitionAreExact(t *testing.T) {
	definitions := CatalogSchema().Definitions()
	if len(definitions) != 114 {
		t.Fatalf("catalog definitions = %d, want 114", len(definitions))
	}
	seen := make(map[[32]byte]Token, len(definitions))
	for _, definition := range definitions {
		token := definition.Token()
		identity, ok := token.Identity()
		if !ok {
			t.Fatalf("issued token has no identity: %#v", token)
		}
		repeated, ok := token.Identity()
		if !ok || repeated != identity {
			t.Fatalf("token identity is not deterministic: %#v", token)
		}
		if previous, exists := seen[identity]; exists {
			t.Fatalf("identity collision: %#v and %#v", previous, token)
		}
		seen[identity] = token
		resolved, ok := Definition(token.Origin(), token.Facet())
		if !ok || resolved != definition {
			t.Fatalf("definition lookup mismatch for origin=%d facet=%d", token.Origin(), token.Facet())
		}
	}

	primary := generatedDefinition(t, OriginProgramFlowStorage, 0).Token()
	facet := generatedDefinition(t, OriginProgramFlowStorage, FacetProgramFlowStorageCell).Token()
	otherRevision := issuedToken(primary.Origin(), primary.Facet(), primary.Revision()+1)
	primaryIdentity, _ := primary.Identity()
	facetIdentity, _ := facet.Identity()
	revisionIdentity, _ := otherRevision.Identity()
	if primaryIdentity == facetIdentity {
		t.Fatal("facet change did not change identity")
	}
	if primaryIdentity == revisionIdentity {
		t.Fatal("revision change did not change identity")
	}
	if _, ok := Definition(0, 0); ok {
		t.Fatal("zero definition lookup accepted")
	}
	if _, ok := Definition(primary.Origin(), Facet(0xFFFF)); ok {
		t.Fatal("unknown facet lookup accepted")
	}
	forged := Token{origin: primary.Origin(), facet: primary.Facet(), revision: primary.Revision()}
	if _, ok := forged.Identity(); ok {
		t.Fatal("forged token identity accepted")
	}
	if _, ok := (Token{}).Identity(); ok {
		t.Fatal("zero token identity accepted")
	}
}

func TestCatalogDefinitionHotLookupDoesNotAllocate(t *testing.T) {
	// Prime cold schema publication separately. Definition must not rebuild the
	// detached denominator on hot lower and assembly paths.
	if CatalogSchema().Count() == 0 {
		t.Fatal("empty generated catalog")
	}
	want, ok := Definition(OriginProgramFlowStorage, FacetProgramFlowStorageCell)
	if !ok || !want.valid() {
		t.Fatal("missing generated definition")
	}
	if allocations := testing.AllocsPerRun(1_000, func() {
		definition, ok := Definition(OriginProgramFlowStorage, FacetProgramFlowStorageCell)
		if !ok || definition != want {
			t.Fatal("missing generated definition")
		}
	}); allocations != 0 {
		t.Fatalf("hot Definition allocations = %v, want 0", allocations)
	}
}

var benchmarkCatalogDefinition RelationDef

func BenchmarkCatalogDefinition(b *testing.B) {
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		definition, ok := Definition(OriginProgramFlowStorage, FacetProgramFlowStorageCell)
		if !ok {
			b.Fatal("missing generated definition")
		}
		benchmarkCatalogDefinition = definition
	}
}

func generatedDefinition(t *testing.T, origin Origin, facet Facet) RelationDef {
	t.Helper()
	for _, definition := range CatalogSchema().Definitions() {
		token := definition.Token()
		if token.Origin() == origin && token.Facet() == facet {
			return definition
		}
	}
	t.Fatalf("missing generated definition origin=%d facet=%d", origin, facet)
	return RelationDef{}
}

func mustPublication(t *testing.T, definition RelationDef, count int) Publication {
	t.Helper()
	publication, err := SealPublication(definition, count)
	if err != nil {
		t.Fatal(err)
	}
	return publication
}
