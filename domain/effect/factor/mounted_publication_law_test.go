package factor_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
)

// TestMountedPublicationIssuesTypedOrdinaryReceipt keeps the ordinary
// publication row on the exact selected Effect path.  In particular, the
// receipt must retain the descriptor consequences and Pack's mounted source
// rows rather than a Target formal ordinal that a later owner would need to
// reinterpret.
func TestMountedPublicationIssuesTypedOrdinaryReceipt(t *testing.T) {
	fixture := newEffectFactorFixture(t, publicationEffectFactorSpec(vocabulary.PublicationEffectSendTransfer, false), "local function sink(left, right) return left end\nsink(1, 2)")
	publications, ok := fixture.factor.SelectedCallMountedPublications(fixture.root, fixture.mountedCall, fixture.owner)
	if !ok || len(publications) != 1 {
		t.Fatalf("SelectedCallMountedPublications() = %d/%v, want one receipt", len(publications), ok)
	}
	publication := publications[0]
	if !publication.Valid() || publication.Role() != effectfactor.MountedPublicationOrdinary || publication.Kind() != vocabulary.PublicationEffectSendTransfer || publication.Escape() != vocabulary.PublicationEscapeSendTransfer || publication.Mutability() != vocabulary.PublicationMutabilityCopyOnWrite || publication.Lifetime() != vocabulary.PublicationLifetimePreserve {
		t.Fatal("ordinary receipt lost typed publication consequences")
	}
	module, occurrence, provenanceOK := publication.CallProvenance()
	if !provenanceOK {
		t.Fatal("ordinary receipt lost mounted call provenance")
	}
	_, expectedModule, expectedOccurrence, identityOK := fixture.factor.MountedCallIdentity(fixture.mountedCall)
	if !identityOK || module != expectedModule || occurrence != expectedOccurrence {
		t.Fatal("ordinary receipt carried incorrect mounted provenance")
	}
	subject, subjectOK := publication.SubjectInput()
	context, contextOK := publication.ContextInput()
	if !subjectOK || !contextOK || !subject.Valid() || !context.Valid() {
		t.Fatal("ordinary send receipt lost subject/context mounted inputs")
	}
	subjectSource, subjectSourceOK := subject.Source()
	contextSource, contextSourceOK := context.Source()
	if !subjectSourceOK || !contextSourceOK || subjectSource.Kind != vocabulary.InputSourceValueFormal || contextSource.Kind != vocabulary.InputSourceValueFormal || subjectSource.Ordinal != 1 || contextSource.Ordinal != 0 {
		t.Fatal("ordinary receipt did not preserve Pack ABI mapping")
	}
	descriptorID, descriptorOK := publication.DescriptorID()
	occurrenceID, occurrenceOK := publication.OccurrenceID()
	atom, atomOK := publication.AtomBinding()
	if !descriptorOK || !occurrenceOK || !atomOK || !atom.MatchesCertificate(atomID(t, atom)) || !descriptorID.Available() || !occurrenceID.Available() {
		t.Fatal("ordinary receipt lost descriptor/occurrence/atom identity")
	}
	if got, ok := publication.ContentID(); !ok || !got.Available() {
		t.Fatal("ordinary receipt did not expose a sealed identity")
	}
}

// TestMountedPublicationKeepsCallbackProvenanceDistinct proves that a
// callback publication cannot be retyped as an ordinary publication merely
// because its effect atom is in the same semantic quotient.
func TestMountedPublicationKeepsCallbackProvenanceDistinct(t *testing.T) {
	fixture := newEffectFactorFixture(t, publicationEffectFactorSpec(vocabulary.PublicationEffectCallbackEscape, true), "local function sink(left, right) return left end\nsink(1, 2)")
	publications, ok := fixture.factor.SelectedCallMountedPublications(fixture.root, fixture.mountedCall, fixture.owner)
	if !ok || len(publications) != 2 {
		t.Fatalf("SelectedCallMountedPublications() = %d/%v, want ordinary + callback receipts", len(publications), ok)
	}
	var ordinary, callback int
	for index, publication := range publications {
		switch publication.Role() {
		case effectfactor.MountedPublicationOrdinary:
			ordinary++
		case effectfactor.MountedPublicationCallback:
			callback++
			if publication.Callback() == 0 {
				t.Fatal("callback receipt lost callback identity")
			}
		default:
			t.Fatalf("publication %d has invalid role %v", index, publication.Role())
		}
	}
	if ordinary != 1 || callback != 1 {
		t.Fatalf("ordinary/callback receipt counts = %d/%d, want 1/1", ordinary, callback)
	}
}

// atomID obtains the existing atom certificate without constructing a new
// Atom.  The helper intentionally uses only the public AtomBinding bridge.
func atomID(t testing.TB, binding effectfactor.AtomBinding) (id identity.ContentID) {
	t.Helper()
	formal, ok := binding.Formal()
	if !ok {
		t.Fatal("atom binding has no formal certificate")
	}
	id, ok = formal.ContentID()
	if !ok {
		t.Fatal("formal atom has no certificate")
	}
	return id
}
