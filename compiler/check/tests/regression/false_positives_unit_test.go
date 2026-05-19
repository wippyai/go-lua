package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// False Positive Reproductions from wippy lint
// These tests document bugs that produce false positive errors

// 1. Bracket notation on maps remains soundly optional until presence is proven.
func TestBracketNotationOnMap_GuardedAccess(t *testing.T) {
	source := `
		local method_names: {[string]: string} = {
			greet = "hello",
			farewell = "goodbye"
		}
		local maybe_name: string? = method_names["greet"]
		assert(method_names["greet"])
		local name: string = method_names["greet"]
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for guarded bracket notation on map, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_BracketNotationOnRecord(t *testing.T) {
	source := `
		local config: {host: string, port: integer} = {
			host = "localhost",
			port = 8080
		}
		local h: string = config["host"]
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for bracket notation on record, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// 2. E0019: Arithmetic on union with nil
// Pattern: revenues[i] - expenses[i] where values are number?
// Note: Map indexing soundly returns T? since key may not exist

func TestFalsePositive_ArithmeticOnOptionalMapElements_SoundBehavior(t *testing.T) {
	// This is expected to fail - map indexing returns T? soundly
	source := `
		local revenues: {[integer]: number} = {10000, 9500, 12000}
		local expenses: {[integer]: number} = {8000, 8500, 9000}

		for i = 1, 3 do
			local profit: number = revenues[i] - expenses[i]
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Errorf("expected error for arithmetic on optional map elements (sound behavior)")
	}
}

func TestFalsePositive_ArithmeticOnTupleElements_BoundedLoop(t *testing.T) {
	// Bounded for-loop with matching tuple length should exclude nil from index result
	source := `
		local revenues = {10000, 9500, 12000}
		local expenses = {8000, 8500, 9000}

		for i = 1, 3 do
			local profit: number = revenues[i] - expenses[i]
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for bounded tuple indexing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_TupleLiteralIndexing(t *testing.T) {
	// Literal index on tuple should return exact element type without nil
	source := `
		local data = {10, 20, 30}
		local v: number = data[1]
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for literal tuple indexing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_TupleDynamicIndexing(t *testing.T) {
	// Dynamic index on tuple returns union with nil (sound but verbose)
	source := `
		local data = {10, 20, 30}

		function get(i: integer): number?
			return data[i]
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for dynamic tuple indexing returning optional, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_CapturedLocalHelperReceivesGuardedParamField(t *testing.T) {
	source := `
		local api = {}

		local function resolve_model(model_identifier)
			local class_name = model_identifier:match("^class:(.+)")
			if class_name then
				return { id = class_name }, nil
			end
			return { id = model_identifier }, nil
		end

		function api.generate(options)
			if not options or not options.model then
				return nil, "model required"
			end

			local model_card, err = resolve_model(options.model)
			if not model_card then
				return nil, err
			end
			return model_card.id
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected guarded param field to satisfy captured helper parameter, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_OptionalMapDefaultEmptyTable(t *testing.T) {
	source := `
local function find(options: {[string]: any}?)
	options = options or {}
	local criteria: {[string]: any} = {}
	for k, v in pairs(options) do
		criteria[k] = v
	end
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected optional map default to empty table to type-check, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_GuardedTableElementKeepsRecordFields(t *testing.T) {
	source := `
		local blocks = {}
		local data = nil :: any
		local event_type = nil :: any

		while true do
			if event_type == "content_block_start" then
				if data.index ~= nil and data.content_block then
					if data.content_block.type == "thinking" then
						blocks[data.index] = {
							type = "thinking",
							thinking = data.content_block.thinking or "",
							signature = data.content_block.signature or "",
						}
					end
				end
			elseif event_type == "content_block_delta" then
				local index = data.index or 0
				local delta = data.delta or {}
				if delta.type == "thinking_delta" then
					local thinking_chunk = delta.thinking or ""
					if blocks[index] then
						blocks[index].thinking = blocks[index].thinking .. thinking_chunk
					end
				elseif delta.type == "signature_delta" then
					local signature_chunk = delta.signature or ""
					if blocks[index] then
						blocks[index].signature = blocks[index].signature .. signature_chunk
					end
				end
			end
			break
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected guarded table element fields to stay available, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_NestedAnyFieldFallbackInArithmetic(t *testing.T) {
	source := `
		local stats = nil :: any
		if stats.cpu_stats and stats.precpu_stats then
			local cpu_delta = (stats.cpu_stats.cpu_usage and stats.cpu_stats.cpu_usage.total_usage or 0) -
				(stats.precpu_stats.cpu_usage and stats.precpu_stats.cpu_usage.total_usage or 0)
			local sys_delta = (stats.cpu_stats.system_cpu_usage or 0) - (stats.precpu_stats.system_cpu_usage or 0)
			if sys_delta > 0 and cpu_delta > 0 then
				local cpu_percent = (cpu_delta / sys_delta) * 100
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected nested any field fallback arithmetic to type-check, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_ParsedAnyBodyNestedFieldFallbackInArithmetic(t *testing.T) {
	source := `
		local function parse_response(body: any)
			if not body then
				return nil
			end
			if type(body) == "table" then
				return body
			end
			if type(body) == "string" then
				return { raw = body }
			end
			return body
		end

		local function container_stats()
			local response = nil :: any
			local result = {
				body = parse_response(response.body),
			}
			return result.body, nil
		end

		local stats, err = container_stats()
		if err then
			return
		end
		if stats.cpu_stats and stats.precpu_stats then
			local cpu_delta = (stats.cpu_stats.cpu_usage and stats.cpu_stats.cpu_usage.total_usage or 0) -
				(stats.precpu_stats.cpu_usage and stats.precpu_stats.cpu_usage.total_usage or 0)
			local sys_delta = (stats.cpu_stats.system_cpu_usage or 0) - (stats.precpu_stats.system_cpu_usage or 0)
			if sys_delta > 0 and cpu_delta > 0 then
				local cpu_percent = (cpu_delta / sys_delta) * 100
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected parsed any body arithmetic to type-check, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_ErrorReturnSuccessWithImplicitNilErrorNarrowsSibling(t *testing.T) {
	httpResponse := typ.NewRecord().
		Field("status_code", typ.Integer).
		OptField("body", typ.String).
		Build()
	httpManifest := io.NewManifest("http_client")
	httpManifest.SetExport(typ.NewRecord().
		Field("get", typ.Func().
			Param("url", typ.String).
			OptParam("options", typ.Any).
			Returns(typ.NewOptional(httpResponse), typ.NewOptional(typ.String)).
			Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})).
			Build()).
		Build())

	testModule := testutil.CheckAndExport(`
		local test = {}

		function test.is_nil(val: any, msg: string?)
			if val ~= nil then
				error(msg or "assertion failed")
			end
		end

		return test
	`, "test_mod", testutil.WithStdlib())
	if testModule.HasError() {
		t.Fatalf("unexpected test module errors: %v", testutil.ErrorMessages(testModule.Errors))
	}

	jsonModule := testutil.CheckAndExport(`
		local json = {}

		function json.decode(raw: string): any
			if raw == "" then
				return nil, "empty"
			end
			return {
				candidates = {
					{ content = { parts = { { text = "Hello" } } } }
				}
			}
		end

		return json
	`, "json", testutil.WithStdlib())
	if jsonModule.HasError() {
		t.Fatalf("unexpected json module errors: %v", testutil.ErrorMessages(jsonModule.Errors))
	}

	source := `
		local http_client = require("http_client")
		local json = require("json")
		local test = require("test_mod")

		local client = {
			_http_client = http_client,
		}

		local function parse_error_response(http_response)
			return {
				status_code = http_response.status_code,
				message = "request failed",
			}
		end

		function client.request(method, url, http_options)
			local response, err = client._http_client.get(url, http_options)
			if not response then
				return nil, {
					status_code = 0,
					message = tostring(err),
				}
			end

			if response.status_code < 200 or response.status_code >= 300 then
				return nil, parse_error_response(response)
			end

			local parsed, parse_err = json.decode(tostring(response.body or ""))
			if parse_err then
				return nil, {
					status_code = response.status_code,
					message = parse_err,
					metadata = {},
				}
			end

			parsed.metadata = {}
			parsed.status_code = response.status_code
			return parsed
		end

		local response, err = client.request("GET", "https://example.test", { headers = {} })
		test.is_nil(err)
		local text = response.candidates[1].content.parts[1].text
		return text
	`
	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("http_client", httpManifest),
		testutil.WithModule("json", jsonModule),
		testutil.WithModule("test_mod", testModule),
	)
	if result.HasError() {
		t.Errorf("expected is_nil(err) to narrow implicit-success error return sibling, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_MethodReceiverParameterEvidenceInfersCapturedSelfFields(t *testing.T) {
	source := `
		type Output = {
			kind: "rendered",
			label: string?,
		}

		type HandlerBuilder = {
			name: string?,
			prefix: string?,
			prefix_with: (self: HandlerBuilder, prefix: string) -> HandlerBuilder,
			build: (self: HandlerBuilder) -> () -> Output,
		}

		type Builder = HandlerBuilder

		local Builder = {}
		Builder.__index = Builder

		local M = {}

		function M.new(): HandlerBuilder
			local self: Builder = {
				name = nil,
				prefix = nil,
				prefix_with = Builder.prefix_with,
				build = Builder.build,
			}
			setmetatable(self, Builder)
			return self
		end

		function Builder:prefix_with(prefix: string): Builder
			self.prefix = prefix
			return self
		end

		function Builder:build(): () -> Output
			local name = self.name or "plugin"
			local prefix = self.prefix or name
			local check_prefix: string = prefix

			return function(): Output
				return {
					kind = "rendered",
					label = prefix,
				}
			end
		end

		local handler = M.new()
			:prefix_with("render")
			:build()

		local out: Output = handler()
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected method receiver evidence to type captured builder fields, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_ErrorReturnDelegatedHelperNarrowsSibling(t *testing.T) {
	testModule := testutil.CheckAndExport(`
		local test = {}

		function test.is_nil(val: any, msg: string?)
			if val ~= nil then
				error(msg or "assertion failed")
			end
		end

		return test
	`, "test_mod", testutil.WithStdlib())
	if testModule.HasError() {
		t.Fatalf("unexpected test module errors: %v", testutil.ErrorMessages(testModule.Errors))
	}

	source := `
		local test = require("test_mod")

		local function finish(ok)
			if not ok then
				return nil, { message = "failed" }, nil
			end
			return {
				candidates = {
					{ content = { parts = { { text = "Hello" } } } }
				}
			}, nil, { source = "finish" }
		end

		local function request(ok)
			if not ok then
				return nil, { message = "failed early" }, nil
			end
			return finish(ok)
		end

		local response, err = request(true)
		test.is_nil(err)
		local text = response.candidates[1].content.parts[1].text
		return text
	`
	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithModule("test_mod", testModule),
	)
	if result.HasError() {
		t.Errorf("expected is_nil(err) to narrow delegated error-return helper sibling, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_ImportedClientMockResponseKeepsDecodedArrayPresence(t *testing.T) {
	jsonManifest := io.NewManifest("json")
	jsonManifest.SetExport(typ.NewRecord().
		Field("encode", typ.Func().
			Param("value", typ.Any).
			Returns(typ.String, typ.NewOptional(typ.LuaError)).
			Build()).
		Field("decode", typ.Func().
			Param("source", typ.String).
			Returns(typ.Any, typ.NewOptional(typ.LuaError)).
			Build()).
		Build())

	testModule := testutil.CheckAndExport(`
		local test = {}

		function test.is_nil(val: any, msg: string?)
			if val ~= nil then
				error(msg or "assertion failed")
			end
		end

		function test.eq(_actual: any, _expected: any, _msg: string?)
		end

		return test
	`, "test_mod", testutil.WithStdlib())
	if testModule.HasError() {
		t.Fatalf("unexpected test module errors: %v", testutil.ErrorMessages(testModule.Errors))
	}

	clientModule := testutil.CheckAndExport(`
		local json = require("json")

		local client = {
			_http_client = nil :: any,
		}

		local function parse_error_response(http_response)
			return {
				status_code = http_response.status_code,
				message = "request failed",
			}
		end

		function client.request(method, url, http_options)
			local response = nil
			local err = nil
			if method == "GET" then
				response, err = client._http_client.get(url, http_options)
			else
				response, err = client._http_client.post(url, http_options)
			end

			if not response then
				return nil, {
					status_code = 0,
					message = tostring(err),
				}
			end

			if response.status_code < 200 or response.status_code >= 300 then
				return nil, parse_error_response(response)
			end

			local parsed, parse_err = json.decode(tostring(response.body or ""))
			if parse_err then
				return nil, {
					status_code = response.status_code,
					message = tostring(parse_err),
					metadata = {},
				}
			end

			parsed.metadata = {}
			parsed.status_code = response.status_code
			return parsed
		end

		return client
	`, "client_mod", testutil.WithStdlib(), testutil.WithManifest("json", jsonManifest))
	if clientModule.HasError() {
		t.Fatalf("unexpected client module errors: %v", testutil.ErrorMessages(clientModule.Errors))
	}

	source := `
		local client = require("client_mod")
		local json = require("json")
		local test = require("test_mod")

		client._http_client = {
			get = function(_url, _options)
				return {
					status_code = 200,
					body = json.encode({
						candidates = {
							{ content = { parts = { { text = "Hello" } } } }
						}
					})
				}
			end
		}

		local response, err = client.request("GET", "https://example.test", { headers = {} })
		test.is_nil(err)
		test.eq(response.candidates[1].content.parts[1].text, "Hello")
	`
	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("json", jsonManifest),
		testutil.WithModule("client_mod", clientModule),
		testutil.WithModule("test_mod", testModule),
	)
	if result.HasError() {
		t.Errorf("expected imported client mock response to preserve decoded array presence after err narrowing, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_ImportedGoogleLikeClientSuiteKeepsCandidatesArray(t *testing.T) {
	jsonManifest := io.NewManifest("json")
	jsonManifest.SetExport(typ.NewRecord().
		Field("encode", typ.Func().
			Param("value", typ.Any).
			Returns(typ.String, typ.NewOptional(typ.LuaError)).
			Build()).
		Field("decode", typ.Func().
			Param("source", typ.String).
			Returns(typ.Any, typ.NewOptional(typ.LuaError)).
			Build()).
		Build())

	streamReaderType := typ.NewInterface("http_client.StreamReader", []typ.Method{
		{Name: "read", Type: typ.Func().Param("self", typ.Self).OptParam("size", typ.Number).Returns(typ.String, typ.NewOptional(typ.LuaError)).Build()},
	})
	httpResponse := typ.NewRecord().
		Field("status_code", typ.Number).
		OptField("body", typ.String).
		OptField("stream", streamReaderType).
		Build()
	httpFn := typ.Func().
		Param("url", typ.String).
		OptParam("options", typ.Any).
		Returns(httpResponse, typ.NewOptional(typ.LuaError)).
		Build()
	httpManifest := io.NewManifest("http_client")
	httpManifest.SetExport(typ.NewRecord().
		Field("get", httpFn).
		Field("post", httpFn).
		Build())

	outputModule := testutil.CheckAndExport(`
		local output = {}

		function output.streamer(_pid: string?, _topic: string?, _buffer_size: any?)
			return {
				buffer_content = function(self, _text: string?) return true end,
				send_tool_call = function(self, _name: string, _arguments: string, _id: string?) return true end,
				send_thinking = function(self, _text: string) return true end,
				send_error = function(self, _kind: string, _message: string, _code: any?) return true end,
				flush = function(self) return true end,
			}, nil
		end

		return output
	`, "output_mod", testutil.WithStdlib())
	if outputModule.HasError() {
		t.Fatalf("unexpected output module errors: %v", testutil.ErrorMessages(outputModule.Errors))
	}

	testModule := testutil.CheckAndExport(`
		local test = {}

		function test.is_nil(val: any, msg: string?)
			if val ~= nil then
				error(msg or "assertion failed")
			end
		end

		function test.eq(_actual: any, _expected: any, _msg: string?)
		end

		function test.not_nil(val: any, msg: string?): any
			if val == nil then
				error(msg or "assertion failed")
			end
			return val
		end

		function test.describe(_name: string, fn: fun())
			fn()
		end

		function test.it(_name: string, fn: fun())
			fn()
		end

		function test.after_each(fn: fun())
			fn()
		end

		function test.run_cases(define_cases_fn: fun())
			return function()
				_G.describe = test.describe
				_G.it = test.it
				_G.after_each = test.after_each
				define_cases_fn()
				_G.describe = nil
				_G.it = nil
				_G.after_each = nil
			end
		end

		return test
	`, "test_mod", testutil.WithStdlib())
	if testModule.HasError() {
		t.Fatalf("unexpected test module errors: %v", testutil.ErrorMessages(testModule.Errors))
	}

	clientModule := testutil.CheckAndExport(`
		local json = require("json")
		local http_client = require("http_client")
		local output = require("output_mod")

		local client = {
			_http_client = http_client,
		}

		local function extract_response_metadata(response_body: any)
			if not response_body then
				return {}
			end
			return {
				model_version = response_body.modelVersion,
				response_id = response_body.responseId,
				create_time = response_body.createTime,
			}
		end

		function client.process_stream(stream_response, callbacks)
			callbacks = callbacks or {}
			local on_done = callbacks.on_done or function(_result) end
			local metadata = stream_response.metadata or {}
			local result = {
				content = "stream",
				tool_calls = {},
				finish_reason = "stop",
				usage = nil,
				metadata = metadata,
			}
			on_done(result)
			return "stream", nil, result
		end

		local function handle_stream_response(response, http_options)
			local streamer = output.streamer(http_options.stream_reply_to, http_options.stream_topic, http_options.stream_buffer_size or 10)
			if not streamer then
				return nil, { status_code = 500, message = "Failed to create streamer" }
			end

			local full_content = ""
			local tool_call_parts = {}
			local finish_reason = nil
			local usage_metadata = nil
			local response_metadata = {}
			local callbacks = {
				on_content = function(chunk: string)
					full_content = full_content .. chunk
					streamer:buffer_content(chunk)
				end,
				on_tool_call = function(tool_part: any)
					table.insert(tool_call_parts, tool_part)
				end,
				on_done = function(result)
					streamer:flush()
					finish_reason = result.finish_reason
					usage_metadata = result.usage
					response_metadata = result.metadata
				end,
			}

			local _, stream_err = client.process_stream({ stream = response.stream, metadata = {} }, callbacks)
			if stream_err then
				return nil, { status_code = 500, message = tostring(stream_err) }
			end

			local parts = {}
			if full_content ~= "" then
				table.insert(parts, { text = full_content })
			end
			for _, tc_part in ipairs(tool_call_parts) do
				table.insert(parts, tc_part)
			end

			return {
				candidates = {
					{
						content = { parts = parts, role = "model" },
						finishReason = finish_reason,
					},
				},
				usageMetadata = usage_metadata,
				metadata = response_metadata,
				status_code = response.status_code or 200,
			}
		end

		function client.request(method, url, http_options)
			http_options.headers["Accept"] = "application/json"
			if http_options.stream then
				url = url .. "?alt=sse"
				http_options.headers["Accept"] = "text/event-stream"
			end

			local response = nil
			local err = nil
			if method == "GET" then
				response, err = client._http_client.get(url, http_options)
			else
				http_options.headers["Content-Type"] = "application/json"
				response, err = client._http_client.post(url, http_options)
			end

			if not response then
				return nil, { status_code = 0, message = tostring(err) }
			end

			if response.status_code < 200 or response.status_code >= 300 then
				return nil, { status_code = response.status_code, message = "bad" }
			end

			if http_options.stream and response.stream then
				return handle_stream_response(response, http_options)
			end

			local parsed, parse_err = json.decode(response.body or "")
			if parse_err then
				return nil, { status_code = response.status_code, message = tostring(parse_err), metadata = {} }
			end

			parsed.metadata = extract_response_metadata(parsed)
			parsed.status_code = response.status_code
			return parsed
		end

		return client
	`, "client_mod",
		testutil.WithStdlib(),
		testutil.WithManifest("json", jsonManifest),
		testutil.WithManifest("http_client", httpManifest),
		testutil.WithModule("output_mod", outputModule),
	)
	if clientModule.HasError() {
		t.Fatalf("unexpected client module errors: %v", testutil.ErrorMessages(clientModule.Errors))
	}

	source := `
		local client = require("client_mod")
		local json = require("json")
		local tests = require("test_mod")

		local function define_tests()
			describe("client", function()
				after_each(function()
					client._http_client = nil
				end)

				it("data response", function()
					client._http_client = {
						get = function(_url, _options)
							return {
								status_code = 200,
								body = json.encode({ data = "test" }),
							}
						end,
					}

					local response, err = client.request("GET", "https://example.test", { headers = {} })
					tests.is_nil(err)
					tests.eq(response.data, "test")
				end)

				it("post response", function()
					client._http_client = {
						post = function(_url, _options)
							return {
								status_code = 200,
								body = json.encode({ data = "test" }),
							}
						end,
					}

					local response, err = client.request("POST", "https://example.test", { headers = {}, body = json.encode({ test = "data" }) })
					tests.is_nil(err)
					tests.eq(response.data, "test")
				end)

				it("candidate response", function()
					client._http_client = {
						get = function(_url, _options)
							return {
								status_code = 200,
								body = json.encode({
									candidates = {
										{ content = { parts = { { text = "Hello" } } } },
									},
								}),
							}
						end,
					}

					local response, err = client.request("GET", "https://example.test", { headers = {} })
					tests.is_nil(err)
					tests.eq(response.candidates[1].content.parts[1].text, "Hello")
				end)
			end)
		end

		return tests.run_cases(define_tests)
	`
	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithManifest("json", jsonManifest),
		testutil.WithModule("client_mod", clientModule),
		testutil.WithModule("test_mod", testModule),
	)
	if result.HasError() {
		t.Errorf("expected imported Google-like client suite to preserve candidates as an array, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_ImportedMutableClientFieldIsCallSiteSensitive(t *testing.T) {
	testModule := testutil.CheckAndExport(`
		local test = {}

		function test.is_nil(val: any, msg: string?)
			if val ~= nil then
				error(msg or "assertion failed")
			end
		end

		return test
	`, "test_mod", testutil.WithStdlib())
	if testModule.HasError() {
		t.Fatalf("unexpected test module errors: %v", testutil.ErrorMessages(testModule.Errors))
	}

	clientModule := testutil.CheckAndExport(`
		local client = {
			_http_client = nil :: any,
		}

		function client.request()
			local response, err = client._http_client.get()
			if not response then
				return nil, { message = tostring(err) }
			end
			return response.body
		end

		return client
	`, "client_mod", testutil.WithStdlib())
	if clientModule.HasError() {
		t.Fatalf("unexpected client module errors: %v", testutil.ErrorMessages(clientModule.Errors))
	}

	source := `
		local client = require("client_mod")
		local test = require("test_mod")

		local function candidate_case()
			client._http_client = {
				get = function()
					return {
						body = {
							candidates = {
								{ content = { parts = { { text = "Hello" } } } }
							}
						}
					}
				end
			}

			local response, err = client.request()
			test.is_nil(err)
			return response.candidates[1].content.parts[1].text
		end

		local function data_case()
			client._http_client = {
				get = function()
					return {
						body = {
							data = "other",
						}
					}
				end
			}

			local response, err = client.request()
			test.is_nil(err)
			return response.data
		end

		return candidate_case(), data_case()
	`
	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithModule("client_mod", clientModule),
		testutil.WithModule("test_mod", testModule),
	)
	if result.HasError() {
		t.Errorf("expected imported mutable client field calls to use the visible mock at each call site, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_ImportedMutableClientFieldIsCallbackLocal(t *testing.T) {
	testModule := testutil.CheckAndExport(`
		local test = { _cases = {} }

		function test.is_nil(val: any, msg: string?)
			if val ~= nil then
				error(msg or "assertion failed")
			end
		end

		function test.eq(_actual: any, _expected: any, _msg: string?)
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
	if testModule.HasError() {
		t.Fatalf("unexpected test module errors: %v", testutil.ErrorMessages(testModule.Errors))
	}

	clientModule := testutil.CheckAndExport(`
		local client = {
			_http_client = nil :: any,
		}

		function client.request(method, url, http_options)
			http_options.headers["Accept"] = "application/json"

			if http_options.stream then
				return {
					candidates = {
						{ content = { parts = { { text = "stream" } }, role = "model" } }
					}
				}, nil
			end

			local response = nil
			local err = nil
			if method == "GET" then
				response, err = client._http_client.get(url, http_options)
			else
				response, err = client._http_client.post(url, http_options)
			end

			if not response then
				return nil, { message = tostring(err) }
			end
			if response.status_code < 200 or response.status_code >= 300 then
				return nil, { status_code = response.status_code, message = "bad" }
			end

			return response.body, nil
		end

		return client
	`, "client_mod", testutil.WithStdlib())
	if clientModule.HasError() {
		t.Fatalf("unexpected client module errors: %v", testutil.ErrorMessages(clientModule.Errors))
	}

	source := `
		local client = require("client_mod")
		local tests = require("test_mod")

		local function define_tests()
			describe("client", function()
				it("candidate response", function()
					client._http_client = {
						get = function(_url, _options)
							return {
								status_code = 200,
								body = {
									candidates = {
										{ content = { parts = { { text = "Hello" } }, role = "model" } }
									}
								}
							}
						end
					}

					local response, err = client.request("GET", "https://example.test", { headers = {} })
					tests.is_nil(err)
					tests.eq(response.candidates[1].content.parts[1].text, "Hello")
				end)

				it("data response", function()
					client._http_client = {
						get = function(_url, _options)
							return {
								status_code = 200,
								body = { data = "test" }
							}
						end
					}

					local response, err = client.request("GET", "https://example.test", { headers = {} })
					tests.is_nil(err)
					tests.eq(response.data, "test")
				end)
			end)
		end

		return tests.run_cases(define_tests)
	`
	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithModule("client_mod", clientModule),
		testutil.WithModule("test_mod", testModule),
	)
	if result.HasError() {
		t.Errorf("expected imported client mock fields to stay callback-local, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_CallbackLocalDelegatedErrorReturnNarrowsSibling(t *testing.T) {
	testModule := testutil.CheckAndExport(`
		local test = { _cases = {} }

		function test.is_nil(val: any, msg: string?)
			if val ~= nil then
				error(msg or "assertion failed")
			end
		end

		function test.eq(_actual: any, _expected: any, _msg: string?)
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
	if testModule.HasError() {
		t.Fatalf("unexpected test module errors: %v", testutil.ErrorMessages(testModule.Errors))
	}

	source := `
		local tests = require("test_mod")

		local client = {
			_http_client = nil :: any,
		}

		local function handle_stream_response(response, http_options)
			if response.err then
				return nil, { message = "stream failed" }
			end
			return {
				candidates = {
					{ content = { parts = { { text = "stream" } }, role = "model" } }
				}
			}
		end

		function client.request(method, url, http_options)
			http_options.headers["Accept"] = "application/json"
			if http_options.stream then
				http_options.headers["Accept"] = "text/event-stream"
			end

			local response = nil
			local err = nil
			if method == "GET" then
				response, err = client._http_client.get(url, http_options)
			else
				response, err = client._http_client.post(url, http_options)
			end

			if not response then
				return nil, { message = tostring(err) }
			end
			if response.status_code < 200 or response.status_code >= 300 then
				return nil, { status_code = response.status_code, message = "bad" }
			end
			if http_options.stream and response.stream then
				return handle_stream_response(response, http_options)
			end
			return response.body
		end

		local function define_tests()
			describe("client", function()
				it("data response", function()
					client._http_client = {
						get = function(_url, _options)
							return {
								status_code = 200,
								body = { data = "test" }
							}
						end
					}

					local response, err = client.request("GET", "https://example.test", { headers = {} })
					tests.is_nil(err)
					tests.eq(response.data, "test")
				end)
			end)
		end

		return tests.run_cases(define_tests)
	`
	result := testutil.Check(source,
		testutil.WithStdlib(),
		testutil.WithModule("test_mod", testModule),
	)
	if result.HasError() {
		t.Errorf("expected delegated error-return relation to narrow inside callback, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_ArithmeticOnOptionalAfterGuard(t *testing.T) {
	source := `
		local values: {[integer]: number} = {10, 20, 30}

		function compute(i: integer): number
			local v = values[i]
			if v == nil then
				return 0
			end
			return v * 2
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after nil guard on map value, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// 3. E0002: Expected function, got never
// Pattern: After reassignment where variable is narrowed incorrectly

func TestFalsePositive_NeverAfterReassignment(t *testing.T) {
	source := `
		type Result = {ok: boolean, value: string?}

		function process(): Result
			local result: Result = {ok = true, value = nil}
			local err: string? = nil

			result, err = {ok = true, value = "first"}, nil
			if err then
				return {ok = false, value = nil}
			end

			result, err = {ok = true, value = "second"}, nil
			if err then
				return {ok = false, value = nil}
			end

			return result
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after multi-assignment, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_NeverAfterMultiReturnReassignment(t *testing.T) {
	source := `
		type Obj = {
			stat: (self: Obj, path: string) -> (boolean, string?),
			write: (self: Obj, data: string) -> (boolean, string?)
		}

		function process(vol: Obj)
			local ok: boolean
			local err: string?

			ok, err = vol:stat("file")
			if err then
				return
			end

			ok, err = vol:write("data")
			if err then
				return
			end

			if ok then
				print("success")
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after multi-return reassignment, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// 4. Reassignment kills old constraints
// Bug: assert.is_nil(vol) on first definition shouldn't affect second definition

func TestFalsePositive_ReassignmentKillsConstraints(t *testing.T) {
	source := `
		type File = {
			open: (self: File, mode: string) -> boolean
		}

		function getFile(name: string): File?
			return nil
		end

		function test()
			local vol: File? = getFile("nonexistent")
			-- Guard pattern: if not nil, return early
			if vol ~= nil then
				return
			end
			-- After guard: vol is narrowed to nil

			-- Reassignment: vol gets new value
			vol = getFile("valid")
			-- After reassignment: vol should be File? (not nil)

			-- Second guard
			if vol == nil then
				return
			end
			-- After guard: vol should be File (not nil)

			local ok: boolean = vol:open("r")
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after reassignment, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFalsePositive_ReassignmentKillsIsNilConstraint(t *testing.T) {
	source := `
		type Obj = {
			method: (self: Obj) -> string
		}

		function getObj(name: string): Obj?
			return nil
		end

		function test()
			local obj: Obj? = getObj("first")
			if obj == nil then
				-- obj is nil here
			end

			obj = getObj("second")
			if obj ~= nil then
				local s: string = obj:method()
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after reassignment kills old constraint, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Additional pattern: method call after multiple guards
func TestFalsePositive_MethodCallAfterMultipleGuards(t *testing.T) {
	source := `
		type FileSystem = {
			exists: (self: FileSystem, path: string) -> boolean,
			read: (self: FileSystem, path: string) -> (string?, string?)
		}

		function loadConfig(fs: FileSystem, path: string): string?
			if not fs:exists(path) then
				return nil
			end

			local content, err = fs:read(path)
			if err then
				return nil
			end

			return content
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for method calls with guards, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Pattern from wippy: error() call in else branch after truthy check
// if result.process_called then ... else error("...") end
func TestFalsePositive_ErrorInElseBranchAfterTruthyCheck(t *testing.T) {
	source := `
		type Result = { process_called: boolean?, process_to_func_id: string? }

		function test(result: Result)
			if result.process_called then
				return "ok"
			else
				error("process_called marker not inherited: got " .. tostring(result.process_called))
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for error() in else branch, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Pattern from wippy: method call on executor returns never
// exec:call("name", args) and exec:async("name", args)
func TestFalsePositive_MethodCallReturnsNever(t *testing.T) {
	source := `
		type Executor = {
			call: (self: Executor, name: string, ...any) -> (any, string?),
			async: (self: Executor, name: string, ...any) -> (any, string?)
		}

		function test(exec: Executor)
			local result, err = exec:call("app.test.funcs:echo", "executor call")
			if err then
				error(err)
			end
			return result
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for method call, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Pattern from wippy: multiple conditional checks then error
func TestFalsePositive_MultipleConditionsThenError(t *testing.T) {
	source := `
		type Meta = { role: string?, department: string? }

		function validate(meta: Meta)
			if meta.role ~= "admin" then
				error("actor role mismatch: expected admin, got " .. tostring(meta.role))
			end
			if meta.department ~= "engineering" then
				error("actor department mismatch: expected engineering, got " .. tostring(meta.department))
			end
			return true
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for conditional error calls, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Pattern from wippy: funcs.call returns any, field access then conditional error
// funcs.new():call() returns (any, error?) - result is any
func TestFalsePositive_AnyFieldAccessThenConditionalError(t *testing.T) {
	source := `
		type Executor = {
			call: (self: Executor, name: string, ...any) -> (any, string?)
		}

		function test(exec: Executor)
			local result, err = exec:call("app.test.ctx:ctx_reader", { "process_to_func_id", "process_called" })
			if err then
				error("call failed: " .. tostring(err))
			end

			-- result is any, result.process_to_func_id is any
			if result.process_to_func_id ~= "ptf-321" then
				error("process_to_func_id not inherited: got " .. tostring(result.process_to_func_id))
			end

			if result.process_called ~= true then
				error("process_called marker not inherited: got " .. tostring(result.process_called))
			end

			return true
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for any field access with conditional error, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Pattern from wippy: type() check on any-typed value makes else branch unreachable
// if x then if type(x) == "table" then ... else tostring(x) end end
func TestFalsePositive_TypeCheckOnAnyElseBranchReachable(t *testing.T) {
	source := `
		type Event = { result: any }

		function test(event: Event)
			if event.result then
				if type(event.result) == "table" then
					return "table"
				else
					return tostring(event.result)
				end
			end
			return "nil"
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for type check else branch, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Pattern from wippy link_explicit: variable assigned in loop, nil guard, then type check
func TestFalsePositive_LoopAssignedVarTypeCheckElseBranch(t *testing.T) {
	source := `
		type Item = { pid: integer, result: any }

		function test(items: {Item})
			local found: Item? = nil
			for _, item in ipairs(items) do
				if item.pid == 123 then
					found = item
					break
				end
			end

			if not found then
				return "not found"
			end

			-- After nil guard, found should be Item (not nil)
			if found.result then
				if type(found.result) == "table" then
					return "table"
				else
					-- This should NOT be unreachable
					return tostring(found.result)
				end
			end
			return "nil result"
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for loop-assigned var type check, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Pattern from wippy link_explicit: OPTIONAL any field, truthy check, type check makes else unreachable
func TestFalsePositive_OptionalAnyFieldTypeCheckElseBranch(t *testing.T) {
	source := `
		type Event = { kind: string, result?: any }
		type Item = { pid: integer, result?: any }

		function test(event: Event)
			local item: Item = { pid = 1, result = event.result }

			if item.result then
				if type(item.result) == "table" then
					return "table"
				else
					-- This should NOT be unreachable
					return tostring(item.result)
				end
			end
			return "nil"
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for optional any field type check, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Pattern from wippy executor: method returning Self, then assert.neq comparison
func TestFalsePositive_SelfMethodThenNeqAssertion(t *testing.T) {
	source := `
		type Executor = {
			with_options: (self: Executor, opts: any) -> Executor,
			call: (self: Executor, name: string, ...any) -> (any, string?)
		}

		function new_executor(): Executor
			return {} :: Executor
		end

		function not_nil(v: any, msg: string)
			if v == nil then error(msg) end
		end

		function neq(a: any, b: any, msg: string)
			if a == b then error(msg) end
		end

		function test()
			local exec = new_executor()
			not_nil(exec, "exec created")

			local exec2 = exec:with_options({ timeout = 1000 })
			not_nil(exec2, "with_options returns executor")
			neq(exec, exec2, "with_options returns new executor")

			-- After neq(exec, exec2), exec should NOT be narrowed to never
			local result, err = exec:call("test", "arg")
			return result
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after neq assertion, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Simplified: just the neq pattern
func TestFalsePositive_NeqAssertionSimple(t *testing.T) {
	source := `
		type Obj = { call: (self: Obj) -> any }

		function make(): Obj
			return {} :: Obj
		end

		function neq(a: any, b: any)
			if a == b then error("equal") end
		end

		function test()
			local x = make()
			local y = make()
			neq(x, y)
			return x:call()
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after simple neq, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// With method call between make and neq
func TestFalsePositive_NeqAfterMethodCall(t *testing.T) {
	source := `
		type Obj = {
			derive: (self: Obj) -> Obj,
			call: (self: Obj) -> any
		}

		function make(): Obj
			return {} :: Obj
		end

		function neq(a: any, b: any)
			if a == b then error("equal") end
		end

		function test()
			local x = make()
			local y = x:derive()
			neq(x, y)
			return x:call()
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after neq with derive, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// With not_nil assertions before neq
func TestFalsePositive_NotNilThenNeq(t *testing.T) {
	source := `
		type Obj = {
			derive: (self: Obj) -> Obj,
			call: (self: Obj) -> any
		}

		function make(): Obj
			return {} :: Obj
		end

		function not_nil(v: any)
			if v == nil then error("nil") end
		end

		function neq(a: any, b: any)
			if a == b then error("equal") end
		end

		function test()
			local x = make()
			not_nil(x)

			local y = x:derive()
			not_nil(y)
			neq(x, y)

			return x:call()
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors after not_nil + neq, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Exact wippy pattern: derive returns same type, then neq with derived, then call original
func TestFalsePositive_DeriveNeqThenCallOriginal(t *testing.T) {
	source := `
		type Executor = {
			with_options: (self: Executor, opts: any) -> Executor,
			call: (self: Executor, name: string, ...any) -> (any, string?)
		}

		function new_executor(): Executor
			return {} :: Executor
		end

		function not_nil(v: any, msg: string)
			if v == nil then error(msg) end
		end

		function neq(a: any, b: any, msg: string)
			if a == b then error(msg) end
		end

		function main()
			local exec = new_executor()
			not_nil(exec, "executor created")

			-- derive new executor from original
			local exec2 = exec:with_options({ timeout = 1000 })
			not_nil(exec2, "with_options returns executor")

			-- assert they're different objects
			neq(exec, exec2, "with_options returns new executor")

			-- call on ORIGINAL - this is where wippy fails
			local result, err = exec:call("test:echo", "arg")
			not_nil(result, "call returns result")

			-- later call async on same original
			local future, aerr = exec:call("test:echo", "arg2")
			return result
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for derive-neq-call pattern, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Minimal: just neq then call - without not_nil assertions
func TestFalsePositive_NeqThenCall_Minimal(t *testing.T) {
	source := `
		type Obj = {
			derive: (self: Obj) -> Obj,
			call: (self: Obj) -> any
		}

		function make(): Obj
			return {} :: Obj
		end

		function neq(a: any, b: any)
			if a == b then error("equal") end
		end

		function test()
			local x = make()
			local y = x:derive()
			neq(x, y)
			return x:call()
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for minimal neq-then-call, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// With not_nil before neq - same types as full test
func TestFalsePositive_NotNilThenNeqExact(t *testing.T) {
	source := `
		type Executor = {
			with_options: (self: Executor, opts: any) -> Executor,
			call: (self: Executor, name: string, ...any) -> (any, string?)
		}

		function new_executor(): Executor
			return {} :: Executor
		end

		function not_nil(v: any, msg: string)
			if v == nil then error(msg) end
		end

		function neq(a: any, b: any, msg: string)
			if a == b then error(msg) end
		end

		function main()
			local exec = new_executor()
			not_nil(exec, "a")
			local exec2 = exec:with_options({})
			not_nil(exec2, "b")
			neq(exec, exec2, "c")
			return exec:call("x", "y")
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// With multi-value assignment after call
func TestFalsePositive_NotNilNeqMultiReturn(t *testing.T) {
	source := `
		type Executor = {
			with_options: (self: Executor, opts: any) -> Executor,
			call: (self: Executor, name: string, ...any) -> (any, string?)
		}

		function new_executor(): Executor
			return {} :: Executor
		end

		function not_nil(v: any, msg: string)
			if v == nil then error(msg) end
		end

		function neq(a: any, b: any, msg: string)
			if a == b then error(msg) end
		end

		function main()
			local exec = new_executor()
			not_nil(exec, "a")
			local exec2 = exec:with_options({})
			not_nil(exec2, "b")
			neq(exec, exec2, "c")
			local result, err = exec:call("x", "y")
			return result
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors with multi-return, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Simplest case: neq then multi-return call
func TestFalsePositive_NeqThenMultiReturn(t *testing.T) {
	source := `
		type Obj = {
			derive: (self: Obj) -> Obj,
			call: (self: Obj) -> (any, string?)
		}

		function make(): Obj
			return {} :: Obj
		end

		function neq(a: any, b: any)
			if a == b then error("eq") end
		end

		function test()
			local x = make()
			local y = x:derive()
			neq(x, y)
			local r, e = x:call()
			return r
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Without neq - just multi-return
func TestFalsePositive_MultiReturnNoNeq(t *testing.T) {
	source := `
		type Obj = {
			derive: (self: Obj) -> Obj,
			call: (self: Obj) -> (any, string?)
		}

		function make(): Obj
			return {} :: Obj
		end

		function test()
			local x = make()
			local y = x:derive()
			local r, e = x:call()
			return r
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors without neq, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// neq without error() - just returns
func TestFalsePositive_NeqNoErrorMultiReturn(t *testing.T) {
	source := `
		type Obj = {
			derive: (self: Obj) -> Obj,
			call: (self: Obj) -> (any, string?)
		}

		function make(): Obj
			return {} :: Obj
		end

		function neq(a: any, b: any)
			if a == b then
				print("equal")
			end
		end

		function test()
			local x = make()
			local y = x:derive()
			neq(x, y)
			local r, e = x:call()
			return r
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors with neq without error, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// neq with error + non-optional multi-return
func TestFalsePositive_NeqErrorNonOptionalMultiReturn(t *testing.T) {
	source := `
		type Obj = {
			derive: (self: Obj) -> Obj,
			call: (self: Obj) -> (any, string)
		}

		function make(): Obj
			return {} :: Obj
		end

		function neq(a: any, b: any)
			if a == b then error("eq") end
		end

		function test()
			local x = make()
			local y = x:derive()
			neq(x, y)
			local r, e = x:call()
			return r
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors with non-optional, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// neq with error + single-return assignment
func TestFalsePositive_NeqErrorSingleReturn(t *testing.T) {
	source := `
		type Obj = {
			derive: (self: Obj) -> Obj,
			call: (self: Obj) -> any
		}

		function make(): Obj
			return {} :: Obj
		end

		function neq(a: any, b: any)
			if a == b then error("eq") end
		end

		function test()
			local x = make()
			local y = x:derive()
			neq(x, y)
			local r = x:call()
			return r
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors with single return, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Type guard else branch should be reachable when field is any
func TestFalsePositive_TypeGuardElseBranchWithAnyField(t *testing.T) {
	source := `
		local exits = {}
		table.insert(exits, {pid = 1, result = "value"})

		local worker_exit = nil
		for _, exit in ipairs(exits) do
			worker_exit = exit
			break
		end

		if not worker_exit then
			return
		end

		if worker_exit.result then
			if type(worker_exit.result) == "table" then
				print("table")
			else
				local s = tostring(worker_exit.result)
				print(s)
			end
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors with type guard else branch, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

// Loop assignment with conditional field access
func TestFalsePositive_LoopAssignmentThenFieldCheck(t *testing.T) {
	source := `
		type Item = {
			pid: integer,
			result: any
		}

		function test()
			local exits: {Item} = {}
			local worker_pid: integer = 1

			local worker_exit: Item? = nil
			for _, exit in ipairs(exits) do
				if exit.pid == worker_pid then
					worker_exit = exit
					break
				end
			end

			if not worker_exit then
				return false, "not found"
			end

			local result_value = worker_exit.result
			if result_value ~= "expected" then
				local result_str = "nil"
				if worker_exit.result then
					result_str = "truthy"
				else
					result_str = tostring(worker_exit.result)
				end
				return false, result_str
			end

			return true
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors with loop assignment pattern, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFP_AndGuardNarrowsOptional(t *testing.T) {
	source := `
		local s: {name: string}? = nil

		function test(): string?
			if s and s.name then
				return s.name
			end
			return nil
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for and-guard on optional, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFP_AndGuardNarrowsOptional_Expression(t *testing.T) {
	source := `
		local s: {name: string}? = nil
		local name = s and s.name or "unknown"
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for and/or expression guard, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFP_AndGuardNarrowsOptional_ErrorReturn(t *testing.T) {
	readerManifest := io.NewManifest("reader")
	readerManifest.SetExport(typ.NewRecord().
		Field("script_by_id", typ.Func().
			Param("id", typ.String).
			Returns(
				typ.NewOptional(typ.NewRecord().Field("name", typ.String).Build()),
				typ.NewOptional(typ.LuaError),
			).
			Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})).
			Build()).
		Build())

	source := `
local reader = require("reader")
local script, _ = reader.script_by_id("id")
local script_name = script and script.name or "Unknown Script"
`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("reader", readerManifest))
	if result.HasError() {
		t.Errorf("expected no errors for and/or guard on error-return value, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFP_OrEmptyStringStaysString(t *testing.T) {
	source := `
		local s: string? = nil
		local r: string = (s or ""):sub(1, 2)
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for or-empty-string, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFP_ErrorReturnOptionalFieldOrEmptyStringStaysString(t *testing.T) {
	responseType := typ.NewRecord().
		Field("status_code", typ.Number).
		OptField("body", typ.String).
		Build()
	httpManifest := io.NewManifest("http_client")
	httpManifest.SetExport(typ.NewRecord().
		Field("post", typ.Func().
			Param("url", typ.String).
			OptParam("opts", typ.Any).
			Returns(typ.NewOptional(responseType), typ.NewOptional(typ.LuaError)).
			Spec(contract.NewSpec().WithEffects(effect.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})).
			Build()).
		Build())

	source := `
		local http = require("http_client")
		local json = require("json")

		local client = {}
		client._http_client = http

		local response, err = client._http_client.post("https://example.local", {})
		if err then
			return nil, err
		end

		return json.decode(response.body or "")
	`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("http_client", httpManifest))
	if result.HasError() {
		t.Errorf("expected no errors for optional response body fallback, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFP_AndGuardNarrowsNestedPath(t *testing.T) {
	source := `
		local rec: {foo: {bar: string}?}? = nil

		function test(): string
			if rec and rec.foo and rec.foo.bar then
				return rec.foo.bar
			end
			return "x"
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for nested and-guard, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFP_AndGuardNarrowsNestedPath_Expression(t *testing.T) {
	source := `
		local rec: {foo: {bar: string}?}? = nil
		local value = rec and rec.foo and rec.foo.bar or "x"
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for nested and/or expression guard, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFP_StringFindSecondReturnNarrowedByFirst(t *testing.T) {
	source := `
		function extract_host(url: string): string
			local start, finish = string.find(url, "://")
			if start then
				return url:sub(finish + 1)
			end
			return url
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for string.find co-correlation, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFP_ModuleExportPreservesParamTypes(t *testing.T) {
	registryMod := testutil.CheckAndExport(`
		local M = {}
		function M.get(id: string): string
			return id
		end
		return M
	`, "registry", testutil.WithStdlib())

	if registryMod.HasError() {
		t.Fatal("provider errors")
	}

	source := `
local registry = require("registry")
local function handler(identifier: string)
	local result = registry.get(identifier)
end
`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithModule("registry", registryMod))
	if result.HasError() {
		t.Errorf("typed param should satisfy module function; got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestFP_TableIndexAfterInitGuard(t *testing.T) {
	source := `
		function test(data: {[string]: string}, key: string): integer
			local start, finish = string.find(data[key] or "", "%d+")
			if not start then
				return 0
			end
			return finish - start + 1
		end
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Errorf("expected no errors for table index after init guard, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
