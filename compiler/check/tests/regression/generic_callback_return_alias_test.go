package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

const callbackReturnProtocolModule = `
type SessionSummary = {
	id: string,
	total_steps: number,
}

local M = {}
M.SessionSummary = SessionSummary
return M
`

const callbackReturnResultModule = `
type ErrorCode = "not_found" | "invalid" | "busy"
type AppError = { code: ErrorCode, message: string, retryable: boolean }
type Result<T> = { ok: true, value: T } | { ok: false, error: AppError }

local M = {}
M.ErrorCode = ErrorCode
M.AppError = AppError
M.Result = Result

function M.and_then<T, U>(r: Result<T>, fn: (T) -> Result<U>): Result<U>
	if r.ok then
		return fn(r.value)
	end
	return { ok = false, error = r.error }
end

return M
`

const declaredReturnStringResultModule = `
type Result<T> = { ok: true, value: T } | { ok: false, error: string }

local M = {}
M.Result = Result

function M.ok<T>(value: T): Result<T>
	return { ok = true, value = value }
end

function M.err<T>(message: string): Result<T>
	return { ok = false, error = message }
end

function M.and_then<T, U>(r: Result<T>, fn: (T) -> Result<U>): Result<U>
	if r.ok then
		return fn(r.value)
	end
	return { ok = false, error = r.error }
end

return M
`

func TestGenericCallbackAnnotatedReturnAliasSatisfiesResultCallback(t *testing.T) {
	protocol := exportModule(t, "protocol", callbackReturnProtocolModule)
	resultMod := exportModule(t, "result", callbackReturnResultModule)

	result := testutil.Check(`
local protocol = require("protocol")
local result = require("result")

type StringResult = { ok: true, value: string } | { ok: false, error: result.AppError }
type SummaryResult = { ok: true, value: protocol.SessionSummary } | { ok: false, error: result.AppError }

local summary_result: SummaryResult = {
	ok = true,
	value = {
		id = "session",
		total_steps = 1,
	},
}

local summary_id = result.and_then(summary_result, function(summary: protocol.SessionSummary): StringResult
	if summary.total_steps == 0 then
		return {
			ok = false,
			error = {
				code = "invalid",
				message = "expected steps",
				retryable = false,
			},
		}
	end
	return { ok = true, value = summary.id }
end)

if summary_id.ok then
	local id: string = summary_id.value
else
	local code: string = summary_id.error.code
end
`, testutil.WithStdlib(), testutil.WithManifest("protocol", protocol), testutil.WithManifest("result", resultMod))

	if result.HasError() {
		t.Fatalf("annotated callback return alias lost compatibility: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

func TestDeclaredResultReturnAliasOwnsCallSummary(t *testing.T) {
	resultMod := exportModule(t, "result", declaredReturnStringResultModule)

	result := testutil.Check(`
local result = require("result")

type Result<T> = { ok: true, value: T } | { ok: false, error: string }
type User = { id: string, name: string, email: string, active: boolean }

local M = {}

function M.find_by_id(id: string): Result<User>
	if id == "" then
		return result.err("missing user")
	end
	return result.ok({
		id = id,
		name = "Ada",
		email = "ada@example.test",
		active = true,
	})
end

function M.find_active(id: string): Result<User>
	local r = M.find_by_id(id)
	return result.and_then(r, function(user: User): Result<User>
		if user.active then
			return result.ok(user)
		end
		return result.err("inactive user")
	end)
end

local found = M.find_active("ada")
if found.ok then
	local email: string = found.value.email
end
`, testutil.WithStdlib(), testutil.WithManifest("result", resultMod))

	if result.HasError() {
		t.Fatalf("declared Result<User> return leaked body summary into call site: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}

const callbackArrayProtocolModule = `
type Event = {
	kind: "metric" | "log",
	name: string,
	tags: {[string]: string},
}

type Metric = {
	name: string,
	value: number,
	tags: {[string]: string},
}

local M = {}
M.Event = Event
M.Metric = Metric
return M
`

const callbackArrayBuilderModule = `
local protocol = require("protocol")

local M = {}

function M.metric(name: string, value: number, tags: {[string]: string}): protocol.Metric
	local metric: protocol.Metric = {name = name, value = value, tags = tags}
	return metric
end

function M.event(metric: protocol.Metric): protocol.Event
	local event: protocol.Event = {
		kind = "metric",
		name = metric.name,
		tags = metric.tags,
	}
	return event
end

return M
`

func TestGenericArrayMapCallbackReturnPreservesImportedAlias(t *testing.T) {
	protocol := exportModule(t, "protocol", callbackArrayProtocolModule)
	builder := exportModule(t, "builder", callbackArrayBuilderModule, testutil.WithManifest("protocol", protocol))
	builderRecord := unwrap.Record(builder.Export)
	if builderRecord == nil {
		t.Fatalf("builder export = %v, want record", builder.Export)
	}
	eventField := builderRecord.GetField("event")
	if eventField == nil {
		t.Fatalf("builder export missing event field: %v", builder.Export)
	}
	eventFn := unwrap.Function(eventField.Type)
	if eventFn == nil || len(eventFn.Returns) != 1 {
		t.Fatalf("builder.event export = %v, want one-return function", eventField.Type)
	}
	eventAlias, ok := eventFn.Returns[0].(*typ.Alias)
	if !ok || eventAlias.Name != "protocol.Event" {
		t.Fatalf("builder.event return = %v, want protocol.Event alias", eventFn.Returns[0])
	}

	result := testutil.Check(`
local builder = require("builder")
local protocol = require("protocol")

local function map<T, U>(items: {T}, fn: (T) -> U): {U}
	local out: {U} = {}
	for _, item in ipairs(items) do
		table.insert(out, fn(item))
	end
	return out
end

local metrics = {
	builder.metric("latency", 42, {source = "api"}),
	builder.metric("errors", 0, {source = "worker"}),
}

local events: {protocol.Event} = map(metrics, function(metric: protocol.Metric): protocol.Event
	return builder.event(metric)
end)

local first = events[1]
if first then
	local kind: "metric" | "log" = first.kind
	local name: string = first.name
end
`, testutil.WithStdlib(), testutil.WithManifest("protocol", protocol), testutil.WithManifest("builder", builder))

	if result.HasError() {
		t.Fatalf("generic array callback return lost imported alias: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
