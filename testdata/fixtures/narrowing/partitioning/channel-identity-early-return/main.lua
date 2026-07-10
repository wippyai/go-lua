type Message = { data: string }
type Timeout = { elapsed: number }

local function consume(messages: Channel<Message>, timeout: Channel<Timeout>): string?
    local result = channel.select {
        messages:case_receive(),
        timeout:case_receive(),
    }

    if result.channel == timeout then
        return nil
    end

    local msg = result.value
    return msg.data
end

return consume
