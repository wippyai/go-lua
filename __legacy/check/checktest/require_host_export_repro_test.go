package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/module/manifest"
)

// Mirrors keeper new_id: a required host module whose method returns
// (string, LuaError?). require("uuid") must rehydrate the manifest export so
// uuid.v4()'s first result is string, not nil.
func TestRequireHostModuleExportRehydratesForConcat(t *testing.T) {
	errorType := typ.NewInterface("Error", []typ.Method{
		{Name: "message", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})
	v4 := typ.Func().Returns(typ.String, typeexpr.Optional(errorType)).Build()

	// Mirror the real uuid manifest exactly: export interface only, NO
	// per-method DefineFunctionSignature.
	uuidSrc := manifest.New("uuid")
	uuidSrc.DefineType("Error", errorType)
	uuidSrc.SetExport(typ.NewInterface("uuid", []typ.Method{{Name: "v4", Type: v4}}))

	// The checker service clones every external manifest via Encode->Decode
	// before solving. Reproduce that exact path here.
	encoded, err := manifest.Encode(uuidSrc)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	uuid, err := manifest.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	program := `
local uuid = require("uuid")
local function new_id()
    local id, err = uuid.v4()
    if err then return "fallback" end
    return id
end
return "ui.response." .. new_id()
`
	result := Check(program, WithStdlib(), WithManifest("uuid", uuid))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("require-host-export: diagnostics = %#v, want new_id typed string via rehydrated uuid export", result.Diagnostics)
	}
}
