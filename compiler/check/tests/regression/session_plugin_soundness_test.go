package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestSessionPlugin_UntypedSessionIDWithoutPresenceProofRejectsStringAPI(t *testing.T) {
	result := testutil.Check(`
type ActiveSession = {
	pid: any,
}

local active_sessions = {} :: {[string]: ActiveSession}

local function graceful_terminate_session(session_id: string, session_info: ActiveSession, reason: string)
	return
end

local function handle_session_close(payload_data)
	if not payload_data then
		return
	end

	local session_id = payload_data.session_id
	if not session_id then
		return
	end

	local session_info = active_sessions[session_id]
	graceful_terminate_session(session_id, session_info, "user_closed")
end

return handle_session_close
`, testutil.WithStdlib())

	if !result.HasError() {
		t.Fatalf("expected error, got none")
	}

	msgs := testutil.ErrorMessages(result.Diagnostics)
	found := false
	for _, msg := range msgs {
		if strings.Contains(msg, "expected string") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected string diagnostic, got %v", msgs)
	}
}
