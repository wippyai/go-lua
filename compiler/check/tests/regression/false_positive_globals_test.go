package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestRegression_CallbackEnvOverlayIsCompilationIsolated proves that a callback
// EnvOverlay declared by one compiled module ({describe,it,after_each}) is not
// overwritten by a prior compilation in the same process that declared a smaller
// overlay ({describe,it}) on a same-signature run_cases helper. Function-type
// identity must distinguish the callback contract so the two run_cases helpers do
// not collapse to one canonical node and lose the larger overlay's after_each.
func TestRegression_CallbackEnvOverlayIsCompilationIsolated(t *testing.T) {
	// First compilation: run_cases sets only describe/it. This seeds the
	// process-global value interner with a same-signature run_cases.
	smallMod := testutil.CheckAndExport(`
local test = {}
function test.describe(_name: string, fn: fun()) fn() end
function test.it(_name: string, fn: fun()) fn() end
function test.run_cases(define_cases_fn: fun())
	return function()
		_G.describe = test.describe
		_G.it = test.it
		define_cases_fn()
		_G.describe = nil
		_G.it = nil
	end
end
return test
`, "small_mod", testutil.WithStdlib())
	if smallMod.HasError() {
		t.Fatalf("unexpected small module errors: %v", testutil.ErrorMessages(smallMod.Errors))
	}

	// Second compilation: run_cases with the same signature but a larger overlay
	// that also sets after_each.
	largeMod := testutil.CheckAndExport(`
local test = {}
function test.describe(_name: string, fn: fun()) fn() end
function test.it(_name: string, fn: fun()) fn() end
function test.after_each(fn: fun()) fn() end
function test.run_cases(define_cases_fn: fun())
	return function()
		_G.describe = test.describe
		_G.it = test.it
		_G.after_each = test.after_each
		define_cases_fn()
		_G.describe = nil
		_G.it = nil
		_G.after_each = nil
	end
end
return test
`, "large_mod", testutil.WithStdlib())
	if largeMod.HasError() {
		t.Fatalf("unexpected large module errors: %v", testutil.ErrorMessages(largeMod.Errors))
	}

	// Consumer of the large module must see after_each inside the callback body.
	source := `
local tests = require("large_mod")
local function define_tests()
	describe("suite", function()
		after_each(function() end)
		it("case", function() end)
	end)
end
return tests.run_cases(define_tests)
`
	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithModule("large_mod", largeMod),
	)
	if result.HasError() {
		t.Fatalf("after_each overlay must survive a prior smaller-overlay compilation, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// TestFalsePositive_UnknownGlobalCalledAsFunction reproduces the wippy test
// framework pattern where describe/it/before_each are globals set dynamically
// by _G.it = test.it at runtime. The checker cannot see these assignments.
func TestFalsePositive_UnknownGlobalCalledAsFunction(t *testing.T) {
	source := `
		local function define_tests()
			describe("test suite", function()
				it("should work", function()
					local x = 1
				end)
			end)
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	for _, d := range result.Diagnostics {
		t.Logf("line %d: %s", d.Position.Line, d.Message)
	}
}
