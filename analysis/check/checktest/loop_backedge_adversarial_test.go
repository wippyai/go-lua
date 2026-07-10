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
		MessageContains: []string{"cannot assign acc", "number | string", "not number"},
		EvidenceMin:     2,
		EvidenceContains: []string{
			"acc has type number | string",
			"n is declared as number",
		},
		EvidenceOrdered: []string{
			"acc has type number | string",
			"n is declared as number",
		},
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"acc", "number | string"},
			},
			{
				Kind:            diagnostic.EvidenceUserAssertion,
				Trust:           diagnostic.TrustClaimed,
				MessageContains: []string{"n", "number"},
			},
		},
		LabelMin: 2,
		LabelContains: []string{
			"assigned value",
			"declared type",
		},
		Sources: diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"error[type.assignment]: cannot assign acc because it is number | string, not number",
			"test.lua:6:23",
			"declared type",
			"6 |     local n: number = acc",
			"assigned value",
			"because:",
			"proven: acc has type number | string",
			"claimed: n is declared as number",
		},
		RenderNotContains: []string{
			"want number",
			"^~",
		},
	})
}

func TestLoopBackedgePreservesAnnotatedRecordAfterGuardedOptionalAssignment(t *testing.T) {
	result := Check(`
type Usage = { input_tokens: number, output_tokens: number }
type DoneEvent = { type: "done", usage: Usage? }
type OtherEvent = { type: "other" }
type Event = DoneEvent | OtherEvent
type StreamResult = { usage: Usage }

local function process(events: {Event}): StreamResult
    local usage: Usage = { input_tokens = 0, output_tokens = 0 }
    for _, event in ipairs(events) do
        if event.type == "done" then
            if event.usage then
                usage = event.usage
            end
        end
    end
    local result: StreamResult = {
        usage = usage,
    }
    return result
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want loop-carried annotated record shape preserved", result.Diagnostics)
	}
}

func TestLoopBackedgePreservesInlineAnnotatedRecordAfterGuardedOptionalAssignment(t *testing.T) {
	result := Check(`
local function process(events: {{ type: "done", usage: { input_tokens: number, output_tokens: number }? } | { type: "other" }}): { usage: { input_tokens: number, output_tokens: number } }
    local usage: { input_tokens: number, output_tokens: number } = { input_tokens = 0, output_tokens = 0 }
    for _, event in ipairs(events) do
        if event.type == "done" then
            if event.usage then
                usage = event.usage
            end
        end
    end
    local result: { usage: { input_tokens: number, output_tokens: number } } = {
        usage = usage,
    }
    return result
end
`, WithStdlib())
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want inline loop-carried annotated record shape preserved", result.Diagnostics)
	}
}
