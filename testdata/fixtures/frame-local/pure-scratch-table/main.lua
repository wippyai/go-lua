type Message = {
    value: integer,
}

local function route(ch: Channel<Message>): integer
    local scratch = {
        a = 1,
        b = 2,
    }

    local total: integer = scratch.a + scratch.b
    local _, _ = ch:receive()
    return total
end

local out: integer = route(nil :: Channel<Message>)
