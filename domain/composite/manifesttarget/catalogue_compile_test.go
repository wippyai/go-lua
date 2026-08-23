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
	// Formal ownership, the explicit authored module-path-to-root relation,
	// and the complete qualified type directory are all part of Target
	// semantic identity. The digest moved when the target type vocabulary
	// became an owned, canonically encoded read surface.
	const want = "e29837dc21373dbb1be0cc122cfffc09ac68c9a1776352860b02ed5192f7a59e"
	if got := contract.ContentID().String(); got != want {
		t.Fatalf("standard-library Target identity = %s, want %s", got, want)
	}
}
