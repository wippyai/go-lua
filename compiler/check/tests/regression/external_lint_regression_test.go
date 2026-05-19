package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
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
