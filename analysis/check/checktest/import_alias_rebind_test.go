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
