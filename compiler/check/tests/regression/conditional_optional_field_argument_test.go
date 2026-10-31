package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestConditionalOptionalFieldArgument(t *testing.T) {
	source := `
		local function list(args: {namespace?: string}) end
		local args = {}
		local ns: string? = "default"
		if ns and ns ~= "" then args.namespace = ns end
		list(args)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("conditional field should satisfy optional argument: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
