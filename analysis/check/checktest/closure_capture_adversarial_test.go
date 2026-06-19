package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestClosureCaptureSeesReassignmentAfterCapture(t *testing.T) {
	src := `local function f(): string
    local x: string? = "ready"
    local get = function(): string?
        return x
    end
    x = nil
    local out: string = get()
    return out
end
`
	result := Check(src)
	diag := requireDiagnosticCode(t, result, diagnostics.CodeDirectCallResultAssignment)
	requireEvidenceMessage(t, diag, "get declares call result 1 as string?")
	requireEvidenceMessage(t, diag, "assignment target out requires string")
	requireEvidenceMessage(t, diag, "no guard on this path proves call result 1 is non-nil before assignment")
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `error[type.call.direct.result_assignment]: call result 1 is string?, not string
 --> test.lua:7:25
  |
  |                ↓ declared type
7 |     local out: string = get()
  |                         ↑ call result

because:
  1. claimed: get declares call result 1 as string?
 --> test.lua:3:17
  |
  |                 ↓ callee declaration
3 |     local get = function(): string?
  2. claimed: assignment target out requires string
  3. missing proof: no guard on this path proves call result 1 is non-nil before assignment

help: Guard the call result before assigning it, provide a default value, or change the target type to accept nil.`
	assertRenderedEqual(t, rendered, want)
}

func TestClosureCaptureSeesLoopReassignmentAfterCapture(t *testing.T) {
	src := `local function f(n: number): string
    local x: string? = "ready"
    local get = function(): string?
        return x
    end
    for i = 1, n do
        x = nil
    end
    local out: string = get()
    return out
end
`
	result := Check(src)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeDirectCallResultAssignment,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            9,
		Column:          25,
		Span:            diagnostic.Span{StartLine: 9, StartCol: 25, EndLine: 9, EndCol: 27},
		MessageContains: []string{"call result 1", "string?", "not string"},
		EvidenceMin:     3,
		EvidenceOrdered: []string{
			"get declares call result 1 as string?",
			"assignment target out requires string",
			"no guard on this path proves call result 1 is non-nil before assignment",
		},
		LabelMin: 3,
		LabelContains: []string{
			"call result",
			"declared type",
			"callee declaration",
		},
		HelpContains: []string{"Guard the call result"},
		Sources:      diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"9 |     local out: string = get()",
			"1. claimed: get declares call result 1 as string?",
			"3 |     local get = function(): string?",
			"2. claimed: assignment target out requires string",
			"3. missing proof: no guard on this path proves call result 1 is non-nil before assignment",
		},
		RenderNotContains: []string{
			"^",
			"get returns string?",
		},
	})
}
