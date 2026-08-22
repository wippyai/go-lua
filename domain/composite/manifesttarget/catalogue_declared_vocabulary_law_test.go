package manifesttarget_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
)

// publicationConsequence is the exact typed consequence tuple one
// PublicationEffectKind admits. The seal owns the relation
// (analysis/program/target/operation/publication.go); this table states the
// same law from the manifest author's side, so every kind the wire vocabulary
// offers has a manifest that declares it and a sealed Target that answers it.
type publicationConsequence struct {
	kind        manifestwire.PublicationEffectKind
	destination manifestwire.PublicationDestinationRole
	context     manifestwire.ValueFormal
	escape      manifestwire.PublicationEscapeDisposition
	mutability  manifestwire.PublicationMutabilityDisposition
	lifetime    manifestwire.PublicationLifetimeDisposition

	wantKind        vocabulary.PublicationEffectKind
	wantDestination vocabulary.PublicationDestinationRole
	wantEscape      vocabulary.PublicationEscapeDisposition
	wantMutability  vocabulary.PublicationMutabilityDisposition
	wantLifetime    vocabulary.PublicationLifetimeDisposition
}

// declaredPublicationConsequences names every admitted (kind, consequence)
// pair in the wire publication vocabulary. Each row is a real manifest
// declaration: the test seals it through SealCatalogue and reads it back out
// of the sealed Target.
func declaredPublicationConsequences() map[string]publicationConsequence {
	return map[string]publicationConsequence{
		"send-transfer-copy-on-write": {
			kind:        manifestwire.PublicationEffectSendTransfer,
			destination: manifestwire.PublicationDestinationValueFormal,
			context:     1,
			escape:      manifestwire.PublicationEscapeSendTransfer,
			mutability:  manifestwire.PublicationMutabilityCopyOnWrite,
			lifetime:    manifestwire.PublicationLifetimePreserve,

			wantKind:        vocabulary.PublicationEffectSendTransfer,
			wantDestination: vocabulary.PublicationDestinationValueFormal,
			wantEscape:      vocabulary.PublicationEscapeSendTransfer,
			wantMutability:  vocabulary.PublicationMutabilityCopyOnWrite,
			wantLifetime:    vocabulary.PublicationLifetimePreserve,
		},
		"send-transfer-preserve": {
			kind:        manifestwire.PublicationEffectSendTransfer,
			destination: manifestwire.PublicationDestinationValueFormal,
			context:     1,
			escape:      manifestwire.PublicationEscapeSendTransfer,
			mutability:  manifestwire.PublicationMutabilityPreserve,
			lifetime:    manifestwire.PublicationLifetimePreserve,

			wantKind:        vocabulary.PublicationEffectSendTransfer,
			wantDestination: vocabulary.PublicationDestinationValueFormal,
			wantEscape:      vocabulary.PublicationEscapeSendTransfer,
			wantMutability:  vocabulary.PublicationMutabilityPreserve,
			wantLifetime:    vocabulary.PublicationLifetimePreserve,
		},
		// A value that leaves through the callable's own return escapes to the
		// caller's owned heap; it is neither sealed nor released on the way out.
		"return-escape": {
			kind:        manifestwire.PublicationEffectReturnEscape,
			destination: manifestwire.PublicationDestinationNone,
			escape:      manifestwire.PublicationEscapeReturn,
			mutability:  manifestwire.PublicationMutabilityPreserve,
			lifetime:    manifestwire.PublicationLifetimePreserve,

			wantKind:        vocabulary.PublicationEffectReturnEscape,
			wantDestination: vocabulary.PublicationDestinationNone,
			wantEscape:      vocabulary.PublicationEscapeReturn,
			wantMutability:  vocabulary.PublicationMutabilityPreserve,
			wantLifetime:    vocabulary.PublicationLifetimePreserve,
		},
		// A value handed to a provider-invoked callback escapes the same way a
		// returned value does, and the provider keeps the caller's mutability.
		"callback-escape": {
			kind:        manifestwire.PublicationEffectCallbackEscape,
			destination: manifestwire.PublicationDestinationNone,
			escape:      manifestwire.PublicationEscapeCallback,
			mutability:  manifestwire.PublicationMutabilityPreserve,
			lifetime:    manifestwire.PublicationLifetimePreserve,

			wantKind:        vocabulary.PublicationEffectCallbackEscape,
			wantDestination: vocabulary.PublicationDestinationNone,
			wantEscape:      vocabulary.PublicationEscapeCallback,
			wantMutability:  vocabulary.PublicationMutabilityPreserve,
			wantLifetime:    vocabulary.PublicationLifetimePreserve,
		},
		// A freeze seals its subject in place: nothing escapes, the lifetime is
		// untouched, and the mutability transition is the whole consequence.
		"freeze-seal": {
			kind:        manifestwire.PublicationEffectFreezeSeal,
			destination: manifestwire.PublicationDestinationNone,
			escape:      manifestwire.PublicationEscapeNone,
			mutability:  manifestwire.PublicationMutabilitySeal,
			lifetime:    manifestwire.PublicationLifetimePreserve,

			wantKind:        vocabulary.PublicationEffectFreezeSeal,
			wantDestination: vocabulary.PublicationDestinationNone,
			wantEscape:      vocabulary.PublicationEscapeNone,
			wantMutability:  vocabulary.PublicationMutabilitySeal,
			wantLifetime:    vocabulary.PublicationLifetimePreserve,
		},
		"write-mutation-write": {
			kind:        manifestwire.PublicationEffectWriteMutation,
			destination: manifestwire.PublicationDestinationNone,
			escape:      manifestwire.PublicationEscapeNone,
			mutability:  manifestwire.PublicationMutabilityWrite,
			lifetime:    manifestwire.PublicationLifetimePreserve,

			wantKind:        vocabulary.PublicationEffectWriteMutation,
			wantDestination: vocabulary.PublicationDestinationNone,
			wantEscape:      vocabulary.PublicationEscapeNone,
			wantMutability:  vocabulary.PublicationMutabilityWrite,
			wantLifetime:    vocabulary.PublicationLifetimePreserve,
		},
		"write-mutation-copy-on-write": {
			kind:        manifestwire.PublicationEffectWriteMutation,
			destination: manifestwire.PublicationDestinationNone,
			escape:      manifestwire.PublicationEscapeNone,
			mutability:  manifestwire.PublicationMutabilityCopyOnWrite,
			lifetime:    manifestwire.PublicationLifetimePreserve,

			wantKind:        vocabulary.PublicationEffectWriteMutation,
			wantDestination: vocabulary.PublicationDestinationNone,
			wantEscape:      vocabulary.PublicationEscapeNone,
			wantMutability:  vocabulary.PublicationMutabilityCopyOnWrite,
			wantLifetime:    vocabulary.PublicationLifetimePreserve,
		},
		// A close releases its subject's lifetime and nothing else: the value
		// does not escape and its mutability is unchanged by the release.
		"close-release": {
			kind:        manifestwire.PublicationEffectCloseRelease,
			destination: manifestwire.PublicationDestinationNone,
			escape:      manifestwire.PublicationEscapeNone,
			mutability:  manifestwire.PublicationMutabilityPreserve,
			lifetime:    manifestwire.PublicationLifetimeRelease,

			wantKind:        vocabulary.PublicationEffectCloseRelease,
			wantDestination: vocabulary.PublicationDestinationNone,
			wantEscape:      vocabulary.PublicationEscapeNone,
			wantMutability:  vocabulary.PublicationMutabilityPreserve,
			wantLifetime:    vocabulary.PublicationLifetimeRelease,
		},
	}
}

func (consequence publicationConsequence) spec() *manifestwire.PublicationEffectSpec {
	return &manifestwire.PublicationEffectSpec{
		Kind:        consequence.kind,
		Subject:     manifestwire.InputSource{Kind: manifestwire.InputSourceValue, Ordinal: 0},
		Destination: consequence.destination,
		Context:     consequence.context,
		Escape:      consequence.escape,
		Mutability:  consequence.mutability,
		Lifetime:    consequence.lifetime,
	}
}

// TestManifestDeclaresEveryPublicationEffectConsequence is the positive law:
// every publication consequence the wire vocabulary offers is declared by a
// real manifest, survives SealCatalogue, and is answered by the sealed Target
// with the exact dispositions the author stated.
func TestManifestDeclaresEveryPublicationEffectConsequence(t *testing.T) {
	for name, consequence := range declaredPublicationConsequences() {
		t.Run(name, func(t *testing.T) {
			contract := sealPublicationCatalogue(t, consequence.spec())
			sink, ok := contract.Operations.Lookup(publicationBinding("sink"))
			if !ok {
				t.Fatal("sink operation missing")
			}
			descriptor, ok := contract.Operations.EffectPublication(sink, 0)
			if !ok || !descriptor.Valid() {
				t.Fatalf("publication descriptor = %#v/%t, want a valid sealed descriptor", descriptor, ok)
			}
			if descriptor.Kind() != consequence.wantKind {
				t.Fatalf("kind = %d, want %d", descriptor.Kind(), consequence.wantKind)
			}
			if descriptor.DestinationRole() != consequence.wantDestination {
				t.Fatalf("destination = %d, want %d", descriptor.DestinationRole(), consequence.wantDestination)
			}
			if descriptor.Escape() != consequence.wantEscape {
				t.Fatalf("escape = %d, want %d", descriptor.Escape(), consequence.wantEscape)
			}
			if descriptor.Mutability() != consequence.wantMutability {
				t.Fatalf("mutability = %d, want %d", descriptor.Mutability(), consequence.wantMutability)
			}
			if descriptor.Lifetime() != consequence.wantLifetime {
				t.Fatalf("lifetime = %d, want %d", descriptor.Lifetime(), consequence.wantLifetime)
			}
			if descriptor.Subject() != (vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 0}) {
				t.Fatalf("subject = %+v, want the declared value formal", descriptor.Subject())
			}
			if consequence.wantDestination == vocabulary.PublicationDestinationValueFormal {
				if descriptor.Context() != vocabulary.ValueFormal(consequence.context) {
					t.Fatalf("context = %d, want %d", descriptor.Context(), consequence.context)
				}
			} else if descriptor.Context() != 0 {
				t.Fatalf("destination-free context = %d, want 0", descriptor.Context())
			}
			identity, identityOK := contract.Operations.PublicationEffectDescriptorID(sink, 0)
			if !identityOK || !identity.Available() {
				t.Fatal("sealed publication carries no descriptor identity")
			}
		})
	}
}

// consequenceKey addresses one admitted publication tuple. The seal relation
// is a conjunction over the three disposition axes, so the tuple - not the
// row that names it - is the identity a perturbation must be checked against.
type consequenceKey struct {
	kind       manifestwire.PublicationEffectKind
	escape     manifestwire.PublicationEscapeDisposition
	mutability manifestwire.PublicationMutabilityDisposition
	lifetime   manifestwire.PublicationLifetimeDisposition
}

func (consequence publicationConsequence) key() consequenceKey {
	return consequenceKey{
		kind: consequence.kind, escape: consequence.escape,
		mutability: consequence.mutability, lifetime: consequence.lifetime,
	}
}

// TestManifestPublicationConsequencesAreExact is the negative law: each kind
// admits exactly the consequence tuples stated above. Because the seal
// relation is a conjunction of per-axis conditions, perturbing one axis at a
// time from an admitted tuple states its exactness completely. Every
// perturbation is refused by name at seal rather than silently accepted and
// reinterpreted.
func TestManifestPublicationConsequencesAreExact(t *testing.T) {
	escapes := []manifestwire.PublicationEscapeDisposition{
		manifestwire.PublicationEscapeNone,
		manifestwire.PublicationEscapeSendTransfer,
		manifestwire.PublicationEscapeReturn,
		manifestwire.PublicationEscapeCallback,
	}
	mutabilities := []manifestwire.PublicationMutabilityDisposition{
		manifestwire.PublicationMutabilityPreserve,
		manifestwire.PublicationMutabilitySeal,
		manifestwire.PublicationMutabilityWrite,
		manifestwire.PublicationMutabilityCopyOnWrite,
	}
	lifetimes := []manifestwire.PublicationLifetimeDisposition{
		manifestwire.PublicationLifetimePreserve,
		manifestwire.PublicationLifetimeRelease,
	}
	declared := declaredPublicationConsequences()
	admitted := map[consequenceKey]bool{}
	for _, consequence := range declared {
		admitted[consequence.key()] = true
	}
	checked := map[consequenceKey]bool{}
	for name, consequence := range declared {
		t.Run(name, func(t *testing.T) {
			perturbations := make([]publicationConsequence, 0, len(escapes)+len(mutabilities)+len(lifetimes))
			for _, escape := range escapes {
				candidate := consequence
				candidate.escape = escape
				perturbations = append(perturbations, candidate)
			}
			for _, mutability := range mutabilities {
				candidate := consequence
				candidate.mutability = mutability
				perturbations = append(perturbations, candidate)
			}
			for _, lifetime := range lifetimes {
				candidate := consequence
				candidate.lifetime = lifetime
				perturbations = append(perturbations, candidate)
			}
			for _, candidate := range perturbations {
				key := candidate.key()
				if admitted[key] || checked[key] {
					continue
				}
				checked[key] = true
				_, err := sealPublicationCatalogueErr(candidate.spec())
				if err == nil {
					t.Fatalf("kind %d admitted escape %d / mutability %d / lifetime %d, want a named refusal",
						key.kind, key.escape, key.mutability, key.lifetime)
				}
				if !strings.Contains(err.Error(), "kind and typed consequences disagree") {
					t.Fatalf("kind %d escape %d mutability %d lifetime %d refused with %v, want the named consequence refusal",
						key.kind, key.escape, key.mutability, key.lifetime, err)
				}
			}
		})
	}
}

// A destination-free publication states no destination context. Carrying one
// anyway is a declaration error, not a field the seal may drop.
func TestManifestDestinationFreePublicationRefusesAContextFormal(t *testing.T) {
	for name, consequence := range declaredPublicationConsequences() {
		if consequence.destination != manifestwire.PublicationDestinationNone {
			continue
		}
		t.Run(name, func(t *testing.T) {
			candidate := consequence
			candidate.context = 1
			_, err := sealPublicationCatalogueErr(candidate.spec())
			if err == nil || !strings.Contains(err.Error(), "destination-free publication carries context formal") {
				t.Fatalf("SealCatalogue error = %v, want the named destination-free context refusal", err)
			}
		})
	}
}
