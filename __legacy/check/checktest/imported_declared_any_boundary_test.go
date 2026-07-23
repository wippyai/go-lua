package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
)

func TestImportedDeclaredAnyRecordFieldDoesNotTrustImplementationReturn(t *testing.T) {
	sessionMod := CheckAndExport(`
type ActiveSession = {
    created_at: any,
    last_activity: any,
}

local M = {}

function M.new(): ActiveSession
    return {
        created_at = "created",
        last_activity = "active",
    }
end

return M
`, "session_state")
	if len(sessionMod.Errors) != 0 {
		t.Fatalf("session module errors = %#v, want none", sessionMod.Errors)
	}

	result := Check(`
local session_state = require("session_state")
local session_info = session_state.new()
local last_activity = session_info.last_activity or session_info.created_at
local elapsed: number = last_activity
`, WithStdlib(), WithModule("session_state", sessionMod))

	diag := requireDiagnosticCode(t, result, diagnostics.CodeAssignmentType)
	if !strings.Contains(diag.Message, "it is any") || strings.Contains(diag.Message, `"active"`) {
		t.Fatalf("diagnostic message = %q, want declared any boundary without implementation literal", diag.Message)
	}
}
