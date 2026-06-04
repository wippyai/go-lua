package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
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
