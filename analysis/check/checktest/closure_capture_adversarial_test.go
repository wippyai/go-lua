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
	diag := requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
	requireEvidenceMessage(t, diag, "get(...) has type nil")
	requireEvidenceMessage(t, diag, "out is declared as string")
	requireEvidenceMessage(t, diag, "no proof on this path shows get(...) is string")
	rendered := diagnostic.Render(diag, diagnostic.RenderOptions{
		Sources:             diagnostic.SourceMap{"test.lua": src},
		ShowSourceLabelRows: true,
	})
	want := `error[type.assignment]: cannot assign get(...) because it is nil, not string
 --> test.lua:7:25
  |
  |                ↓ declared type
7 |     local out: string = get()
  |                         ↑ assigned value

because:
  1. proven: get(...) has type nil
  2. claimed: out is declared as string
  3. missing proof: no proof on this path shows get(...) is string

help: Use a value compatible with the expected type, or change the target type if ` + "`get(...)`" + ` is valid.`
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
		Code:            diagnostics.CodeAssignmentType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            9,
		Column:          25,
		Span:            diagnostic.Span{StartLine: 9, StartCol: 25, EndLine: 9, EndCol: 27},
		MessageContains: []string{"get(...)", "may be nil"},
		EvidenceMin:     3,
		EvidenceOrdered: []string{
			"get(...) can be string or nil here",
			"out is declared as string",
			"no guard on this path proves get(...) is non-nil",
		},
		LabelMin: 2,
		LabelContains: []string{
			"declared type",
			"assigned value",
		},
		HelpContains: []string{"Guard `get(...)`"},
		Sources:      diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"9 |     local out: string = get()",
			"1. proven: get(...) can be string or nil here",
			"2. claimed: out is declared as string",
			"3. missing proof: no guard on this path proves get(...) is non-nil",
		},
		RenderNotContains: []string{
			"^",
			"get returns string?",
		},
	})
}
