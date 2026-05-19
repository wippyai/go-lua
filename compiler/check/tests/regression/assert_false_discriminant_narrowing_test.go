package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// Reproduces llm test pattern:
// - function returns success/error discriminated union
// - helper assertion enforces success == false
// - error_message should be string in following call
func TestRegression_AssertFalseDiscriminantNarrowing(t *testing.T) {
	source := `
type Response =
	{ success: true, result: {content: string} } |
	{ success: false, error: string, error_message: string }

local function is_false(v: any)
	if v ~= false then
		error("expected false")
	end
end

local function contains(str: string, substr: string)
	if type(str) ~= "string" or not string.find(str, substr, 1, true) then
		error("expected string to contain substring")
	end
end

local function handler(): Response
	return {
		success = false,
		error = "invalid_request",
		error_message = "Model is required"
	}
end

local response = handler()
is_false(response.success)
contains(response.error_message, "Model is required")
`

	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected no errors for assert-based discriminant narrowing")
	}
}

func TestRegression_DefaultedAnyFieldDoesNotSilentlyAdoptFallbackType(t *testing.T) {
	source := `
local info = nil :: any
local error_message = info.message or "fallback"

local function needs_string(value: string)
	return value
end

needs_string(error_message)
`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatal("expected defaulted any field to remain dynamic, not become string")
	}
	found := false
	for _, msg := range testutil.ErrorMessages(result.Diagnostics) {
		if strings.Contains(msg, "expected string, got any") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected any-to-string diagnostic, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestRegression_ImportedAssertFalseDiscriminantNarrowing(t *testing.T) {
	testMod := testutil.CheckAndExport(`
local test = {}

function test.is_false(val: any, msg: string?)
	if val ~= false then
		error(msg or "expected false")
	end
end

function test.contains(str: any, substr: string, msg: string?): string
	if type(str) ~= "string" or not string.find(str, substr, 1, true) then
		error(msg or "expected contains")
	end
	return str
end

return test
`, "test_mod", testutil.WithStdlib())
	if testMod.HasError() {
		t.Fatalf("unexpected test module errors: %v", testutil.ErrorMessages(testMod.Errors))
	}

	containsField := unwrap.Record(testMod.Manifest.Export).GetField("contains")
	if containsField == nil {
		t.Fatal("expected exported contains function")
	}
	containsFn := unwrap.Function(containsField.Type)
	if containsFn == nil || len(containsFn.Params) == 0 || !typ.TypeEquals(containsFn.Params[0].Type, typ.Any) {
		t.Fatalf("contains first param = %v, want any", containsField.Type)
	}
	if summary, ok := testMod.Manifest.LookupSummary("contains"); ok && summary != nil && len(summary.Params) > 0 {
		if !typ.TypeEquals(summary.Params[0], typ.Any) {
			t.Fatalf("contains summary first param = %v, want any", summary.Params[0])
		}
	}

	producer := testutil.CheckAndExport(`
local M = {}

function M.handler()
	if true then
		return {
			success = false,
			error = "invalid_request",
			error_message = "Model is required"
		}
	end
	return {
		success = true,
		result = { content = "ok" }
	}
end

return M
`, "producer", testutil.WithStdlib())
	if producer.HasError() {
		t.Fatalf("unexpected producer errors: %v", testutil.ErrorMessages(producer.Errors))
	}

	source := `
local tests = require("test_mod")
local producer = require("producer")

local response = producer.handler()
tests.is_false(response.success)
tests.contains(response.error_message, "Model is required")
`
	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithModule("test_mod", testMod),
		testutil.WithModule("producer", producer),
	)
	if result.HasError() {
		t.Fatalf("expected imported assert false to narrow discriminant, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestRegression_ImportedDiscriminantThroughMultivalueHelper(t *testing.T) {
	testMod := testutil.CheckAndExport(`
local test = {}

function test.is_false(val: any, msg: string?)
	if val ~= false then
		error(msg or "expected false")
	end
end

function test.contains(str: string, substr: string, msg: string?): string
	if type(str) ~= "string" or not string.find(str, substr, 1, true) then
		error(msg or "expected contains")
	end
	return str
end

return test
`, "test_mod", testutil.WithStdlib())
	if testMod.HasError() {
		t.Fatalf("unexpected test module errors: %v", testutil.ErrorMessages(testMod.Errors))
	}

	mapperMod := testutil.CheckAndExport(`
local mapper = {}

local function map_error_type(_status_code, message)
	if message then
		local _lower = message:lower()
	end
	return "invalid_request"
end

function mapper.map_error_response(info)
	local error_message = info.message or "fallback"
	local error_type = map_error_type(info.status_code, error_message)
	return {
		success = false,
		error = error_type,
		error_message = error_message,
		metadata = {}
	}, { message = error_message }
end

function mapper.map_success_response(_response)
	return {
		success = true,
		result = { content = "ok" },
		metadata = {}
	}
end

return mapper
`, "mapper_mod", testutil.WithStdlib())
	if mapperMod.HasError() {
		t.Fatalf("unexpected mapper errors: %v", testutil.ErrorMessages(mapperMod.Errors))
	}

	generateMod := testutil.CheckAndExport(`
local mapper = require("mapper_mod")

local generate = {
	_mapper = mapper,
}

function generate.handler(args)
	if args.bad then
		return generate._mapper.map_error_response({
			message = "bad request",
			status_code = 400,
		})
	end
	if args.remote_bad then
		local response = args.response
		return generate._mapper.map_error_response(response)
	end
	return generate._mapper.map_success_response({})
end

return generate
`, "generate_mod", testutil.WithStdlib(), testutil.WithModule("mapper_mod", mapperMod))
	if generateMod.HasError() {
		t.Fatalf("unexpected generate errors: %v", testutil.ErrorMessages(generateMod.Errors))
	}

	source := `
local tests = require("test_mod")
local generate = require("generate_mod")

local response = generate.handler({ bad = true })
tests.is_false(response.success)
tests.contains(response.error_message, "bad request")
`
	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithModule("test_mod", testMod),
		testutil.WithModule("generate_mod", generateMod),
	)
	if result.HasError() {
		t.Fatalf("expected imported multivalue helper result to narrow by success=false, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestRegression_BDDCallbackLocalImportedDiscriminant(t *testing.T) {
	testMod := testutil.CheckAndExport(`
local test = { _cases = {} }

function test.is_false(val: any, msg: string?)
	if val ~= false then
		error(msg or "expected false")
	end
end

function test.is_true(val: any, msg: string?)
	if val ~= true then
		error(msg or "expected true")
	end
end

function test.contains(str: string, substr: string, msg: string?): string
	if type(str) ~= "string" or not string.find(str, substr, 1, true) then
		error(msg or "expected contains")
	end
	return str
end

function test.describe(_name: string, fn: fun())
	fn()
end

function test.it(_name: string, fn: fun())
	table.insert(test._cases, fn)
end

function test.run_cases(define_cases_fn: fun())
	return function()
		_G.describe = test.describe
		_G.it = test.it
		define_cases_fn()
		_G.describe = nil
		_G.it = nil
	end
end

return test
`, "test_mod", testutil.WithStdlib())
	if testMod.HasError() {
		t.Fatalf("unexpected test module errors: %v", testutil.ErrorMessages(testMod.Errors))
	}

	mapperMod := testutil.CheckAndExport(`
local mapper = {}

local function map_error_type(_status_code, message)
	if message then
		local _lower = message:lower()
	end
	return "invalid_request"
end

function mapper.map_error_response(info)
	local error_message = info.message or "fallback"
	local error_type = map_error_type(info.status_code, error_message)
	return {
		success = false,
		error = error_type,
		error_message = error_message,
		metadata = {}
	}, { message = error_message }
end

function mapper.map_success_response(_response)
	return {
		success = true,
		result = { content = "ok" },
		metadata = {}
	}
end

return mapper
`, "mapper_mod", testutil.WithStdlib())
	if mapperMod.HasError() {
		t.Fatalf("unexpected mapper errors: %v", testutil.ErrorMessages(mapperMod.Errors))
	}

	generateMod := testutil.CheckAndExport(`
local mapper = require("mapper_mod")

local generate = {
	_mapper = mapper,
}

function generate.handler(args)
	if args.bad then
		return generate._mapper.map_error_response({
			message = "bad request",
			status_code = 400,
		})
	end
	if args.remote_bad then
		local response = args.response
		return generate._mapper.map_error_response(response)
	end
	return generate._mapper.map_success_response({})
end

return generate
`, "generate_mod", testutil.WithStdlib(), testutil.WithModule("mapper_mod", mapperMod))
	if generateMod.HasError() {
		t.Fatalf("unexpected generate errors: %v", testutil.ErrorMessages(generateMod.Errors))
	}

	source := `
local tests = require("test_mod")
local generate = require("generate_mod")

local function define_tests()
	describe("generate", function()
		it("error response", function()
			generate._mapper = {
				map_error_response = function(info)
					return {
						success = false,
						error = "invalid_request",
						error_message = info.message,
						metadata = {}
					}
				end,
				map_success_response = function()
					return {
						success = true,
						result = { content = "ok" },
						metadata = {}
					}
				end,
			}

			local response = generate.handler({ bad = true })
			tests.is_false(response.success)
			tests.contains(response.error_message, "bad request")
		end)

		it("success response", function()
			generate._mapper = {
				map_error_response = function(info)
					return {
						success = false,
						error = "invalid_request",
						error_message = info.message,
						metadata = {}
					}
				end,
				map_success_response = function()
					return {
						success = true,
						result = { content = "ok" },
						metadata = {}
					}
				end,
			}

			local response = generate.handler({})
			tests.is_true(response.success)
		end)
	end)
end

return tests.run_cases(define_tests)
`
	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithModule("test_mod", testMod),
		testutil.WithModule("generate_mod", generateMod),
	)
	if result.HasError() {
		t.Fatalf("expected BDD callback-local imported discriminant to narrow, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestRegression_ImportedHandlerUsesVisibleMapperOverrideContract(t *testing.T) {
	testMod := testutil.CheckAndExport(`
local test = { _cases = {} }

function test.is_false(val: any, msg: string?)
	if val ~= false then
		error(msg or "expected false")
	end
end

function test.contains(str: string, substr: string, msg: string?): string
	if type(str) ~= "string" or not string.find(str, substr, 1, true) then
		error(msg or "expected contains")
	end
	return str
end

function test.describe(_name: string, fn: fun())
	fn()
end

function test.it(_name: string, fn: fun())
	table.insert(test._cases, fn)
end

function test.run_cases(define_cases_fn: fun())
	return function()
		_G.describe = test.describe
		_G.it = test.it
		define_cases_fn()
		_G.describe = nil
		_G.it = nil
	end
end

return test
`, "test_mod", testutil.WithStdlib())
	if testMod.HasError() {
		t.Fatalf("unexpected test module errors: %v", testutil.ErrorMessages(testMod.Errors))
	}

	mapperMod := testutil.CheckAndExport(`
local mapper = {}

function mapper.map_error_response(error_info)
	local error_message = error_info.message or "Google API error"
	return {
		success = false,
		error = "server_error",
		error_message = error_message,
		metadata = {}
	}
end

return mapper
`, "mapper_mod", testutil.WithStdlib())
	if mapperMod.HasError() {
		t.Fatalf("unexpected mapper errors: %v", testutil.ErrorMessages(mapperMod.Errors))
	}

	contractMod := testutil.CheckAndExport(`
local contract = {}

function contract.get(_id)
	return nil, "not found"
end

return contract
`, "contract_mod", testutil.WithStdlib())
	if contractMod.HasError() {
		t.Fatalf("unexpected contract errors: %v", testutil.ErrorMessages(contractMod.Errors))
	}

	generateMod := testutil.CheckAndExport(`
local mapper = require("mapper_mod")
local contract = require("contract_mod")

local generate = {
	_mapper = mapper,
	_contract = contract,
}

function generate.handler(args)
	if not args.model then
		return generate._mapper.map_error_response({
			message = "Model is required",
			status_code = 400,
		})
	end

	local _, err = generate._contract.get("client")
	if err then
		return generate._mapper.map_error_response({
			message = "Failed to get client contract: " .. tostring(err),
			status_code = 500,
		})
	end

	return { success = true }
end

return generate
`, "generate_mod", testutil.WithStdlib(),
		testutil.WithModule("mapper_mod", mapperMod),
		testutil.WithModule("contract_mod", contractMod))
	if generateMod.HasError() {
		t.Fatalf("unexpected generate errors: %v", testutil.ErrorMessages(generateMod.Errors))
	}

	source := `
local tests = require("test_mod")
local generate = require("generate_mod")

local function define_tests()
	describe("generate", function()
		it("contract failure", function()
			generate._mapper = {
				map_error_response = function(error_info)
					return {
						success = false,
						error = "server_error",
						error_message = error_info.message,
						metadata = {}
					}
				end
			}

			generate._contract = {
				get = function(_contract_id)
					return nil, "Contract not found"
				end
			}

			local response = generate.handler({
				model = "gemini-1.5-pro",
				messages = {
					{ role = "user", content = {{ type = "text", text = "Test" }} }
				}
			})

			tests.is_false(response.success)
			tests.contains(response.error_message, "Failed to get client contract")
		end)
	end)
end

return tests.run_cases(define_tests)
`
	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithModule("test_mod", testMod),
		testutil.WithModule("generate_mod", generateMod),
	)
	if result.HasError() {
		t.Fatalf("expected visible mapper override contract to prove error_message, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestRegression_ErrorMapperInfersDefaultedMessageField(t *testing.T) {
	mapperMod := testutil.CheckAndExport(`
local output = {
	ERROR_TYPE = {
		SERVER_ERROR = "server_error",
	},
	to_structured_error = function(_response)
		return nil
	end,
}

local mapper = {}

local function map_error_type(_status_code, message)
	if message then
		local lower_msg = message:lower()
		if lower_msg:match("timeout") then
			return "timeout"
		end
	end
	return output.ERROR_TYPE.SERVER_ERROR
end

function mapper.map_error_response(google_error)
	if not google_error then
		local response = {
			success = false,
			error = output.ERROR_TYPE.SERVER_ERROR,
			error_message = "Unknown Google error",
			metadata = {}
		}
		return response, output.to_structured_error(response)
	end

	local error_message = google_error.message or "Google API error"
	local error_type = map_error_type(google_error.status_code, error_message)

	local response = {
		success = false,
		error = error_type,
		error_message = error_message,
		metadata = google_error.metadata or {}
	}
	return response, output.to_structured_error(response)
end

return mapper
`, "mapper_mod", testutil.WithStdlib())
	if mapperMod.HasError() {
		t.Fatalf("unexpected mapper errors: %v", testutil.ErrorMessages(mapperMod.Errors))
	}

	field := unwrap.Record(mapperMod.Manifest.Export).GetField("map_error_response")
	if field == nil {
		t.Fatal("expected exported map_error_response")
	}
	fn := unwrap.Function(field.Type)
	if fn == nil || len(fn.Returns) == 0 {
		t.Fatalf("expected function return, got %v", field.Type)
	}
	rec := unwrap.Record(fn.Returns[0])
	if rec == nil {
		t.Fatalf("expected record return, got %v", fn.Returns[0])
	}
	errMsg := rec.GetField("error_message")
	if errMsg == nil || !typ.TypeEquals(errMsg.Type, typ.String) {
		t.Fatalf("error_message = %v, want string in %v", errMsg, fn.Returns[0])
	}
}

func TestRegression_PartialRecordParameterEvidenceBecomeOptionalFields(t *testing.T) {
	source := `
local mapper = {}

function mapper.map_tokens(usage)
	if not usage then
		return nil
	end
	return {
		prompt_tokens = usage.promptTokenCount or 0,
		completion_tokens = usage.candidatesTokenCount or 0,
		total_tokens = usage.totalTokenCount or 0,
		thinking_tokens = usage.thoughtsTokenCount
	}
end

mapper.map_tokens({ promptTokenCount = 10 })
mapper.map_tokens({ candidatesTokenCount = 20 })
mapper.map_tokens({ totalTokenCount = 30 })
mapper.map_tokens({ thoughtsTokenCount = 40 })
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected partial record parameter observations to form optional fields, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestRegression_NestedPartialRecordParameterEvidenceBecomeOptionalFields(t *testing.T) {
	source := `
local mapper = {}

function mapper.map_tokens(usage)
	if not usage then
		return nil
	end
	return {
		prompt_tokens = usage.promptTokenCount or 0,
		completion_tokens = usage.candidatesTokenCount or 0,
		total_tokens = usage.totalTokenCount or 0,
		thinking_tokens = usage.thoughtsTokenCount
	}
end

function mapper.map_success_response(response)
	return {
		tokens = mapper.map_tokens(response.usageMetadata)
	}
end

mapper.map_success_response({ usageMetadata = { promptTokenCount = 10 } })
mapper.map_success_response({ usageMetadata = { candidatesTokenCount = 20 } })
mapper.map_success_response({ usageMetadata = { totalTokenCount = 30 } })
mapper.map_success_response({ usageMetadata = { thoughtsTokenCount = 40 } })
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected partial record parameter observations to form optional fields, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
