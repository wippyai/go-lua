package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestGuardedMissingField_ProofExpiresAfterBranch(t *testing.T) {
	result := testutil.Check(`
		local node: {name: string} = {name = "one"}
		if node.distance ~= nil then
			local safe = node.distance
		end
		local unsafe = node.distance
	`, testutil.WithStdlib())
	if len(result.Errors) != 1 || result.Errors[0].Position.Line != 6 {
		t.Fatalf("expected only the unguarded read to fail, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
