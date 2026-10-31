package regression

import (
	"os"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
)

// Reduced from keeper.state.tools:explore, explore.lua:401-443.
func TestReproFixpointExploreRecursiveTraversal(t *testing.T) {
	source := `
local function traverse(id, depth, visited)
    if depth > 2 then return { outgoing = {}, incoming = {} } end
    visited = visited or {}
    if visited[id] then return { outgoing = {}, incoming = {} } end
    visited[id] = true
    local result = { outgoing = {}, incoming = {} }
    for _, edge in ipairs({{ target_id = "x" }}) do
        local child = { target_id = edge.target_id }
        local sub = traverse(edge.target_id, depth + 1, visited)
        if sub.outgoing and #sub.outgoing > 0 then
            child.children = sub.outgoing
        end
        table.insert(result.outgoing, child)
    end
    return result
end
local value = traverse("x", 1, {})
`
	result := testutil.Check(source, testutil.WithStdlib())
	expectNoFixpointWarning(t, result, "return type fixpoint did not converge")
}

// Reduced from execution_identity_test.lua:24-38 and writer_test.lua:41-61.
func TestReproFixpointBuilderSelfMethods(t *testing.T) {
	source := `
local function contract()
    local inst = { resolve = function(_self, args)
        return { subject_id = args.subject_id, groups = {"users"} }
    end }
    local builder = {}
    builder.with_actor = function(_self, _actor) return builder end
    builder.with_scope = function(_self, _scope) return builder end
    builder.open = function() return inst, nil end
    return builder
end
local x = contract()
local y = x:with_actor({}):with_scope({}):open()
`
	result := testutil.Check(source, testutil.WithStdlib())
	expectNoFixpointWarning(t, result, "inter-function fixpoint did not converge")
}

// Reduced from keeper.logger:process, process.lua:12-54, 88-99, 127-171,
// 222-275 and 278-383. Buffer writes flow through helper return values and
// back into captured state along the caller's branches.
func TestReproFixpointLoggerBufferHelpers(t *testing.T) {
	source, err := os.ReadFile("testdata/fixpoint_logger.lua")
	if err != nil {
		t.Fatal(err)
	}
	result := testutil.Check(string(source), testutil.WithStdlib())
	expectNoFixpointWarning(t, result, "inter-function fixpoint did not converge")
}

func expectNoFixpointWarning(t *testing.T, result *testutil.Result, message string) {
	t.Helper()
	for _, d := range result.Diagnostics {
		if d.Severity == diag.SeverityWarning && strings.Contains(d.Message, message) {
			t.Fatalf("unexpected non-convergence warning %q: %v", message, result.Diagnostics)
		}
	}
}

// Reduced from execution_identity_test.lua:41-64. Conditional error returns
// make the instance estimate evolve while the builder methods capture themselves.
func TestReproFixpointBuilderWithConditionalOpen(t *testing.T) {
	source := `
local function live_resolver_contract(opts: any): any
    local options = type(opts) == "table" and opts or {}
    local inst = {
        resolve = function(_self: any, args: any)
            if type(options.capture_args) == "table" then options.capture_args.value = args end
            return options.params or {
                subject_id = args.subject_id,
                groups = { "users" },
                meta = { status = "active" },
            }, nil
        end,
    }
    local builder = {}
    builder.with_actor = function(_self: any, _actor: any) return builder end
    builder.open = function()
        if options.open_err ~= nil then return nil, options.open_err end
        return inst, nil
    end
    return builder
end
`
	for _, strict := range []bool{false, true} {
		name := "gradual"
		if strict {
			name = "strict"
		}
		t.Run(name, func(t *testing.T) {
			result := testutil.Check(source, testutil.WithStdlib(), testutil.WithCheckOptions(check.Options{Strict: strict}))
			for _, d := range result.Diagnostics {
				t.Errorf("unexpected diagnostic: %v", d)
			}
		})
	}
}
