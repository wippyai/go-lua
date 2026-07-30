package engine_test

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/lint"
	diag "github.com/wippyai/go-lua/analysis/diagnostic"
)

// claimCodeDiagnostics runs the project check with the unproven-claim code
// enabled, which is the only way that family reaches a caller.
func claimCodeDiagnostics(t *testing.T, source string) string {
	t.Helper()
	result, err := lint.CheckProject(context.Background(), lint.ProjectInput{
		Entries:          []lint.Entry{{Path: "main.lua", ModulePath: "main", Source: source}},
		Targets:          []string{"main"},
		DiagnosticPolicy: diag.Policy{Rules: map[diag.Code]diag.Rule{"lint.claim.unproven": diag.Enable()}},
	})
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	return diagnosticSummaries(result.Diagnostics)
}

// TestDeclarationInsideALoopReadsItsOwnSlot pins the declaration default across
// a cycle. A bare declaration binds a fresh cell on every trip and reads that
// cell's own slot; the marker an earlier trip left there is this declaration's
// own contract, so it states no value the declaration was given and leaves the
// same silence the identical declaration outside the loop has.
func TestDeclarationInsideALoopReadsItsOwnSlot(t *testing.T) {
	outside := claimCodeDiagnostics(t, `
local acc: string
return acc
`)
	if outside != "" {
		t.Fatalf("a bare declaration outside a loop reported a claim:\n%s", outside)
	}
	inside := claimCodeDiagnostics(t, `
local xs = { 1, 2 }
local out = {}
for _, v in ipairs(xs) do
    local acc: string
    out[v] = acc
end
return out
`)
	if inside != "" {
		t.Fatalf("a bare declaration inside a loop reported a claim the same declaration outside one does not:\n%s", inside)
	}
}
