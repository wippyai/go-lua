type Event = { kind: string }

local function wait(events: channel.Channel<Event>?)
    if not events then
        return nil
    end
    local timeout = time.after("1s")
    local r = channel.select({ events:case_receive(), timeout:case_receive() })
    if r.channel == timeout then
        return nil
    end
    local kind: string = r.value.kind
    local t: time.Time = r.value -- expect-error: cannot assign
    return kind
end

local function either(x: channel.Channel<Event>, flag: boolean)
    local ch = flag and x or nil
    if ch then
        local held: channel.Channel<Event> = ch
        return held
    end
    return nil
end

return wait, either
