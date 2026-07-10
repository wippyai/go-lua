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
            local reason: string = event.reason
            return reason
        end
        return event.reason
    end

    return tostring(result.value.elapsed)
end

return consume
