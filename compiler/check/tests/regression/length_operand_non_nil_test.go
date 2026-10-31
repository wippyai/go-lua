package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestSuccessfulLengthComparisonNarrowsOperand(t *testing.T) {
	source := `local function f(value: string?)
		if #value ~= 64 then return end
		local function consume(s: string) end
		consume(value)
	end`
	for _, strict := range []bool{false, true} {
		result := testutil.Check(source, testutil.WithCheckOptions(check.Options{Strict: strict}))
		if result.HasError() {
			t.Errorf("strict=%v: %v", strict, testutil.ErrorMessages(result.Errors))
		}
	}
}
