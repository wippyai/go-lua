package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestLoopLocalConcatAssignmentEveryBranchDoesNotLeakUninitializedNil(t *testing.T) {
	result := Check(`
local function f(parts: {string}): ()
    for i = 1, #parts do
        local note: string
        if parts[i] == "" then
            note = "empty:" .. parts[i]
        else
            note = "value:" .. parts[i]
        end
        local x: string = note
    end
end
`)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for loop-local assigned on every branch", result.Diagnostics)
	}
}

func TestLoopBackedgeConcatTypeChangeIsRejectedAfterLoop(t *testing.T) {
	src := strings.TrimLeft(`
local function f(parts: {string}): ()
    local acc = 0
    for i = 1, #parts do
        acc = acc .. parts[i]
    end
    local n: number = acc
end
`, "\n")
	result := Check(src)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		DiagnosticCount: 1,
		Line:            6,
		Column:          23,
		Span: diagnostic.Span{
			StartLine: 6,
			StartCol:  23,
			EndLine:   6,
			EndCol:    25,
		},
		MessageContains: []string{"cannot assign acc", "string | 0", "not number"},
		EvidenceMin:     2,
		EvidenceContains: []string{
			"acc has type string | 0",
			"n is declared as number",
		},
		EvidenceOrdered: []string{
			"acc has type string | 0",
			"n is declared as number",
		},
		LabelMin: 2,
		LabelContains: []string{
			"assigned value",
			"declared type",
		},
		Sources: diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"error[type.assignment]: cannot assign acc because it is string | 0, not number",
			"test.lua:6:23",
			"declared type",
			"6 |     local n: number = acc",
			"assigned value",
			"because:",
			"proven: acc has type string | 0",
			"claimed: n is declared as number",
		},
		RenderNotContains: []string{
			"want number",
			"^~",
		},
	})
}
