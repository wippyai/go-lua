package lua_test

import (
	lua "github.com/wippyai/go-lua"
	"github.com/wippyai/go-lua/compiler/bytecode"
	"path/filepath"
	"testing"
)

func TestReviewCoverageRegistersRestoredBytecode(t *testing.T) {
	if !lua.CoverageEnabled() {
		t.Skip("requires WIPPY_COVERAGE=1")
	}
	for _, loader := range []string{"LoadProto", "NewFunctionFromProto"} {
		t.Run(loader, func(t *testing.T) {
			original, err := lua.CompileString("return 42", "@review-compiler-identity.lua")
			if err != nil {
				t.Fatal(err)
			}
			// Use an identity absent from this process's collector to model
			// bytecode compiled and cached by an earlier process.
			source := "@" + filepath.ToSlash(filepath.Join(t.TempDir(), loader+".lua"))
			original.SourceName = source
			packed, err := bytecode.Dump(original)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := bytecode.Undump(packed)
			if err != nil {
				t.Fatal(err)
			}
			filter := func(s string) bool { return s == source }
			if found, _ := lua.CoverageSummary(filter); found != 0 {
				t.Fatal("fixture source was already registered")
			}
			L := lua.NewState()
			defer L.Close()
			var fn *lua.LFunction
			if loader == "LoadProto" {
				fn = L.LoadProto(restored)
			} else {
				fn = L.NewFunctionFromProto(restored)
			}
			L.Push(fn)
			if err := L.PCall(0, 0, nil); err != nil {
				t.Fatal(err)
			}
			found, hit := lua.CoverageSummary(filter)
			if found == 0 || hit == 0 {
				t.Fatalf("restored executable source is absent from coverage: %d/%d", hit, found)
			}
		})
	}
}
