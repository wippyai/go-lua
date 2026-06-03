package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestFlow_ArithmeticOperandProvesNumericParam verifies the checker
// derives a numeric precondition for an unannotated parameter used as an arithmetic
// operand, and rejects a cross-module opaque `unknown` forwarded into it at the call
// site.
func TestFlow_ArithmeticOperandProvesNumericParam(t *testing.T) {
	provider := testutil.CheckAndExport(`
		local provider = {}
		function provider.rec(): { value: unknown }
			return { value = 0 }
		end
		return provider
	`, "provider", testutil.WithStdlib())
	if provider.HasError() {
		t.Fatalf("provider errors: %v", testutil.ErrorMessages(provider.Errors))
	}

	source := `
		local provider = require("provider")
		local CONFIG = { rate = 4 }
		local function scale(tokens)
			return tokens * CONFIG.rate
		end
		local function run()
			local r = provider.rec()
			return scale(r.value)
		end
		return run
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithModule("provider", provider))
	if !result.HasError() {
		t.Fatalf("expected flow to reject an unknown forwarded into an arithmetic-constrained parameter")
	}
	if msgs := strings.Join(testutil.ErrorMessages(result.Diagnostics), " | "); !strings.Contains(msgs, "expected number") {
		t.Fatalf("expected numeric precondition diagnostic, got: %s", msgs)
	}
}

// TestFlow_HelperForwardsArgToTypedMethodRejectsAny verifies the checker
// propagates a body method-call argument type into an unannotated helper parameter:
// helper(client, model_id) whose body calls client.invoke(model_id) with invoke's
// first parameter typed string proves model_id: string, so an `any` forwarded
// through the helper is rejected at the helper call site.
func TestFlow_HelperForwardsArgToTypedMethodRejectsAny(t *testing.T) {
	clientModule := testutil.CheckAndExport(`
		local client = {}
		function client.invoke(model_id: string, payload: any, options: any)
			return {ok = true}, nil
		end
		return client
	`, "bedrock_client", testutil.WithStdlib())
	if clientModule.HasError() {
		t.Fatalf("client module errors: %v", testutil.ErrorMessages(clientModule.Errors))
	}

	source := `
		local bedrock_client = require("bedrock_client")
		local function helper(client, model_id)
			return client.invoke(model_id, {}, {})
		end
		local contract_args = nil :: any
		local model_id = contract_args.model
		helper(bedrock_client, model_id)
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithModule("bedrock_client", clientModule))
	if !result.HasError() {
		t.Fatalf("expected flow to reject an any forwarded into a string-typed method through an untyped helper")
	}
	if msgs := strings.Join(testutil.ErrorMessages(result.Diagnostics), " | "); !strings.Contains(msgs, "expected string") {
		t.Fatalf("expected string precondition diagnostic, got: %s", msgs)
	}
}

// TestFlow_UntypedHelperWithUntypedCalleeStaysGradual verifies the companion
// boundary: when the body method-call callee is itself untyped, no precondition is
// proven, so a value forwarded through the helper stays admissible (no false
// positive from the body-usage contract derivation).
func TestFlow_UntypedHelperWithUntypedCalleeStaysGradual(t *testing.T) {
	clientModule := testutil.CheckAndExport(`
		local client = {}
		function client.invoke(model_id, payload, options)
			return {ok = true}
		end
		return client
	`, "bedrock_client", testutil.WithStdlib())
	if clientModule.HasError() {
		t.Fatalf("client module errors: %v", testutil.ErrorMessages(clientModule.Errors))
	}

	source := `
		local bedrock_client = require("bedrock_client")
		local handler = { _client = bedrock_client }
		local function helper(client, model_id, payload, options)
			return client.invoke(model_id, payload, options)
		end
		local result = helper(handler._client, "model", {}, {})
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithModule("bedrock_client", clientModule))
	if result.HasError() {
		t.Fatalf("expected untyped-callee passthrough to stay gradual, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
