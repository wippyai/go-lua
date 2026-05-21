package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestAdvancedTypeSystem_DiscriminatedEventPipelineWithDynamicDecode(t *testing.T) {
	source := `
type MessageEvent = {kind: "message", id: string, text: string, tags: {string}?}
type ToolEvent = {kind: "tool", id: string, name: string, arguments: {[string]: any}}
type ErrorEvent = {kind: "error", id: string, error: {code: string, message: string}}
type Event = MessageEvent | ToolEvent | ErrorEvent

local function require_string(value, fallback: string): string
	if type(value) == "string" then
		return value
	end
	return fallback
end

local function string_array(value): {string}?
	if type(value) ~= "table" then
		return nil
	end
	local out: {string} = {}
	for _, item in ipairs(value) do
		if type(item) == "string" then
			table.insert(out, item)
		end
	end
	return out
end

local function decode_event(raw: any): (Event?, string?)
	if type(raw) ~= "table" then
		return nil, "event must be a table"
	end

	if raw.kind == "message" then
		return {
			kind = "message",
			id = require_string(raw.id, ""),
			text = require_string(raw.text, ""),
			tags = string_array(raw.tags),
		}, nil
	end

	if raw.kind == "tool" then
		return {
			kind = "tool",
			id = require_string(raw.id, ""),
			name = require_string(raw.name, ""),
			arguments = type(raw.arguments) == "table" and (raw.arguments :: {[string]: any}) or {},
		}, nil
	end

	if raw.kind == "error" then
		return {
			kind = "error",
			id = require_string(raw.id, ""),
			error = {
				code = require_string(raw.code, "unknown"),
				message = require_string(raw.message, "failed"),
			},
		}, nil
	end

	return nil, "unknown event"
end

local function render_event(event: Event): string
	if event.kind == "message" then
		return event.id .. ":" .. event.text
	end
	if event.kind == "tool" then
		return event.id .. ":" .. event.name
	end
	return event.id .. ":" .. event.error.code .. ":" .. event.error.message
end

local function render_all(raw_events: {any}): ({string}, {string})
	local rendered: {string} = {}
	local errors: {string} = {}
	for _, raw in ipairs(raw_events) do
		local event, err = decode_event(raw)
		if event then
			table.insert(rendered, render_event(event))
		else
			table.insert(errors, err or "unknown")
		end
	end
	return rendered, errors
end

local rendered, errors = render_all({
	{ kind = "message", id = "m1", text = "hello" },
	{ kind = "tool", id = "t1", name = "search", arguments = { query = "lua" } },
	{ kind = "error", id = "e1", code = "E", message = "boom" },
})

return rendered[1] or errors[1] or ""
`
	assertNoAdvancedStressErrors(t, source)
}

func TestAdvancedTypeSystem_ResultPipelineKeepsMultiReturnCorrelation(t *testing.T) {
	source := `
type Err = {kind: string, message: string}
type User = {id: string, name: string, roles: {string}}
type Session = {id: string, user: User, expires_at: number}

local users: {[string]: User} = {
	["u1"] = { id = "u1", name = "Ada", roles = ({ "admin" } :: {string}) },
}

local function find_user(id: string): (User?, Err?)
	local user = users[id]
	if not user then
		return nil, { kind = "not_found", message = id }
	end
	return user, nil
end

local function create_session(user: User, now: number): (Session?, Err?)
	if #user.roles == 0 then
		return nil, { kind = "forbidden", message = user.id }
	end
	return { id = user.id .. ":" .. tostring(now), user = user, expires_at = now + 3600 }, nil
end

local function with_user(id: string, now: number, fn: (User, number) -> (Session?, Err?)): (Session?, Err?)
	local user, err = find_user(id)
	if err then
		return nil, err
	end
	return fn(user, now)
end

local session, err = with_user("u1", 10, create_session)
if err then
	return err.message
end
return session.user.name .. ":" .. tostring(session.expires_at)
`
	assertNoAdvancedStressErrors(t, source)
}

func TestAdvancedTypeSystem_FluentMetatableBuilderPreservesStateAcrossMethods(t *testing.T) {
	source := `
type Request = {
	method: string,
	path: string,
	headers: {[string]: string},
	query: {[string]: string},
	timeout: number,
}

local Builder = {}
Builder.__index = Builder

function Builder.new()
	return setmetatable({
		method = "GET",
		path = "/",
		headers = {} :: {[string]: string},
		query = {} :: {[string]: string},
		timeout = 30,
	}, Builder)
end

function Builder.with_method(self: Request, method: string): Request
	self.method = method
	return self
end

function Builder.with_header(self: Request, key: string, value: string): Request
	self.headers[key] = value
	return self
end

function Builder.with_query(self: Request, key: string, value: string?): Request
	if value then
		self.query[key] = value
	end
	return self
end

function Builder.with_timeout(self: Request, timeout: number?): Request
	self.timeout = timeout or self.timeout
	return self
end

function Builder.build(self: Request): Request
	return {
		method = self.method,
		path = self.path,
		headers = self.headers,
		query = self.query,
		timeout = self.timeout,
	}
end

local req = Builder.build(
	Builder.with_timeout(
		Builder.with_query(
			Builder.with_header(
				Builder.with_method(Builder.new() :: Request, "POST"),
				"Accept",
				"application/json"
			),
			"q",
			"lua"
		),
		nil
	)
)

return req.method .. ":" .. req.headers.Accept .. ":" .. tostring(req.timeout)
`
	assertNoAdvancedStressErrors(t, source)
}

func TestAdvancedTypeSystem_ModuleBoundaryPreservesTaggedResultAndCallbacks(t *testing.T) {
	repoModule := testutil.CheckAndExport(`
local repo = {}

type Row = {id: string, payload: string, metadata: {[string]: any}?}
type Found = {ok: true, row: Row}
type Missing = {ok: false, error: {kind: "missing", message: string}}
type Result = Found | Missing
type Mapper = (Row) -> string

local rows = {
	["a"] = { id = "a", payload = "hello", metadata = { source = "test" } },
}

function repo.get(id: string): Result
	local row = rows[id]
	if row then
		return { ok = true, row = row }
	end
	return { ok = false, error = { kind = "missing", message = id } }
end

function repo.map(id: string, mapper: Mapper): (string?, string?)
	local result = repo.get(id)
	if result.ok then
		return mapper(result.row), nil
	end
	return nil, result.error.message
end

return repo
`, "repo", testutil.WithStdlib())
	if repoModule.HasError() {
		t.Fatalf("repo module errors: %v", testutil.ErrorMessages(repoModule.Errors))
	}

	source := `
local repo = require("repo")

local value, err = repo.map("a", function(row)
	local source = row.metadata and row.metadata.source or "none"
	return row.id .. ":" .. row.payload .. ":" .. tostring(source)
end)

if err then
	return err
end
return value
`
	result := testutil.Check(source, testutil.WithStdlib(), testutil.WithModule("repo", repoModule))
	if result.HasError() {
		t.Fatalf("expected module boundary to preserve tagged result and callback parameter shape, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestAdvancedTypeSystem_GenericResultCombinatorsPreserveDiscriminantsAndPayloads(t *testing.T) {
	source := `
type Failure = {code: string, message: string}
type Result<T> = {ok: true, value: T} | {ok: false, error: Failure}
type Envelope = {id: string, attrs: {[string]: string}, nested: {attempts: number}}
type View = {label: string, attempts: number}

local function ok<T>(value: T): Result<T>
	return { ok = true, value = value }
end

local function fail<T>(code: string, message: string): Result<T>
	return { ok = false, error = { code = code, message = message } }
end

local function map<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
	if result.ok then
		return ok(fn(result.value))
	end
	return { ok = false, error = result.error }
end

local function and_then<T, U>(result: Result<T>, fn: (T) -> Result<U>): Result<U>
	if result.ok then
		return fn(result.value)
	end
	return { ok = false, error = result.error }
end

local function decode(raw: any): Result<Envelope>
	if type(raw) ~= "table" then
		return fail("shape", "not a table")
	end
	if type(raw.id) ~= "string" then
		return fail("shape", "missing id")
	end
	return ok({
		id = raw.id,
		attrs = type(raw.attrs) == "table" and (raw.attrs :: {[string]: string}) or {},
		nested = { attempts = type(raw.attempts) == "number" and raw.attempts or 0 },
	})
end

local view = and_then(decode({
	id = "evt",
	attrs = { source = "test" },
	attempts = 2,
}), function(env: Envelope): Result<View>
	return map(ok(env), function(inner: Envelope): View
		return {
			label = inner.id .. ":" .. inner.attrs.source,
			attempts = inner.nested.attempts + 1,
		}
	end)
end)

if view.ok then
	local label: string = view.value.label
	local attempts: number = view.value.attempts
	return label .. ":" .. tostring(attempts)
end
return view.error.code .. ":" .. view.error.message
`
	assertNoAdvancedStressErrors(t, source)
}

func TestAdvancedTypeSystem_ExpressionLocalTypeGuardRefinesExpectedTableField(t *testing.T) {
	source := `
type Box = {attempts: number}

local function wrap(value: Box): Box
	return value
end

local function decode(raw: any): Box
	if type(raw) ~= "table" then
		return { attempts = 0 }
	end
	return wrap({
		attempts = type(raw.attempts) == "number" and raw.attempts or 0,
	})
end

local box = decode({ attempts = 2 })
local attempts: number = box.attempts
return tostring(attempts)
`
	assertNoAdvancedStressErrors(t, source)
}

func TestAdvancedTypeSystem_NestedConfigBuilderKeepsPreciseMapAndArrayShapes(t *testing.T) {
	source := `
type Plugin = {id: string, enabled: boolean, config: {[string]: any}}
type Pipeline = {name: string, plugins: {Plugin}, env: {[string]: string}}

local function add_plugin(pipeline: Pipeline, plugin: Plugin): Pipeline
	table.insert(pipeline.plugins, plugin)
	return pipeline
end

local function enable_defaults(pipeline: Pipeline, defaults: {[string]: string}?): Pipeline
	for key, value in pairs(defaults or {}) do
		pipeline.env[key] = value
	end
	return add_plugin(pipeline, {
		id = "logger",
		enabled = true,
		config = { level = pipeline.env.LOG_LEVEL or "info" },
	})
end

local pipeline = enable_defaults({
	name = "deploy",
	plugins = {},
	env = { LOG_LEVEL = "debug" },
}, { REGION = "local" })

local first = pipeline.plugins[1]
if not first then
	return pipeline.env.REGION
end

return first.id .. ":" .. tostring(first.config.level) .. ":" .. pipeline.env.REGION
`
	assertNoAdvancedStressErrors(t, source)
}

func TestAdvancedTypeSystem_SoundnessRejectsTruthyStringFallbackToNumber(t *testing.T) {
	source := `
local function expect_number(value: number)
	return value + 1
end

local options: {timeout: string?} = { timeout = "30s" }
local timeout = options.timeout or 30
return expect_number(timeout)
`
	assertAdvancedStressErrorContains(t, source, "expected number")
}

func TestAdvancedTypeSystem_SoundnessRejectsMetadataFieldAfterTruthyString(t *testing.T) {
	source := `
local meta: string | {content_type: string} = ""
local artifact = { meta = meta }

if artifact.meta then
	local content_type: string = artifact.meta.content_type
	return content_type
end
return "missing"
`
	assertAdvancedStressErrorContains(t, source, "cannot assign")
}

func assertNoAdvancedStressErrors(t *testing.T, source string) {
	t.Helper()
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("expected advanced type-system stress case to type-check, got: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func assertAdvancedStressErrorContains(t *testing.T, source, want string) {
	t.Helper()
	result := testutil.Check(source, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatalf("expected diagnostic containing %q, got no errors", want)
	}
	messages := strings.Join(testutil.ErrorMessages(result.Diagnostics), " | ")
	if !strings.Contains(messages, want) {
		t.Fatalf("expected diagnostic containing %q, got: %s", want, messages)
	}
}
