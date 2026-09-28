package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestGuardedNestedFieldCaptureKeepsRequiredType(t *testing.T) {
	source := `
		type Data = { tokens: { refresh_token: string? }? }
		local function use_token(data: Data, send: ({ refresh_token: string }) -> ())
			if not data.tokens or not data.tokens.refresh_token then return end
			local call = function()
				send({ refresh_token = data.tokens.refresh_token })
			end
			call()
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("guarded immutable capture widened: %+v", result.Diagnostics)
	}
}

func TestGuardedCaptureInvalidatedByLaterFieldWrite(t *testing.T) {
	source := `
		type Data = { token: string? }
		local function use_token(data: Data, send: (string) -> ())
			if not data.token then return end
			local call = function() send(data.token) end
			data.token = nil
			call()
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatal("later write made captured guard stale")
	}
}
