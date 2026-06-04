package canonical_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/diag"
)

func TestCanonicalForwardedParamToTypedMethodReportsOnlyCallerContract(t *testing.T) {
	clientModule := testutil.CheckAndExport(`local client = {}

function client.invoke(model_id: string, payload: any, options: any)
	return { ok = true }, nil
end

return client`, "bedrock_client", testutil.WithStdlib())
	if clientModule.HasError() {
		t.Fatalf("client module errors: %v", testutil.ErrorMessages(clientModule.Errors))
	}

	res := testutil.Check(`local bedrock_client = require("bedrock_client")

local function helper(client, model_id)
	return client.invoke(model_id, {}, {})
end

local function contract_args(model: any): { model: any }
	return { model = model }
end

local model_id = contract_args("model").model
helper(bedrock_client, model_id)`, testutil.WithStdlib(), testutil.WithModule("bedrock_client", clientModule))

	errors := errorDiagnostics(res.Diagnostics)
	if len(errors) != 1 {
		t.Fatalf("expected exactly one caller diagnostic, got %v", diagnosticStrings(res.Diagnostics))
	}
	if errors[0].Position.Line != 12 || !strings.Contains(errors[0].Message, "argument 2: expected string, got any") {
		t.Fatalf("diagnostic = %s, want helper caller argument 2 string/any at line 12", errors[0].String())
	}
}

func TestCanonicalContradictingEntryBodyDemandReportsBodyOperationOnly(t *testing.T) {
	provider := testutil.CheckAndExport(`local provider = {}

function provider.meta(): { name: string }
	return { name = "model" }
end

return provider`, "provider", testutil.WithStdlib())
	if provider.HasError() {
		t.Fatalf("provider module errors: %v", testutil.ErrorMessages(provider.Errors))
	}

	res := testutil.Check(`local provider = require("provider")

local CONFIG = { rate = 4 }

local function scale(tokens)
	return tokens * CONFIG.rate
end

local function run()
	local m = provider.meta()
	return scale(m)
end

return run`, testutil.WithStdlib(), testutil.WithModule("provider", provider))

	errors := errorDiagnostics(res.Diagnostics)
	if len(errors) != 1 {
		t.Fatalf("expected exactly one body diagnostic, got %v", diagnosticStrings(res.Diagnostics))
	}
	if errors[0].Position.Line != 6 || !strings.Contains(errors[0].Message, "cannot perform arithmetic") {
		t.Fatalf("diagnostic = %s, want arithmetic body diagnostic at line 6", errors[0].String())
	}
}

func errorDiagnostics(diags []diag.Diagnostic) []diag.Diagnostic {
	out := make([]diag.Diagnostic, 0, len(diags))
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			out = append(out, d)
		}
	}
	return out
}
