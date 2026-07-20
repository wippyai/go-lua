package program

import "testing"

// Census tests run through runExternalCensusChunk from
// external_roots_census_test.go.

// Distilled from corpus entry app.lib:assert (M.eq comparing two untyped
// parameters). An equality branch over two unbound locals must check
// without an internal "branch: missing condition source" error.
func TestExternalCensusBranchMissingConditionSource(t *testing.T) {
	runExternalCensusChunk(t, `
local a, b
if a ~= b then
end
`)
}

// Distilled from corpus entry app.test.ctx:process_calls_func_worker
// (result.process_called ~= true on a value flowing from an unresolved
// module). A literal comparison over a member of an unknown module value
// must check without an internal "branch: contextual-refinement-kind"
// error.
func TestExternalCensusBranchContextualRefinementKind(t *testing.T) {
	runExternalCensusChunk(t, `
local funcs = require("funcs")
if funcs.x ~= true then
end
`)
}

// Distilled from corpus entry app.test.registry:generation_drain
// (if entry_exists(id) branching on a helper returning x ~= nil). A branch
// whose condition is a call returning a comparison must check without an
// internal "branch: contextual condition source" error.
func TestExternalCensusBranchContextualConditionSource(t *testing.T) {
	runExternalCensusChunk(t, `
local function entry_exists(x)
	return x ~= nil
end

if entry_exists("a") then
end
`)
}
