type Event = { kind: string }

local function wait(events: Channel<Event>)
    local ch = channel.new(1)
    local timeout = time.after("1s")
    local r = channel.select({ events:case_receive(), ch:case_receive(), timeout:case_receive() })
    if r.channel == timeout then
        return nil
    end
    local either: Event = r.value -- expect-error: cannot assign unknown
    if r.channel == events then
        local k: string = r.value.kind
        return k
    end
    local msg = r.value
    local ev: Event = r.value -- expect-error: cannot assign unknown
    return msg.field
end

return wait
