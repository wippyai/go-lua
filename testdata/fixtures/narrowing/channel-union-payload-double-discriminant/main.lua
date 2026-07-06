type Ready = {kind: "ready", payload: {id: string}}
type Failed = {kind: "failed", reason: string}
type Tick = {kind: "tick", elapsed: number}

local function consume(events_ch: Channel<Ready | Failed>, ticks_ch: Channel<Tick>): string
    local result = channel.select {
        events_ch:case_receive(),
        ticks_ch:case_receive(),
    }

    if result.channel == events_ch then
        local event = result.value
        if event.kind == "ready" then
            local id: string = event.payload.id
            return "ready:" .. id
        end

        local reason: string = event.reason
        return "failed:" .. reason
    end

    local tick = result.value
    local elapsed: number = tick.elapsed
    return "tick:" .. tostring(elapsed)
end

return consume
