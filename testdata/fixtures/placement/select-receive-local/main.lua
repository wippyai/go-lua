type Message = {
    id: string,
    payload: {
        route: string,
    },
}

type Envelope = {
    message: Message,
}

local function route(primary: Channel<Message>, retry: Channel<Message>): string
    local selected = channel.select {
        primary:case_receive(),
        retry:case_receive(),
    }

    if selected.channel == primary then
        local message = selected.value
        local envelope: Envelope = { message = message }
        return envelope.message.payload.route
    end

    local message = selected.value
    local envelope: Envelope = { message = message }
    return envelope.message.payload.route
end

local route_name: string = route(nil :: Channel<Message>, nil :: Channel<Message>)
print(route_name)
