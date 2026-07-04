package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestInvalidDeclaredFunctionAssignmentDoesNotCascadeIntoCallDiagnostic(t *testing.T) {
	result := Check(`
local function f(): number
    local t = {}
    local g: fun(): number = t.run
    return g()
end
`)
	requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
	for _, diag := range result.Diagnostics {
		if diag.Code == diagnostics.CodeDirectCallNotCallable {
			t.Fatalf("diagnostics = %#v, want assignment error without downstream callability cascade", result.Diagnostics)
		}
	}
}

func TestInvalidDeclaredArgumentSuppressesSameContractCallCascade(t *testing.T) {
	result := Check(`
local function need_string(value: string): ()
end

local function f(raw: any): ()
    local value: string = raw
    need_string(value)
end
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		DiagnosticCount: 1,
		MessageContains: []string{
			"raw",
			"any",
			"string",
		},
	})
}

func TestInvalidDeclaredArgumentKeepsDifferentContractCallDiagnostic(t *testing.T) {
	result := Check(`
local function need_integer(value: integer): ()
end

local function f(raw: any): ()
    local value: string = raw
    need_integer(value)
end
`)
	requireDiagnostic(t, result, diagnosticExpectation{
		Code: diagnostics.CodeDirectCallArgType,
		MessageContains: []string{
			"value",
			"string",
			"integer",
		},
	})
	assignment := requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
	if assignment.Severity != diagnostic.SeverityError {
		t.Fatalf("assignment severity = %s, want error", assignment.Severity)
	}
}
