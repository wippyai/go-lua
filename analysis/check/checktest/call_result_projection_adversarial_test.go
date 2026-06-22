package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestDirectCallCurrentReturnProjectionHandlesMultiStatementBody(t *testing.T) {
	result := Check(strings.TrimLeft(`
local function make()
    local row = { id = 1 }
    return row
end

local got: { id: string } = make()
`, "\n"))
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		MessageContains: []string{"make(...)", "{id: 1}", "not", "{id: string}"},
		EvidenceContains: []string{
			"make(...) has type {id: 1}",
			"got is declared as {id: string}",
		},
		LabelContains: []string{"assigned value", "declared type"},
	})
}
