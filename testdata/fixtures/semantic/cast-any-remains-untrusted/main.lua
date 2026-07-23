type Payload = {
    id: string,
    count: number,
}

local raw: any = {
    id = "cfg",
    count = 2,
}

local payload: Payload = raw -- expect-error

if raw.id then
    local id: string = raw.id -- expect-error
end

local function consume(payload: Payload): number
    return payload.count + 1
end

local count = consume(raw) -- expect-error

return "ok"
