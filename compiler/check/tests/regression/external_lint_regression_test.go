package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
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

local dynamic_config = nil :: any
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

func TestExternalLint_SplitMetatableMethodTableKeepsInstanceFields(t *testing.T) {
	source := `
local workflow_state = {}
local methods = {}
local workflow_state_mt = { __index = methods }

function workflow_state.new(dataflow_id)
	if not dataflow_id or dataflow_id == "" then
		return nil, "Dataflow ID is required"
	end

	local instance = {
		dataflow_id = dataflow_id,
		nodes = {},
		active_yields = {},
		queued_commands = {},
	}

	return setmetatable(instance, workflow_state_mt), nil
end

function methods:load_state()
	self.nodes["root"] = {
		status = "failed",
		parent_node_id = "parent",
	}
	self.active_yields["parent"] = {
		pending_children = {},
		results = {},
	}
	return self, nil
end

function methods:get_failed_node_errors()
	local failed_nodes = {}
	for node_id, node_data in pairs(self.nodes) do
		if node_data.status == "failed" then
			table.insert(failed_nodes, node_id)
		end
	end
	return table.concat(failed_nodes, "; ")
end

function methods:handle_process_exit(node_id, result_id)
	local new_status = "completed"
	if self.nodes[node_id] then
		self.nodes[node_id].status = new_status
	end
	table.insert(self.queued_commands, {
		type = "UPDATE_NODE",
		payload = {
			node_id = node_id,
			status = new_status,
		},
	})

	local node_data = self.nodes[node_id]
	if node_data and node_data.parent_node_id then
		local yield_info = self.active_yields[node_data.parent_node_id]
		if yield_info and yield_info.pending_children and yield_info.pending_children[node_id] then
			yield_info.results[node_id] = result_id
		end
	end
end

function methods:is_node_active(node_id)
	for _, yield_info in pairs(self.active_yields) do
		if yield_info.pending_children and yield_info.pending_children[node_id] == "pending" then
			return true
		end
	end
	return false
end

local state, err = workflow_state.new("df")
if err then return nil, err end
state:load_state()
state:get_failed_node_errors()
state:handle_process_exit("root", "result")
return state:is_node_active("root")
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected split metatable method table to keep instance fields, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_LastArrayElementAfterNonZeroLengthGuardIsPresent(t *testing.T) {
	source := `
local messages = {}
table.insert(messages, {
	role = "user",
	content = {
		{
			type = "tool_result",
			tool_use_id = "tool",
			content = "ok",
		},
	},
})

if #messages == 0 then
	return nil
else
	local last_msg = messages[#messages]
	if last_msg.role == "user" and last_msg.content and last_msg.content[1] and
		last_msg.content[1].type == "tool_result" then
		return last_msg.content[1].tool_use_id
	end
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected last array element after non-zero length guard to be present, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_LastUnionArrayElementAfterNonZeroLengthGuardIsPresent(t *testing.T) {
	source := `
local messages = {}

local function add_message(kind)
	if kind == "tool" then
		table.insert(messages, {
			role = "user",
			content = {
				{
					type = "tool_result",
					tool_use_id = "tool",
					content = "ok",
				},
			},
		})
	elseif kind == "assistant" then
		table.insert(messages, {
			role = "assistant",
			content = {
				{
					type = "text",
					text = "ok",
				},
			},
		})
	else
		table.insert(messages, {
			role = "user",
			content = {
				{
					type = "text",
					text = "ok",
				},
			},
		})
	end
end

add_message("tool")

if #messages == 0 then
	return nil
else
	local last_msg = messages[#messages]
	if last_msg.role == "user" and last_msg.content and last_msg.content[1] and
		last_msg.content[1].type == "tool_result" then
		return last_msg.content[1].tool_use_id
	end
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected last union array element after non-zero length guard to be present, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_IpairsPreservesDiscriminatedElementVariants(t *testing.T) {
	source := `
local function walk(items)
	for _, item in ipairs(items) do
		if item.role == "function_call" then
			return item.function_call.id
		elseif item.role == "function_result" then
			return item.function_call_id
		else
			return item.content
		end
	end
	return ""
end

return walk({
	{
		role = "function_result",
		function_call_id = "tool",
	},
	{
		role = "developer",
		content = "merge",
	},
})
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected ipairs to preserve discriminated element variants, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_LastLoopBuiltUnionArrayElementAfterNonZeroLengthGuardIsPresent(t *testing.T) {
	source := `
local prompt = {
	ROLE = {
		SYSTEM = "system",
		DEVELOPER = "developer",
		FUNCTION_RESULT = "function_result",
		FUNCTION_CALL = "function_call",
		ASSISTANT = "assistant",
	},
}

local function sanitize_tool_id(id)
	return tostring(id or "")
end

local function process_content_array(content)
	return content or ""
end

local function normalize_tool_arguments(args)
	return args or {}
end

local function map_messages(contract_messages)
	local claude_messages = {}
	local in_system_phase = true

	for _, msg in ipairs(contract_messages) do
		if msg.role == prompt.ROLE.SYSTEM then
			if type(msg.content) == "string" then
				in_system_phase = true
			end
		elseif msg.role == "cache_marker" then
			in_system_phase = in_system_phase
		elseif msg.role == prompt.ROLE.DEVELOPER then
			in_system_phase = false
			local dev_text = type(msg.content) == "string" and msg.content or
				(type(msg.content) == "table" and msg.content[1] and msg.content[1].text) or ""

			if dev_text ~= "" then
				local should_create_new_message = false

				if #claude_messages == 0 then
					should_create_new_message = true
				else
					local last_msg = claude_messages[#claude_messages]
					if last_msg.role == "user" and last_msg.content and last_msg.content[1] and
						last_msg.content[1].type == "tool_result" then
						should_create_new_message = true
					end
				end

				if should_create_new_message then
					table.insert(claude_messages, {
						role = "user",
						content = {
							{
								type = "text",
								text = dev_text,
							},
						},
					})
				else
					local last_msg = claude_messages[#claude_messages]
					for j = #last_msg.content, 1, -1 do
						local part = last_msg.content[j] :: any
						if part.type == "text" then
							part.text = part.text .. dev_text
							break
						end
					end
				end
			end
		elseif msg.role == prompt.ROLE.FUNCTION_RESULT then
			in_system_phase = false
			table.insert(claude_messages, {
				role = "user",
				content = {
					{
						type = "tool_result",
						tool_use_id = sanitize_tool_id(msg.function_call_id),
						content = "ok",
					},
				},
			})
		elseif msg.role == prompt.ROLE.FUNCTION_CALL then
			in_system_phase = false
			local content_blocks = {}
			table.insert(content_blocks, {
				type = "tool_use",
				id = sanitize_tool_id(msg.function_call.id),
				name = msg.function_call.name,
				input = normalize_tool_arguments(msg.function_call.arguments),
			})
			table.insert(claude_messages, {
				role = "assistant",
				content = content_blocks,
			})
		elseif msg.role == prompt.ROLE.ASSISTANT then
			in_system_phase = false
			table.insert(claude_messages, {
				role = msg.role,
				content = {},
			})
		else
			in_system_phase = false
			local content = process_content_array(msg.content)
			if type(content) == "string" then
				content = {
					{
						type = "text",
						text = content,
					},
				}
			end
			table.insert(claude_messages, {
				role = msg.role,
				content = content,
			})
		end
	end

	return claude_messages
end

return map_messages({
	{
		role = "function_result",
		function_call_id = "tool",
		content = "ok",
	},
	{
		role = "developer",
		content = "merge",
	},
})
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected loop-built union array element after non-zero length guard to be present, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_ExportedLoopBuiltUnionArrayElementAfterNonZeroLengthGuardIsPresent(t *testing.T) {
	source := `
local prompt = {
	ROLE = {
		SYSTEM = "system",
		DEVELOPER = "developer",
		FUNCTION_RESULT = "function_result",
		FUNCTION_CALL = "function_call",
		ASSISTANT = "assistant",
	},
}

local mapper = {}

local function sanitize_tool_id(original_id)
	if not original_id then
		return "tool"
	end
	return tostring(original_id)
end

local function convert_image_content(content_part)
	return content_part
end

local function process_content_array(content)
	if type(content) == "string" then
		return content
	elseif type(content) == "table" then
		local processed = {}
		for _, part in ipairs(content) do
			table.insert(processed, convert_image_content(part))
		end
		return processed
	end
	return content
end

local function normalize_tool_arguments(raw_arguments)
	local arguments = raw_arguments
	if not arguments or type(arguments) ~= "table" then
		arguments = { run = true }
	end
	return arguments
end

function mapper.map_messages(contract_messages)
	if not contract_messages or #contract_messages == 0 then
		return {
			messages = {},
			system = nil,
		}
	end

	local claude_messages = {}
	local system_blocks = {}
	local system_cache_positions = {}
	local message_cache_positions = {}
	local in_system_phase = true

	for _, msg in ipairs(contract_messages) do
		if msg.role == prompt.ROLE.SYSTEM then
			if type(msg.content) == "string" then
				table.insert(system_blocks, {
					type = "text",
					text = msg.content,
				})
			elseif type(msg.content) == "table" then
				for _, part in ipairs(msg.content) do
					table.insert(system_blocks, convert_image_content(part))
				end
			end
		elseif msg.role == "cache_marker" then
			if in_system_phase then
				table.insert(system_cache_positions, #system_blocks)
			else
				table.insert(message_cache_positions, #claude_messages)
			end
		elseif msg.role == prompt.ROLE.DEVELOPER then
			in_system_phase = false
			local dev_text = type(msg.content) == "string" and msg.content or
				(type(msg.content) == "table" and msg.content[1] and msg.content[1].text) or ""

			if dev_text ~= "" then
				local should_create_new_message = false

				if #claude_messages == 0 then
					should_create_new_message = true
				else
					local last_msg = claude_messages[#claude_messages]
					if last_msg.role == "user" and last_msg.content and last_msg.content[1] and
						last_msg.content[1].type == "tool_result" then
						should_create_new_message = true
					end
				end

				if should_create_new_message then
					table.insert(claude_messages, {
						role = "user",
						content = {
							{
								type = "text",
								text = dev_text,
							},
						},
					})
				else
					local last_msg = claude_messages[#claude_messages]
					for j = #last_msg.content, 1, -1 do
						local part = last_msg.content[j] :: any
						if part.type == "text" then
							part.text = part.text .. dev_text
							break
						end
					end
				end
			end
		elseif msg.role == prompt.ROLE.FUNCTION_RESULT then
			in_system_phase = false
			local result_text = type(msg.content) == "string" and msg.content or
				(type(msg.content) == "table" and msg.content[1] and msg.content[1].text) or ""
			table.insert(claude_messages, {
				role = "user",
				content = {
					{
						type = "tool_result",
						tool_use_id = sanitize_tool_id(msg.function_call_id),
						content = result_text,
					},
				},
			})
		elseif msg.role == prompt.ROLE.FUNCTION_CALL then
			in_system_phase = false
			local arguments = normalize_tool_arguments(msg.function_call.arguments)
			local content_blocks = {}
			table.insert(content_blocks, {
				type = "tool_use",
				id = sanitize_tool_id(msg.function_call.id),
				name = msg.function_call.name,
				input = arguments,
			})
			table.insert(claude_messages, {
				role = "assistant",
				content = content_blocks,
			})
		elseif msg.role == prompt.ROLE.ASSISTANT then
			in_system_phase = false
			local content_blocks = {}
			local regular_content = process_content_array(msg.content)
			if type(regular_content) == "string" and regular_content ~= "" then
				table.insert(content_blocks, {
					type = "text",
					text = regular_content,
				})
			elseif type(regular_content) == "table" then
				for _, part in ipairs(regular_content) do
					if part.type == "function_call" then
						local arguments = normalize_tool_arguments(part.arguments)
						table.insert(content_blocks, {
							type = "tool_use",
							id = sanitize_tool_id(part.id),
							name = part.name,
							input = arguments,
						})
					elseif part.type == "text" and part.text and part.text ~= "" then
						table.insert(content_blocks, part)
					elseif part.type ~= "text" then
						table.insert(content_blocks, part)
					end
				end
			end
			table.insert(claude_messages, {
				role = msg.role,
				content = content_blocks,
			})
		else
			in_system_phase = false
			local content = process_content_array(msg.content)
			if type(content) == "string" then
				content = {
					{
						type = "text",
						text = content,
					},
				}
			end
			table.insert(claude_messages, {
				role = msg.role,
				content = content,
			})
		end
	end

	return {
		messages = claude_messages,
		system = #system_blocks > 0 and system_blocks or nil,
	}
end

return mapper
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected exported loop-built union array element after non-zero length guard to be present, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_LoopLocalLastElementAfterNonZeroLengthGuardIsPresent(t *testing.T) {
	source := `
local mapper = {}

function mapper.map_messages(items)
	local messages = {}
	for _, item in ipairs(items) do
		if item.kind == "user" then
			table.insert(messages, {
				role = "user",
				content = {
					{
						type = "text",
						text = "ok",
					},
				},
			})
		elseif item.kind == "assistant" then
			table.insert(messages, {
				role = "assistant",
				content = {},
			})
		end

		if #messages == 0 then
			item.empty = true
		else
			local last_msg = messages[#messages]
			if last_msg.role == "user" and last_msg.content and last_msg.content[1] then
				return last_msg.content[1].text
			end
		end
	end
	return nil
end

return mapper
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected loop-local last element after non-zero length guard to be present, got: %v", testutil.ErrorMessages(result.Diagnostics))
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
	table.sort(existing_summaries, function(a, b)
		return (a.time or a.created_at or "") > (b.time or b.created_at or "")
	end)
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

local function handle(args)
if not args.session_id then
	return nil, "session_id is required"
end

local session_reader, session_err = session.open(args.session_id)
if not session_reader then
	return nil, session_err
end

local existing_summaries, ctx_err = session_reader:contexts():type("conversation_summary"):all()
if ctx_err then
	existing_summaries = {}
end

if existing_summaries and #existing_summaries > 0 then
	table.sort(existing_summaries, function(a, b)
		return (a.time or a.created_at or "") > (b.time or b.created_at or "")
	end)
	local first = existing_summaries[1].text
end

return true
end

return handle
`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithModule("session", sessionModule))
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatalf("expected imported nil fallback and error repair to eliminate nil index, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_MultiModuleSessionQueryLengthGuardEliminatesNil(t *testing.T) {
	repoModule := testutil.CheckAndExport(`
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

return repo
`, "session_contexts_repo", testutil.WithStdlib())
	if repoModule.HasError() {
		t.Fatalf("repo module should export cleanly, got: %v", testutil.ErrorMessages(repoModule.Errors))
	}

	readerModule := testutil.CheckAndExport(`
local session_contexts_repo = require("session_contexts_repo")

local session = {}

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

	local contexts, err = session_contexts_repo.list_by_type(self._session_id, self._type_filter)
	if err then
		return nil, err
	end

	return contexts or {}, nil
end

return session
`, "session", testutil.WithStdlib(), testutil.WithModule("session_contexts_repo", repoModule))
	if readerModule.HasError() {
		t.Fatalf("reader module should export cleanly, got: %v", testutil.ErrorMessages(readerModule.Errors))
	}

	result := testutil.Check(`
local session = require("session")

local function handle(args)
if not args.session_id then
	return nil, "session_id is required"
end

local session_reader, session_err = session.open(args.session_id)
if not session_reader then
	return nil, session_err
end

local existing_summaries, ctx_err = session_reader:contexts():type("conversation_summary"):all()
if ctx_err then
	existing_summaries = {}
end

local existing_summary = nil
if existing_summaries and #existing_summaries > 0 then
	table.sort(existing_summaries, function(a, b)
		return (a.time or a.created_at or "") > (b.time or b.created_at or "")
	end)
	existing_summary = existing_summaries[1].text
end

return existing_summary
end

return handle
`, testutil.WithStdlib(), testutil.WithModule("session", readerModule))
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatalf("expected multi-module query fallback and length guard to eliminate nil index, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_SQLBuilderRowsKeepSessionQueryLengthGuard(t *testing.T) {
	queryRowType := typ.NewMap(typ.String, typ.Any)
	queryExecutorType := typ.NewInterface("sql.QueryExecutor", []typ.Method{
		{
			Name: "query",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.NewArray(queryRowType), typ.NewOptional(typ.LuaError)).
				Build(),
		},
	})
	selectBuilderType := typ.NewInterface("sql.SelectBuilder", []typ.Method{
		{Name: "from", Type: typ.Func().Param("self", typ.Self).Param("table", typ.String).Returns(typ.Self).Build()},
		{Name: "where", Type: typ.Func().Param("self", typ.Self).Param("condition", typ.Any).Returns(typ.Self).Build()},
		{Name: "order_by", Type: typ.Func().Param("self", typ.Self).Variadic(typ.String).Returns(typ.Self).Build()},
		{Name: "run_with", Type: typ.Func().Param("self", typ.Self).Param("runner", typ.Any).Returns(queryExecutorType).Build()},
	})
	builderType := typ.NewRecord().
		Field("select", typ.Func().Variadic(typ.String).Returns(selectBuilderType).Build()).
		Field("expr", typ.Func().Param("expr", typ.String).Variadic(typ.Any).Returns(typ.Any).Build()).
		Field("and_", typ.Func().Variadic(typ.Any).Returns(typ.Any).Build()).
		Build()
	sqlManifest := io.NewManifest("sql")
	sqlManifest.SetExport(typ.NewRecord().
		Field("get", typ.Func().Param("dsn", typ.String).Returns(typ.Any, typ.NewOptional(typ.LuaError)).Build()).
		Field("builder", builderType).
		Build())

	repoModule := testutil.CheckAndExport(`
local sql = require("sql")

local repo = {}

function repo.list_by_type(session_id, context_type)
	if not session_id or session_id == "" then
		return nil, "Session ID is required"
	end
	if not context_type or context_type == "" then
		return nil, "Context type is required"
	end

	local db, err = sql.get("app:db")
	if err then
		return nil, err
	end

	local query = sql.builder.select("id", "session_id", "type", "text", "time")
		:from("session_contexts")
		:where(sql.builder.and_({
			sql.builder.expr("session_id = ?", session_id),
			sql.builder.expr("type = ?", context_type)
		}))
		:order_by("id ASC")

	local executor = query:run_with(db)
	local contexts, query_err = executor:query()
	if query_err then
		return nil, "Failed to list session contexts by type: " .. query_err
	end

	return contexts
end

return repo
`, "session_contexts_repo", testutil.WithStdlib(), testutil.WithManifest("sql", sqlManifest))
	if repoModule.HasError() {
		t.Fatalf("repo module should export cleanly, got: %v", testutil.ErrorMessages(repoModule.Errors))
	}

	readerModule := testutil.CheckAndExport(`
local session_contexts_repo = require("session_contexts_repo")

local session = {}

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

	local contexts, err = session_contexts_repo.list_by_type(self._session_id, self._type_filter)
	if err then
		return nil, "Failed to fetch contexts: " .. err
	end

	return contexts or {}, nil
end

return session
`, "session", testutil.WithStdlib(), testutil.WithModule("session_contexts_repo", repoModule))
	if readerModule.HasError() {
		t.Fatalf("reader module should export cleanly, got: %v", testutil.ErrorMessages(readerModule.Errors))
	}

	result := testutil.Check(`
local session = require("session")

local function handle(args)
	if not args.session_id then
		return nil, "session_id is required"
	end

	local session_reader, session_err = session.open(args.session_id)
	if not session_reader then
		return nil, session_err
	end

	local existing_summaries, ctx_err = session_reader:contexts():type("conversation_summary"):all()
	if ctx_err then
		existing_summaries = {}
	end

	local existing_summary = nil
	if existing_summaries and #existing_summaries > 0 then
		table.sort(existing_summaries, function(a, b)
			return (a.time or a.created_at or "") > (b.time or b.created_at or "")
		end)
		existing_summary = existing_summaries[1].text
	end

	return existing_summary
end

return handle
`, testutil.WithStdlib(), testutil.WithModule("session", readerModule))
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatalf("expected SQL builder row array to preserve length-guarded first-element proof, got: %v", testutil.ErrorMessages(result.Diagnostics))
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

func TestExternalLint_CapturedMapMutationFeedsPairsIteratorValue(t *testing.T) {
	source := `
type Time = {
	before: (self: Time, other: Time) -> boolean,
}

local time = {
	now = function(): Time
		return {
			before = function(self: Time, other: Time)
				return false
			end,
		}
	end,
}

local state = {
	active_sessions = {},
	session_count = 0,
}

local function graceful_terminate_session(session_id, session_info, reason)
	if not session_info or not session_info.pid then
		return
	end
	if session_info.terminating then
		return
	end
	session_info.terminating = true
	session_info.terminate_reason = reason
	return session_id
end

local function create_session(session_id, session_pid)
	local now = time.now()
	state.active_sessions[session_id] = {
		pid = session_pid,
		created_at = now,
		last_activity = now,
		terminating = false,
		terminate_reason = nil,
	}
	state.session_count = state.session_count + 1
end

create_session("one", "pid")

for session_id, session_info in pairs(state.active_sessions) do
	graceful_terminate_session(session_id, session_info, "shutdown")
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected captured map mutation to feed pairs iterator value, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_LoopCarriedCapturedMapMutationFeedsSiblingBranchPairs(t *testing.T) {
	source := `
local state = {
	active_sessions = {},
}

local unknown_time: any = nil
local unknown_running: any = nil
local unknown_result: any = nil

local function graceful_terminate_session(session_id, session_info, reason)
	if not session_info or not session_info.pid then
		return
	end
	if session_info.terminating then
		return
	end
	session_info.terminating = true
	session_info.terminate_reason = reason
	return session_id
end

local function create_session(payload_data)
	local session_id = payload_data.session_id
	if not session_id then
		session_id = "generated"
	end
	local session_pid = payload_data.pid
	if session_pid then
		state.active_sessions[session_id] = {
			pid = session_pid,
			terminating = false,
			terminate_reason = nil,
		}
	end
end

local function update_session_activity(session_id)
	if state.active_sessions[session_id] then
		state.active_sessions[session_id].last_activity = unknown_time
	end
end

local running = unknown_running
while running do
	local result = unknown_result
	if not result.ok then
		break
	end
	if result.channel == "inbox" then
		local event = result.value
		if event.topic == "open" then
			create_session(event.payload)
		elseif event.topic == "activity" then
			update_session_activity(event.session_id)
		elseif event.topic == "shutdown" then
			for session_id, session_info in pairs(state.active_sessions) do
				graceful_terminate_session(session_id, session_info, "shutdown")
			end
		end
	end
	running = false
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected loop-carried captured map mutation to feed sibling branch pairs, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_TruthyDynamicMapReadNarrowsKeyForCall(t *testing.T) {
	source := `
local state = {
	active_sessions = {},
}

local function create_session(session_id: string)
	state.active_sessions[session_id] = {
		pid = "pid",
		terminating = false,
	}
end

local function graceful_terminate_session(session_id: string, session_info, reason: string)
	if not session_info or not session_info.pid then
		return
	end
	return session_id .. reason
end

local function handle_session_close(payload_data)
	local session_id = payload_data.session_id
	if not session_id then
		return
	end
	local session_info = state.active_sessions[session_id]
	if session_info then
		graceful_terminate_session(session_id, session_info, "user_closed")
	end
end

create_session("s1")
handle_session_close({ session_id = "s1" })
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected truthy dynamic map read to refine key type for call, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_CapturedMapTimeFieldsSurviveActivityUpdate(t *testing.T) {
	source := `
type Duration = {
	seconds: (self: Duration) -> number,
}

type Time = {
	sub: (self: Time, other: Time) -> Duration,
}

type ActiveSession = {
	pid: any,
	created_at: any,
	last_activity: any,
	terminating: boolean,
}

local time = {
	now = function(): Time
		return {
			sub = function(self: Time, other: Time): Duration
				return {
					seconds = function(self: Duration): number
						return 0
					end,
				}
			end,
		}
	end,
}

local state = {
	active_sessions = {},
}

local function create_session(session_id: string)
	local now = time.now()
	state.active_sessions[session_id] = {
		pid = "pid",
		created_at = now,
		last_activity = now,
		terminating = false,
	}
end

local function update_session_activity(session_id)
	if state.active_sessions[session_id] then
		state.active_sessions[session_id].last_activity = time.now()
	end
end

local function check_inactive_sessions()
	local now = time.now()
	for _, session_info in pairs(state.active_sessions) do
		local last_activity = session_info.last_activity or session_info.created_at
		local elapsed = now:sub(last_activity)
		local seconds: number = elapsed:seconds()
	end
end

create_session("s1")
update_session_activity("s1")
check_inactive_sessions()
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected captured map time fields to survive sibling activity update, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_CapturedMapTimeFieldsSurviveUntypedPayloadKey(t *testing.T) {
	source := `
type Duration = {
	seconds: (self: Duration) -> number,
}

type Time = {
	sub: (self: Time, other: Time) -> Duration,
}

local time = {
	now = function(): Time
		return {
			sub = function(self: Time, other: Time): Duration
				return {
					seconds = function(self: Duration): number
						return 0
					end,
				}
			end,
		}
	end,
}

local uuid = {
	v7 = function(): (string, string?)
		return "generated", nil
	end,
}

local state = {
	active_sessions = {},
}

local function create_session(payload_data)
	local session_id = payload_data.session_id
	if not session_id then
		local id, err = uuid.v7()
		if err then
			return nil, err
		end
		session_id = id
	end

	local now = time.now()
	state.active_sessions[session_id] = {
		pid = "pid",
		created_at = now,
		last_activity = now,
		terminating = false,
	}
	return session_id, nil
end

local function update_session_activity(session_id)
	if state.active_sessions[session_id] then
		state.active_sessions[session_id].last_activity = time.now()
	end
end

local function check_inactive_sessions()
	local now = time.now()
	for _, session_info in pairs(state.active_sessions) do
		local last_activity = session_info.last_activity or session_info.created_at
		local elapsed = now:sub(last_activity)
		local seconds: number = elapsed:seconds()
	end
end

create_session({})
update_session_activity("generated")
check_inactive_sessions()
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected captured map time fields to survive untyped payload key, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_CapturedMapStdlibTimeFieldsSurviveUntypedPayloadKey(t *testing.T) {
	durationType := typ.NewInterface("time.Duration", []typ.Method{
		{Name: "seconds", Type: typ.Func().Param("self", typ.Self).Returns(typ.Number).Build()},
	})
	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "sub", Type: typ.Func().Param("self", typ.Self).Param("other", typ.Self).Returns(durationType).Build()},
	})
	timeManifest := io.NewManifest("time")
	timeManifest.DefineType("Time", timeType)
	timeManifest.DefineType("Duration", durationType)
	timeManifest.SetExport(typ.NewInterface("time", []typ.Method{
		{Name: "now", Type: typ.Func().Returns(timeType).Build()},
		{Name: "parse_duration", Type: typ.Func().Param("s", typ.Any).Returns(durationType, typ.NewOptional(typ.LuaError)).Build()},
	}))

	source := `
local time = require("time")

type ActiveSession = {
	pid: any,
	created_at: any,
	last_activity: any,
	terminating: boolean,
}

local uuid = {
	v7 = function(): (string, string?)
		return "generated", nil
	end,
}

local state = {
	active_sessions = {},
}

local function create_session(payload_data)
	local session_id = payload_data.session_id
	if not session_id then
		local id, err = uuid.v7()
		if err then
			return nil, err
		end
		session_id = id
	end

	local now = time.now()
	state.active_sessions[session_id] = {
		pid = "pid",
		created_at = now,
		last_activity = now,
		terminating = false,
	}
	return session_id, nil
end

local function update_session_activity(session_id)
	if state.active_sessions[session_id] then
		state.active_sessions[session_id].last_activity = time.now()
	end
end

local function check_inactive_sessions()
	local now = time.now()
	for _, session_info in pairs(state.active_sessions) do
		local last_activity = session_info.last_activity or session_info.created_at
		local elapsed = now:sub(last_activity)
		local seconds: number = elapsed:seconds()
	end
end

create_session({})
update_session_activity("generated")
check_inactive_sessions()
`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("time", timeManifest))
	if result.HasError() {
		t.Fatalf("expected captured map stdlib time fields to survive untyped payload key, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_CapturedMapPreWriteReadsDoNotEraseTimeFields(t *testing.T) {
	durationType := typ.NewInterface("time.Duration", []typ.Method{
		{Name: "seconds", Type: typ.Func().Param("self", typ.Self).Returns(typ.Number).Build()},
	})
	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "before", Type: typ.Func().Param("self", typ.Self).Param("other", typ.Self).Returns(typ.Boolean).Build()},
		{Name: "sub", Type: typ.Func().Param("self", typ.Self).Param("other", typ.Self).Returns(durationType).Build()},
	})
	timeManifest := io.NewManifest("time")
	timeManifest.DefineType("Time", timeType)
	timeManifest.DefineType("Duration", durationType)
	timeManifest.SetExport(typ.NewInterface("time", []typ.Method{
		{Name: "now", Type: typ.Func().Returns(timeType).Build()},
		{Name: "parse_duration", Type: typ.Func().Param("s", typ.Any).Returns(durationType, typ.NewOptional(typ.LuaError)).Build()},
	}))

	source := `
local time = require("time")

type ActiveSession = {
	pid: any,
	created_at: any,
	last_activity: any,
	terminating: boolean,
}

local uuid = {
	v7 = function(): (string, string?)
		return "generated", nil
	end,
}

local state = {
	active_sessions = {},
	session_count = 0,
}

local function get_oldest_session()
	local oldest_time = nil
	for _, session_info in pairs(state.active_sessions) do
		if not session_info.terminating then
			local last_activity = session_info.last_activity or session_info.created_at
			if not oldest_time or last_activity:before(oldest_time) then
				oldest_time = last_activity
			end
		end
	end
	return oldest_time
end

local function enforce_session_limit()
	while state.session_count > 10 do
		if not get_oldest_session() then
			break
		end
		state.session_count = state.session_count - 1
	end
end

local function create_session(payload_data)
	enforce_session_limit()
	local session_id = payload_data.session_id
	if not session_id then
		local id, err = uuid.v7()
		if err then
			return nil, err
		end
		session_id = id
	end

	if state.active_sessions[session_id] then
		return session_id, nil
	end

	local now = time.now()
	state.active_sessions[session_id] = {
		pid = "pid",
		created_at = now,
		last_activity = now,
		terminating = false,
	}
	state.session_count = state.session_count + 1
	return session_id, nil
end

local function update_session_activity(session_id)
	if state.active_sessions[session_id] then
		state.active_sessions[session_id].last_activity = time.now()
	end
end

local function check_inactive_sessions()
	local now = time.now()
	for session_id, session_info in pairs(state.active_sessions) do
		local last_activity = session_info.last_activity or session_info.created_at
		local elapsed = now:sub(last_activity)
		if elapsed:seconds() > 10 then
			state.active_sessions[session_id] = nil
		end
	end
end

create_session({})
update_session_activity("generated")
check_inactive_sessions()
`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("time", timeManifest))
	if result.HasError() {
		t.Fatalf("expected pre-write reads not to erase captured map time fields, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_NestedCapturedMapPreWriteReadsDoNotEraseTimeFields(t *testing.T) {
	durationType := typ.NewInterface("time.Duration", []typ.Method{
		{Name: "seconds", Type: typ.Func().Param("self", typ.Self).Returns(typ.Number).Build()},
	})
	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "before", Type: typ.Func().Param("self", typ.Self).Param("other", typ.Self).Returns(typ.Boolean).Build()},
		{Name: "sub", Type: typ.Func().Param("self", typ.Self).Param("other", typ.Self).Returns(durationType).Build()},
	})
	timeManifest := io.NewManifest("time")
	timeManifest.DefineType("Time", timeType)
	timeManifest.DefineType("Duration", durationType)
	timeManifest.SetExport(typ.NewInterface("time", []typ.Method{
		{Name: "now", Type: typ.Func().Returns(timeType).Build()},
		{Name: "parse_duration", Type: typ.Func().Param("s", typ.Any).Returns(durationType, typ.NewOptional(typ.LuaError)).Build()},
	}))

	source := `
local time = require("time")

local uuid = {
	v7 = function(): (string, string?)
		return "generated", nil
	end,
}

local function run(args)
	local state = {
		active_sessions = {},
		session_count = 0,
	}

	local function get_oldest_session()
		local oldest_time = nil
		for _, session_info in pairs(state.active_sessions) do
			if not session_info.terminating then
				local last_activity = session_info.last_activity or session_info.created_at
				if not oldest_time or last_activity:before(oldest_time) then
					oldest_time = last_activity
				end
			end
		end
		return oldest_time
	end

	local function enforce_session_limit()
		while state.session_count > 10 do
			if not get_oldest_session() then
				break
			end
			state.session_count = state.session_count - 1
		end
	end

	local function create_session(payload_data)
		enforce_session_limit()
		local session_id = payload_data.session_id
		if not session_id then
			local id, err = uuid.v7()
			if err then
				return nil, err
			end
			session_id = id
		end

		if state.active_sessions[session_id] then
			return session_id, nil
		end

		local now = time.now()
		state.active_sessions[session_id] = {
			pid = "pid",
			created_at = now,
			last_activity = now,
			terminating = false,
		}
		state.session_count = state.session_count + 1
		return session_id, nil
	end

	local function update_session_activity(session_id)
		if state.active_sessions[session_id] then
			state.active_sessions[session_id].last_activity = time.now()
		end
	end

	local function check_inactive_sessions()
		local now = time.now()
		local inactivity_duration, _ = time.parse_duration("10s")
		for session_id, session_info in pairs(state.active_sessions) do
			local last_activity = session_info.last_activity or session_info.created_at
			local elapsed = now:sub(last_activity)
			if elapsed:seconds() > inactivity_duration:seconds() then
				state.active_sessions[session_id] = nil
			end
		end
	end

	create_session(args)
	update_session_activity("generated")
	check_inactive_sessions()
end

run({})
`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("time", timeManifest))
	if result.HasError() {
		t.Fatalf("expected nested pre-write reads not to erase captured map time fields, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_LoopSiblingCapturedMapTimeFieldsConverge(t *testing.T) {
	durationType := typ.NewInterface("time.Duration", []typ.Method{
		{Name: "seconds", Type: typ.Func().Param("self", typ.Self).Returns(typ.Number).Build()},
	})
	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "sub", Type: typ.Func().Param("self", typ.Self).Param("other", typ.Self).Returns(durationType).Build()},
	})
	timeManifest := io.NewManifest("time")
	timeManifest.DefineType("Time", timeType)
	timeManifest.DefineType("Duration", durationType)
	timeManifest.SetExport(typ.NewRecord().
		Field("now", typ.Func().Returns(timeType).Build()).
		Field("parse_duration", typ.Func().Param("s", typ.Any).Returns(durationType, typ.NewOptional(typ.LuaError)).Build()).
		Build())

	source := `
local time = require("time")

type ActiveSession = {
	pid: any,
	created_at: any,
	last_activity: any,
	terminating: boolean,
}

local uuid = {
	v7 = function(): (string, string?)
		return "generated", nil
	end,
}

local function run()
	local state = {
		active_sessions = {},
		session_count = 0,
	}

	local function create_session(payload_data)
		local session_id = payload_data.session_id
		if not session_id then
			local id, err = uuid.v7()
			if err then
				return nil, err
			end
			session_id = id
		end

		local now = time.now()
		state.active_sessions[session_id] = {
			pid = "pid",
			created_at = now,
			last_activity = now,
			terminating = false,
		}
		state.session_count = state.session_count + 1
		return session_id, nil
	end

	local function update_session_activity(session_id)
		if state.active_sessions[session_id] then
			state.active_sessions[session_id].last_activity = time.now()
		end
	end

	local function check_inactive_sessions()
		local now = time.now()
		local inactivity_duration, _ = time.parse_duration("10s")
		for session_id, session_info in pairs(state.active_sessions) do
			local last_activity = session_info.last_activity or session_info.created_at
			local elapsed = now:sub(last_activity)
			if elapsed:seconds() > inactivity_duration:seconds() then
				state.active_sessions[session_id] = nil
			end
		end
	end

	while true do
		local unknown_result: any = nil
		local result = unknown_result
		if result.channel == "inbox" then
			local payload_data = result.value
			if payload_data.kind == "create" then
				create_session(payload_data)
			elseif payload_data.kind == "activity" then
				update_session_activity(payload_data.session_id)
			end
		elseif result.channel == "gc" then
			check_inactive_sessions()
		end
		break
	end
end

run()
`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithManifest("time", timeManifest))
	if result.HasError() {
		t.Fatalf("expected loop sibling captured map time fields to converge, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_MethodSelfMapReadPreservesDeclaredValueAfterFieldWrite(t *testing.T) {
	source := `
type Time = {}

type Snapshot = {
	id: string,
	opened_at: Time,
	last_seen: Time,
	last_value: string?,
	flags: {[string]: boolean},
}

type Store = {
	sessions: {[string]: Snapshot},
	open: (self: Store, id: string, now: Time) -> Snapshot,
}

local Store = {}
Store.__index = Store

function Store:open(id: string, now: Time): Snapshot
	local existing = self.sessions[id]
	if existing then
		existing.last_seen = now
		return existing
	end

	local created: Snapshot = {
		id = id,
		opened_at = now,
		last_seen = now,
		last_value = nil,
		flags = {},
	}
	self.sessions[id] = created
	return created
end
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected method self map read to preserve declared value after field write, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_TableCreateFreshArrayShapeSurvivesConstructorMethodBoundary(t *testing.T) {
	source := `
local FlowGraph = {}
FlowGraph.__index = FlowGraph

function FlowGraph.new()
	return setmetatable({
		node_order = table.create(16, 0),
	}, FlowGraph)
end

function FlowGraph:add_node(node_id: string)
	table.insert(self.node_order, node_id)
end

function FlowGraph:compute_auto_chain()
	for i = 1, #self.node_order - 1 do
		local current_node_id = self.node_order[i]
		local next_node_id = self.node_order[i + 1]
		if current_node_id and next_node_id then
			local pair = current_node_id .. ":" .. next_node_id
		end
	end
end

local graph = FlowGraph.new()
graph:add_node("a")
graph:add_node("b")
graph:compute_auto_chain()
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected table.create array allocation evidence to survive constructor/method boundary, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_TableCreateFreshHashShapeAcceptsDynamicWrites(t *testing.T) {
	source := `
local Store = {}
Store.__index = Store

function Store.new()
	return setmetatable({
		values = table.create(0, 16),
	}, Store)
end

function Store:set(id: string, value: number)
	self.values[id] = value
end

function Store:get(id: string): number
	return self.values[id] or 0
end

local store = Store.new()
store:set("a", 1)
return store:get("a")
	`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected table.create hash allocation evidence to accept dynamic writes, got: %v", testutil.ErrorMessages(result.Diagnostics))
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
local unknown_condition = nil :: any
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
local unknown_condition = nil :: any
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

func TestExternalLint_TestDslAfterEachModelResetDoesNotPolluteNumericHelper(t *testing.T) {
	source := `
local test = {}
function test.describe(_name: string, fn: fun()) fn() end
function test.it(_name: string, fn: fun()) fn() end
function test.after_each(fn: fun()) fn() end
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

local models = {
	get_by_name = function(model_name)
		return {
			max_tokens = 8000,
			output_tokens = 1000,
		}, nil
	end,
}

local compress = {
	_models = models,
}

local CONFIG = {
	chars_per_token = 4,
	prompt_buffer_tokens = 500,
	context_safety_margin = 0.1,
	output_buffer_tokens = 200,
	default_temperature = 0.2,
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

function compress.to_size(model_name: string, content: string, target_chars: number, options, mock_model_info)
	options = options or {}
	local model_info, err = get_model_info(model_name, mock_model_info)
	if err then
		return nil, err
	end
	model_info = assert(model_info)
	return calculate_safe_max_tokens(target_chars, model_info)
end

function compress.get_stats(model_name: string, content: string, target_chars: number, mock_model_info)
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

local function define_tests()
	describe("compress", function()
		after_each(function()
			compress._models = models
		end)

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

		it("uses stats", function()
			compress._models = {
				get_by_name = function(model_name)
					return {
						max_tokens = 1000,
						output_tokens = 200,
					}, nil
				end,
			}
			return compress.get_stats("small-model", "content", 500)
		end)
	end)
end

return test.run_cases(define_tests)
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected test DSL after_each model resets not to pollute numeric helper, got: %v", testutil.ErrorMessages(result.Diagnostics))
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
