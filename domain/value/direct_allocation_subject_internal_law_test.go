package value

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

// This in-package law intentionally stays fixture-free: grammar imports
// Value, so a full Link/Artifact setup here would form an import cycle. It
// covers the private guard that both the constructor and validation route
// through, including the AllocationResult-key-ID splice case.
func TestDirectAllocationSubjectPrivateKeyIDAndScalarSealLaw(t *testing.T) {
	module := identity.ContentID(sha256.Sum256([]byte("direct-module")))
	semantic := identity.ContentID(sha256.Sum256([]byte("direct-semantic")))
	key := identity.ContentID(sha256.Sum256([]byte("direct-key")))
	splicedKey := identity.ContentID(sha256.Sum256([]byte("spliced-key")))
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
