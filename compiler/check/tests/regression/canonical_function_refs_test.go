package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestCanonicalFunctionRefs_ProductStateBeatsImportedFieldFallback(t *testing.T) {
	moduleSource := `
local M = {}

function M.f(): string
	return "old"
end

return M
`
	exported := testutil.CheckAndExport(moduleSource, "mod", testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	if exported.HasError() {
		t.Fatalf("unexpected module export errors: %v", testutil.ErrorMessages(exported.Errors))
	}

	consumer := `
local M = require("mod")

M.f = function(): number
	return 1
end

local n: number = M.f()
`
	result := testutil.Check(consumer,
		testutil.WithStdlib(),
		testutil.WithModule("mod", exported),
		testutil.WithCheckOption(check.WithCanonicalFlow()),
	)
	if result.HasError() {
		t.Fatalf("expected rebinding to use solved FunctionRefs before imported field fallback, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestCanonicalFunctionRefs_MethodSummaryUsesReceiverNotBareName(t *testing.T) {
	source := `
local A = {}
function A:get(): string
	return "a"
end

local B = {}
function B:get(): number
	return 1
end

local n: number = B:get()
`
	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithCheckOption(check.WithCanonicalFlow()),
	)
	if result.HasError() {
		t.Fatalf("expected method resolution to use receiver field identity, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
