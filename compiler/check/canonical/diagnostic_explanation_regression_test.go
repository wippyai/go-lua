package canonical_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
)

func TestCanonicalDiagnosticsExplainAssignmentMismatch(t *testing.T) {
	d := requireDiagnosticWithMessage(t, `
local x: string = 42
return x
`, "cannot assign")
	requireExplanationContains(t, d, "source expression was observed as")
	requireExplanationContains(t, d, "assignment target requires")
	requireExplanationContains(t, d, "no canonical assignment-source proof")
}

func TestCanonicalDiagnosticsExplainCallArgumentContractMismatch(t *testing.T) {
	d := requireDiagnosticWithMessage(t, `
local function take(n: number)
end
take("bad")
`, "argument 1")
	requireExplanationContains(t, d, "argument 1 was observed as")
	requireExplanationContains(t, d, "solved call contract requires")
	requireExplanationContains(t, d, "no canonical argument proof")
}

func TestCanonicalDiagnosticsExplainOptionalIndexFailure(t *testing.T) {
	d := requireDiagnosticWithMessage(t, `
type QueryResult = {[string]: any}
local function run(result: {QueryResult})
    if result[1] then
        local a = result[1]["k"]
        local b = result[3]["k"]
    end
end
return run
`, "cannot index optional value")
	requireExplanationContains(t, d, "receiver was observed as")
	requireExplanationContains(t, d, "indexing requires a non-nil container")
	requireExplanationContains(t, d, "no canonical branch or product proof")
}

func requireDiagnosticWithMessage(t *testing.T, src, needle string) diag.Diagnostic {
	t.Helper()
	res := testutil.Check(src, testutil.WithStdlib())
	for _, d := range res.Diagnostics {
		if strings.Contains(d.Message, needle) {
			if d.Explanation == "" {
				t.Fatalf("diagnostic %q has empty explanation", d.Message)
			}
			return d
		}
	}
	t.Fatalf("expected diagnostic containing %q, got %v", needle, testutil.ErrorMessages(res.Diagnostics))
	return diag.Diagnostic{}
}

func requireExplanationContains(t *testing.T, d diag.Diagnostic, needle string) {
	t.Helper()
	if !strings.Contains(d.Explanation, needle) {
		t.Fatalf("diagnostic %q explanation missing %q:\n%s", d.Message, needle, d.Explanation)
	}
}
