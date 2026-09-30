-- Bee runtime-pin checker regression: bee.host:binding_protocol:308
-- Expected vs actual: A successful identity predicate permits either union arm; actual string is excluded at its type guard (Bee reports never).
-- Verified: go-lua v1.5.24 passes (0 diagnostics); v1.6.0 fails.
-- Run with pinlint-driver/v1524/checker or pinlint-driver/v160/checker.
type Request = {workspace_id: string}
type Object = {[string]: unknown}
local function workspace(value: unknown, expected: string): string?
    if type(value) ~= "string" or value ~= expected then return nil end
    return value
end
local function expected_identity(expected: string | Request, value: Object): boolean
    if type(expected) == "string" then return workspace(value.workspace_id, expected) ~= nil end
    return value.workspace_id == expected.workspace_id
end
local function reply(value: Object, expected: string | Request): string?
    if not expected_identity(expected, value) then return nil end
    local workspace_id: string
    if type(expected) == "string" then workspace_id = expected else workspace_id = (expected :: Request).workspace_id end
    return workspace_id
end
return reply
