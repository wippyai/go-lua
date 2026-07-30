package checktest

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
	"github.com/wippyai/go-lua/analysis/diagnostic"
)

func TestCheckAndExportPublishesErrorReturnFromImportedGenericResultField(t *testing.T) {
	resultMod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		local result = {}
		result.Result = Result
		return result
	`, "result")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result module errors = %#v, want none", resultMod.Errors)
	}

	repoMod := CheckAndExport(`
		local result = require("result")
		type User = { id: string, email: string }
		local repo = {}
		function repo.find_by_id(id: string): Result<User>
			if id == "" then
				return { ok = false, error = "missing" }
			end
			return { ok = true, value = { id = id, email = "a@test" } }
		end
		return repo
	`, "repo", WithStdlib(), WithModule("result", resultMod))
	if len(repoMod.Errors) != 0 {
		t.Fatalf("repo module errors = %#v, want none", repoMod.Errors)
	}

	serviceMod := CheckAndExport(`
		local repo = require("repo")
		local service = {}
		function service.get_email(id: string): (string?, string?)
			local r = repo.find_by_id(id)
			if r.ok then
				return r.value.email, nil
			end
			return nil, r.error
		end
		return service
	`, "service", WithStdlib(), WithModule("result", resultMod), WithModule("repo", repoMod))
	if len(serviceMod.Errors) != 0 {
		t.Fatalf("service module errors = %#v, want none", serviceMod.Errors)
	}

	sig, ok := serviceMod.Manifest.FunctionSignatures["service.get_email"]
	if !ok {
		t.Fatalf("missing service.get_email function signature: %#v", serviceMod.Manifest.FunctionSignatures)
	}
	if !hasErrorReturn(sig.Effect, 0, 1) {
		t.Fatalf("signature type = %v effect = %v, want ErrorReturn(0, 1)", sig.Type, sig.Effect)
	}
}

func TestImportedResultElseErrorAliasSatisfiesCallee(t *testing.T) {
	errorsMod := CheckFileAndExport(`
type AppError = { code: string, message: string }
local errors = {}
errors.AppError = AppError
function errors.wrap(err: AppError, context: string): AppError
    return { code = err.code, message = context .. err.message }
end
return errors
`, "errors", "errors.lua")
	if len(errorsMod.Errors) != 0 {
		t.Fatalf("errors module errors = %#v, want none", errorsMod.Errors)
	}

	validatorMod := CheckFileAndExport(`
local errors = require("errors")
type ValidationResult = { ok: true, value: string } | { ok: false, error: errors.AppError }
local validator = {}
function validator.validate_name(input: string): ValidationResult
    if input == "" then
        return { ok = false, error = { code = "EMPTY", message = "empty" } }
    end
    return { ok = true, value = input }
end
return validator
`, "validator", "validator.lua", WithStdlib(), WithModule("errors", errorsMod))
	if len(validatorMod.Errors) != 0 {
		t.Fatalf("validator module errors = %#v, want none", validatorMod.Errors)
	}

	result := CheckFile(`
local errors = require("errors")
local validator = require("validator")

local result = validator.validate_name("Alice")
if result.ok then
    local name: string = result.value
else
    local err = result.error
    local wrapped = errors.wrap(err, "registration")
end
`, "main.lua", WithStdlib(), WithModule("errors", errorsMod), WithModule("validator", validatorMod))
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported result else error alias to satisfy AppError parameter", result.Diagnostics)
	}
}

func TestCheckAndExportConsumesErrorReturnFromImportedGenericResultField(t *testing.T) {
	resultMod := CheckFileAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		local result = {}
		result.Result = Result
		function result.ok<T>(value: T): Result<T>
			return { ok = true, value = value }
		end
		function result.err<T>(message: string): Result<T>
			return { ok = false, error = message }
		end
		function result.map<T, U>(r: Result<T>, fn: (T) -> U): Result<U>
			if r.ok then
				return result.ok(fn(r.value))
			end
			return { ok = false, error = r.error }
		end
		function result.and_then<T, U>(r: Result<T>, fn: (T) -> Result<U>): Result<U>
			if r.ok then
				return fn(r.value)
			end
			return { ok = false, error = r.error }
		end
		return result
	`, "result", "result.lua")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result module errors = %#v, want none", resultMod.Errors)
	}

	repoMod := CheckFileAndExport(`
		local result = require("result")
		type User = { id: string, email: string, name: string, active: boolean }
		local repo = {}
		repo.User = User
		local users: {[string]: User} = {
			["u1"] = { id = "u1", email = "a@test", name = "Ada", active = true },
			["u2"] = { id = "u2", email = "b@test", name = "Bob", active = false },
		}
		function repo.find_by_id(id: string): Result<User>
			local user = users[id]
			if not user then
				return result.err("missing")
			end
			return result.ok(user)
		end
		function repo.find_active(id: string): Result<User>
			local r = repo.find_by_id(id)
			return result.and_then(r, function(user: User): Result<User>
				if not user.active then
					return result.err("inactive")
				end
				return result.ok(user)
			end)
		end
		return repo
	`, "repo", "repo.lua", WithStdlib(), WithModule("result", resultMod))
	if len(repoMod.Errors) != 0 {
		t.Fatalf("repo module errors = %#v, want none", repoMod.Errors)
	}

	serviceMod := CheckFileAndExport(`
		local repo = require("repo")
		local result = require("result")
		type Greeting = { message: string, user_name: string }
		local service = {}
		function service.greet_user(id: string): Result<Greeting>
			local user_result = repo.find_active(id)
			return result.map(user_result, function(user: User): Greeting
				return { message = "Hello, " .. user.name, user_name = user.name }
			end)
		end
		function service.get_email(id: string): (string?, string?)
			local r = repo.find_by_id(id)
			if r.ok then
				return r.value.email, nil
			end
			return nil, r.error
		end
		return service
	`, "service", "service.lua", WithStdlib(), WithModule("result", resultMod), WithModule("repo", repoMod))
	if len(serviceMod.Errors) != 0 {
		t.Fatalf("service module errors = %#v, want none", serviceMod.Errors)
	}

	checked := CheckFile(`
		local service = require("service")
		local greeting = service.greet_user("u1")
		if greeting.ok then
			local msg: string = greeting.value.message
			local name: string = greeting.value.user_name
		end
		local fail = service.greet_user("u2")
		if not fail.ok then
			local err_msg: string = fail.error
		end
		local email, err = service.get_email("u1")
		if err == nil then
			local e: string = email
		end
	`, "main.lua", WithStdlib(), WithModule("service", serviceMod))
	if len(checked.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported error-return correlation", checked.Diagnostics)
	}
}

func TestGuardedMapReadLocalKeepsRequiredFieldPresent(t *testing.T) {
	checked := Check(`
type User = { id: string, email: string }
local users: {[string]: User} = {
	["u1"] = { id = "u1", email = "a@test" },
}
local user = users["u1"]
if not user then
	return
end
local email: string = user.email
`, WithStdlib())
	if len(checked.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want guarded map read field present", checked.Diagnostics)
	}
}

func TestImportedDeclaredResultNarrowsValueFieldMember(t *testing.T) {
	resultMod := CheckFileAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		local result = {}
		result.Result = Result
		return result
	`, "result", "result.lua")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result module errors = %#v, want none", resultMod.Errors)
	}

	repoMod := CheckFileAndExport(`
		local result = require("result")
		type User = { id: string, email: string }
		local repo = {}
		function repo.find_by_id(id: string): Result<User>
			return { ok = true, value = { id = id, email = "a@test" } }
		end
		return repo
	`, "repo", "repo.lua", WithStdlib(), WithModule("result", resultMod))
	if len(repoMod.Errors) != 0 {
		t.Fatalf("repo module errors = %#v, want none", repoMod.Errors)
	}

	checked := CheckFile(`
		local repo = require("repo")
		local r = repo.find_by_id("u1")
		if r.ok then
			local email: string = r.value.email
		end
	`, "main.lua", WithStdlib(), WithModule("repo", repoMod))
	if len(checked.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported declared result value member to narrow", checked.Diagnostics)
	}
}

func TestCheckAndExportPublishesErrorReturnFromLocalGenericResultField(t *testing.T) {
	mod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		type User = { id: string, email: string }
		local service = {}
		function service.get_email(r: Result<User>): (string?, string?)
			if r.ok then
				return r.value.email, nil
			end
			return nil, r.error
		end
		return service
	`, "service")
	if len(mod.Errors) != 0 {
		t.Fatalf("module errors = %#v, want none", mod.Errors)
	}

	sig, ok := mod.Manifest.FunctionSignatures["service.get_email"]
	if !ok {
		t.Fatalf("missing service.get_email function signature: %#v", mod.Manifest.FunctionSignatures)
	}
	if !hasErrorReturn(sig.Effect, 0, 1) {
		t.Fatalf("signature type = %v effect = %v, want ErrorReturn(0, 1)", sig.Type, sig.Effect)
	}
}

func TestRequireCheckAndExportedGenericMemberSignatureInstantiatesReturn(t *testing.T) {
	resultMod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		local M = {}
		M.Result = Result
		function M.ok<T>(value: T): Result<T>
			return { ok = true, value = value }
		end
		function M.map<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
			if result.ok then
				return M.ok(fn(result.value))
			end
			return { ok = false, error = result.error }
		end
		return M
	`, "result")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result module errors = %#v, want none", resultMod.Errors)
	}
	sig, ok := resultMod.Manifest.FunctionSignatures["result.map"]
	if !ok {
		t.Fatalf("missing result.map function signature: %#v", resultMod.Manifest.FunctionSignatures)
	}
	if sig.Type == nil || len(sig.Type.TypeParams) != 2 {
		t.Fatalf("result.map signature = %v, want two type params", sig.Type)
	}

	checked := Check(`
		local result = require("result")
		type StringResult = { ok: true, value: string } | { ok: false, error: string }
		local decoded: StringResult = result.ok("name")
		local mapped = result.map(decoded, function(value: string)
			return #value
		end)
		if mapped.ok then
			local n: number = mapped.value
		end
	`, WithStdlib(), WithModule("result", resultMod))
	if len(checked.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported generic return instantiated", checked.Diagnostics)
	}
}

func TestRequireCheckAndExportedGenericIteratorAliasesInstantiateReturns(t *testing.T) {
	iterMod := CheckFileAndExport(`
type Predicate<T> = (item: T) -> boolean
type Reducer<T, A> = (acc: A, item: T) -> A

local M = {}

function M.reduce<T, A>(arr: {T}, fn: Reducer<T, A>, initial: A): A
    local acc = initial
    for _, item in ipairs(arr) do
        acc = fn(acc, item)
    end
    return acc
end

function M.find<T>(arr: {T}, pred: Predicate<T>): T?
    for _, item in ipairs(arr) do
        if pred(item) then
            return item
        end
    end
    return nil
end

return M
`, "iter", "iter.lua", WithStdlib())
	if len(iterMod.Errors) != 0 {
		t.Fatalf("iter diagnostics = %#v, want none", iterMod.Errors)
	}
	if sig, ok := iterMod.Manifest.FunctionSignatures["iter.reduce"]; !ok || sig.Type == nil || len(sig.Type.TypeParams) != 2 {
		t.Fatalf("iter.reduce signature = %#v/%v; signatures = %#v", sig.Type, ok, iterMod.Manifest.FunctionSignatures)
	}
	if sig, ok := iterMod.Manifest.FunctionSignatures["iter.find"]; !ok || sig.Type == nil || len(sig.Type.TypeParams) != 1 {
		t.Fatalf("iter.find signature = %#v/%v; signatures = %#v", sig.Type, ok, iterMod.Manifest.FunctionSignatures)
	}

	checked := CheckFile(`
local iter = require("iter")

type User = {name: string, age: number}

local users: {User} = {
    {name = "Ada", age = 42},
}

local first = iter.find(users, function(user: User): boolean
    return user.age > 18
end)

if first then
    local name: string = first.name
    local age: number = first.age
end

local total: number = iter.reduce(users, function(acc: number, user: User): number
    return acc + user.age
end, 0)
	`, "main.lua", WithStdlib(), WithModule("iter", iterMod))
	if len(checked.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported generic iterator aliases to instantiate returns", checked.Diagnostics)
	}
}

func TestRequireCheckAndExportedGenericMemberSignatureInstantiatesImportedCallbackReturn(t *testing.T) {
	protocolMod := CheckAndExport(`
		type User = { id: string, retries: number }
		type Audit = { user_id: string, event: string }
		local M = {}
		M.User = User
		M.Audit = Audit
		return M
	`, "protocol")
	if len(protocolMod.Errors) != 0 {
		t.Fatalf("protocol module errors = %#v, want none", protocolMod.Errors)
	}
	resultMod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		local M = {}
		function M.ok<T>(value: T): Result<T>
			return { ok = true, value = value }
		end
		function M.map<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
			if result.ok then
				return M.ok(fn(result.value))
			end
			return { ok = false, error = result.error }
		end
		return M
	`, "result")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result module errors = %#v, want none", resultMod.Errors)
	}

	checked := Check(`
		local protocol = require("protocol")
		local result = require("result")
		type UserResult = { ok: true, value: protocol.User } | { ok: false, error: string }
		local decoded: UserResult = result.ok({ id = "u1", retries = 2 })
		local mapped = result.map(decoded, function(user: protocol.User)
			return user.id .. ":" .. tostring(user.retries)
		end)
		if mapped.ok then
			local text: string = mapped.value
		end
	`, WithStdlib(), WithModule("protocol", protocolMod), WithModule("result", resultMod))
	if len(checked.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want imported callback return instantiated", checked.Diagnostics)
	}
}

func TestRequireCheckAndExportedGenericMemberSignatureSeedsUnannotatedCallbackParam(t *testing.T) {
	resultMod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		local M = {}
		function M.ok<T>(value: T): Result<T>
			return { ok = true, value = value }
		end
		function M.map<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
			if result.ok then
				return M.ok(fn(result.value))
			end
			return { ok = false, error = result.error }
		end
		return M
	`, "result")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result module errors = %#v, want none", resultMod.Errors)
	}

	src := `
		local result = require("result")
		type User = { id: string, retries: number }
		type UserResult = { ok: true, value: User } | { ok: false, error: string }
		local decoded: UserResult = result.ok({ id = "u1", retries = 2 })
		local mapped = result.map(decoded, function(user)
			return user.id
		end)
		if mapped.ok then
			local id: string = mapped.value
			local wrong_id: number = mapped.value
		end
	`
	checked := Check(src, WithStdlib(), WithModule("result", resultMod))
	requireDiagnostic(t, checked, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            11,
		Column:          29,
		Span:            diagnostic.Span{StartLine: 11, StartCol: 29, EndLine: 11, EndCol: 40},
		MessageContains: []string{"mapped.value", "string", "not number"},
		EvidenceMin:     2,
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"mapped.value", "string"},
			},
			{
				Kind:            diagnostic.EvidenceUserAssertion,
				Trust:           diagnostic.TrustClaimed,
				MessageContains: []string{"wrong_id", "number"},
			},
		},
		LabelMin:      2,
		LabelContains: []string{"declared type", "assigned value"},
		HelpContains:  []string{"Use a value compatible", "change the target type", "`mapped.value` is valid"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			`error[type.assignment]: cannot assign mapped.value because it is string, not number`,
			"test.lua:11:29",
			"↓ declared type",
			"11 |             local wrong_id: number = mapped.value",
			"↑ assigned value",
			"because:",
			"proven: mapped.value has type string",
			"claimed: wrong_id is declared as number",
			"help: Use a value compatible with the expected type",
		},
		RenderNotContains: []string{
			"want number",
			"^~",
			"missing proof:",
		},
	})
}

func TestRequireCheckAndExportedGenericMemberSignatureInstantiatesNestedCallbackResult(t *testing.T) {
	protocolMod := CheckAndExport(`
		type User = { id: string, retries: number }
		type Audit = { user_id: string, event: string }
		local M = {}
		M.User = User
		M.Audit = Audit
		return M
	`, "protocol")
	if len(protocolMod.Errors) != 0 {
		t.Fatalf("protocol module errors = %#v, want none", protocolMod.Errors)
	}
	resultMod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		local M = {}
		function M.ok<T>(value: T): Result<T>
			return { ok = true, value = value }
		end
		function M.and_then<T, U>(result: Result<T>, fn: (T) -> Result<U>): Result<U>
			if result.ok then
				return fn(result.value)
			end
			return { ok = false, error = result.error }
		end
		return M
	`, "result")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result module errors = %#v, want none", resultMod.Errors)
	}

	checked := Check(`
		local protocol = require("protocol")
		local result = require("result")
		type UserResult = { ok: true, value: protocol.User } | { ok: false, error: string }
		local decoded: UserResult = result.ok({ id = "u1", retries = 2 })
		local audit = result.and_then(decoded, function(user: protocol.User)
			return result.ok({ user_id = user.id, event = "created" })
		end)
		if audit.ok then
			local event: string = audit.value.event
			local wrong_event: number = audit.value.event
		end
	`, WithStdlib(), WithModule("protocol", protocolMod), WithModule("result", resultMod))
	if len(checked.Diagnostics) != 1 {
		debug := "<no checked result>"
		if checked.checked != nil {
			root := checked.checked.RootResult()
			debug = callOutcomeDebug(root)
			for _, fn := range root.FunctionResults() {
				debug += "\nchild: " + callOutcomeDebug(fn)
			}
		}
		t.Fatalf("diagnostics = %#v, want one wrong_event diagnostic\ncalls: %s", checked.Diagnostics, debug)
	}
	if checked.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", checked.Diagnostics[0].Code, diagnostics.CodeAssignmentType)
	}
}

func TestRequireCheckAndExportedGenericMemberSignatureKeepsErrorArmOutOfResultInput(t *testing.T) {
	protocolMod := CheckAndExport(`
		type User = { id: string, retries: number }
		local M = {}
		M.User = User
		return M
	`, "protocol")
	if len(protocolMod.Errors) != 0 {
		t.Fatalf("protocol module errors = %#v, want none", protocolMod.Errors)
	}
	resultMod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		local M = {}
		function M.ok<T>(value: T): Result<T>
			return { ok = true, value = value }
		end
		function M.err<T>(message: string): Result<T>
			return { ok = false, error = message }
		end
		function M.and_then<T, U>(result: Result<T>, fn: (T) -> Result<U>): Result<U>
			if result.ok then
				return fn(result.value)
			end
			return M.err(result.error)
		end
		function M.map<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
			if result.ok then
				return M.ok(fn(result.value))
			end
			return M.err(result.error)
		end
		function M.dispatch<T, U>(value: T, handler: (T) -> Result<U>): Result<U>
			return handler(value)
		end
		return M
	`, "result")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result module errors = %#v, want none", resultMod.Errors)
	}
	validatorMod := CheckAndExport(`
		local protocol = require("protocol")
		local result = require("result")
		type UserResult = { ok: true, value: protocol.User } | { ok: false, error: string }
		local M = {}
		function M.decode_user(raw: any): UserResult
			if type(raw) ~= "table" then
				return result.err("root")
			end
			if type(raw.id) ~= "string" then
				return result.err("id")
			end
			if type(raw.retries) ~= "number" then
				return result.err("retries")
			end
			return result.ok({ id = raw.id, retries = raw.retries })
		end
		return M
	`, "validator", WithStdlib(), WithModule("protocol", protocolMod), WithModule("result", resultMod))
	if len(validatorMod.Errors) != 0 {
		t.Fatalf("validator module errors = %#v, want none", validatorMod.Errors)
	}

	checked := Check(`
		local protocol = require("protocol")
		local result = require("result")
		local validator = require("validator")

		type NumberResult = { ok: true, value: number } | { ok: false, error: string }
		type AuditResult = { ok: true, value: protocol.Audit } | { ok: false, error: string }
		type UserAuditHandler = (protocol.User) -> AuditResult

		local raw: any = { id = "u1", retries = 2 }
		local trusted: protocol.User = raw

		local decoded = validator.decode_user(raw)
		local label = result.map(decoded, function(user: protocol.User)
			return user.id .. ":" .. tostring(user.retries + 1)
		end)
		if label.ok then
			local text: string = label.value
			print(text)
		end

		local audit = result.and_then(decoded, function(user: protocol.User)
			return result.ok({ user_id = user.id, event = "created" })
		end)
		if audit.ok then
			local event: string = audit.value.event
			print(audit.value.user_id .. ":" .. event)
		end

		local wrong_result: NumberResult = result.map(decoded, function(user: protocol.User)
			return user.id
		end)
		local wrong_handler: UserAuditHandler = function(audit: protocol.Audit): AuditResult
			return result.ok(audit)
		end
		local dispatched = result.dispatch({ id = "u2", retries = 1 }, function(user: protocol.User)
			return result.ok(user.id .. ":direct")
		end)
		if dispatched.ok then
			print(dispatched.value)
		end

		local failed = result.and_then(validator.decode_user({ id = 9, retries = "bad" }), function(user: protocol.User)
			return result.ok({ user_id = user.id, event = "never" })
		end)
		if not failed.ok then
			local err: string = failed.error
		end
	`, WithStdlib(), WithModule("protocol", protocolMod), WithModule("result", resultMod), WithModule("validator", validatorMod))
	for _, diag := range checked.Diagnostics {
		if diag.Code == diagnostics.CodeDirectCallArgType && strings.Contains(diag.Message, "string |") {
			t.Fatalf("diagnostics = %#v, want error arm not to pollute Result<T> input", checked.Diagnostics)
		}
	}
}

func TestRequireCheckAndExportedGenericMemberSignatureRejectsAnnotatedCallResult(t *testing.T) {
	resultMod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		local M = {}
		function M.ok<T>(value: T): Result<T>
			return { ok = true, value = value }
		end
		function M.map<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
			if result.ok then
				return M.ok(fn(result.value))
			end
			return { ok = false, error = result.error }
		end
		return M
	`, "result")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result module errors = %#v, want none", resultMod.Errors)
	}

	src := `
		local result = require("result")
		type StringResult = { ok: true, value: string } | { ok: false, error: string }
		type NumberResult = { ok: true, value: number } | { ok: false, error: string }
		local decoded: StringResult = result.ok("u1")
		local wrong_result: NumberResult = result.map(decoded, function(value: string)
			return value .. ":mapped"
		end)
	`
	checked := Check(src, WithStdlib(), WithModule("result", resultMod))
	requireDiagnostic(t, checked, diagnosticExpectation{
		Code:            diagnostics.CodeAssignmentType,
		Severity:        diagnostic.SeverityError,
		DiagnosticCount: 1,
		Line:            6,
		Column:          38,
		Span:            diagnostic.Span{StartLine: 6, StartCol: 38, EndLine: 8, EndCol: 5},
		MessageContains: []string{
			"result.map(...)",
			"{ok: true, value: string}",
			"NumberResult",
		},
		EvidenceMin: 2,
		EvidenceChain: []diagnosticEvidenceExpectation{
			{
				Kind:            diagnostic.EvidenceAbstractFact,
				Trust:           diagnostic.TrustProven,
				MessageContains: []string{"result.map(...)", "{ok: true, value: string}"},
			},
			{
				Kind:            diagnostic.EvidenceUserAssertion,
				Trust:           diagnostic.TrustClaimed,
				MessageContains: []string{"wrong_result", "NumberResult"},
			},
		},
		LabelMin:      2,
		LabelContains: []string{"declared type", "assigned value"},
		HelpContains:  []string{"Use a value compatible", "change the target type", "`result.map(...)` is valid"},
		Sources:       diagnostic.SourceMap{"test.lua": src},
		RenderOrderedContains: []string{
			"error[type.assignment]: cannot assign result.map(...) because it is {ok: true, value: string} | {error: string, ok: false}, not NumberResult",
			"test.lua:6:38",
			"↓ declared type",
			"6 |         local wrong_result: NumberResult = result.map(decoded, function(value: string)",
			"↑ assigned value",
			"because:",
			"proven: result.map(...) has type {ok: true, value: string} | {error: string, ok: false}",
			"claimed: wrong_result is declared as NumberResult",
			"help: Use a value compatible with the expected type",
		},
		RenderNotContains: []string{
			"want number",
			"^~",
		},
	})
}

func TestRequireCheckAndExportedGenericMemberSignatureInstantiatesObjectLiteralArgument(t *testing.T) {
	resultMod := CheckAndExport(`
		type Result<T> = { ok: true, value: T } | { ok: false, error: string }
		local M = {}
		function M.ok<T>(value: T): Result<T>
			return { ok = true, value = value }
		end
		return M
	`, "result")
	if len(resultMod.Errors) != 0 {
		t.Fatalf("result module errors = %#v, want none", resultMod.Errors)
	}

	checked := Check(`
		local result = require("result")
		local wrapped = result.ok({ user_id = "u1", event = "created" })
		if wrapped.ok then
			local event: string = wrapped.value.event
			local wrong_event: number = wrapped.value.event
		end
	`, WithStdlib(), WithModule("result", resultMod))
	if len(checked.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one wrong_event diagnostic", checked.Diagnostics)
	}
	if checked.Diagnostics[0].Code != diagnostics.CodeAssignmentType {
		t.Fatalf("diagnostic code = %s, want %s", checked.Diagnostics[0].Code, diagnostics.CodeAssignmentType)
	}
}
