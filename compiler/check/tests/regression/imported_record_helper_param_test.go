package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestRegression_ImportedRecordPassedThroughUntypedHelper(t *testing.T) {
	clientModule := testutil.CheckAndExport(`
		local client = {}
		client.SERVICE = "bedrock"
		function client.invoke(model_id, payload, options)
			return {ok = true}
		end
		return client
	`, "bedrock_client", testutil.WithStdlib())
	if clientModule.HasError() {
		t.Fatalf("provider errors: %v", testutil.ErrorMessages(clientModule.Errors))
	}

	source := `
		local bedrock_client = require("bedrock_client")

		local handler = {
			_client = bedrock_client,
		}

		local function helper(client, model_id, payload, options)
			return client.invoke(model_id, payload, options)
		end

		local result = helper(handler._client, "model", {}, {})
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithModule("bedrock_client", clientModule))
	if result.HasError() {
		t.Fatalf("expected imported record helper call to type-check, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestRegression_ImportedRecordHelperWithConstrainedMethodUse(t *testing.T) {
	clientModule := testutil.CheckAndExport(`
		local client = {}
		client.SERVICE = "bedrock"
		function client.invoke(model_id: string, payload: any, options: {timeout: number?}?)
			return {ok = true}, nil
		end
		return client
	`, "bedrock_client", testutil.WithStdlib())
	if clientModule.HasError() {
		t.Fatalf("provider errors: %v", testutil.ErrorMessages(clientModule.Errors))
	}

	source := `
		local bedrock_client = require("bedrock_client")

		local handler = {
			_client = bedrock_client,
		}

		local function helper(client, model_id, input, options)
			local payload = { input = input }
			local response, err = client.invoke(model_id, payload, { timeout = options and options.timeout })
			if err then
				return nil, err
			end
			return response
		end

		local result = helper(handler._client, "model", "text", { timeout = 1 })
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithModule("bedrock_client", clientModule))
	if result.HasError() {
		t.Fatalf("expected imported record helper with constrained method use to type-check, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestRegression_ImportedRecordHelperRejectsAnyPassedToStringMethod(t *testing.T) {
	clientModule := testutil.CheckAndExport(`
		local client = {}
		function client.invoke(model_id: string, payload: any, options: any)
			return {ok = true}, nil
		end
		return client
	`, "bedrock_client", testutil.WithStdlib())
	if clientModule.HasError() {
		t.Fatalf("provider errors: %v", testutil.ErrorMessages(clientModule.Errors))
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
		t.Fatalf("expected an error when any flows into imported string-only method")
	}
}

func TestRegression_ImportedRecordHelperWithTableStoredModule(t *testing.T) {
	clientModule := testutil.CheckAndExport(`
		local client = {}
		client.SERVICE = "bedrock"
		function client.invoke(model_id: string, payload: any, options: {timeout: number?}?)
			return {ok = true}, nil
		end
		function client.converse(model_id: string, payload: any, options: {timeout: number?}?)
			return {ok = true}, nil
		end
		return client
	`, "bedrock_client", testutil.WithStdlib())
	if clientModule.HasError() {
		t.Fatalf("provider errors: %v", testutil.ErrorMessages(clientModule.Errors))
	}

	source := `
		local bedrock_client = require("bedrock_client")

		local handler = {
			_client = bedrock_client,
		}

		local function helper(client, model_id, input, options)
			local payload = { input = input }
			local response, err = client.invoke(model_id, payload, { timeout = options and options.timeout })
			if err then
				return nil, err
			end
			return response
		end

		local model_id = "model"
		local input = "text"
		local options = {}
		local result, err
		result, err = helper(handler._client, model_id, input, options)
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithModule("bedrock_client", clientModule))
	if result.HasError() {
		t.Fatalf("expected table-stored imported module helper call to type-check, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
