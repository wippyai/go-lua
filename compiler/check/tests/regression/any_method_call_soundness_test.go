package regression

import (
	"github.com/wippyai/go-lua/compiler/check"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestAny_MethodCallIsAllowedButAssignmentRemainsChecked(t *testing.T) {
	source := `
		local x: any = {}
		local v = x:get_full_context()
		local y: string = v
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithCheckOptions(check.Options{StrictAny: true}))
	if !result.HasError() {
		t.Fatalf("expected assignment error from any to string under strict any")
	}
}

func TestAny_MethodCallResultIsConsistentUnderGradualAny(t *testing.T) {
	source := `
		local x: any = {}
		local v = x:get_full_context()
		local y: string = v
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("an any result is consistent with string, got %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
