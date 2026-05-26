package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestParameterEvidence_ForwardedOptionalFieldsRemainPubliclyUsable(t *testing.T) {
	source := `
		local function pack(params)
			if type(params) ~= "table" then
				return nil, "params must be a table"
			end
			if not params.agent then
				return nil, "agent required"
			end
			local payload = {
				agent = params.agent,
				model = params.model,
				kind = params.kind or "",
				start_func = params.start_func,
				start_params = params.start_params,
				context = params.context,
			}
			return payload
		end

		local result = pack({
			agent = "agent",
			model = "model",
			kind = "kind",
		})
		if result then
			local _: string = result.agent
			local _: string? = result.model
		end
	`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected forwarded optional parameter fields to accept concrete values, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestParameterEvidence_ExportedForwardedOptionalFieldsRemainPubliclyUsable(t *testing.T) {
	moduleSource := `
		local function pack(params)
			if type(params) ~= "table" then
				return nil, "params must be a table"
			end
			if not params.agent then
				return nil, "agent required"
			end
			local payload = {
				agent = params.agent,
				model = params.model,
				kind = params.kind or "",
				start_func = params.start_func,
				start_params = params.start_params,
				context = params.context,
			}
			return payload
		end

		return { pack = pack }
	`
	mod := testutil.CheckAndExport(moduleSource, "start_tokens", testutil.WithStdlib())
	if mod.HasError() {
		t.Fatalf("module should export cleanly, got: %v", testutil.ErrorMessages(mod.Errors))
	}

	consumer := `
		local start_tokens = require("start_tokens")
		start_tokens.pack({
			agent = "agent",
			model = "model",
			kind = "kind",
		})
	`

	result := testutil.Check(consumer, testutil.WithStdlib(), testutil.WithModule("start_tokens", mod))
	if result.HasError() {
		t.Fatalf("expected exported forwarded optional parameter fields to accept concrete values, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestParameterEvidence_BodyDefaultFieldSurvivesInterprocIteration(t *testing.T) {
	moduleSource := `
		local mapper = {}

		local function classify(_status_code, message)
			if message then
				local lower = message:lower()
				if lower:match("timeout") then
					return "timeout"
				end
			end
			return "server_error"
		end

		function mapper.map_error_response(info)
			local error_message = info.message or "fallback"
			local error_type = classify(info.status_code, error_message)
			return {
				success = false,
				error = error_type,
				error_message = error_message,
				metadata = info.metadata or {},
			}
		end

		return mapper
	`
	mod := testutil.CheckAndExport(moduleSource, "mapper_mod", testutil.WithStdlib())
	if mod.HasError() {
		t.Fatalf("module should keep defaulted field evidence across iterations, got: %v", testutil.ErrorMessages(mod.Errors))
	}

	consumer := `
		local mapper = require("mapper_mod")
		local a = mapper.map_error_response({ status_code = 400 })
		local b = mapper.map_error_response({ status_code = 408, message = "timeout" })
		local _: string = a.error_message
		local _: string = b.error_message
	`
	result := testutil.Check(consumer, testutil.WithStdlib(), testutil.WithModule("mapper_mod", mod))
	if result.HasError() {
		t.Fatalf("body-only default evidence should not become a public caller precondition, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
