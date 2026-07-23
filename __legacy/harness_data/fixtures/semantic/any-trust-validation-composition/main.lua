type Request = {
    id: string,
    retries: number,
}

type Result<T> = {ok: true, value: T} | {ok: false, error: string}

local function consume(request: Request): string
    return request.id .. ":" .. request.retries
end

local raw: any = {
    id = "r1",
    retries = 2,
}

local direct: Request = raw -- expect-error
local casted: Request = ({id = "r2", retries = 3} :: any) -- expect-error

if raw.id then
    local id: string = raw.id -- expect-error
end

local raw_label = consume(raw) -- expect-error

local function decode_request(input: any): Result<Request>
    if type(input) ~= "table" then
        return {ok = false, error = "root"}
    end
    if type(input.id) ~= "string" then
        return {ok = false, error = "id"}
    end
    if type(input.retries) ~= "number" then
        return {ok = false, error = "retries"}
    end
    return {ok = true, value = {id = input.id, retries = input.retries}}
end

local decoded = decode_request({id = "r3", retries = 4} :: any)
if decoded.ok then
    local trusted: Request = decoded.value
    local id: string = trusted.id
    local label: string = consume(trusted)
end

return raw_label or "ok"
