type Message = { data: string }
type Timeout = { elapsed: number }

local function consume(messages: Channel<Message>, timeout: Channel<Timeout>): string?
    local result: { channel: any, value: Message | Timeout, ok: boolean } = channel.select {
        messages:case_receive(),
        timeout:case_receive(),
    }

    result = {
        channel = messages,
        value = { elapsed = 1 },
        ok = true,
    }

    if result.channel == messages then
        local data: string = result.value.data
        return data
    end

    return nil
end

return consume
