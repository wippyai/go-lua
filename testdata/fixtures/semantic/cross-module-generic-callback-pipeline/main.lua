local protocol = require("protocol")
local result = require("result")
local validator = require("validator")

type NumberResult = {ok: true, value: number} | {ok: false, error: string}
type AuditResult = {ok: true, value: protocol.Audit} | {ok: false, error: string}
type UserAuditHandler = (protocol.User) -> AuditResult

local raw: any = {
    id = "u1",
    retries = 2,
}

local trusted: protocol.User = raw -- expect-error

local decoded = validator.decode_user(raw)
local label = result.map(decoded, function(user: protocol.User): string
    return user.id .. ":" .. tostring(user.retries + 1)
end)

if label.ok then
    local text: string = label.value
    local wrong_label: number = label.value -- expect-error
    print(text)
end

local audit = result.and_then(decoded, function(user: protocol.User)
    return result.ok({user_id = user.id, event = "created"})
end)

if audit.ok then
    local event: string = audit.value.event
    local wrong_event: number = audit.value.event -- expect-error
    print(audit.value.user_id .. ":" .. event)
end

local wrong_result: NumberResult = result.map(decoded, function(user: protocol.User): string -- expect-error
    return user.id
end)

local wrong_handler: UserAuditHandler = function(audit: protocol.Audit): AuditResult -- expect-error
    return result.ok(audit)
end

local dispatched = result.dispatch({id = "u2", retries = 1}, function(user: protocol.User)
    return result.ok(user.id .. ":direct")
end)

if dispatched.ok then
    print(dispatched.value)
end

local failed = result.and_then(validator.decode_user({id = 9, retries = "bad"}), function(user: protocol.User)
    return result.ok({user_id = user.id, event = "never"})
end)

if not failed.ok then
    print(failed.error)
end
