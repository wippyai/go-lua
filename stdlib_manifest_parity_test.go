package lua

import (
	"sort"
	"testing"

	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/stdlib"
)

// TestNativeStdlibExportsMatchProviderManifests observes the real tables built
// by the runtime. It has no hand-maintained callable or value inventory: the
// provider manifest is the expected surface and the opened table is the actual
// surface.
func TestNativeStdlibExportsMatchProviderManifests(t *testing.T) {
	state := NewState(Options{SkipOpenLibs: true})
	defer state.Close()
	providers := stdlib.Providers()
	if len(providers) != len(luaLibs) {
		t.Fatalf("providers=%d runtime libraries=%d", len(providers), len(luaLibs))
	}
	for index, native := range luaLibs {
		state.Push(native.libFunc)
		state.Push(LString(native.libName))
		state.Call(1, 0)

		declaration := providers[index].Declaration()
		export, ok := declaration.Export.(*typ.Record)
		if !ok {
			t.Fatalf("%q export is %T, want record", providers[index].Identity, declaration.Export)
		}
		table := state.Get(GlobalsIndex).(*LTable)
		if native.libName != "" {
			table, ok = state.GetGlobal(native.libName).(*LTable)
			if !ok {
				t.Fatalf("%q did not mount a module table", providers[index].Identity)
			}
		}
		for _, field := range export.Fields {
			if table.RawGetString(field.Name) == LNil {
				t.Errorf("%q runtime omitted declared export %q", providers[index].Identity, field.Name)
			}
		}
		if native.libName == "" {
			continue // package was intentionally mounted before the global base provider
		}
		actual := make([]string, 0, len(export.Fields))
		table.ForEach(func(key, _ LValue) {
			if name, ok := key.(LString); ok {
				actual = append(actual, string(name))
			}
		})
		expected := make([]string, 0, len(export.Fields))
		for _, field := range export.Fields {
			expected = append(expected, field.Name)
		}
		sort.Strings(actual)
		sort.Strings(expected)
		if len(actual) != len(expected) {
			t.Errorf("%q runtime exports=%v manifest=%v", providers[index].Identity, actual, expected)
			continue
		}
		for i := range actual {
			if actual[i] != expected[i] {
				t.Errorf("%q runtime exports=%v manifest=%v", providers[index].Identity, actual, expected)
				break
			}
		}
	}
}
