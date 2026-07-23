package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
)

const recursiveUnionProtocolModule = `
type Text = {
    kind: "text",
    value: string,
}

type Group = {
    kind: "group",
    children: {Node},
}

type Node = Text | Group

local M = {}
M.Node = Node
M.Group = Group
M.Text = Text
return M
`

func TestImportedRecursiveUnionTypeMemberNarrowingRejectsMissingVariantField(t *testing.T) {
	protocol := CheckAndExport(recursiveUnionProtocolModule, "protocol")
	if len(protocol.Errors) != 0 {
		t.Fatalf("protocol diagnostics = %#v, want none", protocol.Errors)
	}

	result := Check(`
local protocol = require("protocol")

local function inspect(candidate: protocol.Node): ()
    if candidate.kind == "text" then
        local children = candidate.children
    end
end
`, WithModule("protocol", protocol))

	requireChildrenMissingMemberDiagnostic(t, result)
}

func TestImportedRecursiveUnionAnnotatedFunctionStillChecksGenericBodyWhenCalledWithNarrowValue(t *testing.T) {
	protocol := CheckAndExport(recursiveUnionProtocolModule, "protocol")
	if len(protocol.Errors) != 0 {
		t.Fatalf("protocol diagnostics = %#v, want none", protocol.Errors)
	}
	builder := CheckAndExport(`
local protocol = require("protocol")

local M = {}
function M.make(): protocol.Node
    return {kind = "group", children = {}}
end
return M
`, "builder", WithModule("protocol", protocol))

	result := Check(`
local protocol = require("protocol")
local builder = require("builder")

local function inspect(candidate: protocol.Node): ()
    if candidate.kind == "text" then
        local children = candidate.children
    end
end

inspect(builder.make())
`, WithModule("protocol", protocol), WithModule("builder", builder))

	requireChildrenMissingMemberDiagnostic(t, result)
}

func TestImportedRecursiveUnionGuardDoesNotLeakIntoLaterAnnotatedFunctionCall(t *testing.T) {
	protocol := CheckAndExport(recursiveUnionProtocolModule, "protocol")
	if len(protocol.Errors) != 0 {
		t.Fatalf("protocol diagnostics = %#v, want none", protocol.Errors)
	}
	builder := CheckAndExport(`
local protocol = require("protocol")

local M = {}
function M.make(): protocol.Node
    return {
        kind = "group",
        children = {
            {kind = "text", value = "a"},
        },
    }
end
return M
`, "builder", WithModule("protocol", protocol))

	result := Check(`
local protocol = require("protocol")
local builder = require("builder")

local node = builder.make()
if node.kind == "group" then
    local first = node.children[1]
    if first and first.kind == "text" then
        local value: string = first.value
    end
end

local function inspect(candidate: protocol.Node): ()
    if candidate.kind == "text" then
        local children = candidate.children
    end
end

inspect(node)
`, WithModule("protocol", protocol), WithModule("builder", builder))

	requireChildrenMissingMemberDiagnostic(t, result)
}

func TestGenericCallContextOwnsBodyDiagnosticsWhenSpecialized(t *testing.T) {
	result := Check(`
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local function bind<T, U>(r: Result<T>, f: (T) -> Result<U>): Result<U>
    if r.ok then
        return f(r.value)
    end
    return { ok = false, error = r.error }
end

local function parse(n: number): Result<number>
    if n > 0 then return { ok = true, value = n } end
    return { ok = false, error = "non-positive" }
end

local r: Result<number> = { ok = true, value = 5 }
local out = bind(r, parse)
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want generic call context to own specialized body checks", result.Diagnostics)
	}
}

func requireChildrenMissingMemberDiagnostic(t *testing.T, result Result) {
	t.Helper()
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one missing-member diagnostic", result.Diagnostics)
	}
	diag := result.Diagnostics[0]
	if diag.Code != diagnostics.CodeMissingMember || !strings.Contains(diag.Message, `has no member "children"`) {
		t.Fatalf("diagnostic = %#v, want missing children member on narrowed text variant", diag)
	}
}
