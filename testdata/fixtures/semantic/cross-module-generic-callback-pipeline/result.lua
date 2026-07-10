type Result<T> = {ok: true, value: T} | {ok: false, error: string}

local M = {}

M.Result = Result

function M.ok<T>(value: T): Result<T>
    return {ok = true, value = value}
end

function M.err<T>(message: string): Result<T>
    return {ok = false, error = message}
end

function M.map<T, U>(result: Result<T>, fn: (T) -> U): Result<U>
    if result.ok then
        return M.ok(fn(result.value))
    end
    return M.err(result.error)
end

function M.and_then<T, U>(result: Result<T>, fn: (T) -> Result<U>): Result<U>
    if result.ok then
        return fn(result.value)
    end
    return M.err(result.error)
end

function M.dispatch<T, U>(value: T, handler: (T) -> Result<U>): Result<U>
    return handler(value)
end

return M
