package checktest

import "testing"

// A module imported under an alias different from its export path must still
// carry its signature effects. The assert helper proves its argument non-nil on
// normal return; consumed under the alias "assert2", that postcondition must
// survive so a guarded value narrows. Without re-keying on Rebound the lookup
// misses and the optional-receiver false positive returns.
func TestCheckImportAliasReboundPreservesParameterNarrowing(t *testing.T) {
	mod := CheckAndExport(`
local M = {}
function M.not_nil(val, msg)
	if val == nil then
		error((msg or "assertion failed") .. ": expected non-nil value", 2)
	end
	return val
end
return M
`, "app.lib:assert")
	rebound := mod.Manifest.Rebound("assert2")

	result := Check(`
local assert = require("assert2")

type Err = {kind: fun(self): string}

local function run(err: Err?): string
    assert.not_nil(err, "expected error")
    return err:kind()
end
return run
`, WithStdlib(), WithManifest("assert2", rebound))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want aliased assert.not_nil to narrow err", result.Diagnostics)
	}
}

// The same lookup, without rebinding the manifest to the consuming alias, loses
// the postcondition and reports the optional-receiver false positive - pinning
// that Rebound is what restores it.
func TestCheckImportAliasWithoutReboundLosesNarrowing(t *testing.T) {
	mod := CheckAndExport(`
local M = {}
function M.not_nil(val, msg)
	if val == nil then
		error((msg or "assertion failed") .. ": expected non-nil value", 2)
	end
	return val
end
return M
`, "app.lib:assert")

	result := Check(`
local assert = require("assert2")

type Err = {kind: fun(self): string}

local function run(err: Err?): string
    assert.not_nil(err, "expected error")
    return err:kind()
end
return run
`, WithStdlib(), WithManifest("assert2", mod.Manifest))
	if len(result.Diagnostics) == 0 {
		t.Fatal("expected optional-receiver diagnostic when the manifest is not rebound to the alias")
	}
}
