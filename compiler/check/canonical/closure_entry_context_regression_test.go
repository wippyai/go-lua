package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestCanonicalClosureEntryContextCapturesBranchNarrowedCell(t *testing.T) {
	res := testutil.Check(`
local function test(x)
    if type(x) == "number" then
        local f = function()
            local y: number = x
        end
    end
end
`, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))

	var captured []int
	for _, result := range res.Session.Results {
		if result == nil || result.Graph == nil || len(result.Graph.ParamSymbols()) != 0 {
			continue
		}
		symbols := result.Graph.Bindings().SymbolsByName("x")
		if len(symbols) != 1 {
			continue
		}
		for _, sym := range result.Graph.Bindings().CapturedSymbols(result.Graph.Func()) {
			captured = append(captured, int(sym))
		}
		break
	}
	if len(captured) == 0 {
		t.Fatalf("test did not find a closure capturing x; diagnostics=%v", testutil.ErrorMessages(res.Diagnostics))
	}
	if res.HasError() {
		t.Fatalf("branch-narrowed closure capture should type-check; captured=%v diagnostics=%v", captured, testutil.ErrorMessages(res.Diagnostics))
	}
}

func TestCanonicalReturnedFunctionUsesLiveBoundaryClosureContext(t *testing.T) {
	res := testutil.Check(`
local T = {}
local M = {}

function M.make(): string
    return T.render()
end

function T.render(): string
    return "ok"
end

return M
`, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))

	if res.HasError() {
		t.Fatalf("exported function should see live boundary prototype context, diagnostics=%v", testutil.ErrorMessages(res.Diagnostics))
	}
}
