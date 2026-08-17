package semanticsource

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// testSchema is a small sealed-schema fixture for the publication laws. It is
// deliberately not a generated denominator: the production relation table is
// owned by program/relations and is exercised there.
type testSchema struct {
	definitions []RelationDef
	digest      identity.ContentID
}

func (schema *testSchema) Count() int { return len(schema.definitions) }

func (schema *testSchema) DefinitionAt(index int) (RelationDef, bool) {
	if schema == nil || index < 0 || index >= len(schema.definitions) {
		return RelationDef{}, false
	}
	return schema.definitions[index], true
}

func (schema *testSchema) Definition(origin Origin, facet Facet) (RelationDef, bool) {
	if schema == nil {
		return RelationDef{}, false
	}
	for _, definition := range schema.definitions {
		token := definition.Token()
		if token.Origin() == origin && token.Facet() == facet {
			return definition, true
		}
	}
	return RelationDef{}, false
}

func (schema *testSchema) SchemaDigest() identity.ContentID {
	if schema == nil {
		return identity.ContentID{}
	}
	return schema.digest
}

func newTestSchema(t *testing.T, definitions ...RelationDef) *testSchema {
	t.Helper()
	digest, ok := identity.DeriveContentID("semanticsource/law-schema/v1", []byte("fixture"))
	if !ok {
		t.Fatal("failed to issue fixture schema digest")
	}
	return &testSchema{definitions: append([]RelationDef(nil), definitions...), digest: digest}
}

func generatedDefinition(t *testing.T, origin Origin, facet Facet) RelationDef {
	t.Helper()
	definition, ok := Declare(origin, facet)
	if !ok {
		t.Fatalf("missing generated definition origin=%d facet=%d", origin, facet)
	}
	return definition
}

func mustPublication(t *testing.T, definition RelationDef, count int) Publication {
	t.Helper()
	publication, err := SealPublication(definition, count)
	if err != nil {
		t.Fatal(err)
	}
	return publication
}

func TestDeclareUsesTheGeneratedOriginRevisionFence(t *testing.T) {
	primary := generatedDefinition(t, OriginProgramFlowOperators, 0)
	facet := generatedDefinition(t, OriginProgramFlowOperators, FacetProgramFlowUnaryNumeric)
	if primary.Token().Revision() != facet.Token().Revision() {
		t.Fatal("primary and facet do not share the generated origin revision")
	}
	primaryIdentity, primaryOK := primary.Token().Identity()
	facetIdentity, facetOK := facet.Token().Identity()
	if !primaryOK || !facetOK || primaryIdentity == facetIdentity {
		t.Fatal("facet identity did not differ from primary identity")
	}
	repeated, ok := Declare(OriginProgramFlowOperators, FacetProgramFlowUnaryNumeric)
	if !ok || repeated != facet {
		t.Fatal("Declare is not deterministic")
	}
	if _, ok := Declare(Origin(0xDEAD_BEEF), 0); ok {
		t.Fatal("unknown origin accepted")
	}
	if _, ok := Declare(0, 0); ok {
		t.Fatal("zero origin accepted")
	}
}

func TestPublisherConsumesOneProgramSchemaAndPublishesOnlyItsDigest(t *testing.T) {
	primary := generatedDefinition(t, OriginProgramFlowOperators, 0)
	facet := generatedDefinition(t, OriginProgramFlowOperators, FacetProgramFlowUnaryNumeric)
	other := generatedDefinition(t, OriginLinkBoundary, 0)
	schema := newTestSchema(t, other, facet, primary)
	publisher, err := NewPublisher(schema)
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
	publications, err := publisher.Seal()
	if err != nil {
		t.Fatal(err)
	}
	if publications.SchemaDigest() != schema.SchemaDigest() {
		t.Fatal("publication lost the schema digest")
	}
	if publications.Count() != schema.Count() {
		t.Fatalf("publication count = %d, want %d", publications.Count(), schema.Count())
	}
	for index := 1; index < publications.Count(); index++ {
		previous, _ := publications.At(index - 1)
		current, _ := publications.At(index)
		if compareToken(previous.Token(), current.Token()) >= 0 {
			t.Fatalf("measures are not canonical at %d", index)
		}
	}
	measures := publications.Measures()
	measures[0] = Measure{}
	if measure, ok := publications.At(0); !ok || !measure.Token().valid() {
		t.Fatal("published measures are not detached")
	}
	clone := publications.Clone()
	if clone.SchemaDigest() != publications.SchemaDigest() || clone.Count() != publications.Count() {
		t.Fatal("clone changed the published denominator")
	}
}

func TestPublisherRejectsDuplicateMissingForeignAndInvalidValues(t *testing.T) {
	primary := generatedDefinition(t, OriginProgramSourceProvenance, 0)
	other := generatedDefinition(t, OriginProgramSourceOrder, 0)
	schema := newTestSchema(t, primary, other)
	publisher, err := NewPublisher(schema)
	if err != nil {
		t.Fatal(err)
	}
	publication := mustPublication(t, primary, 0)
	if err := publisher.Accept(publication); err != nil {
		t.Fatal(err)
	}
	if err := publisher.Accept(publication); !errors.Is(err, ErrDuplicatePublication) {
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
	foreign, ok := Declare(OriginProgramFlowCall, 0)
	if !ok {
		t.Fatal("failed to issue foreign fixture definition")
	}
	fresh, err := NewPublisher(schema)
	if err != nil {
		t.Fatal(err)
	}
	if err := fresh.Accept(mustPublication(t, foreign, 0)); !errors.Is(err, ErrUnexpectedPublication) {
		t.Fatalf("foreign error = %v, want %v", err, ErrUnexpectedPublication)
	}
}

func TestProgramSchemaRejectsInvalidDenominators(t *testing.T) {
	primary := generatedDefinition(t, OriginProgramSourceProvenance, 0)
	facet := generatedDefinition(t, OriginProgramFlowOperators, FacetProgramFlowUnaryNumeric)
	if _, err := NewPublisher(nil); !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("nil schema error = %v, want %v", err, ErrInvalidSchema)
	}
	if _, err := NewPublisher(newTestSchema(t, facet)); !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("orphan facet error = %v, want %v", err, ErrInvalidSchema)
	}
	invalid := newTestSchema(t, primary)
	invalid.digest = identity.ContentID{}
	if _, err := NewPublisher(invalid); !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("unavailable digest error = %v, want %v", err, ErrInvalidSchema)
	}
}

func TestOriginPublicationsUsesTheProvidedProgramSchema(t *testing.T) {
	primary := generatedDefinition(t, OriginProgramFlowOperators, 0)
	facet := generatedDefinition(t, OriginProgramFlowOperators, FacetProgramFlowUnaryNumeric)
	other := generatedDefinition(t, OriginLinkBoundary, 0)
	schema := newTestSchema(t, primary, facet, other)
	publications := OriginPublications(schema, func(token Token) (int, bool) {
		if token == primary.Token() {
			return 3, true
		}
		return 0, true
	}, OriginProgramFlowOperators)
	if len(publications) != 2 {
		t.Fatalf("flow publications = %d, want 2", len(publications))
	}
	for _, publication := range publications {
		if publication.Definition().Token().Origin() != OriginProgramFlowOperators {
			t.Fatalf("foreign origin published: %v", publication.Definition().Token())
		}
	}
}

func TestForgedAndZeroTokensCannotCrossIdentityBoundary(t *testing.T) {
	definition := generatedDefinition(t, OriginProgramFlowStorage, 0)
	token := definition.Token()
	if _, ok := token.Identity(); !ok {
		t.Fatal("issued token has no identity")
	}
	forged := Token{origin: token.origin, facet: token.facet, revision: token.revision}
	if _, ok := forged.Identity(); ok {
		t.Fatal("forged token identity accepted")
	}
	if _, ok := (Token{}).Identity(); ok {
		t.Fatal("zero token identity accepted")
	}
}
