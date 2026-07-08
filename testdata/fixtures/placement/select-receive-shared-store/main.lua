type Message = {
    id: string,
    payload: {
        route: string,
    },
}

type Holder = {
    envelope: Envelope?,
}

type Envelope = {
    message: Message,
}

local holder: Holder = {}
process.send("worker-1", "holder.ready", holder)

local primary = nil :: Channel<Message>
local retry = nil :: Channel<Message>

local selected = channel.select {
    primary:case_receive(),
    retry:case_receive(),
}

if selected.channel == primary then
    local message = selected.value
    local envelope: Envelope = { message = message }
    holder.envelope = envelope
else
    local message = selected.value
    local envelope: Envelope = { message = message }
    holder.envelope = envelope
end
