local protocol = require("protocol")

local function consume(messages: Channel<protocol.Message>, ticks: Channel<protocol.Tick>): string
    local selected = channel.select {
        messages:case_receive(),
        ticks:case_receive(),
    }

    local unrefined = selected.value
    local unsound_message: protocol.Message = unrefined -- expect-error

    if selected.channel == messages then
        local value = selected.value
        local id: string = value.id
        local bad_id: number = value.id -- expect-error
        return id
    end

    local tick = selected.value
    local elapsed: number = tick.elapsed
    local bad_elapsed: string = tick.elapsed -- expect-error
    return tostring(elapsed)
end

return consume
