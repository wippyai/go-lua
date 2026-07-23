type Ready = {kind: "ready", id: string}
type Failed = {kind: "failed", reason: string}
type Canceled = {kind: "canceled", code: number}
type Event = Ready | Failed | Canceled
type Tick = {kind: "tick", elapsed: number}

local function handle(events_ch: Channel<Event>, ticks_ch: Channel<Tick>): string
    local result = channel.select {
        events_ch:case_receive(),
        ticks_ch:case_receive(),
    }

    if result.channel == events_ch then
        local event = result.value
        if event.kind == "ready" then
            return event.id
        elseif event.kind == "failed" then
            return event.reason
        elseif event.kind == "canceled" then
            return tostring(event.code)
        end
        return ""
    elseif result.channel == ticks_ch then
        return tostring(result.value.elapsed)
    end

    return ""
end

return handle
