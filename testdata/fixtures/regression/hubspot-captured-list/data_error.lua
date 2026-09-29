local M = {}

type DataError = {
    code: string,
    message: string,
    retriable: boolean,
    scope: string,
    auth_expired: boolean?,
}

type Envelope = {
    success: boolean,
    error: DataError,
    retry_after_ms: number?,
}
type FailedResult = {
    status_code: number?,
    error: unknown?,
    retry_after_ms: number?,
}

function M.failure(code: string, message: string, retriable: boolean, scope: string, retry_after_ms: number?): Envelope
    local out: Envelope = {
        success = false,
        error = { code = code, message = tostring(message), retriable = retriable, scope = scope },
    }
    if code == "auth_expired" then out.error.auth_expired = true end
    if retry_after_ms ~= nil then out.retry_after_ms = retry_after_ms end
    return out
end

function M.connection(message: string): Envelope
    return M.failure("auth_expired", message, false, "connection")
end

function M.invalid_config(message: string): Envelope
    return M.failure("invalid_config", message, false, "flow")
end

function M.invalid_request(message: string): Envelope
    return M.failure("invalid_request", message, false, "item")
end

function M.from_result(res: unknown, context: string): Envelope
    local r: FailedResult = {}
    if type(res) == "table" then r = res :: FailedResult end
    local status = tonumber(r.status_code) or 0
    local message = context .. ": " .. tostring(r.error or "request failed")
    if status == 429 then return M.failure("rate_limited", message, true, "provider", tonumber(r.retry_after_ms) or 1000) end
    if status == 401 then return M.failure("auth_expired", message, false, "connection") end
    if status == 403 then return M.failure("permission_denied", message, false, "flow") end
    if status == 404 then return M.failure("not_found", message, false, "flow") end
    if status >= 500 or status == 0 then return M.failure("provider_unavailable", message, true, "provider") end
    return M.failure("provider_error", message, true, "provider")
end

return M

