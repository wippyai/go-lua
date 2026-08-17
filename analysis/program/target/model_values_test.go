package target

import "testing"

func TestModelValuesPublicationDescriptorGettersRetainTypedAuthority(t *testing.T) {
	contract, owner := publicationEffectContract(t, sendPublication(PublicationMutabilityCopyOnWrite), false)
	descriptor, ok := contract.PublicationEffectDescriptor(owner, 0)
	if !ok {
		t.Fatal("publication descriptor unavailable")
	}
	if descriptor.Kind() != PublicationEffectSendTransfer || descriptor.Subject() != 0 || descriptor.Context() != 1 {
		t.Fatalf("publication descriptor identity = kind:%d subject:%d context:%d", descriptor.Kind(), descriptor.Subject(), descriptor.Context())
	}
	if descriptor.Escape() != PublicationEscapeSendTransfer || descriptor.Mutability() != PublicationMutabilityCopyOnWrite || descriptor.Lifetime() != PublicationLifetimePreserve {
		t.Fatal("publication descriptor consequences changed")
	}
}
