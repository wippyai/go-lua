type Event = { payload: { from: string | false } }

local function by_truthy(event: Event, k: string)
    local out = {}
    if event.payload.from then
        out[k] = { from = event.payload.from }
    end
    local entry = out[k]
    if entry then
        local from: string? = entry.from
        return from
    end
    return nil
end

local function by_type(payload: any, k: string)
    if type(payload.respond_to) ~= "string" then
        return nil
    end
    local out = {}
    out[k] = { respond_to = payload.respond_to }
    local entry = out[k]
    if entry then
        local topic: string = entry.respond_to
        local wrong: number = entry.respond_to -- expect-error: cannot assign string to number
        return topic
    end
    return nil
end

return by_truthy, by_type
