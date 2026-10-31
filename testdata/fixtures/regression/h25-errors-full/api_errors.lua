-- Shared HTTP error shaping for the automation API. Every endpoint emits the
-- stable { code, message } envelope; typed runtime error kinds map to canonical
-- API codes, then status is selected by code, never prose.

local http = require("http")
local errors = require("errors")

local M = {}

local STATUS_BY_CODE: { [string]: number } = {
    invalid_json = http.STATUS.BAD_REQUEST,
    invalid_request = http.STATUS.BAD_REQUEST,
    invalid_input = http.STATUS.BAD_REQUEST,
    missing_parameter = http.STATUS.BAD_REQUEST,
    authentication_required = http.STATUS.UNAUTHORIZED,
    access_denied = http.STATUS.FORBIDDEN,
    forbidden = http.STATUS.FORBIDDEN,
    not_found = http.STATUS.NOT_FOUND,
    conflict = http.STATUS.CONFLICT,
}

-- The component access gate reports its intentionally opaque absence/denial
-- result through :kind(), not an endpoint-specific API code.
local CODE_BY_KIND: { [string]: string } = {}
CODE_BY_KIND[tostring(errors.NOT_FOUND)] = "not_found"
CODE_BY_KIND[tostring(errors.PERMISSION_DENIED)] = "access_denied"

local function text(value: any, fallback: string): string
    if type(value) == "string" and value ~= "" then return value end
    return fallback
end

function M.error(code: any, message: any): { code: string, message: string }
    local normalized_code = text(code, "internal_error")
    return {
        code = normalized_code,
        message = text(message, normalized_code),
    }
end

function M.status_for(code: any): number
    return STATUS_BY_CODE[text(code, "internal_error")] or http.STATUS.INTERNAL_ERROR
end

-- Normalize a runtime value at the HTTP boundary. Legacy string errors retain
-- their human message but use the caller's declared fallback code; no message
-- substring is interpreted as a transport policy.
function M.status(err: any, fallback_code: string?, fallback_message: string?): (number, { code: string, message: string })
    local code = text(fallback_code, "internal_error")
    local message = text(fallback_message, "request failed")
    if type(err) == "table" or type(err) == "userdata" then
        local e: any = err
        if type(e.code) == "function" then code = text(e:code(), code) else code = text(e.code, code) end
        if type(e.message) == "function" then message = text(e:message(), message) else message = text(e.message, message) end
        local kind: any = nil
        if type(e.kind) == "function" then kind = e:kind() else kind = e.kind end
        local kind_code = kind ~= nil and CODE_BY_KIND[tostring(kind)] or nil
        if kind_code ~= nil then code = kind_code end
    elseif err ~= nil then
        message = tostring(err)
    end
    local payload = M.error(code, message)
    return M.status_for(payload.code), payload
end

return M
