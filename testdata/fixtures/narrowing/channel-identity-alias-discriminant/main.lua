type Event = {kind: "event", id: string}
type Tick = {kind: "tick", elapsed: number}

local function consume(events_ch: Channel<Event>, ticks_ch: Channel<Tick>): string
    local e = events_ch
    local result = channel.select {
        events_ch:case_receive(),
        ticks_ch:case_receive(),
    }

    if result.channel == e then
        local event = result.value
        local id: string = event.id
        return event.kind .. ":" .. id
    end

    local tick = result.value
    local elapsed: number = tick.elapsed
    return tick.kind .. ":" .. tostring(elapsed)
end

return consume
