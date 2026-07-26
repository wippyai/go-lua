-- A union case is handled when a branch tests its discriminant, not when a
-- branch tests something that reads the same. An integer discriminant and a
-- float written for it are distinct literals that render alike, so the float
-- arm leaves the integer case open.

type Ready = {kind: 1, id: string}
type Failed = {kind: 2, reason: string}
type Canceled = {kind: 3, code: number}
type Event = Ready | Failed | Canceled
type Tick = {kind: 4, elapsed: number}

local function handle(events_ch: Channel<Event>, ticks_ch: Channel<Tick>): string
    local result = channel.select {
        events_ch:case_receive(),
        ticks_ch:case_receive(),
    }

    if result.channel == events_ch then
        local event = result.value
        if event.kind == 1.0 then
            return "one"
        elseif event.kind == 2 then
            return event.reason
        end
        return "unhandled"
    elseif result.channel == ticks_ch then
        return tostring(result.value.elapsed)
    end

    return ""
end

return handle
