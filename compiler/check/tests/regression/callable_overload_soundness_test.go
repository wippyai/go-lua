package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Minimal omitted-mode variant of kickside/platform/transfer/src/bundle_test.lua:11-23,137-139.
func TestLiteralOverloadDoesNotAcceptOmittedArgument(t *testing.T) {
	result := testutil.Check(`
local function open(mode)
    if mode == "w" then return { write = function() end } end
    return nil
end
local h = open()
h:write()
`, testutil.WithStdlib())
	if len(result.Errors) != 1 || result.Errors[0].Position.Line != 7 || result.Errors[0].Message != "cannot call method on optional value without nil check" {
		t.Fatalf("omitted mode must retain the nil obligation: %v", testutil.ErrorMessages(result.Errors))
	}
}

// Nullable callable variant of the handle returned by
// kickside/platform/transfer/src/bundle_test.lua:11-23,137-139.
func TestNullableIntersectionMemberRetainsCallObligation(t *testing.T) {
	result := testutil.Check(`
type F = ((string) -> number)? & ((number) -> number)?
local f: F = nil
local n = f("x")
`, testutil.WithStdlib())
	if len(result.Errors) != 1 || result.Errors[0].Position.Line != 4 || result.Errors[0].Message != "cannot call optional value without nil check" {
		t.Fatalf("nullable intersection member must retain the call obligation: %v", testutil.ErrorMessages(result.Errors))
	}
}

// Alternative opener variant of kickside/platform/transfer/src/bundle_test.lua:11-23,137-139.
func TestUnionCallRetainsRejectedAlternativeReturn(t *testing.T) {
	result := testutil.Check(`
local function good(mode)
    if mode == "w" then return { write = function() end } end
    return nil
end
local function bad(mode: number) return nil end
local function use(flag: boolean)
    local opener = bad
    if flag then opener = good end
    local h = opener("w")
    h:write()
end
`, testutil.WithStdlib())
	var nilCall bool
	for _, d := range result.Errors {
		if d.Position.Line == 11 && d.Message == "cannot call method on optional value without nil check" {
			nilCall = true
		}
	}
	if !nilCall {
		t.Fatalf("bad opener's nil return must survive the union call: %v", testutil.ErrorMessages(result.Errors))
	}
}

// Mutable captured-state variant of kickside/platform/transfer/src/bundle_test.lua:11-23,137-139.
func TestLiteralOverloadDoesNotAssumeCapturedState(t *testing.T) {
	result := testutil.Check(`
local active = true
local function open(mode)
    if mode == "w" and active then return { write = function() end } end
    return nil
end
active = false
local h = open("w")
h:write()
`, testutil.WithStdlib())
	if len(result.Errors) != 1 || result.Errors[0].Position.Line != 9 || result.Errors[0].Message != "cannot call method on optional value without nil check" {
		t.Fatalf("mutable captured state must retain the nil obligation: %v", testutil.ErrorMessages(result.Errors))
	}
}
