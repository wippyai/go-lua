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
	// protocol requirement relation entered the sealed contract: every protocol
	// now frames its requirement rows, and the codec version that fences the
	// preceding layout advanced with it.
	const want = "fa73c5de46e0cce5689e11048e067b16f9c5b0b7adb623214621213ed1ba4b9f"
	if got := contract.ContentID().String(); got != want {
		t.Fatalf("standard-library Target identity = %s, want %s", got, want)
	}
}
