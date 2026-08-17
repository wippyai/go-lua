package lua

import (
	"testing"

	"github.com/wippyai/go-lua/stdlib"
)

func TestNativeLibraryOpenersFollowStdlibCatalogue(t *testing.T) {
	providers := stdlib.Providers()
	if len(luaLibs) != len(providers) {
		t.Fatalf("native openers = %d, providers = %d", len(luaLibs), len(providers))
	}
	for index, provider := range providers {
		native := luaLibs[index]
		if native.libName != provider.Declaration().Path {
			t.Fatalf("native opener %d name = %q, provider = %q",
				index, native.libName, provider.Declaration().Path)
		}
		if native.libFunc == nil {
			t.Fatalf("native opener for %q is nil", provider.Identity)
		}
	}
}

func TestNativeLibraryOpenerBindingRejectsDrift(t *testing.T) {
	if got := func() (panicked bool) {
		defer func() { panicked = recover() != nil }()
		bindStandardLibraries(map[stdlib.ID]nativeLibrary{})
		return false
	}(); !got {
		t.Fatal("incomplete native opener binding did not panic")
	}
}
