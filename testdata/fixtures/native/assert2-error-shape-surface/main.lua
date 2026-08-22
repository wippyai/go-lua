-- The pcall/error-shape half of assert2. throws answers the raised value,
-- not_throws admits a body that returns, the three error readers inspect a
-- raised value, has_error and no_error read a pcall answer pair, and fail
-- answers Never, so a branch that reaches it produces no value at all.
local assert2 = require("assert2")

local function boom()
    error("boom")
end

local function safe(): number
    return 1
end

local function check_error_shape()
    local raised = assert2.throws(boom, "boom raises")
    assert2.not_throws(safe, "safe does not raise")
    assert2.error_kind(raised, "Internal", "the raised error carries a kind")
    assert2.error_message(raised, "boom", "the raised error carries its message")
    assert2.error_contains(raised, "boo", "the raised message carries boo")

    local raised_ok, raised_err = pcall(boom)
    assert2.has_error(raised_ok, raised_err, "the failing call answers an error")
    local safe_ok, safe_err = pcall(safe)
    assert2.no_error(safe_ok, safe_err, "the succeeding call answers no error")
end

-- fail never returns, so the declared string result is satisfied by the one
-- branch that does return.
local function require_branch(flag: boolean): string
    if flag then
        return "taken"
    end
    assert2.fail("no branch produced a value")
end

check_error_shape()
return require_branch
