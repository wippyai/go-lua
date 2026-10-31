package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestTruthinessUnionBranchNarrowing(t *testing.T) {
	source := `
        function classify(v: {payload: number} | false | nil | "ready")
            if v then
                local good: {payload: number} | "ready" = v
            else
                local absent: false | nil = v
            end
        end
        function fallback(v: "ready" | false | nil)
            local truthy: "ready" = v or "ready"
            local falsy: false | nil = v and false
        end
    `
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("unexpected checker errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestTypeKeyBothBranchNarrowing(t *testing.T) {
	source := `
        function classify(v: string | number | false)
            if type(v) == "string" then
                local text: string = v
            else
                local other: number | false = v
            end
            if type(v) ~= "number" then
                local other: string | false = v
            else
                local numeric: number = v
            end
        end
    `
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("unexpected checker errors: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
