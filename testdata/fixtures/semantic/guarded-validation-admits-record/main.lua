type User = {
    id: string,
    retries: number,
}

type Result<T> = {ok: true, value: T} | {ok: false, error: string}

local function ok<T>(value: T): Result<T>
    return {ok = true, value = value}
end

local function invalid<T>(message: string): Result<T>
    return {ok = false, error = message}
end

local function decode_user(raw: any): Result<User>
    if type(raw) ~= "table" then
        return invalid("root")
    end
    if type(raw.id) ~= "string" then
        return invalid("id")
    end
    if type(raw.retries) ~= "number" then
        return invalid("retries")
    end
    return ok({id = raw.id, retries = raw.retries})
end

local decoded = decode_user({id = "u1", retries = 3})
if decoded.ok then
    local id: string = decoded.value.id
    local retries: number = decoded.value.retries + 1
end

return "ok"
