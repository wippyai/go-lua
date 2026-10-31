package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

func TestDeclaredErrorReturnGuardPersistsPastLengthGuard(t *testing.T) {
	crypto := io.NewManifest("crypto")
	crypto.SetExport(typ.NewRecord().Field("random", typ.NewRecord().Field("bytes", typ.Func().Param("count", typ.Integer).Returns(typ.NewOptional(typ.String), typ.NewOptional(typ.String)).Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})).Build()).Build()).Build())
	env := io.NewManifest("env")
	env.SetExport(typ.NewRecord().Field("get", typ.Func().Param("name", typ.String).Returns(typ.NewOptional(typ.String)).Build()).Field("set", typ.Func().Param("name", typ.String).Param("key", typ.String).Returns(typ.Boolean, typ.NewOptional(typ.String)).Build()).Build())
	source := `
local crypto = require("crypto")
local env = require("env")

local function binary_to_hex(data: string): string
	local hex = {}
	for i = 1, #data do
		local byte = string.byte(data, i)
		hex[i] = string.format("%02x", byte)
	end
	return table.concat(hex)
end

local function gen(): (string?, string?)
	local bytes, err = crypto.random.bytes(32)
	if err then return nil, "error: " .. tostring(err) end
	local hex_key = binary_to_hex(bytes)
	return hex_key
end

local function run(options: any?): { status: string, message: string }
	local existing = env.get("K")
	if existing and existing ~= "" then
		return { status = "skipped", message = "already exists" }
	end
	local k, err = gen()
	if err then return { status = "error", message = "failed: " .. tostring(err) } end
	if #k ~= 64 then
		local problem = "bad length: " .. #k .. " (expected 64)"
		return { status = "error", message = problem }
	end
	local set_success, set_err = env.set("K", k)
	if set_err then return { status = "error", message = set_err } end
	return { status = "ok", message = "done" }
end
return { run = run }
`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("crypto", crypto), testutil.WithManifest("env", env))
	if result.HasError() {
		t.Fatalf("explicit error return should narrow through the length guard: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
