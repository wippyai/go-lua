package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestAny_MethodCallIsAllowedButAssignmentRemainsChecked(t *testing.T) {
	source := `
		local x: any = {}
		local v = x:get_full_context()
		local y: string = v
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithCheckOptions(check.Options{Strict: true}))
	if !result.HasError() {
		t.Fatalf("expected assignment error from any to string under strict mode")
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

func TestCheckAndExportPreservesStrict(t *testing.T) {
	source := `
		local x: any = {}
		local y: string = x:get_full_context()
		return y
	`
	result := testutil.CheckAndExport(source, "strict_module", testutil.WithStdlib(), testutil.WithCheckOptions(check.Options{Strict: true}))
	if !result.HasError() {
		t.Fatal("expected strict mode assignment error during module export")
	}
}
