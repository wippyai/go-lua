package lua

import (
	"maps"
	"slices"
	"testing"

	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/stdlib"
)

func TestNativeStdlibCallableInventoryMatchesProviderManifests(t *testing.T) {
	for id, native := range nativeStdlibCallableInventory() {
		declared, ok := stdlib.Signatures(id)
		if !ok {
			t.Fatalf("no declarations for %q", id)
		}
		nativeNames := slices.Sorted(maps.Keys(native))
		declaredNames := slices.Sorted(maps.Keys(declared))
		if !slices.Equal(nativeNames, declaredNames) {
			t.Fatalf("%q callable drift\nnative:   %v\ndeclared: %v", id, nativeNames, declaredNames)
		}
	}
}

func TestNativeStdlibNonCallableInventoryMatchesProviderManifests(t *testing.T) {
	values := map[stdlib.ID][]string{
		stdlib.Package: {"preload", "loaders", "loaded", "path", "cpath", "config"},
		stdlib.Base:    {"_G", "_VERSION", "_GOPHER_LUA_VERSION"},
		stdlib.String:  {"__index"},
		stdlib.Math:    {"pi", "huge", "maxinteger", "mininteger"},
		stdlib.UTF8:    {"charpattern"},
		stdlib.Errors: {
			"NOT_FOUND", "ALREADY_EXISTS", "INVALID", "PERMISSION_DENIED",
			"UNAVAILABLE", "INTERNAL", "CANCELED", "CONFLICT", "TIMEOUT",
			"RATE_LIMITED", "UNKNOWN",
		},
	}
	for _, library := range stdlib.Libraries() {
		m, _ := stdlib.Manifest(library.ID())
		export := m.Export.(*typ.Record)
		callables, _ := stdlib.Signatures(library.ID())
		wantValues := values[library.ID()]
		if len(export.Fields) != len(callables)+len(wantValues) {
			t.Fatalf("%q direct fields = %d, want %d callables + %d values",
				library.ID(), len(export.Fields), len(callables), len(wantValues))
		}
		for _, name := range wantValues {
			if export.GetField(name) == nil {
				t.Errorf("%q manifest omitted native value %q", library.ID(), name)
			}
		}
	}
}

func nativeStdlibCallableInventory() map[stdlib.ID]map[string]LGoFunc {
	base := maps.Clone(baseFuncs)
	base["ipairs"] = baseIpairs
	base["pairs"] = basePairs

	strings := maps.Clone(strFuncs)
	strings["gmatch"] = strGmatch
	strings["gfind"] = strGmatch

	return map[stdlib.ID]map[string]LGoFunc{
		stdlib.Package:   loFuncs,
		stdlib.Base:      base,
		stdlib.Table:     tableFuncs,
		stdlib.String:    strings,
		stdlib.Math:      mathFuncs,
		stdlib.Debug:     debugFuncs,
		stdlib.Coroutine: coFuncs,
		stdlib.UTF8:      utf8Funcs,
		stdlib.Errors: {
			"new": errorsNew, "wrap": errorsWrap,
			"call_stack": errorsCallStack, "is": errorsIs,
		},
	}
}
