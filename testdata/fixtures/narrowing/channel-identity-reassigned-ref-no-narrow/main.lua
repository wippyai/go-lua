type Event = {kind: "event", id: string}
type Tick = {kind: "tick", elapsed: number}

local function consume(events_ch: Channel<Event>, ticks_ch: Channel<Tick>): string
    local e = events_ch
    e = ticks_ch
    local result = channel.select {
        events_ch:case_receive(),
        ticks_ch:case_receive(),
    }

    if result.channel == e then
        local id: string = result.value.id
        return id
    end

    return tostring(result.value.elapsed)
end

return consume
