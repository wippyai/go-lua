package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestResultTypeCrossModuleSiblingNarrowingRegression(t *testing.T) {
	resultSrc := `
type Result<T> = {ok: true, value: T} | {ok: false, error: string}
local M = {}
M.Result = Result
function M.ok<T>(value: T): Result<T>
    return {ok = true, value = value}
end
function M.err<T>(message: string): Result<T>
    return {ok = false, error = message}
end
function M.map<T, U>(r: Result<T>, fn: (T) -> U): Result<U>
    if r.ok then
        return M.ok(fn(r.value))
    end
    return {ok = false, error = r.error}
end
function M.and_then<T, U>(r: Result<T>, fn: (T) -> Result<U>): Result<U>
    if r.ok then
        return fn(r.value)
    end
    return {ok = false, error = r.error}
end
return M
`
	repoSrc := `
local result = require("result")
type User = {id: string, name: string, email: string, active: boolean}
local M = {}
M.User = User
local users: {[string]: User} = {
    ["u1"] = {id = "u1", name = "Alice", email = "alice@test.com", active = true},
    ["u2"] = {id = "u2", name = "Bob", email = "bob@test.com", active = false},
}
function M.find_by_id(id: string): Result<User>
    local user = users[id]
    if not user then
        return result.err("user not found: " .. id)
    end
    return result.ok(user)
end
function M.find_active(id: string): Result<User>
    local r = M.find_by_id(id)
    return result.and_then(r, function(user: User): Result<User>
        if not user.active then
            return result.err("user is inactive: " .. user.name)
        end
        return result.ok(user)
    end)
end
return M
`
	serviceSrc := `
local result = require("result")
local repo = require("repo")
type Greeting = {message: string, user_name: string}
local M = {}
function M.greet_user(id: string): Result<Greeting>
    local user_result = repo.find_active(id)
    return result.map(user_result, function(user: User): Greeting
        return {
            message = "Hello, " .. user.name .. "!",
            user_name = user.name,
        }
    end)
end
function M.get_email(id: string): (string?, string?)
    local r = repo.find_by_id(id)
    if r.ok then
        return r.value.email, nil
    end
    return nil, r.error
end
return M
`
	mainSrc := `
local service = require("service")
local email, err = service.get_email("u1")
if err == nil then
    local e: string = email
end
`
	resultMod := testutil.CheckAndExport(resultSrc, "result", testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	repoMod := testutil.CheckAndExport(repoSrc, "repo", testutil.WithStdlib(), testutil.WithModule("result", resultMod), testutil.WithCheckOption(check.WithCanonicalFlow()))
	serviceMod := testutil.CheckAndExport(serviceSrc, "service", testutil.WithStdlib(), testutil.WithModule("result", resultMod), testutil.WithModule("repo", repoMod), testutil.WithCheckOption(check.WithCanonicalFlow()))

	res := testutil.Check(mainSrc, testutil.WithStdlib(),
		testutil.WithModule("result", resultMod),
		testutil.WithModule("repo", repoMod),
		testutil.WithModule("service", serviceMod),
		testutil.WithCheckOption(check.WithCanonicalFlow()))
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected cross-module result sibling narrowing to be clean, got diagnostics: %v", msgs)
	}
}
