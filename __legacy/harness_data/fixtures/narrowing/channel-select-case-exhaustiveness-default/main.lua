type Event = {kind: string}
type Stop = {reason: string}
type Time = {sec: number, nsec: number}

local function handle(events_ch: Channel<Event>, stop_ch: Channel<Stop>, timeout_ch: Channel<Time>): string
    local result = channel.select {
        events_ch:case_receive(),
        stop_ch:case_receive(),
        timeout_ch:case_receive(),
        default = true,
    }

    if result.channel == events_ch then
        return result.value.kind
    elseif result.channel == stop_ch then
        return result.value.reason
    end
    return "default"
end

return handle
