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
	// are both part of Target semantic identity.
	const want = "eae71f78938c58755d76710ac8cc8c870240bba2b3aa676f095bf7f24e657ee7"
	if got := contract.ContentID().String(); got != want {
		t.Fatalf("standard-library Target identity = %s, want %s", got, want)
	}
}
