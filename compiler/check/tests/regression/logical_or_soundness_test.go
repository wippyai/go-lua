package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestRegression_LogicalOrKeepsTruthyLeftAlternative(t *testing.T) {
	source := `
		local function f(xs: {any}?)
			local ys: {number} = xs or {1}
			return ys
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatal("expected logical-or assignment to reject possible any[] left branch")
	}
	msgs := strings.Join(testutil.ErrorMessages(result.Diagnostics), " | ")
	if !strings.Contains(msgs, "cannot assign") {
		t.Fatalf("expected assignment error, got: %v", msgs)
	}
}
