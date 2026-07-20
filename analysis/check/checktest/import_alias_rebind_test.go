package checktest

import "testing"

func TestCheckLocalAssertWrapperNarrowsMemberArgumentOnNormalReturn(t *testing.T) {
	result := Check(`
local function assertNotNil(val: any): ()
    assert(val, "value must not be nil")
end

local function process(obj: {data: string?}): string
    assertNotNil(obj.data)
    local s: string = obj.data
    return s
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want local assert wrapper to narrow obj.data", result.Diagnostics)
	}
}

func TestCheckRecursiveIPairsElementUsesDeclaredContainerType(t *testing.T) {
	result := Check(`
type Tree = { root: TreeNode? }
type TreeNode = {
    label: string,
    owner: Tree,
    children: {TreeNode},
    parent: TreeNode?,
}

local function depth_of(node: TreeNode?): number
    if node == nil then
        return 0
    end
    local best = 0
    for _, child in ipairs(node.children) do
        local d = depth_of(child)
        if d > best then
            best = d
        end
    end
    return best + 1
end

local tree: Tree = {root = nil}
local node: TreeNode = {label = "root", owner = tree, children = {}, parent = nil}
tree.root = node
return depth_of(tree.root)
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want recursive ipairs element to preserve TreeNode", result.Diagnostics)
	}
}

func TestCheckRecursiveCallOptionalParamKeepsNilAcceptedType(t *testing.T) {
	result := Check(`
type A = { b: B?, tag: "a" }
type B = { c: C?, tag: "b" }
type C = { a: A?, tag: "c" }

local function walk(a: A?): number
    if a == nil then return 0 end
    if a.b == nil then return 1 end
    if a.b.c == nil then return 2 end
    return 3 + walk(a.b.c.a)
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want recursive optional parameter to accept nil", result.Diagnostics)
	}
}

func TestCheckConstrainedGenericMethodReceiverUsesConstraint(t *testing.T) {
	result := Check(`
type Printable = {tostring: (self: Printable) -> string}

local function print_it<T: Printable>(x: T): string
    return x:tostring()
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want constrained generic receiver to use Printable contract", result.Diagnostics)
	}
}

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
`, "app.lib:assert", WithStdlib())
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
`, "app.lib:assert", WithStdlib())

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

// verbatimAssertLibrarySource is the production app.lib:assert table
// (wippy-golua-seam/tests/app/src/lib/assert.lua), byte-for-byte. Every
// production call site that trips the optional-method false positive resolves
// its assert local to this exact module. A minimal one-function stand-in
// cannot rule out a defect that only appears once the manifest carries all 17
// exported functions.
const verbatimAssertLibrarySource = "\n" +
	"local M = {}\n" +
	"\n" +
	"function M.eq(actual, expected, msg)\n" +
	"\tif actual ~= expected then\n" +
	"\t\terror((msg or \"assertion failed\") .. \": expected \" .. tostring(expected) .. \", got \" .. tostring(actual), 2)\n" +
	"\tend\n" +
	"end\n" +
	"\n" +
	"function M.neq(actual, expected, msg)\n" +
	"\tif actual == expected then\n" +
	"\t\terror((msg or \"assertion failed\") .. \": expected not \" .. tostring(expected), 2)\n" +
	"\tend\n" +
	"end\n" +
	"\n" +
	"function M.ok(val, msg)\n" +
	"\tif not val then\n" +
	"\t\terror((msg or \"assertion failed\") .. \": expected truthy value\", 2)\n" +
	"\tend\n" +
	"\treturn val\n" +
	"end\n" +
	"\n" +
	"function M.fail(msg)\n" +
	"\terror(msg or \"assertion failed\", 2)\n" +
	"end\n" +
	"\n" +
	"function M.is_nil(val, msg)\n" +
	"\tif val ~= nil then\n" +
	"\t\terror((msg or \"assertion failed\") .. \": expected nil, got \" .. tostring(val), 2)\n" +
	"\tend\n" +
	"end\n" +
	"\n" +
	"function M.not_nil(val, msg)\n" +
	"\tif val == nil then\n" +
	"\t\terror((msg or \"assertion failed\") .. \": expected non-nil value\", 2)\n" +
	"\tend\n" +
	"\treturn val\n" +
	"end\n" +
	"\n" +
	"function M.is_string(val, msg)\n" +
	"\tif type(val) ~= \"string\" then\n" +
	"\t\terror((msg or \"assertion failed\") .. \": expected string, got \" .. type(val), 2)\n" +
	"\tend\n" +
	"\treturn val\n" +
	"end\n" +
	"\n" +
	"function M.is_number(val, msg)\n" +
	"\tif type(val) ~= \"number\" then\n" +
	"\t\terror((msg or \"assertion failed\") .. \": expected number, got \" .. type(val), 2)\n" +
	"\tend\n" +
	"\treturn val\n" +
	"end\n" +
	"\n" +
	"function M.is_table(val, msg)\n" +
	"\tif type(val) ~= \"table\" then\n" +
	"\t\terror((msg or \"assertion failed\") .. \": expected table, got \" .. type(val), 2)\n" +
	"\tend\n" +
	"\treturn val\n" +
	"end\n" +
	"\n" +
	"function M.is_function(val, msg)\n" +
	"\tif type(val) ~= \"function\" then\n" +
	"\t\terror((msg or \"assertion failed\") .. \": expected function, got \" .. type(val), 2)\n" +
	"\tend\n" +
	"\treturn val\n" +
	"end\n" +
	"\n" +
	"function M.is_boolean(val, msg)\n" +
	"\tif type(val) ~= \"boolean\" then\n" +
	"\t\terror((msg or \"assertion failed\") .. \": expected boolean, got \" .. type(val), 2)\n" +
	"\tend\n" +
	"\treturn val\n" +
	"end\n" +
	"\n" +
	"function M.contains(str, substr, msg)\n" +
	"\tif type(str) ~= \"string\" or not string.find(str, substr, 1, true) then\n" +
	"\t\terror((msg or \"assertion failed\") .. \": expected string to contain '\" .. tostring(substr) .. \"'\", 2)\n" +
	"\tend\n" +
	"\treturn str\n" +
	"end\n" +
	"\n" +
	"function M.has_error(val, err, msg)\n" +
	"\tif val ~= nil then\n" +
	"\t\terror((msg or \"has_error failed\") .. \": expected nil result, got \" .. tostring(val), 2)\n" +
	"\tend\n" +
	"\tif err == nil then\n" +
	"\t\terror((msg or \"has_error failed\") .. \": expected error, got nil\", 2)\n" +
	"\tend\n" +
	"end\n" +
	"\n" +
	"function M.no_error(val, err, msg)\n" +
	"\tif err ~= nil then\n" +
	"\t\terror((msg or \"no_error failed\") .. \": unexpected error: \" .. tostring(err), 2)\n" +
	"\tend\n" +
	"end\n" +
	"\n" +
	"function M.throws(fn, msg)\n" +
	"\tlocal ok, err = pcall(fn)\n" +
	"\tif ok then\n" +
	"\t\terror((msg or \"throws failed\") .. \": expected function to throw\", 2)\n" +
	"\tend\n" +
	"\treturn err\n" +
	"end\n" +
	"\n" +
	"function M.not_throws(fn, msg)\n" +
	"\tlocal ok, err = pcall(fn)\n" +
	"\tif not ok then\n" +
	"\t\terror((msg or \"not_throws failed\") .. \": unexpected error: \" .. tostring(err), 2)\n" +
	"\tend\n" +
	"end\n" +
	"\n" +
	"function M.error_kind(err, expected_kind, msg)\n" +
	"\tif err == nil then\n" +
	"\t\terror((msg or \"error_kind failed\") .. \": error is nil\", 2)\n" +
	"\tend\n" +
	"\tif type(err) ~= \"table\" then\n" +
	"\t\terror((msg or \"error_kind failed\") .. \": error is not structured (got \" .. type(err) .. \")\", 2)\n" +
	"\tend\n" +
	"\tif err.kind ~= expected_kind then\n" +
	"\t\terror((msg or \"error_kind failed\") .. \": expected kind '\" .. tostring(expected_kind) .. \"', got '\" .. tostring(err.kind) .. \"'\", 2)\n" +
	"\tend\n" +
	"end\n" +
	"\n" +
	"function M.error_message(err, expected_msg, msg)\n" +
	"\tif err == nil then\n" +
	"\t\terror((msg or \"error_message failed\") .. \": error is nil\", 2)\n" +
	"\tend\n" +
	"\tlocal actual_msg = type(err) == \"table\" and err.message or tostring(err)\n" +
	"\tif actual_msg ~= expected_msg then\n" +
	"\t\terror((msg or \"error_message failed\") .. \": expected message '\" .. tostring(expected_msg) .. \"', got '\" .. tostring(actual_msg) .. \"'\", 2)\n" +
	"\tend\n" +
	"end\n" +
	"\n" +
	"function M.error_contains(err, substr, msg)\n" +
	"\tif err == nil then\n" +
	"\t\terror((msg or \"error_contains failed\") .. \": error is nil\", 2)\n" +
	"\tend\n" +
	"\tlocal actual_msg = type(err) == \"table\" and err.message or tostring(err)\n" +
	"\tif not string.find(actual_msg, substr, 1, true) then\n" +
	"\t\terror((msg or \"error_contains failed\") .. \": expected error to contain '\" .. tostring(substr) .. \"', got '\" .. tostring(actual_msg) .. \"'\", 2)\n" +
	"\tend\n" +
	"end\n" +
	"\n" +
	"return M\n"

// Byte-for-byte port of the production assert.lua exported under its real
// module path (app.lib:assert), rebound to its real consuming alias
// (assert2), with a caller matching the real corpus shape: err is a LOCAL
// bound from a multi-return call (not a function parameter), a sibling
// variable is asserted first, and the guarded method call is an argument
// expression on the next statement rather than a bare return - the exact
// shape of wippy-golua-seam/tests/app/src/test/compress/errors.lua.
func TestCheckImportAliasReboundPreservesNarrowingForRealAssertLibraryAndLocal(t *testing.T) {
	mod := CheckAndExport(verbatimAssertLibrarySource, "app.lib:assert", WithStdlib())
	rebound := mod.Manifest.Rebound("assert2")

	result := Check(`
local assert = require("assert2")

type Err = {kind: fun(self): string}

local function make(): (any, Err?)
    return nil, nil
end

local function main(): string
    local result, err = make()
    assert.is_nil(result, "empty encode returns nil")
    assert.not_nil(err, "empty encode returns error")
    return assert.eq(err:kind(), "expected", "empty encode error kind")
end
return main
`, WithStdlib(), WithManifest("assert2", rebound))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want the real assert library to narrow a locally-bound err across the intervening sibling assertion", result.Diagnostics)
	}
}

// Same as above but without Rebound, pinning that the full 17-function
// production table reproduces the same without-Rebound failure signature as
// the one-function stand-in - so a fix that only special-cases a minimal
// manifest would not be a fix for the real corpus.
func TestCheckImportAliasWithoutReboundLosesNarrowingForRealAssertLibraryAndLocal(t *testing.T) {
	mod := CheckAndExport(verbatimAssertLibrarySource, "app.lib:assert", WithStdlib())

	result := Check(`
local assert = require("assert2")

type Err = {kind: fun(self): string}

local function make(): (any, Err?)
    return nil, nil
end

local function main(): string
    local result, err = make()
    assert.is_nil(result, "empty encode returns nil")
    assert.not_nil(err, "empty encode returns error")
    return assert.eq(err:kind(), "expected", "empty encode error kind")
end
return main
`, WithStdlib(), WithManifest("assert2", mod.Manifest))
	if len(result.Diagnostics) == 0 {
		t.Fatal("expected optional-receiver diagnostic when the manifest is not rebound to the alias")
	}
}
