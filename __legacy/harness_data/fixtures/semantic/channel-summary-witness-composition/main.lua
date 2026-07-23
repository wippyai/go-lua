local protocol = require("protocol")
local source = require("source")

local function consume(
    messages: Channel<protocol.Message>,
    ticks: Channel<protocol.Tick>
): string
    local boxed = source.new_source(messages, ticks)
    local stream = boxed.value

    local selected = channel.select {
        stream.messages:case_receive(),
        stream.ticks:case_receive(),
    }

    if selected.channel == stream.messages then
        local message = selected.value
        local wrong_tick: protocol.Tick = selected.value -- expect-error

        if message.kind == "ready" then
            local id: string = message.payload.id
            local bad_id: number = message.payload.id -- expect-error
            return "ready:" .. id
        end

        local reason: string = message.reason
        local bad_reason: number = message.reason -- expect-error
        return "failed:" .. reason
    end

    local tick = selected.value
    local elapsed: number = tick.elapsed
    local wrong_message: protocol.Message = selected.value -- expect-error
    local bad_elapsed: string = tick.elapsed -- expect-error
    return "tick:" .. tostring(elapsed)
end

return consume
