package manifesttarget_test

import (
	"testing"

	"github.com/wippyai/go-lua/domain/composite/manifesttarget"
	"github.com/wippyai/go-lua/manifest"
	"github.com/wippyai/go-lua/stdlib"
)

// TestCompileCatalogueStandardLibraryContentIdentity pins the canonical
// semantic identity of the provider-to-Target compilation. This is a
// cutover gate for the sealed ABI, not a duplicate provider inventory.
func TestCompileCatalogueStandardLibraryContentIdentity(t *testing.T) {
	catalogue, err := manifest.Seal(stdlib.Providers()...)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := manifesttarget.SealCatalogue(catalogue)
	if err != nil {
		t.Fatal(err)
	}
	// Formal ownership and the explicit authored module-path-to-root relation
	// are both part of Target semantic identity. The digest moved when the
	// sealed class vocabulary entered the contract as its semantic column:
	// the target now carries the decoded, canonically encoded denominator its
	// declarations name, and the codec version that fences the preceding
	// layout advanced with it.
	const want = "a79ac2e8feac4f51442f9188fcd6ac0a9fbc2947c182b1c7ca7cff506e2aade0"
	if got := contract.ContentID().String(); got != want {
		t.Fatalf("standard-library Target identity = %s, want %s", got, want)
	}
}
