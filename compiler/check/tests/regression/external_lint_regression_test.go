package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/typ"
)

func TestExternalLint_OptionalResponseBodyDefaultIsStringAtCall(t *testing.T) {
	source := `
local json = {}
function json.decode(raw: string): any
	return {}
end

type Stream = {
	read: (self: Stream, n: number?) -> (string?, string?),
}

type Response = {
	status_code: number,
	body: string?,
	stream: Stream?,
}

local function get_response(): Response
	local stream: Stream = {
		read = function(self: Stream, n: number?)
			return "chunk", nil
		end,
	}
	return { status_code = 500, stream = stream }
end

local response = get_response()
if response.status_code >= 300 then
	if response.stream and not response.body then
		local body_data = response.stream:read(4096)
		response.body = body_data
	end
end

local parsed, parse_err = json.decode(response.body or "")
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected optional body fallback and guarded stream read to type-check, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_ImportedOptionalResponseBodyDefaultIsStringAtCall(t *testing.T) {
	jsonModule := testutil.CheckAndExport(`
local json = {}
function json.decode(raw: string): any
	return {}
end
return json
`, "json", testutil.WithStdlib())
	if jsonModule.HasError() {
		t.Fatalf("json module errors: %v", testutil.ErrorMessages(jsonModule.Errors))
	}

	source := `
local json = require("json")

type Response = {
	status_code: number,
	body: string?,
}

local function request(): (Response?, string?)
	return { status_code = 200 }, nil
end

local response, err = request()
if not response then
	return nil, err
end

local parsed, parse_err = json.decode(response.body or "")
return parsed, parse_err
`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithModule("json", jsonModule))
	if result.HasError() {
		t.Fatalf("expected imported optional body fallback to feed string call argument, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_MethodSelectedOptionalResponseBodyDefaultIsStringAtCall(t *testing.T) {
	jsonModule := testutil.CheckAndExport(`
local json = {}
function json.decode(raw: string): any
	return {}
end
return json
`, "json", testutil.WithStdlib())
	if jsonModule.HasError() {
		t.Fatalf("json module errors: %v", testutil.ErrorMessages(jsonModule.Errors))
	}

	httpModule := testutil.CheckAndExport(`
local http = {}
type Response = {status_code: number, body: string?}
function http.get(url: string, options: {[string]: any}?): (Response?, string?)
	return { status_code = 200, body = nil }, nil
end
function http.post(url: string, options: {[string]: any}?): (Response?, string?)
	return { status_code = 200, body = nil }, nil
end
return http
`, "http_client", testutil.WithStdlib())
	if httpModule.HasError() {
		t.Fatalf("http module errors: %v", testutil.ErrorMessages(httpModule.Errors))
	}

	source := `
local json = require("json")
local http_client = require("http_client")

local function request(method: string)
	local response, err
	if method == "GET" then
		response, err = http_client.get("https://example.test", {})
	else
		response, err = http_client.post("https://example.test", {})
	end

	if not response then
		return nil, err
	end

	local parsed, parse_err = json.decode(response.body or "")
	return parsed, parse_err
end
`
	result := testutil.Check(source, testutil.WithStdlib(),
		testutil.WithModule("json", jsonModule),
		testutil.WithModule("http_client", httpModule))
	if result.HasError() {
		t.Fatalf("expected selected HTTP method body fallback to feed string call argument, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_StdlibJsonOptionalResponseBodyDefaultIsStringAtCall(t *testing.T) {
	source := `
local json = require("json")

type Response = {
	body: string?,
}

local response: Response = {}
return json.decode(response.body or "")
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected stdlib json optional body fallback to feed string call argument, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_GuardedOptionsModelSurvivesProviderBranches(t *testing.T) {
	source := `
local models = {
	get_by_name = function(model_id: string)
		return { name = model_id, providers = { { id = "provider", provider_model = model_id, options = {} } } }, nil
	end,
	get_by_class = function(class_id: string)
		return { { name = class_id, providers = { { id = "provider", provider_model = class_id, options = {} } } } }, nil
	end,
}

local providers = {
	open = function(provider_id: string, options: table)
		return {
			generate = function(self, args)
				return { success = true, result = args.model }, nil
			end,
		}, nil
	end,
}

local security = {
	actor = function()
		return nil
	end,
}

local llm = {}
llm._models = nil
llm._providers = nil

local function resolve_model(model_identifier)
	local models_module = llm._models or models
	local class_name = model_identifier:match("^class:(.+)")
	if class_name then
		local class_models, err = models_module.get_by_class(class_name)
		if err then
			return nil, err
		end
		if class_models and #class_models > 0 then
			return class_models[1]
		end
		return nil, "No models found"
	end

	local model_card, err = models_module.get_by_name(model_identifier)
	if model_card then
		return model_card
	end

	local class_models, class_err = models_module.get_by_class(model_identifier)
	if not class_err and class_models and #class_models > 0 then
		return class_models[1]
	end

	return nil, "Model not found"
end

local function merge_user_options(contract_args, user_options, exclude_keys)
	exclude_keys = exclude_keys or {}
	for k, v in pairs(user_options) do
		local should_exclude = false
		for _, exclude_key in ipairs(exclude_keys) do
			if k == exclude_key then
				should_exclude = true
				break
			end
		end
		if not should_exclude then
			contract_args.options[k] = v
		end
	end
end

function llm.generate(prompt_input, options)
	if not options or not options.model then
		return nil, "Model is required in options"
	end

	local actor = security.actor()
	if actor then
		options.user = actor:id()
	end

	if options.provider_id then
		local providers_module = llm._providers or providers
		local provider_instance, err = providers_module.open(options.provider_id, {})
		if not provider_instance then
			return nil, err
		end

		local contract_args = {
			messages = prompt_input,
			model = options.model,
			options = {},
		}
		merge_user_options(contract_args, options, {"model", "provider_id"})
		return (provider_instance as any):generate(contract_args)
	end

	local model_card, err = resolve_model(options.model)
	if not model_card then
		return nil, err
	end

	local provider_info = model_card.providers[1]
	local providers_module = llm._providers or providers
	local provider_instance, open_err = providers_module.open(provider_info.id, provider_info.options or {})
	if not provider_instance then
		return nil, open_err
	end

	local contract_args = {
		messages = prompt_input,
		model = provider_info.provider_model,
		options = {},
	}
	merge_user_options(contract_args, options, {"model"})
	return (provider_instance as any):generate(contract_args)
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatalf("expected guarded options.model/provider_id to satisfy helper/provider contracts, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_GuardedParamFieldFeedsKnownCall(t *testing.T) {
	source := `
local providers = {
	open = function(provider_id: string, options: table)
		return { id = provider_id }, nil
	end,
}

local function generate(options)
	if options.provider_id then
		return providers.open(options.provider_id, {})
	end
	return nil, nil
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected guarded parameter field to infer from known call expectation, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_GuardedParamFieldFeedsFallbackModuleCall(t *testing.T) {
	source := `
local providers = {
	open = function(provider_id: string, options: table)
		return { id = provider_id }, nil
	end,
}

local api = {}
api._providers = nil

local function generate(options)
	if options.provider_id then
		local providers_module = api._providers or providers
		return providers_module.open(options.provider_id, {})
	end
	return nil, nil
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected guarded parameter field to infer through fallback module alias, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_GuardedParamFieldSurvivesSiblingFieldMutation(t *testing.T) {
	source := `
local providers = {
	open = function(provider_id: string, options: table)
		return { id = provider_id }, nil
	end,
}

local security = {
	actor = function()
		return nil
	end,
}

local api = {}
api._providers = nil

local function generate(options)
	if not options or not options.model then
		return nil, "model required"
	end
	local actor = security.actor()
	if actor then
		options.user = actor:id()
	end
	if options.provider_id then
		local providers_module = api._providers or providers
		return providers_module.open(options.provider_id, {})
	end
	return nil, nil
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected guarded parameter field to survive sibling field mutation, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_CastFieldExpressionFeedsCallArgument(t *testing.T) {
	source := `
local funcs = {}

function funcs.new()
	return {
		call = function(self, name: string, context: table)
			return { id = name }, nil
		end,
	}
end

local function get_page_data(page)
	if not page or not page.data_func or page.data_func == "" then
		return {}, nil
	end

	local executor = funcs.new()
	local result, err = executor:call(page.data_func :: string, {})
	return result, err
end

local result, err = get_page_data({ data_func = true })
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected explicit field cast to feed call argument checking, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_CastUnknownImportedFieldFeedsImportedMethodCall(t *testing.T) {
	templatesModule := testutil.CheckAndExport(`
local templates = {}
function templates.get(id: string)
	return {
		render = function(self, name: string, context: table)
			return name, nil
		end,
		release = function(self)
		end,
	}, nil
end
return templates
`, "templates", testutil.WithStdlib())
	if templatesModule.HasError() {
		t.Fatalf("templates module errors: %v", testutil.ErrorMessages(templatesModule.Errors))
	}

	source := `
local templates = require("templates")

local function get_page()
	return {
		template_set = "main",
		template_name = nil :: unknown,
	}
end

local page = get_page()
local tmpl, err = templates.get(page.template_set)
if err then
	return nil, err
end

local content, render_err = tmpl:render(page.template_name :: string, {})
tmpl:release()
return content, render_err
`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithModule("templates", templatesModule))
	if result.HasError() {
		t.Fatalf("expected explicit cast of imported field to feed method argument checking, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_ErrorGuardedImportedPageFieldCastFeedsMethodCall(t *testing.T) {
	templatesModule := testutil.CheckAndExport(`
local templates = {}
function templates.get(id: string)
	return {
		render = function(self, name: string, context: table)
			return name, nil
		end,
		release = function(self)
		end,
	}, nil
end
return templates
`, "templates", testutil.WithStdlib())
	if templatesModule.HasError() {
		t.Fatalf("templates module errors: %v", testutil.ErrorMessages(templatesModule.Errors))
	}

	pageRegistryModule := testutil.CheckAndExport(`
local pages = {}
function pages.get(id: string)
	if id == "" then
		return nil, "missing"
	end
	return {
		template_set = "main",
		template_name = nil :: unknown,
	}, nil
end
return pages
`, "page_registry", testutil.WithStdlib())
	if pageRegistryModule.HasError() {
		t.Fatalf("page_registry module errors: %v", testutil.ErrorMessages(pageRegistryModule.Errors))
	}

	source := `
local templates = require("templates")
local page_registry = require("page_registry")

local page, err = page_registry.get("home")
if err then
	return nil, err
end

local template_set: string = page.template_set
local tmpl, tmpl_get_err = templates.get(template_set)
if tmpl_get_err then
	return nil, tmpl_get_err
end

local content, render_err = tmpl:render(page.template_name :: string, {})
tmpl:release()
return content, render_err
`
	result := testutil.Check(source, testutil.WithStdlib(),
		testutil.WithModule("templates", templatesModule),
		testutil.WithModule("page_registry", pageRegistryModule))
	if result.HasError() {
		t.Fatalf("expected error guard plus field cast to feed imported method call, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_TruthinessGuardedFieldCastFeedsMethodCall(t *testing.T) {
	source := `
local funcs = {}
function funcs.new()
	return {
		call = function(self, id: string, context)
			return context, nil
		end,
	}
end

local function get_page_data(page)
	if not page or not page.data_func or page.data_func == "" then
		return {}, nil
	end
	local executor = funcs.new()
	return executor:call(page.data_func :: string, {})
end

return get_page_data({ data_func = true })
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected explicit cast of truthy guarded field to feed method argument checking, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_TableLiteralFieldCastSatisfiesRecordAssignment(t *testing.T) {
	source := `
type PageResponse = {
	id: string,
	configOverrides: {[string]: any}?,
}

local page = {
	id = "home",
	config_overrides = dynamic_config,
}

local page_info: PageResponse = {
	id = type(page.id) == "string" and page.id or tostring(page.id),
	configOverrides = page.config_overrides :: {[string]: any}?,
}

return page_info
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected casted table-literal field to satisfy record assignment, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_LengthGuardNarrowsLastElementIndex(t *testing.T) {
	source := `
type YieldResult = {
	content: string,
}

local function latest_content(yield_result_data: {YieldResult}?)
	if not yield_result_data or #yield_result_data == 0 then
		return nil, "No yield result data found"
	end
	local latest_yield_result = yield_result_data[#yield_result_data]
	return latest_yield_result.content
end

return latest_content({ { content = "ok" } })
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected positive length guard to prove last element index, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_ErrorReturnGuardNarrowsCurrentValue(t *testing.T) {
	source := `
type Content = {
	metadata: {[string]: any}?,
}

local content_repo = {}
function content_repo.get(content_id): (Content?, string?)
	if content_id == "" then
		return nil, "not found"
	end
	return { metadata = {} }, nil
end

local function update_metadata(content_id, metadata)
	local current, err = content_repo.get(content_id)
	if err then
		return nil, "Failed to get current metadata: " .. err
	end
	local current_metadata = current.metadata or {}
	for k, v in pairs(metadata) do
		current_metadata[k] = v
	end
	return current_metadata
end

return update_metadata("id", { kind = "text" })
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected error-return guard to narrow current value before field access, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_InsertedSuiteShapeSurvivesIpairs(t *testing.T) {
	source := `
type Suite = {
	name: string,
	tests: {any},
	children: {Suite},
	full_path: string,
	before_all: any?,
	after_all: any?,
	before_each: any?,
	after_each: any?,
}

local test = {}
local _default_context = {
	suites_hierarchy = {},
}

function test.suite(name: string): Suite
	return {
		name = name,
		tests = {},
		children = {},
		full_path = name,
	}
end

local function run_suite_with_children(suite: Suite)
	for _, child in ipairs(suite.children) do
		run_suite_with_children(child)
	end
end

local suite: Suite = test.suite("top")
table.insert(_default_context.suites_hierarchy, suite)

for _, top_suite in ipairs(_default_context.suites_hierarchy) do
	run_suite_with_children(top_suite)
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatalf("expected inserted suite shape to survive ipairs, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_CapturedTableInsertFeedsCleanupLoop(t *testing.T) {
	source := `
type Suite = {
	name: string,
	tests: {any},
	children: {Suite},
	full_path: string,
	before_all: any?,
	after_all: any?,
	before_each: any?,
	after_each: any?,
}

local test = {}
local _default_context = {
	tests = {},
	suites_hierarchy = {},
	results = {
		tests = {},
	},
}

function test.suite(name: string): Suite
	return {
		name = name,
		tests = {},
		children = {},
		full_path = name,
	}
end

function test.describe(name: string)
	local new_suite = test.suite(name)
	table.insert(_default_context.suites_hierarchy, new_suite)
	table.insert(_default_context.tests, new_suite)
	return new_suite
end

local function clear_suite_references(suite: Suite)
	if suite.tests then
		for i, test_case in ipairs(suite.tests) do
			suite.tests[i].fn = nil
		end
	end
	suite.before_all = nil
	suite.after_all = nil
	suite.before_each = nil
	suite.after_each = nil
	suite.children = {}
	for _, child in ipairs(suite.children or {}) do
		clear_suite_references(child)
	end
end

local function cleanup_test_resources()
	for _, suite in ipairs(_default_context.suites_hierarchy) do
		clear_suite_references(suite)
	end
	_default_context.tests = {}
	_default_context.suites_hierarchy = {}
	_default_context.results.tests = {}
end

test.describe("top")
cleanup_test_resources()
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatalf("expected inserted suite shape to survive context resets, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_ExportedDescribeFeedsLaterRunLoop(t *testing.T) {
	source := `
type Suite = {
	name: string,
	tests: {any},
	children: {Suite},
	full_path: string,
	before_all: any?,
	after_all: any?,
	before_each: any?,
	after_each: any?,
}

local test = {}
local _default_context = {
	suites_hierarchy = {},
	current_describe = nil,
}

function test.suite(name: string): Suite
	return {
		name = name,
		tests = {},
		children = {},
		full_path = name,
	}
end

function test.describe(name: string, fn: any)
	local old_describe = _default_context.current_describe
	local new_suite = test.suite(name)
	if old_describe then
		new_suite.parent = old_describe
		table.insert(old_describe.children, new_suite)
		new_suite.full_path = old_describe.full_path .. " > " .. name
	else
		table.insert(_default_context.suites_hierarchy, new_suite)
	end
	_default_context.current_describe = new_suite
	fn()
	_default_context.current_describe = old_describe
	return new_suite
end

function test.run()
	local function clear_suite_references(suite: Suite)
		suite.before_all = nil
		suite.after_all = nil
		suite.before_each = nil
		suite.after_each = nil
		suite.children = {}
	end
	for _, suite in ipairs(_default_context.suites_hierarchy) do
		clear_suite_references(suite)
	end
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatalf("expected exported describe table insert to feed later run loop, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_BodyCallExpectationInfersWholeParameter(t *testing.T) {
	source := `
local http = {
	get = function(url: string, options: {headers: {[string]: string}, stream?: boolean})
		return { status_code = 200, body = "{}" }, nil
	end,
}

local client = {
	_http_client = http,
}

function client.request(method, url, http_options)
	http_options.headers["Accept"] = "application/json"
	if http_options.stream then
		url = url .. "?alt=sse"
	end
	return client._http_client.get(url, http_options)
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatalf("expected body call expectations to infer whole parameter shape, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_GuardedBodyUseDoesNotEraseOptionalParamBoundary(t *testing.T) {
	source := `
type Executor = {
	with_context: (self: Executor, context: {[string]: any}) -> Executor,
	call: (self: Executor, id: string, data: any) -> (any, string?),
}

local funcs = {
	new = function(): Executor
		return {
			with_context = function(self: Executor, context: {[string]: any})
				return self
			end,
			call = function(self: Executor, id: string, data: any)
				return data, nil
			end,
		}
	end,
}

local function call_func(func_id: string, data: any, context: {[string]: any}?)
	local executor = funcs.new()
	if context ~= nil then
		executor = executor:with_context(context)
	end
	return executor:call(func_id, data)
end

local maybe_context = nil :: {[string]: any}?
call_func("map", {}, maybe_context)
call_func("filter", {}, nil)
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatalf("expected guarded body use to preserve optional parameter boundary, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_LocalMetatableInstanceKeepsLaterMethods(t *testing.T) {
	source := `
local module = {}
local Class = {}
local class_mt = { __index = Class }

function module.new()
	return setmetatable({
		nodes = {},
	}, class_mt)
end

function Class:is_empty()
	return next(self.nodes) == nil
end

function Class:has_cycles()
	return false, nil
end

function module.build()
	local graph = module.new()
	if graph:is_empty() then
		return graph, nil
	end
	local has_cycles, cycle_desc = graph:has_cycles()
	if has_cycles then
		return nil, cycle_desc
	end
	return graph, nil
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatalf("expected local metatable instance to keep class methods, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_ImportedMetatableQueryLengthGuardNarrowsFirstElement(t *testing.T) {
	sessionSource := `
local session = {}

local context_query = {}
context_query.__index = context_query

local session_reader = {}
session_reader.__index = session_reader

function session.open()
	return setmetatable({}, session_reader), nil
end

function session_reader:contexts()
	local query = setmetatable({}, context_query)
	return query
end

function context_query:type(_context_type)
	return self
end

function context_query:all()
	local contexts, err = { { text = "summary", created_at = "now" } }, nil
	if err then
		return nil, err
	end
	return contexts or {}, nil
end

return session
`
	sessionModule := testutil.CheckAndExport(sessionSource, "session", testutil.WithStdlib())
	if sessionModule.HasError() {
		t.Fatalf("session module should export cleanly, got: %v", testutil.ErrorMessages(sessionModule.Errors))
	}

	source := `
local session = require("session")

local reader, open_err = session.open()
if not reader then
	return nil, open_err
end

local existing_summaries, ctx_err = reader:contexts():type("conversation_summary"):all()
if ctx_err then
	existing_summaries = {}
end

local existing_summary = nil
if existing_summaries and #existing_summaries > 0 then
	existing_summary = existing_summaries[1].text
end

return existing_summary
`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithModule("session", sessionModule))
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatalf("expected imported query length guard to prove first element, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_LengthGuardEliminatesEmptyTableFallback(t *testing.T) {
	source := `
type Context = {
	text: string,
	created_at: string,
}

local repo = {}
function repo.list_by_type(): ({Context}?, string?)
	return { { text = "summary", created_at = "now" } }, nil
end

local query = {}
function query:all()
	local contexts, err = repo.list_by_type()
	if err then
		return nil, err
	end
	return contexts or {}, nil
end

local existing_summaries, err = query:all()
if err then
	existing_summaries = {}
end

local existing_summary = nil
if existing_summaries and #existing_summaries > 0 then
	existing_summary = existing_summaries[1].text
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected positive length guard to eliminate empty fallback before literal index, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_ImportedUntypedRepositoryFallbackEliminatesNil(t *testing.T) {
	sessionSource := `
local session = {}

local executor = {}
function executor:query(): any
	return nil
end

local repo = {}
function repo.list_by_type(_session_id, _context_type)
	local contexts, err = executor:query()
	if err then
		return nil, err
	end
	return contexts
end

local context_query = {
	_session_id = nil :: string?,
	_type_filter = nil :: string?,
	_error = nil :: string?,
}
context_query.__index = context_query

local session_reader = {
	session_id = nil :: string?,
}
session_reader.__index = session_reader

function session.open(session_id)
	return setmetatable({ session_id = session_id }, session_reader), nil
end

function session_reader:contexts()
	local query = setmetatable({}, context_query)
	query._session_id = self.session_id
	query._type_filter = nil
	query._error = nil
	return query
end

function context_query:type(context_type)
	if not context_type then
		self._error = "Context type is required"
		return self
	end
	self._type_filter = context_type
	return self
end

function context_query:all()
	if self._error then
		return nil, self._error
	end
	local contexts, err = repo.list_by_type(self._session_id, self._type_filter)
	if err then
		return nil, err
	end
	return contexts or {}, nil
end

return session
`
	sessionModule := testutil.CheckAndExport(sessionSource, "session", testutil.WithStdlib())
	if sessionModule.HasError() {
		t.Fatalf("session module should export cleanly, got: %v", testutil.ErrorMessages(sessionModule.Errors))
	}

	source := `
local session = require("session")

local session_reader, session_err = session.open("s1")
if not session_reader then
	return nil, session_err
end

local existing_summaries, ctx_err = session_reader:contexts():type("conversation_summary"):all()
if ctx_err then
	existing_summaries = {}
end

if existing_summaries and #existing_summaries > 0 then
	local first = existing_summaries[1]
end
`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithModule("session", sessionModule))
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatalf("expected imported nil fallback and error repair to eliminate nil index, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_QueryBuilderReaderBackReferenceSurvivesMethodChain(t *testing.T) {
	source := `
local session = {}

local message_query = {
	_session_id = nil :: string?,
	_reader = nil :: any,
	_after_message_id = nil :: string?,
	_error = nil :: string?,
}
message_query.__index = message_query

local session_reader = {
	session_id = nil :: string?,
}
session_reader.__index = session_reader

function session.open(session_id)
	return setmetatable({ session_id = session_id }, session_reader), nil
end

function session_reader:get_context(_key)
	return "checkpoint", nil
end

function session_reader:messages()
	local query = setmetatable({}, message_query)
	query._session_id = self.session_id
	query._reader = self
	query._after_message_id = nil
	query._error = nil
	return query
end

function message_query:from_checkpoint()
	if not self._reader then
		self._error = "Reader reference missing"
		return self
	end
	local checkpoint_id = self._reader:get_context("current_checkpoint")
	if checkpoint_id then
		self._after_message_id = checkpoint_id
	end
	return self
end

function message_query:all()
	if self._error then
		return nil, self._error
	end
	return {}, nil
end

local reader, err = session.open("s1")
if not reader then
	return nil, err
end

local messages_after_checkpoint, msg_err = reader:messages():from_checkpoint():all()
if msg_err then
	return nil, msg_err
end

local all_messages, all_err = reader:messages():all()
if all_err then
	return nil, all_err
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatalf("expected query builder reader back-reference to survive method chains, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_ImportedQueryBuilderReaderBackReferenceSurvivesMethodChain(t *testing.T) {
	sessionSource := `
local session = {}

local message_query = {
	_session_id = nil :: string?,
	_reader = nil :: any,
	_after_message_id = nil :: string?,
	_error = nil :: string?,
}
message_query.__index = message_query

local session_reader = {
	session_id = nil :: string?,
}
session_reader.__index = session_reader

function session.open(session_id)
	return setmetatable({ session_id = session_id }, session_reader), nil
end

function session_reader:get_context(_key)
	return "checkpoint", nil
end

function session_reader:messages()
	local query = setmetatable({}, message_query)
	query._session_id = self.session_id
	query._reader = self
	query._after_message_id = nil
	query._error = nil
	return query
end

function message_query:from_checkpoint()
	if not self._reader then
		self._error = "Reader reference missing"
		return self
	end
	local checkpoint_id = self._reader:get_context("current_checkpoint")
	if checkpoint_id then
		self._after_message_id = checkpoint_id
	end
	return self
end

function message_query:all()
	if self._error then
		return nil, self._error
	end
	return {}, nil
end

return session
`
	sessionModule := testutil.CheckAndExport(sessionSource, "session", testutil.WithStdlib())
	if sessionModule.HasError() {
		t.Fatalf("session module should export cleanly, got: %v", testutil.ErrorMessages(sessionModule.Errors))
	}

	source := `
local session = require("session")

local reader, err = session.open("s1")
if not reader then
	return nil, err
end

local messages_after_checkpoint, msg_err = reader:messages():from_checkpoint():all()
if msg_err then
	return nil, msg_err
end

local all_messages, all_err = reader:messages():all()
if all_err then
	return nil, all_err
end
`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithModule("session", sessionModule))
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatalf("expected imported query builder reader back-reference to survive method chains, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_MultipleQueryBuilderPrototypesKeepMethodReceiversSeparate(t *testing.T) {
	source := `
local session = {}

local message_query = {
	_session_id = nil :: string?,
	_reader = nil :: any,
	_error = nil :: string?,
}
message_query.__index = message_query

local artifact_query = {
	_session_id = nil :: string?,
	_error = nil :: string?,
}
artifact_query.__index = artifact_query

local context_query = {
	_session_id = nil :: string?,
	_error = nil :: string?,
}
context_query.__index = context_query

local session_reader = {
	session_id = nil :: string?,
	_session_data = nil :: any,
	_primary_context_cache = nil :: any,
}
session_reader.__index = session_reader

function session.open(session_id)
	return setmetatable({
		session_id = session_id,
		_session_data = {},
		_primary_context_cache = nil,
	}, session_reader), nil
end

function session_reader:get_context(_key)
	return "checkpoint", nil
end

function session_reader:messages()
	local query = setmetatable({}, message_query)
	query._session_id = self.session_id
	query._reader = self
	query._error = nil
	return query
end

function session_reader:artifacts()
	local query = setmetatable({}, artifact_query)
	query._session_id = self.session_id
	query._error = nil
	return query
end

function session_reader:contexts()
	local query = setmetatable({}, context_query)
	query._session_id = self.session_id
	query._error = nil
	return query
end

function message_query:from_checkpoint()
	if not self._reader then
		self._error = "Reader reference missing"
		return self
	end
	local checkpoint_id = self._reader:get_context("current_checkpoint")
	return self
end

function message_query:all()
	if self._error then
		return nil, self._error
	end
	return {}, nil
end

function artifact_query:all()
	if self._error then
		return nil, self._error
	end
	return {}, nil
end

function context_query:all()
	if self._error then
		return nil, self._error
	end
	return {}, nil
end

local reader, err = session.open("s1")
if not reader then
	return nil, err
end

local messages_after_checkpoint, msg_err = reader:messages():from_checkpoint():all()
if msg_err then
	return nil, msg_err
end

local all_messages, all_err = reader:messages():all()
if all_err then
	return nil, all_err
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatalf("expected multiple query builder prototypes to keep all() receiver facts separate, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_SessionReaderQueryBuilderRealShape(t *testing.T) {
	source := `
type Context = {
	text: string,
	created_at: string,
	time: string?,
}

local session = {
	_session_contexts_repo = {},
}

function session._session_contexts_repo.list_by_type(session_id: string?, context_type: string?): ({Context}?, string?)
	return { { text = "summary", created_at = "now" } }, nil
end

local context_query = {
	_session_id = nil :: string?,
	_type_filter = nil :: string?,
	_error = nil :: string?,
}
context_query.__index = context_query

local session_reader = {
	session_id = nil :: string?,
}
session_reader.__index = session_reader

function session.open()
	return setmetatable({ session_id = "s1" }, session_reader), nil
end

function session_reader:contexts()
	local query = setmetatable({}, context_query)
	query._session_id = self.session_id
	query._type_filter = nil
	query._error = nil
	return query
end

function context_query:type(context_type)
	if not context_type then
		self._error = "Context type is required"
		return self
	end
	self._type_filter = context_type
	return self
end

function context_query:all()
	if self._error then
		return nil, self._error
	end

	local contexts, err
	if self._type_filter then
		contexts, err = session._session_contexts_repo.list_by_type(self._session_id, self._type_filter)
	else
		contexts, err = session._session_contexts_repo.list_by_type(self._session_id, self._type_filter)
	end

	if err then
		return nil, "Failed to fetch contexts: " .. err
	end

	return contexts or {}, nil
end

local session_reader, session_err = session.open()
if not session_reader then
	return nil, session_err
end

local existing_summaries, ctx_err = session_reader:contexts():type("conversation_summary"):all()
if ctx_err then
	existing_summaries = {}
end

local existing_summary = nil
if existing_summaries and #existing_summaries > 0 then
	existing_summary = existing_summaries[1].text
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatalf("expected real-shaped session query builder to preserve length-guarded element, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_TypeProbeAllowsOptionalDynamicFieldFallback(t *testing.T) {
	source := `
local page = {
	id = "home",
	name = "Home",
}

local placement: string = type(page.placement) == "string" and page.placement or "default"
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected type() field probe fallback to type-check, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_NestedTableInsertFeedsIpairs(t *testing.T) {
	source := `
local state = {
	items = {},
}

local value: string = "x"
table.insert(state.items, value)

for _, item in ipairs(state.items) do
	local s: string = item
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatalf("expected nested table.insert to feed ipairs element type, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_DiscriminatedArrayElementFeedsBranchHelper(t *testing.T) {
	source := `
local function convert_image_to_converse(content_part)
	if content_part.type == "image" and content_part.source then
		return { image = content_part.source }
	end
	return nil
end

local message = {
	content = {
		{ type = "text", text = "hello" },
		{ type = "image", source = { media_type = "image/png", data = "abc" } },
	},
}

local content_blocks = {}
for _, part in ipairs(message.content) do
	if part.type == "text" and part.text and part.text ~= "" then
		table.insert(content_blocks, { text = part.text })
	elseif part.type == "image" then
		local img = convert_image_to_converse(part)
		if img then
			table.insert(content_blocks, img)
		end
	end
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatalf("expected discriminated array element to feed image helper, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_DiscriminatedArrayElementDoesNotInheritAccumulatorShape(t *testing.T) {
	source := `
type ImagePart = {
	type: string,
	source: any?,
	text: string?,
}

local function convert_image_to_converse(content_part: ImagePart)
	if content_part.type == "image" and content_part.source then
		return { image = content_part.source }
	end
	return nil
end

local message = {
	content = {
		{ type = "text", text = "hello" },
		{ type = "image", source = { media_type = "image/png", data = "abc" } },
	},
}

local content_blocks = {}
for _, part in ipairs(message.content) do
	if part.type == "text" and part.text and part.text ~= "" then
		table.insert(content_blocks, { text = part.text })
	elseif part.type == "image" then
		local img = convert_image_to_converse(part)
		if img then
			table.insert(content_blocks, img)
		end
	end
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatalf("expected discriminated source array element not to inherit accumulator-only shapes, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_UntypedDiscriminatedArrayElementFeedsTypedBranchHelper(t *testing.T) {
	source := `
type ImagePart = {
	type: string,
	source: any?,
	text: string?,
}

local prompt = {
	ROLE = {
		ASSISTANT = "assistant",
	}
}

local function convert_image_to_converse(content_part: ImagePart)
	if content_part.type == "image" and content_part.source then
		return { image = content_part.source }
	end
	return nil
end

local function map_messages(contract_messages)
	local converse_messages = {}
	for _, msg in ipairs(contract_messages) do
		if msg.role == prompt.ROLE.ASSISTANT then
			local content_blocks = {}
			local content = msg.content
			if type(content) == "string" then
				if content ~= "" then
					table.insert(content_blocks, { text = content })
				end
			elseif type(content) == "table" then
				for _, part in ipairs(content) do
					if part.type == "text" and part.text and part.text ~= "" then
						table.insert(content_blocks, { text = part.text })
					elseif part.type == "function_call" then
						table.insert(content_blocks, { toolUse = { name = part.name or "" } })
					elseif part.type == "image" then
						local img = convert_image_to_converse(part)
						if img then
							table.insert(content_blocks, img)
						end
					end
				end
			end
			if #content_blocks > 0 then
				table.insert(converse_messages, { role = "assistant", content = content_blocks })
			end
		end
	end
	return converse_messages
end

map_messages(nil :: any)
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatalf("expected untyped discriminated source element to feed typed image helper, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_CapturedStateFieldMapPairsPreservesValueShape(t *testing.T) {
	source := `
type Time = {
	after: (self: Time, other: Time) -> boolean,
}

type ActiveSession = {
	pid: any,
	created_at: Time,
	last_activity: Time?,
}

local state = {
	active_sessions = {} :: {[string]: ActiveSession},
}

local function check()
	local most_recent_time: Time? = nil
	for sid, session_info in pairs(state.active_sessions) do
		local last_activity: Time = session_info.last_activity or session_info.created_at
		if not most_recent_time or last_activity:after(most_recent_time) then
			most_recent_time = last_activity
		end
	end
	return most_recent_time
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected captured state field map pairs to preserve ActiveSession values, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_OptionalNumericFieldsDefaultBeforeArithmetic(t *testing.T) {
	source := `
local CONFIG = {
	prompt_buffer_tokens = 256,
	chars_per_token = 4,
}

local function tokens_to_chars(tokens)
	return math.floor(tokens * CONFIG.chars_per_token)
end

local function budget(model_card: {max_tokens: integer?, output_tokens: integer?})
	local max_context_tokens = model_card.max_tokens or 8000
	local max_output_tokens = model_card.output_tokens or 1000
	local usable_input_tokens = max_context_tokens - max_output_tokens - CONFIG.prompt_buffer_tokens
	return tokens_to_chars(usable_input_tokens)
end

return budget({ max_tokens = nil, output_tokens = nil })
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected numeric field defaults to remove nil before arithmetic, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_ExportedNumericDefaultsRemainNonNilAtConsumer(t *testing.T) {
	modelsModule := testutil.CheckAndExport(`
local models = {}

function models._build_model_card(entry)
	return {
		max_tokens = entry.data and entry.data.max_tokens or 0,
		output_tokens = entry.data and entry.data.output_tokens or 0,
	}
end

function models.get_by_name(name)
	if not name then
		return nil, "name required"
	end
	return models._build_model_card({ data = {} }), nil
end

return models
`, "models", testutil.WithStdlib())
	if modelsModule.HasError() {
		t.Fatalf("models module errors: %v", testutil.ErrorMessages(modelsModule.Errors))
	}

	source := `
local models = require("models")

local function budget(model_name)
	local model_card, err = models.get_by_name(model_name)
	if not model_card then
		return nil, err
	end

	local max_context_tokens = model_card.max_tokens or 8000
	local max_output_tokens = model_card.output_tokens or 1000
	return max_context_tokens - max_output_tokens - 256
end

return budget("default")
`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithModule("models", modelsModule))
	if result.HasError() {
		t.Fatalf("expected exported numeric defaults to remain non-nil at consumer arithmetic, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_TableFieldModuleNumericDefaultsRemainNonNil(t *testing.T) {
	modelsModule := testutil.CheckAndExport(`
local models = {}

function models._build_model_card(entry)
	return {
		max_tokens = entry.data and entry.data.max_tokens or 0,
		output_tokens = entry.data and entry.data.output_tokens or 0,
	}
end

function models.get_by_name(name)
	if not name then
		return nil, "name required"
	end
	return models._build_model_card({ data = {} }), nil
end

return models
`, "models", testutil.WithStdlib())
	if modelsModule.HasError() {
		t.Fatalf("models module errors: %v", testutil.ErrorMessages(modelsModule.Errors))
	}

	source := `
local models = require("models")

local compress = {
	_models = models,
}

local function budget(model_name)
	local model_card, err = compress._models.get_by_name(model_name)
	if not model_card then
		return nil, err
	end

	local max_context_tokens = model_card.max_tokens or 8000
	local max_output_tokens = model_card.output_tokens or 1000
	return max_context_tokens - max_output_tokens - 256
end

return budget("default")
`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithModule("models", modelsModule))
	if result.HasError() {
		t.Fatalf("expected table-held imported module numeric defaults to remain non-nil at consumer arithmetic, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_RegistryDerivedNumericDefaultsRemainNonNil(t *testing.T) {
	entryType := typ.NewRecord().
		Field("data", typ.NewOptional(typ.NewRecord().
			Field("max_tokens", typ.NewOptional(typ.Integer)).
			Field("output_tokens", typ.NewOptional(typ.Integer)).
			Build())).
		Build()
	registryType := typ.NewRecord().
		Field("find", typ.Func().
			Param("query", typ.NewMap(typ.String, typ.Any)).
			Returns(typ.NewOptional(typ.NewArray(entryType)), typ.NewOptional(typ.String)).
			Build()).
		Build()

	registryModule := testutil.CheckAndExport(`
local registry = {}
function registry.find(query)
	return { { data = {} } }, nil
end
return registry
`, "registry", testutil.WithStdlib(), testutil.WithTypes(map[string]typ.Type{
		"registry": registryType,
	}))
	if registryModule.HasError() {
		t.Fatalf("registry module errors: %v", testutil.ErrorMessages(registryModule.Errors))
	}

	modelsModule := testutil.CheckAndExport(`
local registry = require("registry")
local models = {}

function models._build_model_card(entry)
	return {
		max_tokens = entry.data and entry.data.max_tokens or 0,
		output_tokens = entry.data and entry.data.output_tokens or 0,
	}
end

function models.get_by_name(name)
	if not name then
		return nil, "name required"
	end
	local entries, err = registry.find({ name = name })
	if err then
		return nil, err
	end
	if not entries or #entries == 0 then
		return nil, "not found"
	end
	return models._build_model_card(entries[1])
end

return models
`, "models", testutil.WithStdlib(), testutil.WithModule("registry", registryModule))
	if modelsModule.HasError() {
		t.Fatalf("models module errors: %v", testutil.ErrorMessages(modelsModule.Errors))
	}

	source := `
local models = require("models")

local compress = {
	_models = models,
}

local function budget(model_name)
	local model_card, err = compress._models.get_by_name(model_name)
	if not model_card then
		return nil, err
	end

	local max_context_tokens = model_card.max_tokens or 8000
	local max_output_tokens = model_card.output_tokens or 1000
	return max_context_tokens - max_output_tokens - 256
end

return budget("default")
`
	result := testutil.Check(source, testutil.WithStdlib(),
		testutil.WithModule("registry", registryModule),
		testutil.WithModule("models", modelsModule))
	if result.HasError() {
		t.Fatalf("expected registry-derived numeric defaults to remain non-nil at consumer arithmetic, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_CompressModelInfoNumericHelpersStayNonNil(t *testing.T) {
	modelsModule := testutil.CheckAndExport(`
local models = {}

function models.get_by_name(name)
	if not name then
		return nil, "name required"
	end
	return {
		max_tokens = 8000,
		output_tokens = 1000,
	}, nil
end

return models
`, "models", testutil.WithStdlib())
	if modelsModule.HasError() {
		t.Fatalf("models module errors: %v", testutil.ErrorMessages(modelsModule.Errors))
	}

	source := `
local models = require("models")

local compress = {
	_models = models,
}

local CONFIG = {
	chars_per_token = 4,
	prompt_buffer_tokens = 500,
	context_safety_margin = 0.1,
	output_buffer_tokens = 200,
}

local function tokens_to_chars(tokens)
	return math.floor(tokens * CONFIG.chars_per_token)
end

local function chars_to_tokens(chars)
	return math.floor((tonumber(chars) or 0) / CONFIG.chars_per_token)
end

local function get_model_info(model_name, mock_model_info)
	if mock_model_info then
		return mock_model_info, nil
	end

	local model_card, err = compress._models.get_by_name(model_name)
	if not model_card then
		return nil, err
	end

	local max_context_tokens = model_card.max_tokens or 8000
	local max_output_tokens = model_card.output_tokens or 1000
	local usable_input_tokens = max_context_tokens - max_output_tokens - CONFIG.prompt_buffer_tokens
	local usable_input_chars = tokens_to_chars(usable_input_tokens)
	local safe_input_chars = math.floor(usable_input_chars * (1 - CONFIG.context_safety_margin))
	local safe_output_chars = tokens_to_chars(max_output_tokens)

	return {
		max_context_tokens = max_context_tokens,
		max_output_tokens = max_output_tokens,
		usable_input_chars = safe_input_chars,
		usable_input_tokens = chars_to_tokens(safe_input_chars),
		max_output_chars = safe_output_chars,
	}, nil
end

local function calculate_safe_max_tokens(target_chars, model_info)
	local needed_tokens = chars_to_tokens(target_chars) + CONFIG.output_buffer_tokens
	return math.min(needed_tokens, tonumber(model_info.max_output_tokens) or 1000)
end

function compress.to_size(model_name, content, target_chars, options, mock_model_info)
	options = options or {}
	local model_info, err = get_model_info(model_name, mock_model_info)
	if err then
		return nil, err
	end
	model_info = assert(model_info)
	return calculate_safe_max_tokens(target_chars, model_info)
end

function compress.get_stats(model_name, content, target_chars, mock_model_info)
	local model_info, err = get_model_info(model_name, mock_model_info)
	if err then
		return nil, err
	end

	local content_chars = #content
	return {
		model_max_context_tokens = model_info.max_context_tokens,
		model_max_output_tokens = model_info.max_output_tokens,
		model_usable_input_chars = model_info.usable_input_chars,
		model_max_output_chars = model_info.max_output_chars,
		fits_in_context = content_chars <= model_info.usable_input_chars,
		safe_max_tokens_for_target = calculate_safe_max_tokens(target_chars, model_info),
	}
end

return compress.to_size("model", "content", 1000, nil, nil)
`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithModule("models", modelsModule))
	if result.HasError() {
		t.Fatalf("expected compress-style numeric helpers to keep defaulted tokens non-nil, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_UnionModelResolverGuardKeepsNumericDefaultsNonNil(t *testing.T) {
	source := `
local resolver
if unknown_condition then
	resolver = {
		get_by_name = function(model_name)
			return {
				max_tokens = 128000,
				output_tokens = 16384,
			}, nil
		end,
	}
else
	resolver = {
		get_by_name = function(model_name)
			return nil, "Model not found"
		end,
	}
end

local function get_model_info(model_name)
	local model_card, err = resolver.get_by_name(model_name)
	if not model_card then
		return nil, err
	end

	local max_context_tokens = model_card.max_tokens or 8000
	local max_output_tokens = model_card.output_tokens or 1000
	return max_context_tokens - max_output_tokens - 500
end

return get_model_info("model")
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected guarded union resolver return to keep numeric defaults non-nil, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_MutableModelResolverFieldGuardKeepsNumericDefaultsNonNil(t *testing.T) {
	source := `
local compress = {
	_models = {
		get_by_name = function(model_name)
			return {
				max_tokens = 128000,
				output_tokens = 16384,
			}, nil
		end,
	},
}

if unknown_condition then
	compress._models = {
		get_by_name = function(model_name)
			return nil, "Model not found"
		end,
	}
end

local function tokens_to_chars(tokens)
	return math.floor(tokens * 4)
end

local function get_model_info(model_name)
	local model_card, err = compress._models.get_by_name(model_name)
	if not model_card then
		return nil, err
	end

	local max_context_tokens = model_card.max_tokens or 8000
	local max_output_tokens = model_card.output_tokens or 1000
	local usable_input_tokens = max_context_tokens - max_output_tokens - 500
	return tokens_to_chars(usable_input_tokens), tokens_to_chars(max_output_tokens)
end

return get_model_info("model")
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected mutable resolver field guard to keep numeric defaults non-nil, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_NestedReturnTableCallFeedsHelperParamEvidence(t *testing.T) {
	source := `
local function tokens_to_chars(tokens)
	return math.floor(tokens * 4)
end

local function model_info()
	local usable_input_tokens = 6500
	local max_output_tokens = 1000
	return {
		usable_input_chars = tokens_to_chars(usable_input_tokens),
		max_output_chars = tokens_to_chars(max_output_tokens),
	}
end

return model_info().usable_input_chars
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected calls nested in returned table fields to feed helper parameter evidence, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_NestedAssignedTableCallFeedsHelperParamEvidence(t *testing.T) {
	source := `
local function tokens_to_chars(tokens)
	return math.floor(tokens * 4)
end

local function model_info()
	local usable_input_tokens = 6500
	local info = {
		usable_input_chars = tokens_to_chars(usable_input_tokens),
	}
	return info
end

return model_info().usable_input_chars
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected calls nested in assigned table fields to feed helper parameter evidence, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_ConditionCallFeedsHelperParamEvidence(t *testing.T) {
	source := `
local function has_budget(tokens)
	return math.floor(tokens) > 0
end

local function run()
	local usable_input_tokens = 6500
	if has_budget(usable_input_tokens) then
		return true
	end
	return false
end

return run()
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected calls in branch conditions to feed helper parameter evidence, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_NumericForBoundCallFeedsHelperParamEvidence(t *testing.T) {
	source := `
local function clamp_bound(tokens)
	return math.floor(tokens)
end

local function run()
	local max_tokens = 3
	local total = 0
	for i = 1, clamp_bound(max_tokens) do
		total = total + i
	end
	return total
end

return run()
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected calls in numeric for bounds to feed helper parameter evidence, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_GuardedConfigUpdateKeepsUnchangedNumericFields(t *testing.T) {
	source := `
local CONFIG = {
	chars_per_token = 4,
	prompt_buffer_tokens = 500,
	default_temperature = 0.2,
}

local function tokens_to_chars(tokens)
	return math.floor(tokens * CONFIG.chars_per_token)
end

local function usable_chars()
	local max_context_tokens = 8000
	local max_output_tokens = 1000
	local usable_input_tokens = max_context_tokens - max_output_tokens - CONFIG.prompt_buffer_tokens
	return tokens_to_chars(usable_input_tokens)
end

local function configure(new_config)
	for key, value in pairs(new_config) do
		if CONFIG[key] ~= nil then
			CONFIG[key] = value
		end
	end
end

configure({ default_temperature = 0.8 })
configure({ unknown_key = "value" })
return usable_chars()
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected guarded config updates not to optionalize unrelated numeric fields, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_GuardedExportedConfigUpdateKeepsUnchangedNumericFields(t *testing.T) {
	source := `
local compress = {}
local CONFIG = {
	chars_per_token = 4,
	prompt_buffer_tokens = 500,
	default_temperature = 0.2,
}

local function tokens_to_chars(tokens)
	return math.floor(tokens * CONFIG.chars_per_token)
end

local function usable_chars()
	local max_context_tokens = 8000
	local max_output_tokens = 1000
	local usable_input_tokens = max_context_tokens - max_output_tokens - CONFIG.prompt_buffer_tokens
	return tokens_to_chars(usable_input_tokens)
end

function compress.configure(new_config)
	for key, value in pairs(new_config) do
		if CONFIG[key] ~= nil then
			CONFIG[key] = value
		end
	end
end

compress.configure({ default_temperature = 0.8 })
compress.configure({ unknown_key = "value" })
return usable_chars()
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected guarded exported config updates not to optionalize unrelated numeric fields, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_GuardedConfigRoundTripKeepsUnchangedNumericFields(t *testing.T) {
	source := `
local compress = {}
local CONFIG = {
	chars_per_token = 4,
	prompt_buffer_tokens = 500,
	default_temperature = 0.2,
}

local function tokens_to_chars(tokens)
	return math.floor(tokens * CONFIG.chars_per_token)
end

local function usable_chars()
	local max_context_tokens = 8000
	local max_output_tokens = 1000
	local usable_input_tokens = max_context_tokens - max_output_tokens - CONFIG.prompt_buffer_tokens
	return tokens_to_chars(usable_input_tokens)
end

function compress.configure(new_config)
	for key, value in pairs(new_config) do
		if CONFIG[key] ~= nil then
			CONFIG[key] = value
		end
	end
end

function compress.get_config()
	local config_copy = {}
	for key, value in pairs(CONFIG) do
		config_copy[key] = value
	end
	return config_copy
end

local original = compress.get_config().default_temperature
compress.configure({ default_temperature = 0.8 })
compress.configure({ default_temperature = original })
compress.configure({ unknown_key = "value" })
return usable_chars()
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected guarded config round-trip not to optionalize unrelated numeric fields, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_TestDslMutableModelResolverDoesNotPolluteNumericHelper(t *testing.T) {
	source := `
local test = {}
function test.describe(_name: string, fn: fun()) fn() end
function test.it(_name: string, fn: fun()) fn() end
function test.run_cases(define_cases_fn: fun())
	return function()
		_G.describe = test.describe
		_G.it = test.it
		define_cases_fn()
		_G.describe = nil
		_G.it = nil
	end
end

local compress = {
	_models = {
		get_by_name = function(model_name)
			return {
				max_tokens = 8000,
				output_tokens = 1000,
			}, nil
		end,
	},
}

local function tokens_to_chars(tokens)
	return math.floor(tokens * 4)
end

local function get_model_info(model_name)
	local model_card, err = compress._models.get_by_name(model_name)
	if not model_card then
		return nil, err
	end

	local max_context_tokens = model_card.max_tokens or 8000
	local max_output_tokens = model_card.output_tokens or 1000
	local usable_input_tokens = max_context_tokens - max_output_tokens - 500
	return {
		usable_input_chars = tokens_to_chars(usable_input_tokens),
		max_output_chars = tokens_to_chars(max_output_tokens),
	}, nil
end

function compress.to_size(model_name: string, content: string, target_chars: number)
	local model_info, err = get_model_info(model_name)
	if err then
		return nil, err
	end
	if not model_info then
		return nil, err
	end
	return model_info.usable_input_chars
end

local function define_tests()
	describe("compress", function()
		it("uses a large model", function()
			compress._models = {
				get_by_name = function(model_name)
					return {
						max_tokens = 128000,
						output_tokens = 16384,
					}, nil
				end,
			}
			return compress.to_size("gpt-4o-mini", "content", 100)
		end)

		it("handles model not found", function()
			compress._models = {
				get_by_name = function(model_name)
					return nil, "Model not found"
				end,
			}
			return compress.to_size("unknown-model", "content", 100)
		end)
	end)
end

return test.run_cases(define_tests)
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected test DSL mutable model mocks not to pollute numeric helper, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_ModelCardBuilderPreludeDoesNotOptionalizeNumericDefaults(t *testing.T) {
	source := `
local function build_model_card(entry)
	local dimensions: number? = nil
	if type(entry.data) == "table" then
		local parsed_dimensions = tonumber(entry.data.dimensions)
		if type(parsed_dimensions) == "number" then
			dimensions = parsed_dimensions
		end
	end

	return {
		max_tokens = entry.data and entry.data.max_tokens or 0,
		output_tokens = entry.data and entry.data.output_tokens or 0,
		dimensions = dimensions,
	}
end

local entry: {data: {max_tokens: integer?, output_tokens: integer?, dimensions: any?}?} = {
	data = { max_tokens = 1000 },
}
local model_card = build_model_card(entry)
local max_context_tokens = model_card.max_tokens or 8000
local max_output_tokens = model_card.output_tokens or 1000
return max_context_tokens - max_output_tokens - 500
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected model-card builder prelude not to optionalize numeric defaults, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_DynamicModelCardNumericFieldsRequireProof(t *testing.T) {
	source := `
local function build_model_card(entry)
	return {
		max_tokens = entry.data and entry.data.max_tokens or 0,
		output_tokens = entry.data and entry.data.output_tokens or 0,
	}
end

local entry = {
	data = unknown_condition and {
		max_tokens = "not numeric",
		output_tokens = 1000,
	} or {
		max_tokens = 8000,
		output_tokens = 1000,
	},
}

local model_card = build_model_card(entry)
local max_context_tokens = model_card.max_tokens or 8000
local max_output_tokens = model_card.output_tokens or 1000
return max_context_tokens - max_output_tokens
`
	result := testutil.Check(source, testutil.WithStdlib())
	requireExternalLintErrorContaining(t, result, "cannot perform arithmetic")
}

func TestExternalLint_AnyProviderModelRequiresStringProof(t *testing.T) {
	source := `
local function merge_provider_options(contract_args: {model: string, options: table}, provider_info)
	return contract_args
end

local provider_info = {
	provider_model = "gpt-4o-mini",
} as any

local contract_args = {
	model = provider_info.provider_model,
	options = {},
}

return merge_provider_options(contract_args, provider_info)
`
	result := testutil.Check(source, testutil.WithStdlib())
	requireExternalLintErrorContaining(t, result, "expected")
}

func TestExternalLint_DynamicResponseTextRequiresStringProof(t *testing.T) {
	source := `
local function parse_text_tool_call(text: string?, tool_names)
	return text
end

local converse_response = unknown_response
local text_blocks = {}

for _, block in ipairs(converse_response.output.message.content) do
	if block.text then
		table.insert(text_blocks, block.text)
	end
end

for _, text in ipairs(text_blocks) do
	parse_text_tool_call(text, {})
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	requireExternalLintErrorContaining(t, result, "expected string?")
}

func TestExternalLint_GuardedStringFieldAccumulatorFeedsHelper(t *testing.T) {
	source := `
local function parse_text_tool_call(text: string?, tool_names)
	if not text or not tool_names then
		return nil
	end
	return { name = text }
end

local function extract(converse_response: {output: {message: {content: {{text: string?}}}}}, tool_names)
	local text_blocks = {}
	for _, block in ipairs(converse_response.output.message.content) do
		if block.text then
			table.insert(text_blocks, block.text)
		end
	end

	for _, text in ipairs(text_blocks) do
		local parsed = parse_text_tool_call(text, tool_names)
		if parsed then
			return parsed.name
		end
	end
	return nil
end

return extract({ output = { message = { content = { { text = "call" } } } } }, {})
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected guarded string field accumulator to feed optional-string helper, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func requireExternalLintErrorContaining(t *testing.T, result *testutil.Result, want string) {
	t.Helper()
	if !result.HasError() {
		t.Fatalf("expected diagnostic containing %q, got no errors", want)
	}
	messages := strings.Join(testutil.ErrorMessages(result.Diagnostics), " | ")
	if !strings.Contains(messages, want) {
		t.Fatalf("expected diagnostic containing %q, got: %s", want, messages)
	}
}

func TestExternalLint_TypeTableGuardKeepsDynamicFieldReadsOpen(t *testing.T) {
	source := `
local function run(stats_data)
	if type(stats_data) == "table" then
		return stats_data.sum or stats_data.count or 0
	end
	return 0
end

return run({})
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected type(table) guard on untyped value to allow dynamic field fallback reads, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
