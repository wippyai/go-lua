package publication

import (
	"testing"
)

// This in-package law covers the private guard that both the constructor and
// validation route through, including the AllocationResult-key-ID splice case.
// The live Value/Pack owner fences are proved separately at the external
// fixture boundary, which seals real schemas.
func TestDirectAllocationSubjectPrivateKeyIDAndScalarSealLaw(t *testing.T) {
	module := publicationLawID("direct-module")
	semantic := publicationLawID("direct-semantic")
	key := publicationLawID("direct-key")
	splicedKey := publicationLawID("spliced-key")
	id := directAllocationSubjectID(module, semantic, key, 1)
	if !id.Available() || !directAllocationSubjectKeyIDMatches(key, key) || !directAllocationSubjectSealMatches(id, module, semantic, key, 1) {
		t.Fatal("direct allocation key-ID setup")
	}
	if directAllocationSubjectKeyIDMatches(key, splicedKey) {
		t.Fatal("spliced AllocationResult KeyID matched issued Heap Key")
	}
	if directAllocationSubjectSealMatches(id, module, semantic, splicedKey, 1) ||
		directAllocationSubjectSealMatches(id, module, semantic, key, 2) ||
		directAllocationSubjectSealMatches(splicedKey, module, semantic, key, 1) {
		t.Fatal("mutated stored direct receipt seal survived validation")
	}
}
