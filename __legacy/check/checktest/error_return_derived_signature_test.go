package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/module/manifest"
)

// luaErrorInterface mirrors moduletypes.LuaError: the structural error value
// every Wippy module returns in its second result slot.
func luaErrorInterface() *typ.Interface {
	return typ.NewInterface("Error", []typ.Method{
		{Name: "message", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})
}

// plainSeamContractManifest reproduces how the real contract module declares its
// signatures: every error-returning member returns (value, Optional(LuaError))
// with NO ErrorReturn effect attached and NO per-function tagging. The call
// boundary must recover the correlation structurally from the canonical error
// type so callers narrow the value sibling after an `if err` guard.
func plainSeamContractManifest() (*manifest.Manifest, typ.Type) {
	luaError := luaErrorInterface()
	instanceType := typ.NewInterface("contract.Instance", []typ.Method{
		{Name: "list", Type: typ.Func().Param("self", typ.Self).Param("filter", typ.Any).Returns(typ.Any, typeexpr.Optional(luaError)).Build()},
	})
	openType := typ.Func().Param("self", typ.Self).Returns(instanceType, typeexpr.Optional(luaError)).Build()
	loadType := typ.Func().Returns(typeexpr.Optional(instanceType), typeexpr.Optional(luaError)).Build()
	contractType := typ.NewInterface("contract.Contract", []typ.Method{{Name: "open", Type: openType}})
	getType := typ.Func().Param("name", typ.String).Returns(contractType, typeexpr.Optional(luaError)).Build()
	moduleType := typ.NewInterface("contract", []typ.Method{
		{Name: "get", Type: getType},
		{Name: "load", Type: loadType},
	})

	m := manifest.New("contract")
	m.DefineType("Contract", contractType)
	m.DefineType("Instance", instanceType)
	m.SetExport(moduleType)
	return m, luaError
}

// TestCheckDerivedErrorReturnNarrowsModuleSignatureChain pins that a module
// signature following the (value, Optional(error)) convention proves the
// value/error inverse purely from the canonical error type, even across an
// inferred wrapper that delegates a method tail call. No ErrorReturn effect is
// declared anywhere; the correlation is derived structurally at the call site.
func TestCheckDerivedErrorReturnNarrowsModuleSignatureChain(t *testing.T) {
	m, errType := plainSeamContractManifest()
	m.ErrorType = errType
	result := Check(`
local contract = require("contract")

local function open_contract()
    local def, err = contract.get("containers")
    if err then
        return nil, err
    end
    return def:open()
end

local function run(): ()
    local c, err = open_contract()
    if err then
        return
    end
    c:list({})
end
`, WithStdlib(), WithManifest("contract", m))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want canonical-error derivation to prove c present after err guard", result.Diagnostics)
	}
}

// Without the canonical error type the derivation is inert for a pure imported
// signature: there is no local function body to prove value/error correlation.
func TestCheckWithoutCanonicalErrorTypeStillReportsOptionalReceiver(t *testing.T) {
	m, _ := plainSeamContractManifest()
	result := Check(`
local contract = require("contract")

local function run(): ()
    local c, err = contract.load()
    if err then
        return
    end
    c:list({})
end
`, WithStdlib(), WithManifest("contract", m))
	if len(result.Diagnostics) == 0 {
		t.Fatal("expected optional-receiver diagnostic without canonical error type")
	}
}
