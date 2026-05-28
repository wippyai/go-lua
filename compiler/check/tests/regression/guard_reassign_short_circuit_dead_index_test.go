package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Soundness: a string-literal local reassigned to a table inside its own
// type(...)=="string" guard is fully a table after the if; indexing must not
// see the eliminated string.
func TestGuardReassignReplacesGuardedValueAfterIf(t *testing.T) {
	source := `
local content = "merge"
if type(content) == "string" then
	content = { { type = "text", text = content } }
end
local x = content[1]
return x
`
	if result := testutil.Check(source, testutil.WithStdlib()); result.HasError() {
		t.Fatalf("guard reassign should be table after if: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Soundness: a short-circuit type guard that is statically false for a string
// literal makes the guarded index unreachable; it must not error.
func TestShortCircuitTypeGuardDeadIndexNoError(t *testing.T) {
	source := `
local c = "merge"
local x = type(c) == "table" and c[1] or nil
return x
`
	if result := testutil.Check(source, testutil.WithStdlib()); result.HasError() {
		t.Fatalf("dead short-circuit index should not error: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Soundness counter-check: when the guarded value can genuinely be a string at
// the index, the short-circuit guard does NOT prove the branch dead, so the
// index on the string member must still be flagged.
func TestShortCircuitTypeGuardReachableIndexStillErrors(t *testing.T) {
	source := `
local function f(x: string)
	return type(x) == "string" and x[1] or nil
end
return f
`
	if result := testutil.Check(source, testutil.WithStdlib()); !result.HasError() {
		t.Fatalf("indexing a string under a true type guard must still error")
	}
}
