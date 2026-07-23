type Msg = { id: string, payload: number }
local function pump(inbox: Channel<Msg>): string
    local m = inbox:receive()
    return m.id
end
