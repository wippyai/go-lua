package lua

import (
	"testing"

	"github.com/wippyai/go-lua/stdlib"
)

func TestNativeLibraryOpenersFollowStdlibCatalogue(t *testing.T) {
	catalogue := stdlib.Libraries()
	if len(luaLibs) != len(catalogue) {
		t.Fatalf("native openers = %d, catalogue = %d", len(luaLibs), len(catalogue))
	}
	for index, library := range catalogue {
		native := luaLibs[index]
		if native.libName != library.Name() {
			t.Fatalf("native opener %d name = %q, catalogue = %q",
				index, native.libName, library.Name())
		}
		if native.libFunc == nil {
			t.Fatalf("native opener for %q is nil", library.ID())
		}
	}
}

func TestNativeLibraryOpenerBindingRejectsDrift(t *testing.T) {
	if got := func() (panicked bool) {
		defer func() { panicked = recover() != nil }()
		bindStandardLibraryOpeners(map[stdlib.ID]LGoFunc{})
		return false
	}(); !got {
		t.Fatal("incomplete native opener binding did not panic")
	}
}
