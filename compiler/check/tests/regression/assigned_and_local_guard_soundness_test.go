package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestAssignedAndDoesNotKeepStaleLocalGuard(t *testing.T) {
	for _, source := range []string{
		`local row: { decision: string }? = { decision = "blocked" }
local prior = row and row.decision == "blocked"
row = nil
if prior then
    local decision: string = row.decision
end`,
		`local row: { decision: string }? = { decision = "blocked" }
local function clear(): boolean row = nil; return true end
local prior = row and clear()
if prior then
    local decision: string = row.decision
end`,
	} {
		result := testutil.Check(source, testutil.WithStdlib())
		if !result.HasError() {
			t.Fatalf("expected stale row guard to be rejected: %s", source)
		}
	}
}
