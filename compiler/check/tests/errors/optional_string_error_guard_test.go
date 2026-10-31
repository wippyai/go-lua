package errors

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

func TestOptionalStringResultNarrowsAfterErrorGuard(t *testing.T) {
	source := `
local env = require("env")
local crypto = require("crypto")
local function encode(data: string): string
    return data
end

local function make_key(): (string?, string?)
    local bytes, err = crypto.random.bytes(32)
    if err then return nil, "generation failed" end
    return encode(bytes)
end

local function run()
    local key, err = make_key()
    if err then return end
    env.set("ENCRYPTION_KEY", key)
end
return {run=run}
`
	envManifest := io.NewManifest("env")
	envManifest.SetExport(typ.NewRecord().Field("set", typ.Func().Param("name", typ.String).Param("value", typ.String).Build()).Build())
	cryptoManifest := io.NewManifest("crypto")
	cryptoManifest.SetExport(typ.NewRecord().Field("random", typ.NewRecord().Field("bytes", typ.Func().Param("size", typ.Number).Returns(typ.String, typ.NewOptional(typ.LuaError)).Build()).Build()).Build())
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("env", envManifest), testutil.WithManifest("crypto", cryptoManifest))
	if result.HasError() {
		t.Fatalf("guarded successful result must be a string: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
