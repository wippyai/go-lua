package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/formal"
)

func diagnosticPublicationFixture(t *testing.T) (DiagnosticDescriptor, DiagnosticDescriptor, DeclaredCheckContext, formal.Root, ContentID, ContentID, ContentID) {
	t.Helper()
	provider := registryTestOwner(61)
	formalBoundary := formal.NewRoot(provider, 1, formal.Output)
	declared := DeclaredCheckContext{
		Artifact: contentID([]byte("provider-artifact")), Body: provider, Registry: contentID([]byte("provider-registry")),
	}
	callee := DiagnosticDescriptor{
		Candidate: "provider-body-failure", Owner: DiagnosticOwnerCalleeCheck, SourceAnchor: contentID([]byte("provider-source")),
		Predicate: "provider-fails", EvidenceRecipe: "provider-evidence",
	}
	application := DiagnosticDescriptor{
		Candidate: "imported-require-obligation", Owner: DiagnosticOwnerApplication, SourceAnchor: contentID([]byte("provider-source")),
		GuardAtoms: []string{"provider-absent"}, Predicate: "caller-requires-provider", EvidenceRecipe: "provider-evidence", BoundaryLens: "provider-output",
	}
	return callee, application, declared, formalBoundary, contentID([]byte("formal-guard")), contentID([]byte("predicate")), contentID([]byte("evidence"))
}

func diagnosticApplicationContext(t *testing.T, formalBoundary formal.Root) BoundApplicationContext {
	t.Helper()
	caller := registryTestOwner(62)
	context := BoundApplicationContext{
		CallerArtifact: contentID([]byte("caller-artifact")), CallAnchor: contentID([]byte("call-anchor")), Binding: contentID([]byte("exact-boundary-binding")),
		Lenses: []BoundaryLens{{Formal: formalBoundary, Caller: formal.NewRoot(caller, 1, formal.Input)}},
	}
	if context.CanonicalBytes() == nil {
		t.Fatal("fixture has invalid bound application context")
	}
	return context
}

func TestDiagnosticPublisherCalleeCheckPublishesOnceForAllImporters(t *testing.T) {
	callee, _, declared, _, _, _, _ := diagnosticPublicationFixture(t)
	publisher := NewDiagnosticPublisher()
	first, published, err := publisher.PublishCalleeCheck(callee, declared, true)
	if err != nil || !published || first.Owner != DiagnosticOwnerCalleeCheck {
		t.Fatalf("first callee publication = %#v, %t, %v", first, published, err)
	}
	second, published, err := publisher.PublishCalleeCheck(callee, declared, true)
	if err != nil || published || second.Descriptor != first.Descriptor || second.Owner != first.Owner || second.Declared.ContentID() != first.Declared.ContentID() {
		t.Fatalf("duplicate callee publication = %#v, %t, %v", second, published, err)
	}
	if _, published, err := publisher.PublishCalleeCheck(callee, declared, false); err != nil || published {
		t.Fatalf("unreportable callee check publication = %t, %v", published, err)
	}
}

func TestDiagnosticPublisherAppliesImportedResidualOnlyWithExactProvenFeasibility(t *testing.T) {
	_, descriptor, declared, formalBoundary, guard, predicate, evidence := diagnosticPublicationFixture(t)
	residual, err := NewApplicationResidual(descriptor, declared, formalBoundary, guard, predicate, evidence, true)
	if err != nil {
		t.Fatal(err)
	}
	bound := diagnosticApplicationContext(t, formalBoundary)
	publisher := NewDiagnosticPublisher()
	proof := FeasibilityCertificate{
		Descriptor: residual.Descriptor(), BoundState: contentID([]byte("bound-guarded-state")), Guard: guard, Binding: bound.Binding,
		Application: bound.ContentID(), Verdict: FeasibilityProven,
	}
	publication, published, err := publisher.PublishApplication(residual, bound, proof)
	if err != nil || !published || publication.Owner != DiagnosticOwnerApplication {
		t.Fatalf("proven imported residual publication = %#v, %t, %v", publication, published, err)
	}
	if _, published, err := publisher.PublishApplication(residual, bound, proof); err != nil || published {
		t.Fatalf("duplicate caller publication = %t, %v", published, err)
	}

	for _, verdict := range []FeasibilityVerdict{FeasibilityInfeasible, FeasibilityPossiblyFeasible} {
		candidate := proof
		candidate.Verdict = verdict
		if _, published, err := NewDiagnosticPublisher().PublishApplication(residual, bound, candidate); err != nil || published {
			t.Fatalf("%s residual publication = %t, %v; want silent", verdict, published, err)
		}
	}
	wrongGuard := proof
	wrongGuard.Guard = contentID([]byte("other-guard"))
	if _, _, err := NewDiagnosticPublisher().PublishApplication(residual, bound, wrongGuard); err == nil {
		t.Fatal("publication accepted feasibility proof for a different guarded residual")
	}
	wrongApplication := proof
	wrongApplication.Application = contentID([]byte("other-bound-application"))
	if _, _, err := NewDiagnosticPublisher().PublishApplication(residual, bound, wrongApplication); err == nil {
		t.Fatal("publication accepted feasibility proof from a different bound application")
	}
}

func TestApplicationResidualIsPortableAndRequiresItsBoundaryLens(t *testing.T) {
	_, descriptor, declared, formalBoundary, guard, predicate, evidence := diagnosticPublicationFixture(t)
	residual, err := NewApplicationResidual(descriptor, declared, formalBoundary, guard, predicate, evidence, true)
	if err != nil {
		t.Fatal(err)
	}
	bytes := residual.CanonicalBytes()
	if len(bytes) == 0 || !residual.ContentID().Valid() {
		t.Fatal("portable residual has no content identity")
	}
	bound := diagnosticApplicationContext(t, formalBoundary)
	bound.CallerArtifact = contentID([]byte("different-importer"))
	if string(bytes) != string(residual.CanonicalBytes()) {
		t.Fatal("caller application leaked into portable residual bytes")
	}
	bound.Lenses = nil
	proof := FeasibilityCertificate{
		Descriptor: residual.Descriptor(), BoundState: contentID([]byte("bound-state")), Guard: guard, Binding: bound.Binding, Application: bound.ContentID(), Verdict: FeasibilityProven,
	}
	if _, _, err := NewDiagnosticPublisher().PublishApplication(residual, bound, proof); err == nil {
		t.Fatal("application publication accepted residual without its frozen BoundaryLens")
	}
}

func TestImportedRequireAdjudicationIsNotProviderGreenGating(t *testing.T) {
	callee, descriptor, declared, formalBoundary, guard, predicate, evidence := diagnosticPublicationFixture(t)
	publisher := NewDiagnosticPublisher()
	if _, published, err := publisher.PublishCalleeCheck(callee, declared, true); err != nil || !published {
		t.Fatalf("provider body failure = %t, %v", published, err)
	}
	residual, err := NewApplicationResidual(descriptor, declared, formalBoundary, guard, predicate, evidence, true)
	if err != nil {
		t.Fatal(err)
	}
	bound := diagnosticApplicationContext(t, formalBoundary)
	possible := FeasibilityCertificate{
		Descriptor: residual.Descriptor(), BoundState: contentID([]byte("importer-state")), Guard: guard, Binding: bound.Binding, Application: bound.ContentID(), Verdict: FeasibilityPossiblyFeasible,
	}
	if _, published, err := publisher.PublishApplication(residual, bound, possible); err != nil || published {
		t.Fatalf("importer with only a widened possible route = %t, %v; want no caller publication", published, err)
	}
	proven := possible
	proven.Verdict = FeasibilityProven
	if _, published, err := publisher.PublishApplication(residual, bound, proven); err != nil || !published {
		t.Fatalf("importer with a proven failing route = %t, %v; want caller publication", published, err)
	}
}
