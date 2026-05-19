package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

func TestExternalLint_OptionalStringFallbackIsConcreteString(t *testing.T) {
	result := testutil.Check(`
local json = {
	decode = function(value: string): any
		return value
	end,
}

local response: {body: string?} = {}
local decoded = json.decode(response.body or "")
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected string? fallback to produce string, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_ConstantTableNumericFieldStaysNonNil(t *testing.T) {
	result := testutil.Check(`
local CONFIG = {
	chars_per_token = 4,
}

local function limit(tokens)
	return tokens * CONFIG.chars_per_token
end

local value: number = limit(4)
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected constant table numeric field to remain non-nil, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_MethodArgumentCastSatisfiesParameter(t *testing.T) {
	result := testutil.Check(`
local renderer = {}
function renderer:render(name: string): string
	return name
end

local page: {template_name: string?} = {template_name = "main"}
local rendered = renderer:render(page.template_name :: string)
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected explicit argument cast to satisfy method parameter, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_UntypedOverlayURLRequiresStringProof(t *testing.T) {
	result := testutil.Check(`
local http = {
	get = function(url: string, options: table)
		return { url = url, options = options }, nil
	end,
}

local function main(args)
	local url = (args and args.url) or "http://localhost:8085/hello"
	return http.get(url, { timeout = "2s" })
end

return main
`, testutil.WithStdlib())
	requireExternalLintErrorContaining(t, result, "expected string")
}

func TestExternalLint_GuardedOverlayURLFeedsStringContract(t *testing.T) {
	result := testutil.Check(`
local http = {
	get = function(url: string, options: table)
		return { url = url, options = options }, nil
	end,
}

local function main(args)
	local url = "http://localhost:8085/hello"
	if args and type(args.url) == "string" then
		url = args.url
	end
	return http.get(url, { timeout = "2s" })
end

return main
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected guarded url override to feed string contract, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_DynamicResourceIDsRequireStringProof(t *testing.T) {
	result := testutil.Check(`
local function qualify_id(entry_id: string, relative_id: string?)
	if relative_id:find(":") then
		return relative_id
	end
	return entry_id .. ":" .. relative_id
end

local function collect(entry)
	local resources = {}
	if entry.data.resources then
		for i, resource_id in ipairs(entry.data.resources) do
			resources[i] = qualify_id(entry.id, resource_id)
		end
	end
	return resources
end

return collect
`, testutil.WithStdlib())
	requireExternalLintErrorContaining(t, result, "expected string")
}

func TestExternalLint_GuardedResourceIDsFeedQualifier(t *testing.T) {
	result := testutil.Check(`
local function qualify_id(entry_id: string, relative_id: string?)
	if relative_id:find(":") then
		return relative_id
	end
	return entry_id .. ":" .. relative_id
end

local function collect(entry)
	local resources = {}
	if entry.data.resources then
		for i, resource_id in ipairs(entry.data.resources) do
			if type(resource_id) == "string" then
				resources[i] = qualify_id(entry.id, resource_id)
			end
		end
	end
	return resources
end

return collect
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected guarded resource ids to feed string qualifier, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_UntypedPageIDRequiresStringProofForAccessibleRoutes(t *testing.T) {
	result := testutil.Check(`
local function can_access(_page)
	return true
end

local unknown_id: any = nil
local all_pages = {
	{ id = unknown_id, mount_route = "/ok/:part(.*)*", secure = false },
}
local routes_map: {[string]: string} = {
	["/ok/:part(.*)*"] = "page:ok",
}
local accessible: {[string]: string} = {}

for _, page in ipairs(all_pages) do
	local mr = page.mount_route
	if mr and routes_map[mr] == page.id and (not page.secure or can_access(page)) then
		accessible[mr] = page.id
	end
end

return accessible
`, testutil.WithStdlib())
	requireExternalLintErrorContaining(t, result, "cannot assign")
}

func TestExternalLint_GuardedPageIDFeedsAccessibleRoutes(t *testing.T) {
	result := testutil.Check(`
local function can_access(_page)
	return true
end

local unknown_id: any = "page:ok"
local all_pages = {
	{ id = unknown_id, mount_route = "/ok/:part(.*)*", secure = false },
}
local routes_map: {[string]: string} = {
	["/ok/:part(.*)*"] = "page:ok",
}
local accessible: {[string]: string} = {}

for _, page in ipairs(all_pages) do
	local mr = page.mount_route
	if type(page.id) == "string" and mr and routes_map[mr] == page.id and (not page.secure or can_access(page)) then
		accessible[mr] = page.id
	end
end

return accessible
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected guarded page id to feed accessible route map, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_UntypedTestRunnerEntryIDRequiresStringProof(t *testing.T) {
	result := testutil.Check(`
local function short_name(id: string): string
	return id
end

local function run_suite(tests: {any})
	for _, entry in ipairs(tests) do
		local test_name = short_name(entry.id)
	end
end

run_suite({ { id = 42 } })
`, testutil.WithStdlib())
	requireExternalLintErrorContaining(t, result, "expected string")
}

func TestExternalLint_GuardedTestRunnerEntryIDFeedsShortName(t *testing.T) {
	result := testutil.Check(`
local function short_name(id: string): string
	return id
end

local function run_suite(tests: {any})
	for _, entry in ipairs(tests) do
		if type(entry.id) == "string" then
			local test_name = short_name(entry.id)
		end
	end
end

run_suite({ { id = "suite:test" } })
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected guarded runner entry id to feed short_name, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_UntypedConfigMutationCanInvalidateNumericReads(t *testing.T) {
	result := testutil.Check(`
local CONFIG = {
	chars_per_token = 4,
}

local function tokens_to_chars(tokens)
	return math.floor(tokens * CONFIG.chars_per_token)
end

local function configure(new_config)
	for key, value in pairs(new_config) do
		if CONFIG[key] ~= nil then
			CONFIG[key] = value
		end
	end
end

configure({ chars_per_token = "bad" })
return tokens_to_chars(10)
`, testutil.WithStdlib())
	requireExternalLintErrorContaining(t, result, "cannot assign")
}

func TestExternalLint_TypedConfigMutationPreservesNumericReads(t *testing.T) {
	result := testutil.Check(`
type ConfigUpdate = {
	chars_per_token: integer?,
}

local CONFIG = {
	chars_per_token = 4,
}

local function tokens_to_chars(tokens)
	return math.floor(tokens * CONFIG.chars_per_token)
end

local function configure(new_config: ConfigUpdate)
	for key, value in pairs(new_config) do
		if CONFIG[key] ~= nil then
			CONFIG[key] = value
		end
	end
end

configure({ chars_per_token = 5 })
return tokens_to_chars(10)
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected typed config update to preserve numeric reads, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_StringMetadataRequiresStructuredProof(t *testing.T) {
	result := testutil.Check(`
local artifact = {
	meta = "",
}

local content_type = artifact.meta.content_type
return content_type
`, testutil.WithStdlib())
	requireExternalLintErrorContaining(t, result, "field 'content_type' does not exist")
}

func TestExternalLint_GuardedStructuredMetadataAllowsFieldAccess(t *testing.T) {
	result := testutil.Check(`
local artifact: {meta: string | {content_type: string}} = {
	meta = { content_type = "text/plain" },
}

if type(artifact.meta) == "table" then
	local content_type: string = artifact.meta.content_type
end
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected metadata table guard to allow field access, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_PairsSchemaFilterWritesRecursiveValueBackToSameKey(t *testing.T) {
	result := testutil.Check(`
local function filter_tool_schema(schema)
	local function recursive_filter(obj)
		if type(obj) ~= "table" then
			return obj
		end

		obj.multipleOf = nil
		obj.additionalProperties = nil

		for key, value in pairs(obj) do
			if type(value) == "table" then
				obj[key] = recursive_filter(value)
			end
		end

		return obj
	end

	schema.examples = nil
	return recursive_filter(schema)
end

return filter_tool_schema
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected recursive schema filter to write table value back to same pairs key, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_RejectsPairsWriteThatChangesClosedFieldDomain(t *testing.T) {
	result := testutil.Check(`
local item = {
	count = 1,
	name = "ready",
}

for key, value in pairs(item) do
	item[key] = tostring(value)
end

local count: number = item.count
return count
`, testutil.WithStdlib())
	requireExternalLintErrorContaining(t, result, "cannot assign")
}

func TestExternalLint_RejectsPairsWriteThatWeakensTypedMapElement(t *testing.T) {
	result := testutil.Check(`
type Item = {
	id: string,
}

local items: {[string]: Item} = {
	one = { id = "one" },
}

for key, value in pairs(items) do
	if type(value) == "table" then
		items[key] = {}
	end
end

return items
`, testutil.WithStdlib())
	requireExternalLintErrorContaining(t, result, "cannot assign")
}

func TestExternalLint_TypedMapNilWriteDeletesEntry(t *testing.T) {
	result := testutil.Check(`
type Hub = {
	pid: string,
}

local hubs: {[string]: Hub} = {
	one = { pid = "p1" },
}

local id = "one"
hubs[id] = nil
return hubs
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected nil write to delete typed map entry, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_RequiredRecordFieldNilWriteStillRejected(t *testing.T) {
	result := testutil.Check(`
type Hub = {
	pid: string,
}

local hub: Hub = { pid = "p1" }
local key = "pid"
hub[key] = nil
return hub
`, testutil.WithStdlib())
	requireExternalLintErrorContaining(t, result, "cannot assign")
}

func TestExternalLint_ExplicitAnyArrayParameterKeepsBroadCallContract(t *testing.T) {
	result := testutil.Check(`
local function consume(responses: {any})
	return responses[1] or { ok = false }
end

consume({
	{ ok = true, value = "first" },
	{ ok = false },
})
consume({})
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected explicit {any} parameter annotation to accept empty array call, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_ManifestAssertionWithoutSummaryDoesNotNarrow(t *testing.T) {
	assertManifest := io.NewManifest("assert_mod")
	assertManifest.SetExport(typ.NewRecord().
		Field("not_nil", typ.Func().
			Param("value", typ.Any).
			OptParam("message", typ.String).
			Returns(typ.Any).
			Build()).
		Build())

	result := testutil.Check(`
local assert_mod = require("assert_mod")

local sent = nil
assert_mod.not_nil(sent)
local topic = sent.topic
return topic
`, testutil.WithStdlib(), testutil.WithManifest("assert_mod", assertManifest))
	requireExternalLintErrorContaining(t, result, "cannot index type nil")
}

func TestExternalLint_ImportedNotNilNarrowsNilInitializedCapturedLocal(t *testing.T) {
	testModule := testutil.CheckAndExport(`
local test = {}

function test.not_nil(val: any, msg: string?): any
	if val == nil then
		error(msg or "assertion failed")
	end
	return val
end

return test
`, "test_mod", testutil.WithStdlib())
	if testModule.HasError() {
		t.Fatalf("test module should type-check, got: %v", testutil.ErrorMessages(testModule.Errors))
	}

	result := testutil.Check(`
local test = require("test_mod")

local sent = nil
local function assign_later(value)
	sent = value
end

assign_later({ type = "__next", topic = "process_data" })
test.not_nil(sent)
local topic: string = sent.topic
return topic
`, testutil.WithStdlib(), testutil.WithModule("test_mod", testModule))
	if result.HasError() {
		t.Fatalf("expected imported not_nil to narrow nil-initialized local after callback-style assignment, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_ImportedNotNilMakesNilOnlyPathUnreachable(t *testing.T) {
	testModule := testutil.CheckAndExport(`
local test = {}

function test.not_nil(val: any, msg: string?): any
	if val == nil then
		error(msg or "assertion failed")
	end
	return val
end

return test
`, "test_mod", testutil.WithStdlib())
	if testModule.HasError() {
		t.Fatalf("test module should type-check, got: %v", testutil.ErrorMessages(testModule.Errors))
	}

	result := testutil.Check(`
local test = require("test_mod")

local sent = nil
test.not_nil(sent)
local topic = sent.topic
return topic
`, testutil.WithStdlib(), testutil.WithModule("test_mod", testModule))
	if result.HasError() {
		t.Fatalf("expected imported not_nil on nil-only input to make following path unreachable, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_ImportedNotNilNarrowsCapturedTableMethodWriteLocal(t *testing.T) {
	testModule := testutil.CheckAndExport(`
local test = {}

function test.not_nil(val: any, msg: string?): any
	if val == nil then
		error(msg or "assertion failed")
	end
	return val
end

return test
`, "test_mod", testutil.WithStdlib())
	if testModule.HasError() {
		t.Fatalf("test module should type-check, got: %v", testutil.ErrorMessages(testModule.Errors))
	}

	result := testutil.Check(`
local test = require("test_mod")

local sent = nil
local chan = {}
chan.send = function(self, value)
	sent = value
	return true
end

test.not_nil(sent)
local topic = sent.topic
return topic
`, testutil.WithStdlib(), testutil.WithModule("test_mod", testModule))
	if result.HasError() {
		t.Fatalf("expected imported not_nil to narrow captured table-method local, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_OptionalFieldAcceptsOptionalSourceValue(t *testing.T) {
	result := testutil.Check(`
type ToolCall = {
	context: table?,
}
type ValidatedTool = {
	context: table?,
	valid: boolean,
}

local function validate(tool_call: ToolCall): ValidatedTool
	return {
		context = tool_call.context,
		valid = true,
	}
end

return validate
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected optional field to accept optional source value, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_OptionalFieldMapEntryAcceptsOptionalSourceValue(t *testing.T) {
	result := testutil.Check(`
type ToolCall = {
	id: string,
	context: table?,
}

local function validate(tool_call: ToolCall)
	local validated_tools: {[string]: any} = {}
	validated_tools[tool_call.id] = {
		context = tool_call.context,
		valid = true,
	}
	return validated_tools
end

return validate
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected optional field in map entry to accept optional source value, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_UntypedCommandHandlerCannotEnterTypedRegistry(t *testing.T) {
	result := testutil.Check(`
type Handler = (any, any) -> (any, string?)

local handlers: {[string]: Handler} = {}
local handler_func: (...any) -> any = function(...)
	return nil
end

local name = "stop"
handlers[name] = handler_func
`, testutil.WithStdlib())
	requireExternalLintErrorContaining(t, result, "cannot assign")
}

func TestExternalLint_TypedCommandHandlerCanEnterTypedRegistry(t *testing.T) {
	result := testutil.Check(`
type Handler = (any, any) -> (any, string?)

local handlers: {[string]: Handler} = {}
local handler_func: Handler = function(ctx, msg)
	return nil, nil
end

local name = "stop"
handlers[name] = handler_func
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected typed handler function to satisfy handler registry, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_UntypedLimitRequiresNumberProof(t *testing.T) {
	result := testutil.Check(`
local repo = {
	list = function(limit: number)
		return limit
	end,
}

local function load(args: any)
	return repo.list(args.limit)
end

return load
`, testutil.WithStdlib())
	requireExternalLintErrorContaining(t, result, "expected number")
}

func TestExternalLint_GuardedLimitFeedsNumberContract(t *testing.T) {
	result := testutil.Check(`
local repo = {
	list = function(limit: number)
		return limit
	end,
}

local function load(args)
	local limit = 100
	if type(args.limit) == "number" then
		limit = args.limit
	end
	return repo.list(limit)
end

return load
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected guarded numeric limit to satisfy number contract, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_DynamicControlPayloadRequiresTypedProof(t *testing.T) {
	result := testutil.Check(`
type ControlOp = {
	from_pid: string,
}

local function handle(op: ControlOp)
	return op.from_pid
end

local function dispatch(op: any)
	return handle(op)
end

return dispatch
`, testutil.WithStdlib())
	requireExternalLintErrorContaining(t, result, "expected")
}

func TestExternalLint_GuardedControlPayloadFeedsTypedHandler(t *testing.T) {
	result := testutil.Check(`
type ControlOp = {
	from_pid: string,
}

local function handle(op: ControlOp)
	return op.from_pid
end

local function dispatch(op: any)
	if type(op) == "table" and type(op.from_pid) == "string" then
		return handle(op)
	end
	return nil
end

return dispatch
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected guarded control payload to satisfy handler contract, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_StartOptionsRejectsPlainString(t *testing.T) {
	result := testutil.Check(`
type StartOptions = {
	kind?: string,
}

local function start(opts: StartOptions?)
	return opts
end

start("not a table")
`, testutil.WithStdlib())
	requireExternalLintErrorContaining(t, result, "expected")
}

func TestExternalLint_GuardedStartOptionsFeedsOptionalRecord(t *testing.T) {
	result := testutil.Check(`
type StartOptions = {
	kind?: string,
}

local function start(opts: StartOptions?)
	return opts
end

local function run(raw: any)
	if raw == nil then
		return start(nil)
	end
	if type(raw) == "table" then
		if raw.kind == nil or type(raw.kind) == "string" then
			return start(raw)
		end
	end
	return nil
end

return run
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected guarded start options to satisfy optional record contract, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_TruthyAnyFieldRequiresStringProof(t *testing.T) {
	result := testutil.Check(`
local function parse_text(text: string?)
	return text
end

local function extract(block: any)
	if block.text then
		return parse_text(block.text)
	end
	return nil
end

return extract
`, testutil.WithStdlib())
	requireExternalLintErrorContaining(t, result, "expected string")
}

func TestExternalLint_GuardedAnyFieldFeedsStringParser(t *testing.T) {
	result := testutil.Check(`
local function parse_text(text: string?)
	return text
end

local function extract(block: any)
	if type(block.text) == "string" then
		return parse_text(block.text)
	end
	return nil
end

return extract
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected string field guard to feed parser contract, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_GuardedProviderModelFeedsContractArgs(t *testing.T) {
	result := testutil.Check(`
type ContractArgs = {
	model: string,
	options: table,
}

local function merge_provider_options(args: ContractArgs)
	return args
end

local function generate(provider_info: any)
	if type(provider_info.provider_model) == "string" then
		local contract_args = {
			model = provider_info.provider_model,
			options = {},
		}
		return merge_provider_options(contract_args)
	end
	return nil
end

return generate
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected guarded provider model to feed contract args, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_LengthGuardOnAnyDoesNotProveFirstElementShape(t *testing.T) {
	result := testutil.Check(`
local function load_rows(): any
	return {}
end

local rows = load_rows()
if rows and #rows > 0 then
	local first_name: string = rows[1].name
	return first_name
end
return nil
`, testutil.WithStdlib())
	requireExternalLintErrorContaining(t, result, "cannot assign")
}

func TestExternalLint_LocalPredicateGuardNarrowsNumberAfterEarlyReturn(t *testing.T) {
	result := testutil.Check(`
local DEFAULTS = {
	BATCH_SIZE = 10,
}

local function validate_batch_size(size)
	return type(size) == "number" and size > 0 and size <= 1000
end

local function run(config: any, items: {any})
	local batch_size = config.batch_size or DEFAULTS.BATCH_SIZE
	if not validate_batch_size(batch_size) then
		return nil
	end

	for batch_start = 1, #items, batch_size do
		local batch_end = math.min(batch_start + batch_size - 1, #items)
	end
	return true
end

return run
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected local predicate guard to narrow batch size for numeric loop, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_DirectPredicateTrueBranchNarrowsArgument(t *testing.T) {
	result := testutil.Check(`
local function is_positive_number(value)
	return type(value) == "number" and value > 0
end

local function run(value: any)
	if is_positive_number(value) then
		local narrowed: number = value
		return narrowed + 1
	end
	return 0
end

return run
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected direct predicate true branch to narrow argument, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_AssignedPredicateTrueBranchNarrowsArgument(t *testing.T) {
	result := testutil.Check(`
local function is_positive_number(value)
	return type(value) == "number" and value > 0
end

local function run(value: any)
	local ok = is_positive_number(value)
	if ok then
		local narrowed: number = value
		return narrowed + 1
	end
	return 0
end

return run
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected assigned predicate true branch to narrow argument, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_OneSidedPredicateFalseBranchDoesNotNarrowArgument(t *testing.T) {
	result := testutil.Check(`
local function is_positive_number(value)
	return type(value) == "number" and value > 0
end

local function run(value: any)
	if not is_positive_number(value) then
		local narrowed: number = value
		return narrowed
	end
	return 0
end

return run
`, testutil.WithStdlib())
	requireExternalLintErrorContaining(t, result, "cannot assign")
}

func TestExternalLint_AssignedOneSidedPredicateFalseBranchDoesNotNarrowArgument(t *testing.T) {
	result := testutil.Check(`
local function is_positive_number(value)
	return type(value) == "number" and value > 0
end

local function run(value: any)
	local ok = is_positive_number(value)
	if not ok then
		local narrowed: number = value
		return narrowed
	end
	return 0
end

return run
`, testutil.WithStdlib())
	requireExternalLintErrorContaining(t, result, "cannot assign")
}

func TestExternalLint_LogicalPredicateTruePathNarrowsThroughLoop(t *testing.T) {
	result := testutil.Check(`
local function is_count(value)
	return type(value) == "number" and value >= 1
end

local function run(value: any, items: {string})
	if is_count(value) and value <= #items then
		local total = 0
		for i = 1, value do
			total = total + #items[i]
		end
		return total
	end
	return 0
end

return run
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected logical predicate true path to narrow loop bound, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_LogicalPredicateElsePathDoesNotOverNarrow(t *testing.T) {
	result := testutil.Check(`
local function is_count(value)
	return type(value) == "number" and value >= 1
end

local function run(value: any, flag: boolean)
	if is_count(value) and flag then
		return value + 1
	else
		local narrowed: number = value
		return narrowed
	end
end

return run
`, testutil.WithStdlib())
	requireExternalLintErrorContaining(t, result, "cannot assign")
}

func TestExternalLint_ReturnedCallbackUsesExpectedParameterTypesInBody(t *testing.T) {
	result := testutil.Check(`
type Time = { unix: integer }
type Projection = {
	updated_at: Time?,
}
type State = {
	projections: {[string]: Projection},
}
type Projector = (State, Time) -> ()

local function build(): Projector
	return function(state, at)
		state.projections["id"] = {
			updated_at = at,
		}
	end
end

return build
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected returned callback params to use declared return function type, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_ReturnedMethodCallbackUsesExpectedParameterTypesInBody(t *testing.T) {
	result := testutil.Check(`
type Time = { unix: integer }
type Projection = {
	updated_at: Time?,
}
type State = {
	projections: {[string]: Projection},
}
type Builder = {
	build: (self: Builder) -> Projector,
}
type Projector = (State, Time) -> ()

local Builder = {}
Builder.__index = Builder

function Builder:build(): Projector
	return function(state, at)
		state.projections["id"] = {
			updated_at = at,
		}
	end
end

return Builder
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected returned method callback params to use declared return function type, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_ReturnedCallbackContextFlowsThroughLocalProjectionWrite(t *testing.T) {
	result := testutil.Check(`
type Time = { unix: integer }
type Projection = {
	id: string,
	updated_at: Time?,
}
type State = {
	projections: {[string]: Projection},
}
type Projector = (State, Time) -> ()

local function build(): Projector
	return function(state, at)
		local projection = state.projections["id"]
		if not projection then
			projection = {
				id = "id",
				updated_at = at,
			}
			state.projections["id"] = projection
		end
	end
end

return build
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected returned callback context to flow through local projection write, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_ReturnedImportedCallbackContextFlowsThroughProjectionWrite(t *testing.T) {
	timeManifest := io.NewManifest("time")
	timeType := typ.NewInterface("time.Time", []typ.Method{
		{Name: "unix", Type: typ.Func().Param("self", typ.Self).Returns(typ.Integer).Build()},
	})
	timeManifest.DefineType("Time", timeType)
	timeManifest.SetExport(typ.NewInterface("time", []typ.Method{
		{Name: "now", Type: typ.Func().Returns(timeType).Build()},
	}))

	protocol := testutil.CheckAndExport(`
local time = require("time")

type Event = {
	id: string,
	queue: string,
}
type TaskProjection = {
	id: string,
	queue: string,
	status: "queued" | "started" | "completed" | "failed",
	worker: string?,
	output: string?,
	error_code: string?,
	retryable: boolean?,
	source: string?,
	updated_at: time.Time?,
}
type BusState = {
	projections: {[string]: TaskProjection},
}
type Projector = (BusState, Event, time.Time) -> ()

local M = {}
M.Event = Event
M.TaskProjection = TaskProjection
M.BusState = BusState
M.Projector = Projector
return M
`, "protocol", testutil.WithStdlib(), testutil.WithManifest("time", timeManifest))
	if protocol.HasError() {
		t.Fatalf("protocol module should type-check, got: %v", testutil.ErrorMessages(protocol.Session.Diagnostics))
	}

	result := testutil.Check(`
local protocol = require("protocol")

type Builder = {
	build: (self: Builder) -> protocol.Projector,
}

local Builder = {}
Builder.__index = Builder

function Builder:build(): protocol.Projector
	return function(state: protocol.BusState, event: protocol.Event, at)
		local projection = state.projections[event.id]
		if not projection then
			projection = {
				id = event.id,
				queue = event.queue,
				status = "queued",
				worker = nil,
				output = nil,
				error_code = nil,
				retryable = nil,
				source = nil,
				updated_at = at,
			}
			state.projections[event.id] = projection
		end
	end
end

return Builder
`, testutil.WithStdlib(), testutil.WithManifest("time", timeManifest), testutil.WithModule("protocol", protocol))
	if result.HasError() {
		t.Fatalf("expected imported returned callback context to flow through projection write, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestExternalLint_RejectsEscapedNarrowProjectionAliasBeforeWideWrite(t *testing.T) {
	result := testutil.Check(`
type QueuedProjection = {
	status: "queued",
}
type Projection = {
	status: "queued" | "started",
}
type State = {
	projections: {[string]: Projection},
}

local state: State = { projections = {} }
local queued: QueuedProjection = { status = "queued" }
local projection = queued
local alias = projection
local id: string = "id"

state.projections[id] = projection
state.projections[id].status = "started"
local still_queued: "queued" = alias.status
return still_queued
	`, testutil.WithStdlib())
	requireExternalLintErrorContaining(t, result, "cannot assign")
}
